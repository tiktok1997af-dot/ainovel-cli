package appruntime

import (
	"context"
	"encoding/json"
	"time"
)

// AppRuntime is the only supported facade between the desktop product and the
// existing Go core. Concrete behavior is implemented incrementally across G02.
type AppRuntime interface {
	Snapshot(ctx context.Context) (DesktopSnapshot, error)
	Query(ctx context.Context, req QueryRequest) (QueryResult, error)
	Dispatch(ctx context.Context, cmd CommandRequest) (CommandResult, error)
	Subscribe(ctx context.Context, cursor EventCursor) (EventSubscription, error)
	Close(ctx context.Context) error
}

// DesktopSnapshot is the transport-neutral root projection consumed by the
// desktop shell. G02.3 extends it with typed product/project/runtime sections.
type DesktopSnapshot struct {
	Revision    uint64    `json:"revision"`
	GeneratedAt time.Time `json:"generated_at"`
}

// QueryKind identifies a read-only AppRuntime query.
type QueryKind string

// QueryRequest is a bounded read request. Payload stays JSON-typed at the
// bridge boundary and is replaced by typed per-query payloads as queries land.
type QueryRequest struct {
	Kind    QueryKind       `json:"kind"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// QueryResult is the serialization-safe result envelope for the query plane.
type QueryResult struct {
	Kind QueryKind       `json:"kind"`
	Data json.RawMessage `json:"data,omitempty"`
}

// CommandKind identifies a state-changing desktop command.
type CommandKind string

// CommandRequest is the only mutation envelope accepted from desktop code.
// Run/task/resource identities are additive hooks for the later orchestrator.
type CommandRequest struct {
	ID       string          `json:"id"`
	Kind     CommandKind     `json:"kind"`
	RunID    string          `json:"run_id,omitempty"`
	TaskID   string          `json:"task_id,omitempty"`
	Resource string          `json:"resource,omitempty"`
	Payload  json.RawMessage `json:"payload,omitempty"`
}

// CommandResult acknowledges command acceptance. Runtime progress/completion
// is reported through DesktopEvent rather than by holding the dispatch call.
type CommandResult struct {
	CommandID string `json:"command_id"`
	Accepted  bool   `json:"accepted"`
	RunID     string `json:"run_id,omitempty"`
	TaskID    string `json:"task_id,omitempty"`
	Status    string `json:"status,omitempty"`
}

// EventCursor requests replay/subscription after a known durable sequence.
type EventCursor struct {
	AfterSeq int64 `json:"after_seq,omitempty"`
}

// DesktopEvent is the typed outer envelope for realtime desktop updates.
type DesktopEvent struct {
	Seq      int64           `json:"seq,omitempty"`
	Time     time.Time       `json:"time"`
	Category string          `json:"category"`
	Type     string          `json:"type"`
	Level    string          `json:"level,omitempty"`
	RunID    string          `json:"run_id,omitempty"`
	TaskID   string          `json:"task_id,omitempty"`
	Summary  string          `json:"summary,omitempty"`
	Payload  json.RawMessage `json:"payload,omitempty"`
}

// EventSubscription is intentionally transport-neutral. G02.4 supplies the
// non-blocking projection/replay implementation over Host + RuntimeStore.
type EventSubscription interface {
	Events() <-chan DesktopEvent
	Close() error
}
