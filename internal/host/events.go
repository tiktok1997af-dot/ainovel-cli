package host

import (
	"time"
)

// Event 是 TUI 消费的结构化事件。
//
// 对于 TOOL / DISPATCH / DECISION 三类调用事件，同一次调用的开始与结束共用一个 ID：
// 开始时先发 FinishedAt 为零值的事件（TUI 渲染为"进行中"样式）；
// 结束时再发一条同 ID 的事件，填入 FinishedAt + Duration（+ Failed），
// TUI 按 ID 定位原行原地更新，避免"开始一行、完成又一行"的冗余。
//
// SYSTEM / ERROR / CONTEXT 等非调用类事件 ID 为空，每条独立追加。
type Event struct {
	ID         string    // 同一次调用的开始/结束共用；非调用事件为空
	Time       time.Time // 首次发出时间（开始时刻）
	FinishedAt time.Time // 零值 = 进行中；非零 = 已完成
	Failed     bool      // 已完成但失败（仅完成态有意义）
	Category   string    // DISPATCH / TOOL / DECISION / SYSTEM / REVIEW / CHECK / ERROR / CONTEXT
	Agent      string    // 产生事件的 agent
	Summary    string
	Detail     string        // 完整文案，写入日志不截断供排查；为空回退 Summary。UI 只读 Summary
	Kind       string        // 错误分类（如 stream_idle），随日志输出供过滤/告警；为空不输出
	Level      string        // info / warn / error / success
	Depth      int           // 0 = Engine 层, 1 = Worker 层
	Duration   time.Duration // 完成时的执行耗时
	RetryAt    time.Time     // 重试类事件：下次重试的截止时刻；UI 据此逐秒倒计时，到点即清（请求已在途）
}

// Running 返回事件是否处于进行中。
// 仅调用类事件（有 ID 的 TOOL / DISPATCH / DECISION）可能进行中；其它类型总是返回 false。
func (e Event) Running() bool {
	return e.hasLifecycle() && e.FinishedAt.IsZero()
}

func (e Event) hasLifecycle() bool {
	if e.ID == "" {
		return false
	}
	switch e.Category {
	case "TOOL", "DISPATCH", "DECISION":
		return true
	default:
		return false
	}
}

// WebAITelemetryUnavailable states the WEB-only telemetry boundary explicitly.
// The browser bridge observes visible page interaction, not provider billing APIs.
const WebAITelemetryUnavailable = "Gemini Web không cung cấp số token, chi phí billing hoặc cache telemetry đáng tin cậy cho browser bridge."

// UISnapshot 是 TUI 渲染所需的聚合状态快照。
type UISnapshot struct {
	Provider             string
	BookTitle            string
	ModelName            string
	ModelContextWindow   int // 当前默认模型的上下文窗口（随 /model 切换实时解析）
	ThinkingLevel        string
	Style                string
	RuntimeState         string // idle / running / pausing / paused / completed
	StatusLabel          string
	Phase                string
	Flow                 string
	CurrentChapter       int
	TotalChapters        int
	CompletedCount       int
	TotalWordCount       int
	InProgressChapter    int
	PendingRewrites      []int
	RewriteReason        string
	PendingSteer         string
	AdvanceMode          string
	AdvancePermitChapter int
	HasAdvanceHold       bool
	AdvanceHoldReason    string
	RecoveryLabel        string
	IsRunning            bool
	Agents               []AgentSnapshot

	// Gemini Web does not expose authoritative billing/token/cache telemetry to the browser bridge.
	AITelemetryStatus string

	// 基础设定
	Synopsis         string
	Premise          string
	Outline          []OutlineSnapshot
	Characters       []string
	SupportingCount  int      // 配角名册中的次要角色总数
	RecentSupporting []string // 最近活跃的次要角色（最多 5 个，按 LastSeenChapter 倒序）
	Layered          bool
	CurrentVolumeArc string
	NextVolumeTitle  string
	CompassDirection string
	CompassScale     string

	// 详情
	LastCommitSummary  string
	LastReviewSummary  string
	LastCheckpointName string
	RecentSummaries    []string
}

// OutlineSnapshot 是大纲条目的展示摘要。
type OutlineSnapshot struct {
	Chapter   int
	Title     string
	CoreEvent string
}

// AgentSnapshot 是 Agent 状态的展示投影。
type AgentSnapshot struct {
	Name      string
	State     string
	TaskID    string
	TaskKind  string
	Summary   string
	Tool      string
	Turn      int
	Context   AgentContextSnapshot
	UpdatedAt time.Time
}

// AgentContextSnapshot 是 Agent 上下文使用情况。
type AgentContextSnapshot struct {
	Tokens          int
	ContextWindow   int
	Percent         float64
	Scope           string
	Strategy        string
	ActiveMessages  int
	SummaryMessages int
	CompactedCount  int
	KeptCount       int
}

// CoCreateMessage 是共创对话的消息。
type CoCreateMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// CoCreateReply 是共创对话的 LLM 回复。Raw 保留模型完整四段原文，
// 用于写回 history 让下一轮模型看到自己上一轮的 [DRAFT]，从而真正在
// 已有草稿上累积更新（仅 Message 不含 [DRAFT]，会导致模型每轮凭对话重新归纳）。
// Suggestions 是 AI 主动给的"接下来你可能想说"，用户卡壳时按数字键一键填入输入框。
type CoCreateReply struct {
	Message     string
	Prompt      string
	Ready       bool
	Suggestions []string
	Raw         string
}
