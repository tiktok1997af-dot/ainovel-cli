package appruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/webai"
)

type RunLifecycleState string

const (
	RunStateQueued          RunLifecycleState = "queued"
	RunStateBlockedResource RunLifecycleState = "blocked_resource"
	RunStateBlockedLane     RunLifecycleState = "blocked_lane"
	RunStateStarting        RunLifecycleState = "starting"
	RunStateRunning         RunLifecycleState = "running"
	RunStatePausing         RunLifecycleState = "pausing"
	RunStatePaused          RunLifecycleState = "paused"
	RunStateStopping        RunLifecycleState = "stopping"
	RunStateStopped         RunLifecycleState = "stopped"
	RunStateCancelling      RunLifecycleState = "cancelling"
	RunStateCancelled       RunLifecycleState = "cancelled"
	RunStateRecovering      RunLifecycleState = "recovering"
	RunStateFailed          RunLifecycleState = "failed"
	RunStateCompleted       RunLifecycleState = "completed"
)

const runSchedulerPollInterval = 100 * time.Millisecond

type runBackend interface {
	DesktopRunsList() ([]domain.RunRegistryRecord, error)
	DesktopRunLoad(domain.RunID) (*domain.RunRegistryRecord, error)
	DesktopRunSave(domain.RunRegistryRecord) (domain.RunRegistryRecord, error)
	DesktopRunAppendHistory(domain.RunHistoryRecord) (domain.RunHistoryRecord, error)
	DesktopRunHistory(domain.RunID) ([]domain.RunHistoryRecord, error)

	DesktopScheduledRuns() ([]domain.RunScheduleTicket, error)
	DesktopEnqueueRun(domain.RunID, domain.RunSchedulerPriority) (domain.RunScheduleTicket, error)
	DesktopDequeueScheduledRun(domain.RunID) (*domain.RunScheduleTicket, error)
	DesktopRemoveScheduledRun(domain.RunID) error

	DesktopStoryResourceKey() (domain.ResourceKey, error)
	DesktopResourceLocks() ([]domain.ResourceLockState, error)
	DesktopRequestRunResource(domain.RunID, domain.ResourceKey) (domain.ResourceLockDecision, error)
	DesktopReleaseRunResourceClaims(domain.RunID) error

	DesktopEngineRunning() bool
	DesktopRuntimeState() string
	DesktopEnsureWebSession(context.Context) error
	Resume() (string, error)
	Abort() bool
}

type runLanePool interface {
	Allocate(context.Context, domain.RunID) (webai.BrowserLaneProjection, error)
	Refresh(context.Context, domain.RunID) (webai.BrowserLaneProjection, error)
	Release(domain.RunID) error
	List() []webai.BrowserLaneProjection
	StopAll() error
}

type runCoordinator struct {
	backend runBackend
	lanes   runLanePool
	owner   *Runtime

	mu         sync.Mutex
	activeRun  domain.RunID
	activeFlag atomic.Bool
	stopTarget RunLifecycleState

	ctx    context.Context
	cancel context.CancelFunc
	wakeCh chan struct{}
	wg     sync.WaitGroup
}

func newRunCoordinator(backend runBackend, lanes runLanePool) *runCoordinator {
	return &runCoordinator{
		backend: backend,
		lanes:   lanes,
		wakeCh:  make(chan struct{}, 1),
	}
}

func (c *runCoordinator) start(owner *Runtime) {
	if c == nil || c.backend == nil || c.lanes == nil || owner == nil {
		return
	}
	c.owner = owner
	c.ctx, c.cancel = context.WithCancel(context.Background())
	c.wg.Add(1)
	go c.loop()
}

func (c *runCoordinator) close() {
	if c == nil {
		return
	}
	if c.cancel != nil {
		c.cancel()
	}
	c.wg.Wait()
	if c.lanes != nil {
		_ = c.lanes.StopAll()
	}
}

func (c *runCoordinator) wake() {
	if c == nil {
		return
	}
	select {
	case c.wakeCh <- struct{}{}:
	default:
	}
}

