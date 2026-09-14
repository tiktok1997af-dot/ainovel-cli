package appruntime

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/webai"
)

func TestD06CircuitOpensAfterTwoFinalFailuresAndHalfOpenSuccessCloses(t *testing.T) {
	layer := newProviderResilienceLayer()
	now := time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC)
	layer.now = func() time.Time { return now }

	for i := 0; i < d06ProviderFailureThreshold; i++ {
		if err := layer.beginAdmission(ProviderGeminiWeb); err != nil {
			t.Fatalf("begin failure %d: %v", i+1, err)
		}
		layer.recordFailure(ProviderGeminiWeb)
	}
	open := layer.snapshot(ProviderGeminiWeb)
	if open.State != providerCircuitOpen || open.ConsecutiveFailures != d06ProviderFailureThreshold {
		t.Fatalf("open circuit = %+v", open)
	}
	if err := layer.beginAdmission(ProviderGeminiWeb); !errors.Is(err, errD06ProviderCircuitOpen) {
		t.Fatalf("open circuit admission err=%v", err)
	}

	now = now.Add(d06ProviderOpenCooldown + time.Millisecond)
	if err := layer.beginAdmission(ProviderGeminiWeb); err != nil {
		t.Fatalf("half-open probe rejected: %v", err)
	}
	halfOpen := layer.snapshot(ProviderGeminiWeb)
	if halfOpen.State != providerCircuitHalfOpen || !halfOpen.ProbeInFlight {
		t.Fatalf("half-open snapshot = %+v", halfOpen)
	}
	if err := layer.beginAdmission(ProviderGeminiWeb); !errors.Is(err, errD06ProviderCircuitOpen) {
		t.Fatalf("second half-open probe err=%v", err)
	}

	layer.recordSuccess(ProviderGeminiWeb)
	closed := layer.snapshot(ProviderGeminiWeb)
	if closed.State != providerCircuitHealthy || closed.ConsecutiveFailures != 0 || closed.ProbeInFlight {
		t.Fatalf("closed circuit = %+v", closed)
	}
}

func TestD06FailedHalfOpenProbeReopensCircuit(t *testing.T) {
	layer := newProviderResilienceLayer()
	now := time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC)
	layer.now = func() time.Time { return now }
	for i := 0; i < d06ProviderFailureThreshold; i++ {
		if err := layer.beginAdmission(ProviderChatGPTWeb); err != nil {
			t.Fatal(err)
		}
		layer.recordFailure(ProviderChatGPTWeb)
	}
	now = now.Add(d06ProviderOpenCooldown + time.Millisecond)
	if err := layer.beginAdmission(ProviderChatGPTWeb); err != nil {
		t.Fatal(err)
	}
	layer.recordFailure(ProviderChatGPTWeb)
	reopened := layer.snapshot(ProviderChatGPTWeb)
	if reopened.State != providerCircuitOpen || reopened.OpenedAt != now {
		t.Fatalf("reopened circuit = %+v", reopened)
	}
}

func TestD06ProviderCircuitsAreIsolated(t *testing.T) {
	layer := newProviderResilienceLayer()
	for i := 0; i < d06ProviderFailureThreshold; i++ {
		if err := layer.beginAdmission(ProviderGeminiWeb); err != nil {
			t.Fatal(err)
		}
		layer.recordFailure(ProviderGeminiWeb)
	}
	if got := layer.snapshot(ProviderGeminiWeb).State; got != providerCircuitOpen {
		t.Fatalf("Gemini state=%q", got)
	}
	if got := layer.snapshot(ProviderChatGPTWeb); got.State != providerCircuitHealthy || got.ConsecutiveFailures != 0 {
		t.Fatalf("ChatGPT circuit contaminated by Gemini: %+v", got)
	}
}

type d06FakeLanePool struct {
	owner domain.RunID

	allocateProjection webai.BrowserLaneProjection
	allocateErr        error
	refreshProjection  webai.BrowserLaneProjection
	refreshErr         error
	recoverProjection  webai.BrowserLaneProjection
	recoverErr         error

	allocateCalls int
	refreshCalls  int
	recoverCalls  int
	releaseCalls  int
}

func (f *d06FakeLanePool) Allocate(_ context.Context, runID domain.RunID) (webai.BrowserLaneProjection, error) {
	f.allocateCalls++
	if errors.Is(f.allocateErr, webai.ErrNoBrowserLaneAvailable) {
		return f.allocateProjection, f.allocateErr
	}
	f.owner = runID
	out := f.allocateProjection
	if out.LaneID == "" {
		out.LaneID = "lane-001"
	}
	if out.RunID == "" {
		out.RunID = runID
	}
	return out, f.allocateErr
}

