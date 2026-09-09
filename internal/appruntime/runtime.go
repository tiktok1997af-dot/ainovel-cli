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
}

var _ AppRuntime = (*Runtime)(nil)

// New wraps exactly one existing Host. It does not create a second Engine,
// Store, WebAI session, or project database.
func New(core *host.Host) (*Runtime, error) {
	if core == nil {
		return nil, ErrNilHost
	}
	return &Runtime{core: core}, nil
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
	return nil, ErrNotImplemented
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
