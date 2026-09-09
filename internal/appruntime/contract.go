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
// desktop shell. It deliberately contains only desktop DTOs, never host/webai
// implementation structs or pointers.
type DesktopSnapshot struct {
	Revision       uint64                     `json:"revision"`
	GeneratedAt    time.Time                  `json:"generated_at"`
	Product        ProductViewSnapshot        `json:"product"`
	Project        ProjectViewSnapshot        `json:"project"`
	Runtime        RuntimeViewSnapshot        `json:"runtime"`
	CurrentChapter ChapterViewSnapshot        `json:"current_chapter"`
	Agents         []AgentViewSnapshot        `json:"agents"`
	Browser        BrowserViewSnapshot        `json:"browser"`
	Recovery       RecoveryViewSnapshot       `json:"recovery"`
	QualitySummary QualitySummaryViewSnapshot `json:"quality_summary"`
}

type ProductViewSnapshot struct {
	Name          string `json:"name"`
	CoreBaseline  string `json:"core_baseline"`
	ExecutionMode string `json:"execution_mode"`
}

type ProjectViewSnapshot struct {
	Title            string                `json:"title,omitempty"`
	OutputDir        string                `json:"output_dir,omitempty"`
	Synopsis         string                `json:"synopsis,omitempty"`
	Premise          string                `json:"premise,omitempty"`
	Style            string                `json:"style,omitempty"`
	Layered          bool                  `json:"layered"`
	Outline          []OutlineItemSnapshot `json:"outline,omitempty"`
	Characters       []string              `json:"characters,omitempty"`
	SupportingCount  int                   `json:"supporting_count"`
	RecentSupporting []string              `json:"recent_supporting,omitempty"`
	CurrentVolumeArc string                `json:"current_volume_arc,omitempty"`
	NextVolumeTitle  string                `json:"next_volume_title,omitempty"`
	CompassDirection string                `json:"compass_direction,omitempty"`
	CompassScale     string                `json:"compass_scale,omitempty"`
}

type OutlineItemSnapshot struct {
	Chapter   int    `json:"chapter"`
	Title     string `json:"title,omitempty"`
	CoreEvent string `json:"core_event,omitempty"`
}

type RuntimeViewSnapshot struct {
	State                string `json:"state"`
	Status               string `json:"status,omitempty"`
	Phase                string `json:"phase,omitempty"`
	Flow                 string `json:"flow,omitempty"`
	IsRunning            bool   `json:"is_running"`
	Provider             string `json:"provider,omitempty"`
	Model                string `json:"model,omitempty"`
	ModelContextWindow   int    `json:"model_context_window,omitempty"`
	ThinkingLevel        string `json:"thinking_level,omitempty"`
	PendingSteer         string `json:"pending_steer,omitempty"`
	AdvanceMode          string `json:"advance_mode,omitempty"`
	AdvancePermitChapter int    `json:"advance_permit_chapter,omitempty"`
	HasAdvanceHold       bool   `json:"has_advance_hold"`
	AdvanceHoldReason    string `json:"advance_hold_reason,omitempty"`
	AITelemetryStatus    string `json:"ai_telemetry_status,omitempty"`
}

type ChapterViewSnapshot struct {
	Current        int `json:"current"`
	InProgress     int `json:"in_progress"`
	Total          int `json:"total"`
	Completed      int `json:"completed"`
	TotalWordCount int `json:"total_word_count"`
}

type AgentViewSnapshot struct {
	Name      string                   `json:"name"`
	State     string                   `json:"state"`
	TaskID    string                   `json:"task_id,omitempty"`
	TaskKind  string                   `json:"task_kind,omitempty"`
	Summary   string                   `json:"summary,omitempty"`
	Tool      string                   `json:"tool,omitempty"`
	Turn      int                      `json:"turn"`
	Context   AgentContextViewSnapshot `json:"context"`
	UpdatedAt time.Time                `json:"updated_at"`
}

type AgentContextViewSnapshot struct {
	Tokens          int     `json:"tokens"`
	ContextWindow   int     `json:"context_window"`
	Percent         float64 `json:"percent"`
	Scope           string  `json:"scope,omitempty"`
	Strategy        string  `json:"strategy,omitempty"`
	ActiveMessages  int     `json:"active_messages"`
	SummaryMessages int     `json:"summary_messages"`
	CompactedCount  int     `json:"compacted_count"`
	KeptCount       int     `json:"kept_count"`
}

type BrowserViewSnapshot struct {
	State       string    `json:"state"`
	Site        string    `json:"site,omitempty"`
	BrowserPath string    `json:"browser_path,omitempty"`
	ProfileDir  string    `json:"profile_dir,omitempty"`
	PID         int       `json:"pid,omitempty"`
	StartedAt   time.Time `json:"started_at,omitempty"`
	ChangedAt   time.Time `json:"changed_at,omitempty"`
	Reason      string    `json:"reason,omitempty"`
}

type RecoveryViewSnapshot struct {
	Label          string `json:"label,omitempty"`
	CanResume      bool   `json:"can_resume"`
	LastCheckpoint string `json:"last_checkpoint,omitempty"`
}

type QualitySummaryViewSnapshot struct {
	LastCommitSummary string   `json:"last_commit_summary,omitempty"`
	LastReviewSummary string   `json:"last_review_summary,omitempty"`
	PendingRewrites   []int    `json:"pending_rewrites,omitempty"`
	RewriteReason     string   `json:"rewrite_reason,omitempty"`
	RecentSummaries   []string `json:"recent_summaries,omitempty"`
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