func (f *d06FakeLanePool) Refresh(_ context.Context, runID domain.RunID) (webai.BrowserLaneProjection, error) {
	f.refreshCalls++
	out := f.refreshProjection
	if out.LaneID == "" {
		out.LaneID = "lane-001"
	}
	if out.RunID == "" {
		out.RunID = runID
	}
	return out, f.refreshErr
}

func (f *d06FakeLanePool) Recover(_ context.Context, _ domain.BrowserLaneID) (webai.BrowserLaneProjection, error) {
	f.recoverCalls++
	out := f.recoverProjection
	if out.LaneID == "" {
		out.LaneID = "lane-001"
	}
	if out.RunID == "" {
		out.RunID = f.owner
	}
	return out, f.recoverErr
}

func (f *d06FakeLanePool) Release(runID domain.RunID) error {
	f.releaseCalls++
	if f.owner == runID {
		f.owner = ""
	}
	return nil
}

func (f *d06FakeLanePool) List() []webai.BrowserLaneProjection {
	state := webai.BrowserLaneReady
	if f.owner != "" {
		state = webai.BrowserLaneBusy
	}
	return []webai.BrowserLaneProjection{{LaneID: "lane-001", State: state, RunID: f.owner}}
}

func (f *d06FakeLanePool) StopAll() error {
	f.owner = ""
	return nil
}

func TestD06GeminiAdmissionUsesOneBoundedPoolRecovery(t *testing.T) {
	lanes := &d06FakeLanePool{
		allocateProjection: webai.BrowserLaneProjection{LaneID: "lane-001", State: webai.BrowserLaneDegraded},
		refreshProjection:  webai.BrowserLaneProjection{LaneID: "lane-001", State: webai.BrowserLaneDegraded},
		refreshErr:         errors.New("readiness degraded"),
		recoverProjection:  webai.BrowserLaneProjection{LaneID: "lane-001", State: webai.BrowserLaneBusy},
	}
	rt := &Runtime{run: &runCoordinator{lanes: lanes}}
	authority := newResilientDualWebLaneAuthority(rt)

	if err := authority.Acquire(context.Background(), ProviderGeminiWeb, domain.RunID("run-d06-gemini")); err != nil {
		t.Fatalf("Gemini resilient acquire: %v", err)
	}
	if lanes.allocateCalls != 1 || lanes.refreshCalls != 1 || lanes.recoverCalls != 1 {
		t.Fatalf("Gemini calls allocate=%d refresh=%d recover=%d", lanes.allocateCalls, lanes.refreshCalls, lanes.recoverCalls)
	}
	if lanes.owner != "run-d06-gemini" {
		t.Fatalf("Gemini recovery lost run ownership: %q", lanes.owner)
	}
	if got := authority.health.snapshot(ProviderGeminiWeb); got.State != providerCircuitHealthy || got.ConsecutiveFailures != 0 {
		t.Fatalf("Gemini health after recovery = %+v", got)
	}
}

type d06SequenceChatGPTLane struct {
	current        webai.ChatGPTLaneSnapshot
	startSnapshots []webai.ChatGPTLaneSnapshot
	startErrs      []error
	startCalls     int
	refreshCalls   int
	stopCalls      int
}

func (f *d06SequenceChatGPTLane) Start(context.Context) (webai.ChatGPTLaneSnapshot, error) {
	idx := f.startCalls
	f.startCalls++
	if idx < len(f.startSnapshots) {
		f.current = f.startSnapshots[idx]
	}
	var err error
	if idx < len(f.startErrs) {
		err = f.startErrs[idx]
	}
	return f.current, err
}

func (f *d06SequenceChatGPTLane) Refresh(context.Context) (webai.ChatGPTLaneSnapshot, error) {
	f.refreshCalls++
	return f.current, nil
}

func (f *d06SequenceChatGPTLane) Stop() error {
	f.stopCalls++
	f.current = webai.ChatGPTLaneSnapshot{LaneID: webai.ChatGPTWebLaneID, State: webai.SessionStopped}
	return nil
}

func (f *d06SequenceChatGPTLane) Snapshot() webai.ChatGPTLaneSnapshot { return f.current }

