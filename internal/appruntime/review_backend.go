package appruntime

import (
	"context"
	"fmt"
	"sync"

	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/host"
)

// reviewAwareRunBackend is an adapter, not a second scheduler. Every queue,
// resource, browser-lane and restart operation is forwarded to the existing G05
// Host/Store authority. Only Resume/engine-state are specialized after G05 has
// selected a run whose durable record contains ReviewWork.
type reviewAwareRunBackend struct {
	base *host.Host

	mu      sync.Mutex
	runID   domain.RunID
	ctx     context.Context
	cancel  context.CancelFunc
	started bool
	running bool
	err     error
	wg      sync.WaitGroup
}

func newReviewAwareRunBackend(base *host.Host) *reviewAwareRunBackend {
	return &reviewAwareRunBackend{base: base}
}

func (b *reviewAwareRunBackend) DesktopRunsList() ([]domain.RunRegistryRecord, error) {
	return b.base.DesktopRunsList()
}
func (b *reviewAwareRunBackend) DesktopRunLoad(runID domain.RunID) (*domain.RunRegistryRecord, error) {
	return b.base.DesktopRunLoad(runID)
}
func (b *reviewAwareRunBackend) DesktopRunSave(record domain.RunRegistryRecord) (domain.RunRegistryRecord, error) {
	return b.base.DesktopRunSave(record)
}
func (b *reviewAwareRunBackend) DesktopRunAppendHistory(entry domain.RunHistoryRecord) (domain.RunHistoryRecord, error) {
	return b.base.DesktopRunAppendHistory(entry)
}
func (b *reviewAwareRunBackend) DesktopRunHistory(runID domain.RunID) ([]domain.RunHistoryRecord, error) {
	return b.base.DesktopRunHistory(runID)
}
func (b *reviewAwareRunBackend) DesktopScheduledRuns() ([]domain.RunScheduleTicket, error) {
	return b.base.DesktopScheduledRuns()
}
func (b *reviewAwareRunBackend) DesktopEnqueueRun(runID domain.RunID, priority domain.RunSchedulerPriority) (domain.RunScheduleTicket, error) {
	return b.base.DesktopEnqueueRun(runID, priority)
}
func (b *reviewAwareRunBackend) DesktopDequeueScheduledRun(runID domain.RunID) (*domain.RunScheduleTicket, error) {
	return b.base.DesktopDequeueScheduledRun(runID)
}
func (b *reviewAwareRunBackend) DesktopRemoveScheduledRun(runID domain.RunID) error {
	return b.base.DesktopRemoveScheduledRun(runID)
}
func (b *reviewAwareRunBackend) DesktopStoryResourceKey() (domain.ResourceKey, error) {
	return b.base.DesktopStoryResourceKey()
}
func (b *reviewAwareRunBackend) DesktopResourceLocks() ([]domain.ResourceLockState, error) {
	return b.base.DesktopResourceLocks()
}
func (b *reviewAwareRunBackend) DesktopRequestRunResource(runID domain.RunID, resource domain.ResourceKey) (domain.ResourceLockDecision, error) {
	return b.base.DesktopRequestRunResource(runID, resource)
}
func (b *reviewAwareRunBackend) DesktopReleaseRunResourceClaims(runID domain.RunID) error {
	return b.base.DesktopReleaseRunResourceClaims(runID)
}
func (b *reviewAwareRunBackend) DesktopEnsureWebSession(ctx context.Context) error {
	return b.base.DesktopEnsureWebSession(ctx)
}
func (b *reviewAwareRunBackend) DesktopRecoverResourceLocks() ([]domain.ResourceLockState, error) {
	return b.base.DesktopRecoverResourceLocks()
}

