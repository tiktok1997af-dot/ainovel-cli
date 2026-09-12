package webai

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/voocel/ainovel-cli/internal/domain"
)

func TestAdoptedBrowserLanePoolUsesExistingHostSessionWithoutSecondLaunch(t *testing.T) {
	launcher := &fakeBrowserLauncher{}
	primary := NewSessionManager(SessionConfig{
		Site:        "gemini-web",
		BrowserPath: fakeBrowserExecutable(t),
		ProfileDir:  filepath.Join(t.TempDir(), "primary"),
		Launcher:    launcher,
		Probe: fixedReadinessProbe{result: ReadinessResult{
			State: SessionReady,
		}},
	})
	if _, err := primary.Start(context.Background()); err != nil {
		t.Fatalf("start primary: %v", err)
	}
	defer primary.Stop()

	pool, err := NewAdoptedBrowserLanePool(primary)
	if err != nil {
		t.Fatalf("NewAdoptedBrowserLanePool: %v", err)
	}
	projection, err := pool.Allocate(context.Background(), domain.RunID("run-001"))
	if err != nil {
		t.Fatalf("Allocate adopted lane: %v", err)
	}
	if projection.LaneID != "lane-001" || projection.RunID != "run-001" || projection.State != BrowserLaneBusy {
		t.Fatalf("unexpected adopted projection: %+v", projection)
	}
	if got := len(launcher.configs); got != 1 {
		t.Fatalf("adopted allocation launched another browser: launches=%d", got)
	}
}

func TestAdoptedBrowserLanePoolReleaseThenRestartSameSession(t *testing.T) {
	launcher := &fakeBrowserLauncher{}
	primary := NewSessionManager(SessionConfig{
		Site:        "gemini-web",
		BrowserPath: fakeBrowserExecutable(t),
		ProfileDir:  filepath.Join(t.TempDir(), "primary"),
		Launcher:    launcher,
		Probe: fixedReadinessProbe{result: ReadinessResult{
			State: SessionReady,
		}},
	})
	if _, err := primary.Start(context.Background()); err != nil {
		t.Fatalf("start primary: %v", err)
	}
	pool, err := NewAdoptedBrowserLanePool(primary)
	if err != nil {
		t.Fatalf("NewAdoptedBrowserLanePool: %v", err)
	}
	if _, err := pool.Allocate(context.Background(), domain.RunID("run-001")); err != nil {
		t.Fatalf("Allocate run-001: %v", err)
	}
	if err := pool.Release(domain.RunID("run-001")); err != nil {
		t.Fatalf("Release run-001: %v", err)
	}
	if got := primary.Snapshot().State; got != SessionStopped {
		t.Fatalf("primary state after release = %s, want STOPPED", got)
	}
	projection, err := pool.Allocate(context.Background(), domain.RunID("run-002"))
	if err != nil {
		t.Fatalf("Allocate run-002: %v", err)
	}
	if projection.RunID != "run-002" || projection.LaneID != "lane-001" {
		t.Fatalf("unexpected reused lane: %+v", projection)
	}
	if got := len(launcher.configs); got != 2 {
		t.Fatalf("restart launches=%d, want 2 total", got)
	}
}