func TestD06ChatGPTAdmissionUsesOneBoundedStopStartRecovery(t *testing.T) {
	lane := &d06SequenceChatGPTLane{
		current: webai.ChatGPTLaneSnapshot{LaneID: webai.ChatGPTWebLaneID, State: webai.SessionStopped},
		startSnapshots: []webai.ChatGPTLaneSnapshot{
			{LaneID: webai.ChatGPTWebLaneID, State: webai.SessionDegraded},
			d03ChatGPTReadySnapshot("chatgpt-observed", "rev-d06"),
		},
		startErrs: []error{errors.New("readiness failed"), nil},
	}
	rt := &Runtime{
		run:         &runCoordinator{},
		dualWeb:     newDualWebProviderRuntime(),
		chatGPTLane: lane,
	}
	authority := newResilientDualWebLaneAuthority(rt)

	if err := authority.Acquire(context.Background(), ProviderChatGPTWeb, domain.RunID("run-d06-chatgpt")); err != nil {
		t.Fatalf("ChatGPT resilient acquire: %v", err)
	}
	if lane.startCalls != 2 || lane.stopCalls != 1 || lane.refreshCalls != 0 {
		t.Fatalf("ChatGPT recovery calls start=%d stop=%d refresh=%d", lane.startCalls, lane.stopCalls, lane.refreshCalls)
	}
	if got := authority.health.snapshot(ProviderChatGPTWeb); got.State != providerCircuitHealthy || got.ConsecutiveFailures != 0 {
		t.Fatalf("ChatGPT health after recovery = %+v", got)
	}
}

func TestD06OpenCircuitRejectsBeforeTouchingGeminiLane(t *testing.T) {
	lanes := &d06FakeLanePool{
		allocateProjection: webai.BrowserLaneProjection{LaneID: "lane-001", State: webai.BrowserLaneDegraded},
		refreshProjection:  webai.BrowserLaneProjection{LaneID: "lane-001", State: webai.BrowserLaneDegraded},
		recoverProjection:  webai.BrowserLaneProjection{LaneID: "lane-001", State: webai.BrowserLaneFailed},
		recoverErr:         errors.New("recovery failed"),
	}
	rt := &Runtime{run: &runCoordinator{lanes: lanes}}
	authority := newResilientDualWebLaneAuthority(rt)

	for i := 0; i < d06ProviderFailureThreshold; i++ {
		if err := authority.Acquire(context.Background(), ProviderGeminiWeb, domain.RunID("run-d06-fail")); err == nil {
			t.Fatalf("failure attempt %d unexpectedly succeeded", i+1)
		}
	}
	before := lanes.allocateCalls
	if err := authority.Acquire(context.Background(), ProviderGeminiWeb, domain.RunID("run-d06-fail")); !errors.Is(err, ErrMutationPrecondition) {
		t.Fatalf("open-circuit error=%v", err)
	}
	if lanes.allocateCalls != before {
		t.Fatalf("open circuit touched browser lane: before=%d after=%d", before, lanes.allocateCalls)
	}
}

func TestD06AuthCapacityAndCancellationDoNotPoisonGeminiCircuit(t *testing.T) {
	authLanes := &d06FakeLanePool{
		allocateProjection: webai.BrowserLaneProjection{LaneID: "lane-001", State: webai.BrowserLaneAuthRequired},
	}
	auth := newResilientDualWebLaneAuthority(&Runtime{run: &runCoordinator{lanes: authLanes}})
	for i := 0; i < 3; i++ {
		if err := auth.Acquire(context.Background(), ProviderGeminiWeb, domain.RunID("run-d06-auth")); err == nil {
			t.Fatal("auth-required admission unexpectedly succeeded")
		}
	}
	if got := auth.health.snapshot(ProviderGeminiWeb); got.State != providerCircuitHealthy || got.ConsecutiveFailures != 0 {
		t.Fatalf("auth required poisoned circuit: %+v", got)
	}

	capacityLanes := &d06FakeLanePool{allocateErr: webai.ErrNoBrowserLaneAvailable}
	capacity := newResilientDualWebLaneAuthority(&Runtime{run: &runCoordinator{lanes: capacityLanes}})
	for i := 0; i < 3; i++ {
		if err := capacity.Acquire(context.Background(), ProviderGeminiWeb, domain.RunID("run-d06-capacity")); !errors.Is(err, webai.ErrNoBrowserLaneAvailable) {
			t.Fatalf("capacity err=%v", err)
		}
	}
	if got := capacity.health.snapshot(ProviderGeminiWeb); got.State != providerCircuitHealthy || got.ConsecutiveFailures != 0 {
		t.Fatalf("capacity contention poisoned circuit: %+v", got)
	}

	cancelled := newResilientDualWebLaneAuthority(&Runtime{run: &runCoordinator{lanes: &d06FakeLanePool{}}})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := cancelled.Acquire(ctx, ProviderGeminiWeb, domain.RunID("run-d06-cancel")); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel err=%v", err)
	}
	if got := cancelled.health.snapshot(ProviderGeminiWeb); got.State != providerCircuitHealthy || got.ConsecutiveFailures != 0 {
		t.Fatalf("cancellation poisoned circuit: %+v", got)
	}
}
