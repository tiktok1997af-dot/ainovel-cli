package appruntime

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/webai"
)

type fakeRunBackend struct {
	runs          map[domain.RunID]domain.RunRegistryRecord
	history       map[domain.RunID][]domain.RunHistoryRecord
	tickets       []domain.RunScheduleTicket
	resource      domain.ResourceKey
	resourceOwner domain.RunID
	waiters       []domain.RunID
	engineRunning bool
	runtimeState  string
	resumeLabel   string
	resumeErr     error
	abortAccepted bool
}

func newFakeRunBackend() *fakeRunBackend {
	return &fakeRunBackend{
		runs:          make(map[domain.RunID]domain.RunRegistryRecord),
		history:       make(map[domain.RunID][]domain.RunHistoryRecord),
		resource:      domain.ResourceKey("story:test"),
		runtimeState:  "idle",
		resumeLabel:   "resume",
		abortAccepted: true,
	}
}

func (f *fakeRunBackend) DesktopRunsList() ([]domain.RunRegistryRecord, error) {
	out := make([]domain.RunRegistryRecord, 0, len(f.runs))
	for _, record := range f.runs {
		out = append(out, record)
	}
	return out, nil
}

func (f *fakeRunBackend) DesktopRunLoad(runID domain.RunID) (*domain.RunRegistryRecord, error) {
	record, ok := f.runs[runID]
	if !ok {
		return nil, nil
	}
	copy := record
	return &copy, nil
}

func (f *fakeRunBackend) DesktopRunSave(record domain.RunRegistryRecord) (domain.RunRegistryRecord, error) {
	if record.Version == 0 {
		record.Version = domain.CurrentRunRegistryRecordVersion
	}
	if record.Source == "" {
		record.Source = domain.RunRecordSourceRegistry
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now().UTC()
	}
	if record.UpdatedAt.IsZero() {
		record.UpdatedAt = time.Now().UTC()
	}
	f.runs[record.RunID] = record
	return record, nil
}

func (f *fakeRunBackend) DesktopRunAppendHistory(entry domain.RunHistoryRecord) (domain.RunHistoryRecord, error) {
	entry.Seq = int64(len(f.history[entry.RunID]) + 1)
	if entry.Time.IsZero() {
		entry.Time = time.Now().UTC()
	}
	f.history[entry.RunID] = append(f.history[entry.RunID], entry)
	return entry, nil
}

func (f *fakeRunBackend) DesktopRunHistory(runID domain.RunID) ([]domain.RunHistoryRecord, error) {
	return append([]domain.RunHistoryRecord(nil), f.history[runID]...), nil
}

func (f *fakeRunBackend) DesktopScheduledRuns() ([]domain.RunScheduleTicket, error) {
	return append([]domain.RunScheduleTicket(nil), f.tickets...), nil
}

func (f *fakeRunBackend) DesktopEnqueueRun(runID domain.RunID, priority domain.RunSchedulerPriority) (domain.RunScheduleTicket, error) {
	for _, ticket := range f.tickets {
		if ticket.RunID == runID {
			return ticket, nil
		}
	}
	if priority == "" {
		priority = domain.DefaultRunSchedulerPriority
	}
	ticket := domain.RunScheduleTicket{
		RunID:      runID,
		Priority:   priority,
		EnqueueSeq: int64(len(f.tickets) + 1),
	}
	f.tickets = append(f.tickets, ticket)
	return ticket, nil
}

func (f *fakeRunBackend) DesktopDequeueScheduledRun(runID domain.RunID) (*domain.RunScheduleTicket, error) {
	for i, ticket := range f.tickets {
		if ticket.RunID == runID {
			f.tickets = append(f.tickets[:i], f.tickets[i+1:]...)
			copy := ticket
			return &copy, nil
		}
	}
	return nil, fmt.Errorf("run not queued")
}

func (f *fakeRunBackend) DesktopRemoveScheduledRun(runID domain.RunID) error {
	for i, ticket := range f.tickets {
		if ticket.RunID == runID {
			f.tickets = append(f.tickets[:i], f.tickets[i+1:]...)
			return nil
		}
	}
	return nil
}

func (f *fakeRunBackend) DesktopStoryResourceKey() (domain.ResourceKey, error) {
	return f.resource, nil
}