func (c *runCoordinator) loop() {
	defer c.wg.Done()
	ticker := time.NewTicker(runSchedulerPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
		case <-c.wakeCh:
		}
		if c.owner == nil || c.owner.closed.Load() {
			continue
		}
		c.owner.commandMu.Lock()
		c.mu.Lock()
		_ = c.reconcileAndScheduleLocked(c.ctx)
		c.mu.Unlock()
		c.owner.commandMu.Unlock()
	}
}

func (r *Runtime) dispatchRunControl(ctx context.Context, cmd CommandRequest) (CommandResult, error) {
	r.commandMu.Lock()
	defer r.commandMu.Unlock()

	cmd.ID = r.ensureCommandID(cmd.ID)
	result := CommandResult{
		CommandID: cmd.ID,
		RunID:     cmd.RunID,
		TaskID:    cmd.TaskID,
	}
	if r.run == nil {
		return result, ErrNotImplemented
	}
	runID := domain.RunID(strings.TrimSpace(cmd.RunID))
	if err := (domain.RunIdentity{RunID: runID}).Validate(); err != nil || runID == domain.LegacySingleRunID {
		return result, fmt.Errorf("%w: invalid run identity", ErrInvalidCommand)
	}
	if len(cmd.Payload) != 0 && string(cmd.Payload) != "null" && string(cmd.Payload) != "{}" {
		return result, fmt.Errorf("%w: run control payload is not supported", ErrInvalidCommand)
	}

	r.run.mu.Lock()
	defer r.run.mu.Unlock()

	out, err := r.run.dispatchLocked(ctx, cmd, runID, result)
	if err == nil {
		r.run.wake()
	}
	return out, err
}

func (c *runCoordinator) dispatchLocked(ctx context.Context, cmd CommandRequest, runID domain.RunID, result CommandResult) (CommandResult, error) {
	if err := ctx.Err(); err != nil {
		return result, err
	}
	record, err := c.backend.DesktopRunLoad(runID)
	if err != nil {
		return result, err
	}

	switch cmd.Kind {
	case CommandRunStart:
		if record == nil {
			record = &domain.RunRegistryRecord{
				RunID:  runID,
				Source: domain.RunRecordSourceRegistry,
			}
		} else if runStateIsActiveOrWaiting(record.State) {
			result.Accepted = true
			result.Status = record.State
			return result, nil
		} else if record.State != "" {
			// Terminal/restartable records use run.retry so start and retry remain
			// distinct lifecycle intents instead of silently collapsing together.
			return result, runCommandStateError(cmd.Kind, record.State)
		}
		if err := c.queueRecordLocked(record, RunStateQueued, "waiting for deterministic scheduler"); err != nil {
			return result, err
		}
		result.Accepted = true
		result.Status = string(RunStateQueued)
		if err := c.reconcileAndScheduleLocked(ctx); err != nil {
			return result, err
		}
		if latest, loadErr := c.backend.DesktopRunLoad(runID); loadErr == nil && latest != nil {
			result.Status = latest.State
		}
		return result, nil

	case CommandRunResume:
		if record == nil || record.State != string(RunStatePaused) {
			return result, runCommandStateError(cmd.Kind, runRecordState(record))
		}
		if err := c.queueRecordLocked(record, RunStateQueued, "resume waiting for deterministic scheduler"); err != nil {
			return result, err
		}
		result.Accepted = true
		result.Status = string(RunStateQueued)
		if err := c.reconcileAndScheduleLocked(ctx); err != nil {
			return result, err
		}
		return result, nil

	case CommandRunRetry:
		if record == nil || !runStateAllowsRetry(record.State) {
			return result, runCommandStateError(cmd.Kind, runRecordState(record))
		}
		if err := c.setRunStateLocked(record, RunStateRecovering, "retry restoring durable project facts"); err != nil {
			return result, err
		}
		if _, err := c.backend.DesktopEnqueueRun(runID, domain.DefaultRunSchedulerPriority); err != nil {
			return result, err
		}
		if err := c.setRunStateLocked(record, RunStateQueued, "retry waiting for deterministic scheduler"); err != nil {
			return result, err
		}
		result.Accepted = true
		result.Status = string(RunStateQueued)
		if err := c.reconcileAndScheduleLocked(ctx); err != nil {
			return result, err
		}
		return result, nil

	case CommandRunPause:
		return c.stopLikeLocked(ctx, cmd, record, runID, RunStatePausing, RunStatePaused, result)
	case CommandRunStop:
		return c.stopLikeLocked(ctx, cmd, record, runID, RunStateStopping, RunStateStopped, result)
	case CommandRunCancel:
		return c.stopLikeLocked(ctx, cmd, record, runID, RunStateCancelling, RunStateCancelled, result)
	default:
		return result, fmt.Errorf("%w: %q", ErrInvalidCommand, cmd.Kind)
	}
}

