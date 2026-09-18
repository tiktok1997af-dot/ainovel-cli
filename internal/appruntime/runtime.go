package appruntime

import (
	"context"
	"encoding/json"
	"fmt"
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
	closeErr         error
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

	run           *runCoordinator
	reviewBackend *reviewAwareRunBackend
	coCreate      *coCreateRuntimeAdapter
	dualWeb       *dualWebProviderRuntime
	chatGPTLane   chatGPTLaneRuntime
	parallel      *dualWebParallelAuthority
}

var _ AppRuntime = (*Runtime)(nil)

func New(core *host.Host) (*Runtime, error) {
	if core == nil {
		return nil, normalizeAppError(ErrNilHost)
	}

	settings, err := core.DesktopSettingsRead()
	if err != nil {
		return nil, normalizeAppError(fmt.Errorf("load D07 runtime pipeline settings: %w", err))
	}
	policy, err := NewD07PipelinePolicy(settings.Runtime.PipelineMode, settings.Runtime.CheckpointChapters)
	if err != nil {
		return nil, normalizeAppError(fmt.Errorf("resolve D07 runtime pipeline policy: %w", err))
	}
	if err := core.DesktopBindReviewCheckpoint(policy.Config.CheckpointChapters); err != nil {
		return nil, normalizeAppError(fmt.Errorf("bind D07 review checkpoint: %w", err))
	}
	keepCheckpointBinding := false
	defer func() {
		if !keepCheckpointBinding {
			core.DesktopUnbindReviewCheckpoint()
		}
	}()

	lanes, err := core.DesktopNewBrowserLanePool()
	if err != nil {
		return nil, normalizeAppError(fmt.Errorf("initialize browser lane pool: %w", err))
	}
	reviewBackend := newReviewAwareRunBackend(core)
	admissionBackend := newD07AdmissionRunBackend(reviewBackend)
	rt := &Runtime{
		core:           core,
		lifecycleState: lifecycleFromCore(core.Snapshot().RuntimeState),
		subscribers:    make(map[uint64]*desktopSubscription),
		reviewBackend:  reviewBackend,
		coCreate:       newCoCreateRuntimeAdapter(core),
		dualWeb:        newDualWebProviderRuntime(),
		chatGPTLane:    newAppRuntimeChatGPTLane(core),
	}
	if err := rt.syncChatGPTProviderSnapshot(); err != nil {
		return nil, normalizeAppError(fmt.Errorf("initialize ChatGPT Web lane projection: %w", err))
	}
	// Run Center remains the sole scheduler. Its backend is wrapped only at the
	// Resume/release seam so every production AI execution must pass through the
	// D04 dual-provider admission authority after Run Center already owns the
	// canonical story resource.
	rt.run = newRunCoordinator(admissionBackend, lanes)
	rt.parallel = newDualWebParallelAuthority(newResilientDualWebLaneAuthority(rt), reviewBackend, policy.Config.Mode)
	admissionBackend.bindParallel(rt.parallel)
	if err := rt.run.recoverRestart(context.Background()); err != nil {
		return nil, normalizeAppError(fmt.Errorf("recover multi-run runtime: %w", err))
	}
	rt.run.start(rt)
	keepCheckpointBinding = true
	return rt, nil
}


// SuspendProjectHandoff temporarily releases project-external WEB browser
// ownership while keeping the project runtime itself alive for rollback.
func (r *Runtime) SuspendProjectHandoff(ctx context.Context) error {
	if err := r.ready(ctx); err != nil {
		return normalizeAppError(err)
	}
	if err := r.core.DesktopSuspendWebSessionForProjectHandoff(ctx); err != nil {
		return normalizeAppError(fmt.Errorf("suspend project WEB session for handoff: %w", err))
	}
	return nil
}

