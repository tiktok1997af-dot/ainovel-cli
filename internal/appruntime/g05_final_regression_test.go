package appruntime

import (
	"context"
	"testing"

	"github.com/voocel/ainovel-cli/internal/domain"
)

// fakeRunBackend implements the G05.8 restart-only recovery seam in tests. The
// recovery primitive releases stale ownership while preserving FIFO waiters,
// matching RuntimeStore.RecoverResourceLocks.
func (f *fakeRunBackend) DesktopRecoverResourceLocks() ([]domain.ResourceLockState, error) {
	if f.resourceOwner == "" {
		return nil, nil
	}
	state := domain.ResourceLockState{
		Resource:   f.resource,
		OwnerRunID: f.resourceOwner,
	}
	for i, runID := range f.waiters {
		state.Waiters = append(state.Waiters, domain.ResourceLockWaiter{
			RunID:      runID,
			RequestSeq: int64(i + 1),
		})
	}
	f.resourceOwner = ""
	return []domain.ResourceLockState{state}, nil
}

func g058RunRecord(runID domain.RunID, state RunLifecycleState) domain.RunRegistryRecord {
	return domain.RunRegistryRecord{
		Version: domain.CurrentRunRegistryRecordVersion,
		RunID:   runID,
		Source:  domain.RunRecordSourceRegistry,
		State:   string(state),
	}
}

func TestG058SerializesConcurrentRunIntentsAcrossResourceAndLane(t *testing.T) {
	backend := newFakeRunBackend()
	lanes := &fakeRunLanePool{}
	rt := &Runtime{subscribers: make(map[uint64]*desktopSubscription)}
	coord := newRunCoordinator(backend, lanes)
	coord.owner = rt

	if _, err := coord.dispatchLocked(
		context.Background(),
		CommandRequest{ID: "cmd-a", Kind: CommandRunStart, RunID: "run-a"},
		"run-a",
		CommandResult{CommandID: "cmd-a", RunID: "run-a"},
	); err != nil {
		t.Fatalf("start run-a: %v", err)
	}
	if backend.runs["run-a"].State != string(RunStateRunning) {
		t.Fatalf("run-a state=%q, want running", backend.runs["run-a"].State)
	}

	if _, err := coord.dispatchLocked(
		context.Background(),
		CommandRequest{ID: "cmd-b", Kind: CommandRunStart, RunID: "run-b"},
		"run-b",
		CommandResult{CommandID: "cmd-b", RunID: "run-b"},
	); err != nil {
		t.Fatalf("start run-b: %v", err)
	}
	if backend.runs["run-b"].State != string(RunStateBlockedResource) {
		t.Fatalf("run-b state=%q, want blocked_resource", backend.runs["run-b"].State)
	}
	if backend.resourceOwner != "run-a" || lanes.owner != "run-a" {
		t.Fatalf("run-a lost exclusive authority: resource=%q lane=%q", backend.resourceOwner, lanes.owner)
	}
	if len(backend.waiters) != 1 || backend.waiters[0] != "run-b" {
		t.Fatalf("run-b did not become deterministic FIFO waiter: %#v", backend.waiters)
	}

	backend.engineRunning = false
	backend.runtimeState = "completed"
	if err := coord.reconcileAndScheduleLocked(context.Background()); err != nil {
		t.Fatalf("handoff after run-a completion: %v", err)
	}
	if backend.runs["run-a"].State != string(RunStateCompleted) {
		t.Fatalf("run-a state=%q, want completed", backend.runs["run-a"].State)
	}
	if backend.runs["run-b"].State != string(RunStateRunning) {
		t.Fatalf("run-b state=%q, want running", backend.runs["run-b"].State)
	}
	if backend.resourceOwner != "run-b" || lanes.owner != "run-b" || coord.activeRun != "run-b" {
		t.Fatalf("serialized handoff drifted: resource=%q lane=%q active=%q", backend.resourceOwner, lanes.owner, coord.activeRun)
	}

	backend.engineRunning = false
	backend.runtimeState = "completed"
	if err := coord.reconcileAndScheduleLocked(context.Background()); err != nil {
		t.Fatalf("finalize run-b: %v", err)
	}
	if backend.resourceOwner != "" || lanes.owner != "" || coord.activeRun != "" || len(backend.tickets) != 0 {
		t.Fatalf("terminal cleanup leaked authority: resource=%q lane=%q active=%q tickets=%#v", backend.resourceOwner, lanes.owner, coord.activeRun, backend.tickets)
	}
}