func (c *runCoordinator) stopLikeLocked(
	ctx context.Context,
	cmd CommandRequest,
	record *domain.RunRegistryRecord,
	runID domain.RunID,
	transition RunLifecycleState,
	final RunLifecycleState,
	result CommandResult,
) (CommandResult, error) {
	if record == nil {
		return result, runCommandStateError(cmd.Kind, "")
	}
	if c.activeRun == runID {
		if record.State != string(RunStateRunning) && record.State != string(RunStateStarting) {
			return result, runCommandStateError(cmd.Kind, record.State)
		}
		if !c.backend.Abort() {
			return result, fmt.Errorf("%w: core did not accept %s", ErrCommandRejected, cmd.Kind)
		}
		c.stopTarget = final
		if err := c.setRunStateLocked(record, transition, string(transition)); err != nil {
			return result, err
		}
		result.Accepted = true
		result.Status = string(transition)
		return result, nil
	}

	switch RunLifecycleState(record.State) {
	case RunStateQueued, RunStateBlockedResource, RunStateBlockedLane, RunStateRecovering:
		if err := c.backend.DesktopRemoveScheduledRun(runID); err != nil {
			return result, err
		}
		if err := c.backend.DesktopReleaseRunResourceClaims(runID); err != nil {
			return result, err
		}
		_ = c.lanes.Release(runID)
		if err := c.setRunStateLocked(record, final, string(final)); err != nil {
			return result, err
		}
		result.Accepted = true
		result.Status = string(final)
		return result, nil
	case RunStatePaused:
		if final == RunStatePaused {
			result.Accepted = true
			result.Status = record.State
			return result, nil
		}
		if err := c.setRunStateLocked(record, final, string(final)); err != nil {
			return result, err
		}
		result.Accepted = true
		result.Status = string(final)
		return result, nil
	default:
		return result, runCommandStateError(cmd.Kind, record.State)
	}
}

func (c *runCoordinator) reconcileAndScheduleLocked(ctx context.Context) error {
	if c.activeRun != "" {
		if c.backend.DesktopEngineRunning() {
			return c.registerNextResourceWaiterLocked()
		}
		if err := c.finalizeActiveLocked(ctx); err != nil {
			return err
		}
	}
	if c.backend.DesktopEngineRunning() {
		return nil
	}
	return c.startNextLocked(ctx)
}

func (c *runCoordinator) registerNextResourceWaiterLocked() error {
	tickets, err := c.backend.DesktopScheduledRuns()
	if err != nil || len(tickets) == 0 {
		return err
	}
	resource, err := c.backend.DesktopStoryResourceKey()
	if err != nil {
		return err
	}
	runID := tickets[0].RunID
	decision, err := c.backend.DesktopRequestRunResource(runID, resource)
	if err != nil {
		return err
	}
	if decision.Status == domain.ResourceLockStatusWaiting {
		record, loadErr := c.backend.DesktopRunLoad(runID)
		if loadErr != nil {
			return loadErr
		}
		if record != nil && record.State != string(RunStateBlockedResource) {
			return c.setRunStateLocked(record, RunStateBlockedResource, "waiting for canonical story resource")
		}
	}
	return nil
}