func (f *fakeRunBackend) DesktopResourceLocks() ([]domain.ResourceLockState, error) {
	if f.resourceOwner == "" && len(f.waiters) == 0 {
		return nil, nil
	}
	state := domain.ResourceLockState{Resource: f.resource, OwnerRunID: f.resourceOwner}
	for i, runID := range f.waiters {
		state.Waiters = append(state.Waiters, domain.ResourceLockWaiter{
			RunID:      runID,
			RequestSeq: int64(i + 1),
		})
	}
	return []domain.ResourceLockState{state}, nil
}

func (f *fakeRunBackend) DesktopRequestRunResource(runID domain.RunID, resource domain.ResourceKey) (domain.ResourceLockDecision, error) {
	if resource != f.resource {
		return domain.ResourceLockDecision{}, fmt.Errorf("unexpected resource")
	}
	if f.resourceOwner == runID {
		return domain.ResourceLockDecision{
			Status:     domain.ResourceLockStatusAcquired,
			Resource:   resource,
			RunID:      runID,
			OwnerRunID: runID,
		}, nil
	}
	if f.resourceOwner != "" || (len(f.waiters) > 0 && f.waiters[0] != runID) {
		for _, existing := range f.waiters {
			if existing == runID {
				return domain.ResourceLockDecision{
					Status:     domain.ResourceLockStatusWaiting,
					Resource:   resource,
					RunID:      runID,
					OwnerRunID: f.resourceOwner,
				}, nil
			}
		}
		f.waiters = append(f.waiters, runID)
		return domain.ResourceLockDecision{
			Status:     domain.ResourceLockStatusWaiting,
			Resource:   resource,
			RunID:      runID,
			OwnerRunID: f.resourceOwner,
		}, nil
	}
	if len(f.waiters) > 0 && f.waiters[0] == runID {
		f.waiters = f.waiters[1:]
	}
	f.resourceOwner = runID
	return domain.ResourceLockDecision{
		Status:     domain.ResourceLockStatusAcquired,
		Resource:   resource,
		RunID:      runID,
		OwnerRunID: runID,
	}, nil
}

func (f *fakeRunBackend) DesktopReleaseRunResourceClaims(runID domain.RunID) error {
	if f.resourceOwner == runID {
		f.resourceOwner = ""
	}
	filtered := f.waiters[:0]
	for _, waiter := range f.waiters {
		if waiter != runID {
			filtered = append(filtered, waiter)
		}
	}
	f.waiters = append([]domain.RunID(nil), filtered...)
	return nil
}

func (f *fakeRunBackend) DesktopEngineRunning() bool                    { return f.engineRunning }
func (f *fakeRunBackend) DesktopRuntimeState() string                   { return f.runtimeState }
func (f *fakeRunBackend) DesktopEnsureWebSession(context.Context) error { return nil }
func (f *fakeRunBackend) Resume() (string, error) {
	if f.resumeErr != nil {
		return "", f.resumeErr
	}
	if f.resumeLabel != "" {
		f.engineRunning = true
		f.runtimeState = "running"
	}
	return f.resumeLabel, nil
}
func (f *fakeRunBackend) Abort() bool {
	if !f.abortAccepted {
		return false
	}
	f.engineRunning = false
	f.runtimeState = "paused"
	return true
}

type fakeRunLanePool struct {
	owner domain.RunID
}

func (f *fakeRunLanePool) Allocate(_ context.Context, runID domain.RunID) (webai.BrowserLaneProjection, error) {
	if f.owner != "" && f.owner != runID {
		return webai.BrowserLaneProjection{}, webai.ErrNoBrowserLaneAvailable
	}
	f.owner = runID
	return webai.BrowserLaneProjection{
		LaneID: "lane-001",
		State:  webai.BrowserLaneBusy,
		RunID:  runID,
	}, nil
}

func (f *fakeRunLanePool) Refresh(_ context.Context, runID domain.RunID) (webai.BrowserLaneProjection, error) {
	return f.Allocate(context.Background(), runID)
}

func (f *fakeRunLanePool) Release(runID domain.RunID) error {
	if f.owner == runID {
		f.owner = ""
	}
	return nil
}

