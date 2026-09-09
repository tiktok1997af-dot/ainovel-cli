package appruntime

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/voocel/ainovel-cli/internal/host"
)

// Runtime is the desktop-facing facade over the existing Host. The core Host
// remains private so desktop callers cannot bypass the AppRuntime boundary.
type Runtime struct {
	core             *host.Host
	closeOnce        sync.Once
	closed           atomic.Bool
	snapshotRevision atomic.Uint64

	commandMu      sync.Mutex
	lifecycleMu    sync.Mutex
	lifecycleState DesktopLifecycleState
	nextCommandID  atomic.Uint64
	commandWG      sync.WaitGroup

	eventHubOnce     sync.Once
	eventMu          sync.Mutex
	eventHubCancel   context.CancelFunc
	eventHubDone     chan struct{}
	subscribers      map[uint64]*desktopSubscription
	nextSubscriberID atomic.Uint64
}

var _ AppRuntime = (*Runtime)(nil)

func New(core *host.Host) (*Runtime, error) {
	if core == nil {
		return nil, normalizeAppError(ErrNilHost)
	}
	return &Runtime{
		core:           core,
		lifecycleState: lifecycleFromCore(core.Snapshot().RuntimeState),
		subscribers:    make(map[uint64]*desktopSubscription),
	}, nil
}

func (r *Runtime) Snapshot(ctx context.Context) (DesktopSnapshot, error) {
	if err := r.ready(ctx); err != nil {
		return DesktopSnapshot{}, normalizeAppError(err)
	}
	coreSnapshot := r.core.Snapshot()
	r.reconcileLifecycle(coreSnapshot)
	revision := r.snapshotRevision.Add(1)
	snapshot := projectDesktopSnapshot(coreSnapshot, r.core.WebSessionSnapshot(), r.core.Dir(), revision, time.Now().UTC())
	r.applyLifecycleProjection(&snapshot)
	return snapshot, nil
}

func (r *Runtime) Query(ctx context.Context, req QueryRequest) (QueryResult, error) {
	result := QueryResult{ContractVersion: ContractVersion, Kind: req.Kind}
	if err := r.ready(ctx); err != nil {
		appErr := normalizeAppError(err)
		result.Error = appErr
		return result, appErr
	}
	if err := validateContractVersion(req.ContractVersion); err != nil {
		appErr := normalizeAppError(err)
		result.Error = appErr
		return result, appErr
	}
	data, err := r.routeQuery(ctx, req)
	if err != nil {
		appErr := normalizeAppError(err)
		result.Error = appErr
		return result, appErr
	}
	result.Data = data
	return result, nil
}

func (r *Runtime) Dispatch(ctx context.Context, cmd CommandRequest) (CommandResult, error) {
	result := CommandResult{ContractVersion: ContractVersion, CommandID: cmd.ID, RunID: cmd.RunID, TaskID: cmd.TaskID}
	if err := r.ready(ctx); err != nil {
		appErr := normalizeAppError(err)
		result.Error = appErr
		return result, appErr
	}
	if err := validateContractVersion(cmd.ContractVersion); err != nil {
		appErr := normalizeAppError(err)
		result.Error = appErr
		return result, appErr
	}
	out, err := r.dispatchLifecycle(ctx, cmd)
	out.ContractVersion = ContractVersion
	if err != nil {
		appErr := normalizeAppError(err)
		if cmd.Kind == CommandResume || cmd.Kind == CommandRetry {
			appErr = recoveryError(err)
		}
		out.Error = appErr
		return out, appErr
	}
	return out, nil
}

func (r *Runtime) Subscribe(ctx context.Context, cursor EventCursor) (EventSubscription, error) {
	if err := r.ready(ctx); err != nil {
		return nil, normalizeAppError(err)
	}
	if err := validateContractVersion(cursor.ContractVersion); err != nil {
		return nil, normalizeAppError(err)
	}
	if cursor.AfterSeq < 0 {
		cursor.AfterSeq = 0
	}

	r.startEventHub()
	replay, err := r.core.DesktopRuntimeQueueAfter(cursor.AfterSeq)
	if err != nil {
		return nil, normalizeAppError(err)
	}

	var subID uint64
	sub := newDesktopSubscription(len(replay)+desktopEventBuffer, func() {
		r.removeSubscription(subID)
	})
	for _, item := range replay {
		sub.offer(projectRuntimeQueueItem(item))
	}
	subID = r.registerSubscription(sub)

	lastSeq := cursor.AfterSeq
	if len(replay) > 0 {
		lastSeq = replay[len(replay)-1].Seq
	}
	if catchup, catchupErr := r.core.DesktopRuntimeQueueAfter(lastSeq); catchupErr == nil {
		for _, item := range catchup {
			sub.offer(projectRuntimeQueueItem(item))
		}
	}

	if done := ctx.Done(); done != nil {
		go func() {
			<-done
			_ = sub.Close()
		}()
	}
	return sub, nil
}

func (r *Runtime) Close(ctx context.Context) error {
	if r == nil || r.core == nil {
		return normalizeAppError(ErrRuntimeUnavailable)
	}
	if err := ctx.Err(); err != nil {
		return normalizeAppError(err)
	}
	r.closeOnce.Do(func() {
		r.closed.Store(true)
		r.stopEventHub()
		r.core.Close()
		r.commandWG.Wait()
	})
	return nil
}

func (r *Runtime) ready(ctx context.Context) error {
	if r == nil || r.core == nil {
		return ErrRuntimeUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if r.closed.Load() {
		return ErrClosed
	}
	return nil
}
