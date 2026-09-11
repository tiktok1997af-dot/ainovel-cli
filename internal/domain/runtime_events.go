package domain

import "time"

// RuntimeQueuePriority 表示运行时队列优先级。
type RuntimeQueuePriority string

const (
	RuntimePriorityControl    RuntimeQueuePriority = "control"
	RuntimePriorityBackground RuntimeQueuePriority = "background"
)

// RunSchedulerPriority is the bounded product scheduling priority frozen by G05.4.
// It is distinct from RuntimeQueuePriority, which remains the legacy audit-channel
// classification used by queue.jsonl readers.
type RunSchedulerPriority string

const (
	RunSchedulerPriorityHigh    RunSchedulerPriority = "high"
	RunSchedulerPriorityNormal  RunSchedulerPriority = "normal"
	RunSchedulerPriorityLow     RunSchedulerPriority = "low"
	DefaultRunSchedulerPriority RunSchedulerPriority = RunSchedulerPriorityNormal
	RunSchedulerAgingQuantum    int64                = 8
)

// Valid reports whether the scheduler priority is one of the bounded G05.4 levels.
func (p RunSchedulerPriority) Valid() bool {
	switch p {
	case RunSchedulerPriorityHigh, RunSchedulerPriorityNormal, RunSchedulerPriorityLow:
		return true
	default:
		return false
	}
}

// BaseRank returns the deterministic rank before aging; lower is selected first.
func (p RunSchedulerPriority) BaseRank() (int, bool) {
	switch p {
	case RunSchedulerPriorityHigh:
		return 0, true
	case RunSchedulerPriorityNormal:
		return 1, true
	case RunSchedulerPriorityLow:
		return 2, true
	default:
		return 0, false
	}
}

// EffectiveRank applies deterministic transition-count aging. Waiting runs gain
// one rank every RunSchedulerAgingQuantum persisted scheduler transitions, capped
// at high rank. No wall clock participates, so replay after restart is stable.
func (p RunSchedulerPriority) EffectiveRank(ageTurns int64) (int, bool) {
	rank, ok := p.BaseRank()
	if !ok {
		return 0, false
	}
	if ageTurns < 0 {
		ageTurns = 0
	}
	boost := int(ageTurns / RunSchedulerAgingQuantum)
	rank -= boost
	if rank < 0 {
		rank = 0
	}
	return rank, true
}

const (
	RuntimeQueueCategoryRunEnqueued        = "run_scheduler.enqueued"
	RuntimeQueueCategoryRunPriorityChanged = "run_scheduler.priority_changed"
	RuntimeQueueCategoryRunDequeued        = "run_scheduler.dequeued"
	RuntimeQueueCategoryRunRemoved         = "run_scheduler.removed"
)

// IsRunSchedulerQueueCategory reports whether an append-only queue record belongs
// to the G05.4 scheduler replay protocol.
func IsRunSchedulerQueueCategory(category string) bool {
	switch category {
	case RuntimeQueueCategoryRunEnqueued,
		RuntimeQueueCategoryRunPriorityChanged,
		RuntimeQueueCategoryRunDequeued,
		RuntimeQueueCategoryRunRemoved:
		return true
	default:
		return false
	}
}

// RunScheduleTicket is the replayed active queue projection. EnqueueTurn counts
// scheduler transitions only; EnqueueSeq remains the durable queue.jsonl sequence.
type RunScheduleTicket struct {
	RunID       RunID                `json:"run_id"`
	Priority    RunSchedulerPriority `json:"priority"`
	EnqueueSeq  int64                `json:"enqueue_seq"`
	EnqueueTurn int64                `json:"enqueue_turn"`
}

// CompareRunScheduleTickets returns -1 when a should be selected before b, +1
// when b should be selected first, and 0 only for identical scheduling keys.
// Callers must validate priorities before comparing.
func CompareRunScheduleTickets(a, b RunScheduleTicket, currentTurn int64) int {
	aRank, _ := a.Priority.EffectiveRank(currentTurn - a.EnqueueTurn)
	bRank, _ := b.Priority.EffectiveRank(currentTurn - b.EnqueueTurn)
	if aRank < bRank {
		return -1
	}
	if aRank > bRank {
		return 1
	}
	if a.EnqueueSeq < b.EnqueueSeq {
		return -1
	}
	if a.EnqueueSeq > b.EnqueueSeq {
		return 1
	}
	if a.RunID < b.RunID {
		return -1
	}
	if a.RunID > b.RunID {
		return 1
	}
	return 0
}

// RuntimeQueueItem 是统一运行时队列的持久化记录。
type RuntimeQueueItem struct {
	Seq         int64                `json:"seq"`
	Time        time.Time            `json:"time"`
	Priority    RuntimeQueuePriority `json:"priority"`
	RunID       RunID                `json:"run_id,omitempty"`
	RunPriority RunSchedulerPriority `json:"run_priority,omitempty"`
	TaskID      string               `json:"task_id,omitempty"`
	Agent       string               `json:"agent,omitempty"`
	Category    string               `json:"category,omitempty"`
	Summary     string               `json:"summary,omitempty"`
	Payload     any                  `json:"payload,omitempty"`
}

// RuntimeTaskLogEntry 是单任务运行日志的持久化记录。
type RuntimeTaskLogEntry struct {
	Time    time.Time `json:"time"`
	TaskID  string    `json:"task_id,omitempty"`
	Agent   string    `json:"agent,omitempty"`
	Event   string    `json:"event"`
	Tool    string    `json:"tool,omitempty"`
	Summary string    `json:"summary,omitempty"`
	Payload any       `json:"payload,omitempty"`
}