func (c *runCoordinator) startNextLocked(ctx context.Context) error {
	tickets, err := c.backend.DesktopScheduledRuns()
	if err != nil || len(tickets) == 0 {
		return err
	}
	resource, err := c.backend.DesktopStoryResourceKey()
	if err != nil {
		return err
	}
	candidate := tickets[0].RunID
	locks, err := c.backend.DesktopResourceLocks()
	if err != nil {
		return err
	}
	for _, state := range locks {
		if state.Resource != resource {
			continue
		}
		if state.OwnerRunID != "" {
			decision, requestErr := c.backend.DesktopRequestRunResource(candidate, resource)
			if requestErr != nil {
				return requestErr
			}
			if decision.Status == domain.ResourceLockStatusWaiting {
				record, loadErr := c.backend.DesktopRunLoad(candidate)
				if loadErr != nil {
					return loadErr
				}
				if record != nil {
					return c.setRunStateLocked(record, RunStateBlockedResource, "waiting for canonical story resource")
				}
			}
			return nil
		}
		if len(state.Waiters) > 0 && scheduledRunExists(tickets, state.Waiters[0].RunID) {
			candidate = state.Waiters[0].RunID
		}
		break
	}

	decision, err := c.backend.DesktopRequestRunResource(candidate, resource)
	if err != nil {
		return err
	}
	record, err := c.backend.DesktopRunLoad(candidate)
	if err != nil {
		return err
	}
	if record == nil {
		return fmt.Errorf("run %q disappeared before orchestration", candidate)
	}
	if decision.Status == domain.ResourceLockStatusWaiting {
		return c.setRunStateLocked(record, RunStateBlockedResource, "waiting for canonical story resource")
	}

	lane, laneErr := c.lanes.Allocate(ctx, candidate)
	if errors.Is(laneErr, webai.ErrNoBrowserLaneAvailable) {
		_ = c.backend.DesktopReleaseRunResourceClaims(candidate)
		return c.setRunStateLocked(record, RunStateBlockedLane, "waiting for browser lane")
	}
	if laneErr != nil && (lane.State == webai.BrowserLaneFailed || lane.State == webai.BrowserLaneStopped) {
		_ = c.backend.DesktopReleaseRunResourceClaims(candidate)
		_ = c.backend.DesktopRemoveScheduledRun(candidate)
		return c.setRunStateLocked(record, RunStateFailed, "browser lane failed before execution")
	}
	if lane.State != webai.BrowserLaneBusy {
		refreshed, refreshErr := c.lanes.Refresh(ctx, candidate)
		if refreshErr != nil && refreshed.State == webai.BrowserLaneFailed {
			_ = c.backend.DesktopReleaseRunResourceClaims(candidate)
			_ = c.backend.DesktopRemoveScheduledRun(candidate)
			return c.setRunStateLocked(record, RunStateFailed, "browser lane readiness failed")
		}
		lane = refreshed
	}
	if lane.State != webai.BrowserLaneBusy {
		_ = c.backend.DesktopReleaseRunResourceClaims(candidate)
		return c.setRunStateLocked(record, RunStateBlockedLane, "browser lane requires readiness")
	}

	if _, err := c.backend.DesktopDequeueScheduledRun(candidate); err != nil {
		_ = c.backend.DesktopReleaseRunResourceClaims(candidate)
		_ = c.lanes.Release(candidate)
		return err
	}
	if err := c.setRunStateLocked(record, RunStateStarting, "scheduler selected runnable execution"); err != nil {
		_ = c.backend.DesktopReleaseRunResourceClaims(candidate)
		_ = c.lanes.Release(candidate)
		return err
	}

	label, err := c.backend.Resume()
	if err != nil {
		_ = c.setRunStateLocked(record, RunStateFailed, "core resume failed")
		_ = c.backend.DesktopReleaseRunResourceClaims(candidate)
		_ = c.lanes.Release(candidate)
		_ = c.backend.DesktopEnsureWebSession(context.Background())
		return err
	}
	if strings.TrimSpace(label) == "" {
		if err := c.setRunStateLocked(record, RunStateCompleted, "no resumable project work"); err != nil {
			return err
		}
		_ = c.backend.DesktopReleaseRunResourceClaims(candidate)
		_ = c.lanes.Release(candidate)
		_ = c.backend.DesktopEnsureWebSession(context.Background())
		return nil
	}

	c.activeRun = candidate
	c.activeFlag.Store(true)
	c.stopTarget = ""
	if c.owner != nil {
		c.owner.setLifecycleState(LifecycleRunning)
	}
	return c.setRunStateLocked(record, RunStateRunning, "engine running")
}

