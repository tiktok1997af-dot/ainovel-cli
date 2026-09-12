package webai

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/voocel/ainovel-cli/internal/domain"
)

func TestBrowserLanePoolRejectsUnboundedCapacity(t *testing.T) {
	for _, capacity := range []int{-1, 0, MaxBrowserLaneCount + 1} {
		if _, err := NewBrowserLanePool(BrowserLanePoolConfig{Capacity: capacity}); err == nil {
			t.Fatalf("capacity %d should be rejected", capacity)
		}
	}
}

func TestBrowserLanePoolAllocationIsBoundedAndProfilesAreIsolated(t *testing.T) {
	launcher := &fakeBrowserLauncher{}
	profileRoot := filepath.Join(t.TempDir(), "lane-profiles")
	pool, err := NewBrowserLanePool(BrowserLanePoolConfig{
		Capacity: 2,
		Session: SessionConfig{
			Site:        "gemini-web",
			BrowserPath: fakeBrowserExecutable(t),
			ProfileDir:  profileRoot,
			Launcher:    launcher,
			Probe:       fixedReadinessProbe{result: ReadinessResult{State: SessionReady}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer pool.StopAll()

	first, err := pool.Allocate(context.Background(), domain.RunID("run-a"))
	if err != nil {
		t.Fatalf("allocate run-a: %v", err)
	}
	second, err := pool.Allocate(context.Background(), domain.RunID("run-b"))
	if err != nil {
		t.Fatalf("allocate run-b: %v", err)
	}
	if first.LaneID != "lane-001" || second.LaneID != "lane-002" {
		t.Fatalf("unexpected deterministic lane order: first=%s second=%s", first.LaneID, second.LaneID)
	}
	if first.State != BrowserLaneBusy || second.State != BrowserLaneBusy {
		t.Fatalf("allocated READY lanes should project BUSY: first=%s second=%s", first.State, second.State)
	}
	if _, err := pool.Allocate(context.Background(), domain.RunID("run-c")); !errors.Is(err, ErrNoBrowserLaneAvailable) {
		t.Fatalf("third allocation err = %v, want ErrNoBrowserLaneAvailable", err)
	}

	launcher.mu.Lock()
	configs := append([]BrowserLaunchConfig(nil), launcher.configs...)
	launcher.mu.Unlock()
	if len(configs) != 2 {
		t.Fatalf("launch count = %d, want 2", len(configs))
	}
	wantFirst := filepath.Join(profileRoot, "lane-001")
	wantSecond := filepath.Join(profileRoot, "lane-002")
	if configs[0].ProfileDir != wantFirst || configs[1].ProfileDir != wantSecond {
		t.Fatalf("lane profiles are not isolated: %#v", configs)
	}
	if configs[0].ProfileDir == configs[1].ProfileDir {
		t.Fatal("two lanes must never share one profile directory")
	}

	encoded, err := json.Marshal(pool.List())
	if err != nil {
		t.Fatal(err)
	}
	serialized := string(encoded)
	for _, forbidden := range []string{"profile_dir", "browser_path", "pid", profileRoot} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("sanitized projection leaked %q: %s", forbidden, serialized)
		}
	}
}

func TestBrowserLanePoolRepeatedAllocationIsIdempotent(t *testing.T) {
	launcher := &fakeBrowserLauncher{}
	pool, err := NewBrowserLanePool(BrowserLanePoolConfig{
		Capacity: 1,
		Session: SessionConfig{
			BrowserPath: fakeBrowserExecutable(t),
			ProfileDir:  filepath.Join(t.TempDir(), "profiles"),
			Launcher:    launcher,
			Probe:       fixedReadinessProbe{result: ReadinessResult{State: SessionReady}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer pool.StopAll()

	first, err := pool.Allocate(context.Background(), domain.RunID("run-a"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := pool.Allocate(context.Background(), domain.RunID("run-a"))
	if err != nil {
		t.Fatal(err)
	}
	if first.LaneID != second.LaneID || second.RunID != "run-a" {
		t.Fatalf("idempotent allocation changed identity: first=%+v second=%+v", first, second)
	}
	launcher.mu.Lock()
	launches := len(launcher.configs)
	launcher.mu.Unlock()
	if launches != 1 {
		t.Fatalf("idempotent allocation launched browser %d times, want 1", launches)
	}
}

func TestBrowserLanePoolReleaseStopsBeforeDeterministicReuse(t *testing.T) {
	launcher := &fakeBrowserLauncher{}
	profileRoot := filepath.Join(t.TempDir(), "profiles")
	pool, err := NewBrowserLanePool(BrowserLanePoolConfig{
		Capacity: 1,
		Session: SessionConfig{
			BrowserPath: fakeBrowserExecutable(t),
			ProfileDir:  profileRoot,
			Launcher:    launcher,
			Probe:       fixedReadinessProbe{result: ReadinessResult{State: SessionReady}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer pool.StopAll()

	first, err := pool.Allocate(context.Background(), domain.RunID("run-a"))
	if err != nil {
		t.Fatal(err)
	}
	if err := pool.Release(domain.RunID("run-a")); err != nil {
		t.Fatalf("release run-a: %v", err)
	}
	stopped, err := pool.Snapshot(first.LaneID)
	if err != nil {
		t.Fatal(err)
	}
	if stopped.State != BrowserLaneStopped || stopped.RunID != "" {
		t.Fatalf("released lane = %+v, want STOPPED and unowned", stopped)
	}

	second, err := pool.Allocate(context.Background(), domain.RunID("run-b"))
	if err != nil {
		t.Fatalf("allocate run-b after release: %v", err)
	}
	if second.LaneID != first.LaneID || second.State != BrowserLaneBusy {
		t.Fatalf("deterministic lane reuse failed: first=%+v second=%+v", first, second)
	}
	launcher.mu.Lock()
	configs := append([]BrowserLaunchConfig(nil), launcher.configs...)
	launcher.mu.Unlock()
	if len(configs) != 2 {
		t.Fatalf("launch count = %d, want 2", len(configs))
	}
	if configs[0].ProfileDir != configs[1].ProfileDir || configs[0].ProfileDir != filepath.Join(profileRoot, "lane-001") {
		t.Fatalf("lane reuse changed isolated profile ownership: %#v", configs)
	}
}

type sequenceLaneProbe struct {
	mu      sync.Mutex
	results []ReadinessResult
	errs    []error
	calls   int
}

func (p *sequenceLaneProbe) Probe(context.Context, SessionSnapshot) (ReadinessResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	idx := p.calls
	p.calls++
	if idx >= len(p.results) {
		idx = len(p.results) - 1
	}
	var err error
	if idx >= 0 && idx < len(p.errs) {
		err = p.errs[idx]
	}
	if idx < 0 {
		return ReadinessResult{}, err
	}
	return p.results[idx], err
}

func TestBrowserLanePoolRecoverKeepsOwnerAndRestartsSameLane(t *testing.T) {
	launcher := &fakeBrowserLauncher{}
	probe := &sequenceLaneProbe{
		results: []ReadinessResult{
			{},
			{State: SessionReady, Reason: "ready after restart"},
		},
		errs: []error{errors.New("transient readiness failure"), nil},
	}
	profileRoot := filepath.Join(t.TempDir(), "profiles")
	pool, err := NewBrowserLanePool(BrowserLanePoolConfig{
		Capacity: 1,
		Session: SessionConfig{
			BrowserPath: fakeBrowserExecutable(t),
			ProfileDir:  profileRoot,
			Launcher:    launcher,
			Probe:       probe,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer pool.StopAll()

	degraded, err := pool.Allocate(context.Background(), domain.RunID("run-a"))
	if err == nil {
		t.Fatal("first allocation should surface transient readiness error")
	}
	if degraded.State != BrowserLaneDegraded || degraded.RunID != "run-a" {
		t.Fatalf("degraded allocation lost ownership: %+v", degraded)
	}

	recovered, err := pool.Recover(context.Background(), degraded.LaneID)
	if err != nil {
		t.Fatalf("Recover: %v", err)
	}
	if recovered.LaneID != degraded.LaneID || recovered.RunID != "run-a" {
		t.Fatalf("recovery changed lane/run ownership: before=%+v after=%+v", degraded, recovered)
	}
	if recovered.State != BrowserLaneBusy || recovered.RecoveryCount != 1 {
		t.Fatalf("unexpected recovered projection: %+v", recovered)
	}
	launcher.mu.Lock()
	configs := append([]BrowserLaunchConfig(nil), launcher.configs...)
	launcher.mu.Unlock()
	if len(configs) != 2 {
		t.Fatalf("recovery launch count = %d, want 2", len(configs))
	}
	if configs[0].ProfileDir != configs[1].ProfileDir {
		t.Fatalf("recovery changed lane profile: %#v", configs)
	}
}

func TestBrowserLanePoolStopAllClearsTransientOwnership(t *testing.T) {
	pool, err := NewBrowserLanePool(BrowserLanePoolConfig{
		Capacity: 2,
		Session: SessionConfig{
			BrowserPath: fakeBrowserExecutable(t),
			ProfileDir:  filepath.Join(t.TempDir(), "profiles"),
			Launcher:    &fakeBrowserLauncher{},
			Probe:       fixedReadinessProbe{result: ReadinessResult{State: SessionReady}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Allocate(context.Background(), domain.RunID("run-a")); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Allocate(context.Background(), domain.RunID("run-b")); err != nil {
		t.Fatal(err)
	}
	if err := pool.StopAll(); err != nil {
		t.Fatal(err)
	}
	for _, lane := range pool.List() {
		if lane.State != BrowserLaneStopped || lane.RunID != "" {
			t.Fatalf("StopAll left transient ownership: %+v", lane)
		}
	}
}
