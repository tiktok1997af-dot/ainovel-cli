package desktopui

import (
	"context"

	"github.com/voocel/ainovel-cli/internal/appruntime"
)

// RuntimeClient is the only core-facing dependency allowed inside the Creative
// Studio UI layer. It intentionally mirrors AppRuntime's five-method facade so
// renderers can be added later without exposing Host, Store, WebAI, filesystem,
// or engine internals to UI code.
type RuntimeClient interface {
	Snapshot(ctx context.Context) (appruntime.DesktopSnapshot, error)
	Query(ctx context.Context, req appruntime.QueryRequest) (appruntime.QueryResult, error)
	Dispatch(ctx context.Context, cmd appruntime.CommandRequest) (appruntime.CommandResult, error)
	Subscribe(ctx context.Context, cursor appruntime.EventCursor) (appruntime.EventSubscription, error)
	Close(ctx context.Context) error
}

var _ RuntimeClient = (appruntime.AppRuntime)(nil)