func (c *runCoordinator) finalizeActiveLocked(ctx context.Context) error {
	runID := c.activeRun
	record, err := c.backend.DesktopRunLoad(runID)
	if err != nil {
		return err
	}
	if record == nil {
		return fmt.Errorf("active run %q is not registered", runID)
	}

	final := c.stopTarget
	status := ""
	if final == "" {
		switch strings.ToLower(strings.TrimSpace(c.backend.DesktopRuntimeState())) {
		case "completed":
			final = RunStateCompleted
			status = "project execution completed"
		case "paused", "pausing":
			final = RunStatePaused
			status = "engine paused with durable facts retained"
		default:
			final = RunStateFailed
			status = "engine stopped outside an accepted run transition"
		}
	}
	if status == "" {
		status = string(final)
	}
	if err := c.setRunStateLocked(record, final, status); err != nil {
		return err
	}

	_ = c.backend.DesktopReleaseRunResourceClaims(runID)
	_ = c.lanes.Release(runID)
	c.activeRun = ""
	c.activeFlag.Store(false)
	c.stopTarget = ""
	if c.owner != nil {
		c.owner.setLifecycleState(desktopLifecycleForRun(final))
	}
	if err := c.backend.DesktopEnsureWebSession(ctx); err != nil {
		// Run completion remains durable even if browser warm-up is degraded.
		return nil
	}
	return nil
}

func (c *runCoordinator) queueRecordLocked(record *domain.RunRegistryRecord, state RunLifecycleState, status string) error {
	if record == nil {
		return fmt.Errorf("run record is required")
	}
	if record.State != string(state) {
		if err := c.setRunStateLocked(record, state, status); err != nil {
			return err
		}
	}
	_, err := c.backend.DesktopEnqueueRun(record.RunID, domain.DefaultRunSchedulerPriority)
	return err
}

func (c *runCoordinator) setRunStateLocked(record *domain.RunRegistryRecord, state RunLifecycleState, status string) error {
	if record == nil {
		return fmt.Errorf("run record is required")
	}
	previous := record.State
	now := time.Now().UTC()
	record.State = string(state)
	record.Status = strings.TrimSpace(status)
	record.UpdatedAt = now
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}
	if (state == RunStateStarting || state == RunStateRunning) && record.StartedAt.IsZero() {
		record.StartedAt = now
	}
	if runStateIsTerminal(string(state)) {
		record.FinishedAt = now
	} else {
		record.FinishedAt = time.Time{}
	}
	saved, err := c.backend.DesktopRunSave(*record)
	if err != nil {
		return err
	}
	*record = saved
	level := "info"
	if state == RunStateFailed {
		level = "error"
	}
	if _, err := c.backend.DesktopRunAppendHistory(domain.RunHistoryRecord{
		Time:     now,
		RunID:    record.RunID,
		TaskID:   record.TaskID,
		Category: RunEventCategory,
		Type:     EventTypeRunState,
		Level:    level,
		Summary:  record.Status,
	}); err != nil {
		return err
	}
	c.emitRunStateLocked(record.RunID, previous, state, record.Status)
	return nil
}