// ResumeProjectHandoff restores the same project's WEB browser ownership after
// a replacement runtime failed to initialize.
func (r *Runtime) ResumeProjectHandoff(ctx context.Context) error {
	if err := r.ready(ctx); err != nil {
		return normalizeAppError(err)
	}
	if err := r.core.DesktopResumeWebSessionForProjectHandoff(ctx); err != nil {
		return normalizeAppError(fmt.Errorf("resume project WEB session after handoff failure: %w", err))
	}
	return nil
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
	var data json.RawMessage
	var err error
	if req.Kind == QuerySettingsGet {
		data, err = r.querySettingsGet(req)
	} else if isDualWebQueryKind(req.Kind) {
		if r.dualWeb == nil {
			err = ErrRuntimeUnavailable
		} else if syncErr := r.syncChatGPTProviderSnapshot(); syncErr != nil {
			err = syncErr
		} else {
			data, err = r.dualWeb.query(req)
		}
	} else if isRunCenterQueryKind(req.Kind) {
		data, err = r.routeRunCenterQuery(ctx, req)
	} else {
		data, err = r.routeQuery(ctx, req)
	}
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

	var out CommandResult
	var err error
	switch {
	case cmd.Kind == CommandSettingsUpdate:
		out, err = r.dispatchSettingsUpdate(ctx, cmd)
	case isDualWebCommandKind(cmd.Kind):
		if r.dualWeb == nil {
			out = result
			err = ErrRuntimeUnavailable
		} else if syncErr := r.syncChatGPTProviderSnapshot(); syncErr != nil {
			out = result
			err = syncErr
		} else {
			out, err = r.dualWeb.dispatch(cmd)
		}
	case isCoCreateCommandKind(cmd.Kind):
		out, err = r.dispatchCoCreate(ctx, cmd)
	case isRunCenterCommandKind(cmd.Kind):
		out, err = r.dispatchRunControl(ctx, cmd)
	case cmd.Kind == CommandReviewPromoteOfficial:
		if r.run != nil && r.run.hasManagedWork() {
			out = result
			err = ErrMutationPrecondition
		} else {
			out, err = r.dispatchReviewPromotion(ctx, cmd)
		}
	case isReviewCommandKind(cmd.Kind):
		out, err = r.dispatchReviewCommand(ctx, cmd)
	case isLifecycleCommand(cmd.Kind):
		if cmd.Kind == CommandStart {
			if gateErr := r.enforceDesktopStartModelGate(ctx, cmd); gateErr != nil {
				out = result
				err = gateErr
				break
			}
		}
		if r.run != nil && r.run.hasActiveRun() {
			out = result
			err = fmt.Errorf("%w: active Run Center execution owns lifecycle control", ErrCommandNotAllowed)
		} else {
			out, err = r.dispatchLifecycle(ctx, cmd)
		}
	default:
		if r.run != nil && r.run.hasManagedWork() {
			out = result
			err = ErrMutationPrecondition
		} else {
			out, err = r.dispatchMutation(ctx, cmd)
		}
	}
	out.ContractVersion = ContractVersion
	if err != nil {
		appErr := normalizeAppError(err)
		if cmd.Kind == CommandResume || cmd.Kind == CommandRetry || cmd.Kind == CommandRunResume || cmd.Kind == CommandRunRetry {
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
		if r.reviewBackend != nil {
			r.reviewBackend.close()
		}
		if r.parallel != nil {
			if err := r.parallel.shutdown(ctx); err != nil {
				r.closeErr = fmt.Errorf("shutdown dual-web parallel authority: %w", err)
			}
		}
		if r.run != nil {
			r.run.close()
		}
		if err := r.stopChatGPTLane(); err != nil {
			if r.closeErr != nil {
				r.closeErr = fmt.Errorf("%v; stop ChatGPT Web lane: %w", r.closeErr, err)
			} else {
				r.closeErr = fmt.Errorf("stop ChatGPT Web lane: %w", err)
			}
		}
		r.core.DesktopUnbindReviewCheckpoint()
		r.core.Close()
		r.commandWG.Wait()
	})
	if r.closeErr != nil {
		return normalizeAppError(r.closeErr)
	}
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