func TestG058RestartRecoversInterruptedRunningRunThroughDurableResume(t *testing.T) {
	backend := newFakeRunBackend()
	backend.runs["run-orphan"] = g058RunRecord("run-orphan", RunStateRunning)
	backend.resourceOwner = "run-orphan"
	lanes := &fakeRunLanePool{}
	coord := newRunCoordinator(backend, lanes)

	if err := coord.recoverRestart(context.Background()); err != nil {
		t.Fatalf("recover restart: %v", err)
	}
	if backend.resourceOwner != "" {
		t.Fatalf("stale owner survived restart: %q", backend.resourceOwner)
	}
	if got := backend.runs["run-orphan"].State; got != string(RunStateQueued) {
		t.Fatalf("recovered state=%q, want queued", got)
	}
	if len(backend.tickets) != 1 || backend.tickets[0].RunID != "run-orphan" || backend.tickets[0].Priority != domain.DefaultRunSchedulerPriority {
		t.Fatalf("orphan was not deterministically requeued: %#v", backend.tickets)
	}
	if len(backend.history["run-orphan"]) < 2 {
		t.Fatalf("restart recovery history missing transitions: %#v", backend.history["run-orphan"])
	}

	if err := coord.reconcileAndScheduleLocked(context.Background()); err != nil {
		t.Fatalf("resume recovered run: %v", err)
	}
	if got := backend.runs["run-orphan"].State; got != string(RunStateRunning) {
		t.Fatalf("resumed state=%q, want running", got)
	}
	if backend.resourceOwner != "run-orphan" || lanes.owner != "run-orphan" || coord.activeRun != "run-orphan" {
		t.Fatalf("recovered run did not reacquire bounded authorities: resource=%q lane=%q active=%q", backend.resourceOwner, lanes.owner, coord.activeRun)
	}
}

func TestG058RestartPreservesQueuedPriorityAndResourceFIFO(t *testing.T) {
	backend := newFakeRunBackend()
	backend.runs["run-wait"] = g058RunRecord("run-wait", RunStateBlockedResource)
	backend.tickets = []domain.RunScheduleTicket{{
		RunID:       "run-wait",
		Priority:    domain.RunSchedulerPriorityHigh,
		EnqueueSeq:  17,
		EnqueueTurn: 9,
	}}
	backend.waiters = []domain.RunID{"run-wait"}
	lanes := &fakeRunLanePool{}
	coord := newRunCoordinator(backend, lanes)

	if err := coord.recoverRestart(context.Background()); err != nil {
		t.Fatalf("recover restart: %v", err)
	}
	if got := backend.runs["run-wait"].State; got != string(RunStateBlockedResource) {
		t.Fatalf("blocked state drifted to %q", got)
	}
	if len(backend.tickets) != 1 || backend.tickets[0].Priority != domain.RunSchedulerPriorityHigh || backend.tickets[0].EnqueueSeq != 17 || backend.tickets[0].EnqueueTurn != 9 {
		t.Fatalf("scheduler priority/age drifted across restart: %#v", backend.tickets)
	}
	if len(backend.waiters) != 1 || backend.waiters[0] != "run-wait" {
		t.Fatalf("FIFO waiter drifted across restart: %#v", backend.waiters)
	}
}

func TestG058RestartCompletesAcceptedStopLikeIntentWithoutAutoResume(t *testing.T) {
	cases := []struct {
		name string
		from RunLifecycleState
		want RunLifecycleState
	}{
		{name: "pause", from: RunStatePausing, want: RunStatePaused},
		{name: "stop", from: RunStateStopping, want: RunStateStopped},
		{name: "cancel", from: RunStateCancelling, want: RunStateCancelled},
		{name: "already-paused", from: RunStatePaused, want: RunStatePaused},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			backend := newFakeRunBackend()
			backend.runs["run-intent"] = g058RunRecord("run-intent", tc.from)
			backend.tickets = []domain.RunScheduleTicket{{RunID: "run-intent", Priority: domain.RunSchedulerPriorityNormal, EnqueueSeq: 1, EnqueueTurn: 1}}
			backend.resourceOwner = "run-intent"
			lanes := &fakeRunLanePool{owner: "run-intent"}
			coord := newRunCoordinator(backend, lanes)

			if err := coord.recoverRestart(context.Background()); err != nil {
				t.Fatalf("recover restart: %v", err)
			}
			if got := backend.runs["run-intent"].State; got != string(tc.want) {
				t.Fatalf("state=%q, want %q", got, tc.want)
			}
			if len(backend.tickets) != 0 || backend.resourceOwner != "" || lanes.owner != "" {
				t.Fatalf("restart intent cleanup leaked authority: tickets=%#v resource=%q lane=%q", backend.tickets, backend.resourceOwner, lanes.owner)
			}
			if backend.engineRunning {
				t.Fatal("stable stop-like state auto-resumed Engine")
			}
		})
	}
}

func TestG058RestartFailsClosedOnUnknownPersistedState(t *testing.T) {
	backend := newFakeRunBackend()
	record := g058RunRecord("run-unknown", RunStateQueued)
	record.State = "unknown_state"
	backend.runs[record.RunID] = record
	backend.tickets = []domain.RunScheduleTicket{{RunID: record.RunID, Priority: domain.RunSchedulerPriorityNormal, EnqueueSeq: 1, EnqueueTurn: 1}}
	backend.resourceOwner = record.RunID
	lanes := &fakeRunLanePool{owner: record.RunID}
	coord := newRunCoordinator(backend, lanes)

	if err := coord.recoverRestart(context.Background()); err != nil {
		t.Fatalf("recover restart: %v", err)
	}
	if got := backend.runs[record.RunID].State; got != string(RunStateFailed) {
		t.Fatalf("unknown persisted state recovered as %q, want failed", got)
	}
	if len(backend.tickets) != 0 || backend.resourceOwner != "" || lanes.owner != "" {
		t.Fatalf("fail-closed recovery leaked authority: tickets=%#v resource=%q lane=%q", backend.tickets, backend.resourceOwner, lanes.owner)
	}
}