func (b *reviewAwareRunBackend) Resume() (string, error) {
	resource, err := b.base.DesktopStoryResourceKey()
	if err != nil {
		return "", err
	}
	locks, err := b.base.DesktopResourceLocks()
	if err != nil {
		return "", err
	}
	var owner domain.RunID
	for _, lock := range locks {
		if lock.Resource == resource {
			owner = lock.OwnerRunID
			break
		}
	}
	if owner == "" {
		b.clearReviewExecution()
		return b.base.Resume()
	}
	record, err := b.base.DesktopRunLoad(owner)
	if err != nil {
		return "", err
	}
	if record == nil || record.ReviewWork == nil {
		b.clearReviewExecution()
		return b.base.Resume()
	}
	if err := record.ReviewWork.Validate(); err != nil {
		return "", fmt.Errorf("invalid durable review work for run %q: %w", owner, err)
	}

	b.mu.Lock()
	if b.cancel != nil {
		b.cancel()
	}
	b.runID = owner
	b.ctx, b.cancel = context.WithCancel(context.Background())
	b.started = false
	b.running = true
	b.err = nil
	b.mu.Unlock()
	return "review:" + record.ReviewWork.Action, nil
}

func (b *reviewAwareRunBackend) DesktopEngineRunning() bool {
	b.mu.Lock()
	if b.runID == "" {
		b.mu.Unlock()
		return b.base.DesktopEngineRunning()
	}
	if !b.running {
		b.mu.Unlock()
		return false
	}
	if b.started {
		b.mu.Unlock()
		return true
	}
	b.started = true
	runID := b.runID
	ctx := b.ctx
	b.wg.Add(1)
	b.mu.Unlock()

	go func() {
		defer b.wg.Done()
		err := b.executeReview(ctx, runID)
		b.mu.Lock()
		b.err = err
		b.running = false
		b.mu.Unlock()
	}()
	return true
}

func (b *reviewAwareRunBackend) executeReview(ctx context.Context, runID domain.RunID) error {
	record, err := b.base.DesktopRunLoad(runID)
	if err != nil {
		return err
	}
	if record == nil || record.ReviewWork == nil {
		return fmt.Errorf("review run %q lost durable work before execution", runID)
	}
	work := *record.ReviewWork
	if !work.Prepared && work.ExpectedFingerprint != "" {
		target := reviewTargetDTOFromWork(work.Target)
		snapshot, err := b.base.DesktopReviewRead(hostReviewTarget(target))
		if err != nil {
			return err
		}
		status, err := aggregateReviewStatus(target, snapshot)
		if err != nil {
			return err
		}
		if status.Freshness.Fingerprint != work.ExpectedFingerprint {
			return fmt.Errorf("%w: review target fingerprint changed while waiting for execution", ErrMutationPrecondition)
		}
	}
	return b.base.DesktopExecuteReviewWork(ctx, runID)
}

func (b *reviewAwareRunBackend) DesktopRuntimeState() string {
	b.mu.Lock()
	if b.runID == "" {
		b.mu.Unlock()
		return b.base.DesktopRuntimeState()
	}
	running := b.running
	err := b.err
	b.mu.Unlock()
	if running {
		return "running"
	}
	if err != nil {
		return "failed"
	}
	return "completed"
}

func (b *reviewAwareRunBackend) Abort() bool {
	b.mu.Lock()
	if b.runID != "" && b.running {
		if b.cancel != nil {
			b.cancel()
		}
		b.mu.Unlock()
		return true
	}
	b.mu.Unlock()
	return b.base.Abort()
}

func (b *reviewAwareRunBackend) close() {
	b.mu.Lock()
	if b.cancel != nil {
		b.cancel()
	}
	b.mu.Unlock()
	b.wg.Wait()
}

func (b *reviewAwareRunBackend) clearReviewExecution() {
	b.mu.Lock()
	if b.cancel != nil {
		b.cancel()
	}
	b.runID = ""
	b.ctx = nil
	b.cancel = nil
	b.started = false
	b.running = false
	b.err = nil
	b.mu.Unlock()
}

func reviewTargetDTOFromWork(target domain.ReviewWorkTarget) ReviewTargetDTO {
	return ReviewTargetDTO{
		Scope:          ReviewScope(target.Scope),
		Chapter:        target.Chapter,
		Volume:         target.Volume,
		Arc:            target.Arc,
		ThroughChapter: target.ThroughChapter,
	}
}