func (c *runCoordinator) emitRunStateLocked(runID domain.RunID, previous string, state RunLifecycleState, status string) {
	if c.owner == nil {
		return
	}
	resource, laneID := c.runOwnershipLocked(runID)
	payload, _ := json.Marshal(RunStateEventPayloadDTO{
		PreviousState: previous,
		State:         string(state),
		Status:        status,
		WaitReason:    runWaitReason(string(state)),
		Resource:      resource,
		LaneID:        laneID,
	})
	c.owner.broadcast(DesktopEvent{
		Time:     time.Now().UTC(),
		Category: RunEventCategory,
		Type:     EventTypeRunState,
		Level:    runEventLevel(state),
		RunID:    string(runID),
		Summary:  string(state),
		Payload:  payload,
	})
}

func (c *runCoordinator) runOwnershipLocked(runID domain.RunID) (domain.ResourceKey, domain.BrowserLaneID) {
	var resource domain.ResourceKey
	if locks, err := c.backend.DesktopResourceLocks(); err == nil {
		for _, lock := range locks {
			if lock.OwnerRunID == runID {
				resource = lock.Resource
				break
			}
			for _, waiter := range lock.Waiters {
				if waiter.RunID == runID {
					resource = lock.Resource
					break
				}
			}
		}
	}
	var laneID domain.BrowserLaneID
	for _, lane := range c.lanes.List() {
		if lane.RunID == runID {
			laneID = lane.LaneID
			break
		}
	}
	return resource, laneID
}

func (c *runCoordinator) hasActiveRun() bool {
	return c != nil && c.activeFlag.Load()
}

func (c *runCoordinator) hasManagedWork() bool {
	if c == nil {
		return false
	}
	if c.activeFlag.Load() {
		return true
	}
	tickets, err := c.backend.DesktopScheduledRuns()
	return err == nil && len(tickets) > 0
}

func runRecordState(record *domain.RunRegistryRecord) string {
	if record == nil {
		return ""
	}
	return record.State
}

func runCommandStateError(kind CommandKind, state string) error {
	if strings.TrimSpace(state) == "" {
		state = "missing"
	}
	return fmt.Errorf("%w: command %s is not allowed from %s", ErrCommandNotAllowed, kind, state)
}

func runStateAllowsRetry(state string) bool {
	switch RunLifecycleState(state) {
	case RunStateFailed, RunStateCancelled, RunStateStopped:
		return true
	default:
		return false
	}
}

func runStateIsActiveOrWaiting(state string) bool {
	switch RunLifecycleState(state) {
	case RunStateQueued, RunStateBlockedResource, RunStateBlockedLane, RunStateStarting, RunStateRunning,
		RunStatePausing, RunStateStopping, RunStateCancelling, RunStateRecovering:
		return true
	default:
		return false
	}
}

func runStateIsTerminal(state string) bool {
	switch RunLifecycleState(state) {
	case RunStateStopped, RunStateCancelled, RunStateFailed, RunStateCompleted:
		return true
	default:
		return false
	}
}

func scheduledRunExists(tickets []domain.RunScheduleTicket, runID domain.RunID) bool {
	for _, ticket := range tickets {
		if ticket.RunID == runID {
			return true
		}
	}
	return false
}

func runWaitReason(state string) string {
	switch RunLifecycleState(state) {
	case RunStateQueued:
		return "scheduler"
	case RunStateBlockedResource:
		return "resource"
	case RunStateBlockedLane:
		return "browser_lane"
	default:
		return ""
	}
}

func runEventLevel(state RunLifecycleState) string {
	if state == RunStateFailed {
		return "error"
	}
	return "info"
}

func desktopLifecycleForRun(state RunLifecycleState) DesktopLifecycleState {
	switch state {
	case RunStatePaused:
		return LifecyclePaused
	case RunStateStopped:
		return LifecycleStopped
	case RunStateCancelled:
		return LifecycleCancelled
	case RunStateFailed:
		return LifecycleFailed
	case RunStateCompleted:
		return LifecycleCompleted
	default:
		return LifecycleReady
	}
}