func (f *fakeRunLanePool) List() []webai.BrowserLaneProjection {
	state := webai.BrowserLaneReady
	if f.owner != "" {
		state = webai.BrowserLaneBusy
	}
	return []webai.BrowserLaneProjection{{
		LaneID: "lane-001",
		State:  state,
		RunID:  f.owner,
	}}
}

func (f *fakeRunLanePool) StopAll() error {
	f.owner = ""
	return nil
}

func TestRunCoordinatorStartsQueuedRunThroughAllAuthorities(t *testing.T) {
	backend := newFakeRunBackend()
	lanes := &fakeRunLanePool{}
	rt := &Runtime{subscribers: make(map[uint64]*desktopSubscription)}
	coord := newRunCoordinator(backend, lanes)
	coord.owner = rt

	result, err := coord.dispatchLocked(
		context.Background(),
		CommandRequest{ID: "cmd-1", Kind: CommandRunStart, RunID: "run-001"},
		domain.RunID("run-001"),
		CommandResult{CommandID: "cmd-1", RunID: "run-001"},
	)
	if err != nil {
		t.Fatalf("run.start: %v", err)
	}
	if !result.Accepted {
		t.Fatalf("run.start not accepted: %+v", result)
	}
	record := backend.runs["run-001"]
	if record.State != string(RunStateRunning) {
		t.Fatalf("state=%q, want running", record.State)
	}
	if backend.resourceOwner != "run-001" || lanes.owner != "run-001" || len(backend.tickets) != 0 {
		t.Fatalf("authorities not owned by active run: resource=%q lane=%q tickets=%+v", backend.resourceOwner, lanes.owner, backend.tickets)
	}
	if !backend.engineRunning {
		t.Fatal("engine was not resumed")
	}
}

func TestRunCoordinatorFinalizesCompletedRunAndReleasesAuthorities(t *testing.T) {
	backend := newFakeRunBackend()
	lanes := &fakeRunLanePool{}
	rt := &Runtime{subscribers: make(map[uint64]*desktopSubscription)}
	coord := newRunCoordinator(backend, lanes)
	coord.owner = rt

	if _, err := coord.dispatchLocked(
		context.Background(),
		CommandRequest{ID: "cmd-1", Kind: CommandRunStart, RunID: "run-001"},
		domain.RunID("run-001"),
		CommandResult{},
	); err != nil {
		t.Fatalf("run.start: %v", err)
	}
	backend.engineRunning = false
	backend.runtimeState = "completed"
	if err := coord.reconcileAndScheduleLocked(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	record := backend.runs["run-001"]
	if record.State != string(RunStateCompleted) {
		t.Fatalf("state=%q, want completed", record.State)
	}
	if backend.resourceOwner != "" || lanes.owner != "" || coord.activeRun != "" {
		t.Fatalf("terminal cleanup leaked authority: resource=%q lane=%q active=%q", backend.resourceOwner, lanes.owner, coord.activeRun)
	}
}

func TestRunCoordinatorQueuedCancelNeverStartsEngine(t *testing.T) {
	backend := newFakeRunBackend()
	backend.engineRunning = true // legacy execution temporarily occupies Host.
	lanes := &fakeRunLanePool{}
	coord := newRunCoordinator(backend, lanes)

	if _, err := coord.dispatchLocked(
		context.Background(),
		CommandRequest{Kind: CommandRunStart, RunID: "run-001"},
		domain.RunID("run-001"),
		CommandResult{},
	); err != nil {
		t.Fatalf("queue run: %v", err)
	}
	backend.engineRunning = false
	record, _ := backend.DesktopRunLoad("run-001")
	result, err := coord.stopLikeLocked(
		context.Background(),
		CommandRequest{Kind: CommandRunCancel, RunID: "run-001"},
		record,
		"run-001",
		RunStateCancelling,
		RunStateCancelled,
		CommandResult{},
	)
	if err != nil {
		t.Fatalf("cancel queued run: %v", err)
	}
	if !result.Accepted || backend.runs["run-001"].State != string(RunStateCancelled) {
		t.Fatalf("queued cancel result=%+v record=%+v", result, backend.runs["run-001"])
	}
	if len(backend.tickets) != 0 || backend.resourceOwner != "" || lanes.owner != "" {
		t.Fatal("queued cancel leaked scheduler/resource/lane authority")
	}
}
