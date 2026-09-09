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

	eventHubOnce     sync.Once
	eventMu          sync.Mutex
	eventHubCancel   context.CancelFunc
	eventHubDone     chan struct{}
	subscribers      map[uint64]*desktopSubscription
	nextSubscriberID atomic.Uint64
}

var _ AppRuntime = (*Runtime)(nil)

// New wraps exactly one existing Host. It does not create a second Engine,
// Store, WebAI session, or project database.
func New(core *host.Host) (*Runtime, error) {
	if core == nil {
		return nil, ErrNilHost
	}
	return &Runtime{
		core:        core,
		subscribers: make(map[uint64]*desktopSubscription),
	}, nil
}

func (r *Runtime) Snapshot(ctx context.Context) (DesktopSnapshot, error) {
	if err := r.ready(ctx); err != nil {
		return DesktopSnapshot{}, err
	}
	revision := r.snapshotRevision.Add(1)
	return projectDesktopSnapshot(
		r.core.Snapshot(),
		r.core.WebSessionSnapshot(),
		r.core.Dir(),
		revision,
		time.Now().UTC(),
	), nil
}

func (r *Runtime) Query(ctx context.Context, req QueryRequest) (QueryResult, error) {
	if err := r.ready(ctx); err != nil {
		return QueryResult{}, err
	}
	return QueryResult{Kind: req.Kind}, ErrNotImplemented
}

func (r *Runtime) Dispatch(ctx context.Context, cmd CommandRequest) (CommandResult, error) {
	if err := r.ready(ctx); err != nil {
		return CommandResult{}, err
	}
	return CommandResult{CommandID: cmd.ID}, ErrNotImplemented
}

func (r *Runtime) Subscribe(ctx context.Context, cursor EventCursor) (EventSubscription, error) {
	if err := r.ready(ctx); err != nil {
		return nil, err
	}
	if cursor.AfterSeq < 0 {
		cursor.AfterSeq = 0
	}

	r.startEventHub()
	replay, err := r.core.DesktopRuntimeQueueAfter(cursor.AfterSeq)
	if err != nil {
		return nil, err
	}

	var subID uint64
	sub := newDesktopSubscription(len(replay)+desktopEventBuffer, func() {
		r.removeSubscription(subID)
	})
	for _, item := range replay {
		sub.offer(projectRuntimeQueueItem(item))
	}
	subID = r.registerSubscription(sub)

	// Cover durable events appended between the initial replay load and
	// registration. Duplicates are harmless because durable Seq is stable.
	lastSeq := cursor.AfterSeq
	if len(replay) > 0 {
		lastSeq = replay[len(replay)-1].Seq
	}
	if catchup, catchupErr := r.core.DesktopRuntimeQueueAfter(lastSeq); catchupErr == nil {
		for _, item := range catchup {
			sub.offer(projectRuntimeQueueItem(item))
		}
	}

	go func() {
		<-ctx.Done()
		_ = sub.Close()
	}()
	return sub, nil
}

// Close owns disposal of the wrapped Host for the desktop facade. Host.Close
// is already idempotent; closeOnce also prevents future facade behavior from
// accidentally running desktop-side cleanup more than once.
func (r *Runtime) Close(ctx context.Context) error {
	if r == nil || r.core == nil {
		return ErrRuntimeUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	r.closeOnce.Do(func() {
		r.closed.Store(true)
		r.stopEventHub()
		r.core.Close()
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
