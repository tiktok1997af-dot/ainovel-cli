package appruntime

import (
	"fmt"
	"strings"
	"time"

	"github.com/voocel/ainovel-cli/internal/domain"
)

// G05.2 freezes the serialization contract only. The owning later gates add
// persistence, scheduling, locking, browser-lane allocation and orchestration.
const (
	QueryRunsList     QueryKind = "runs.list"
	QueryRunsGet      QueryKind = "runs.get"
	QueryRunsActivity QueryKind = "runs.activity"
)

const (
	CommandRunStart  CommandKind = "run.start"
	CommandRunPause  CommandKind = "run.pause"
	CommandRunResume CommandKind = "run.resume"
	CommandRunStop   CommandKind = "run.stop"
	CommandRunCancel CommandKind = "run.cancel"
	CommandRunRetry  CommandKind = "run.retry"
)

const (
	RunEventCategory     = "RUN"
	EventTypeRunState    = "run_state"
	EventTypeRunProgress = "run_progress"
	EventTypeRunTask     = "run_task"
)

type RunCenterContractCatalog struct {
	QueryKinds   []QueryKind   `json:"query_kinds"`
	CommandKinds []CommandKind `json:"command_kinds"`
	EventTypes   []string      `json:"event_types"`
}

func CurrentRunCenterContractCatalog() RunCenterContractCatalog {
	return RunCenterContractCatalog{
		QueryKinds:   []QueryKind{QueryRunsList, QueryRunsGet, QueryRunsActivity},
		CommandKinds: []CommandKind{CommandRunStart, CommandRunPause, CommandRunResume, CommandRunStop, CommandRunCancel, CommandRunRetry},
		EventTypes:   []string{EventTypeRunState, EventTypeRunProgress, EventTypeRunTask},
	}
}

type RunsListQuery struct {
	PageQuery
	State string `json:"state,omitempty"`
}

type RunsGetQuery struct {
	RunID domain.RunID `json:"run_id"`
}

type RunsActivityQuery struct {
	PageQuery
	RunID domain.RunID `json:"run_id"`
}

type RunSummaryDTO struct {
	RunID      domain.RunID         `json:"run_id"`
	State      string               `json:"state"`
	Status     string               `json:"status,omitempty"`
	TaskID     domain.TaskID        `json:"task_id,omitempty"`
	Priority   string               `json:"priority,omitempty"`
	WaitReason string               `json:"wait_reason,omitempty"`
	Resource   domain.ResourceKey   `json:"resource,omitempty"`
	LaneID     domain.BrowserLaneID `json:"lane_id,omitempty"`
	CreatedAt  time.Time            `json:"created_at,omitempty"`
	UpdatedAt  time.Time            `json:"updated_at,omitempty"`
	StartedAt  time.Time            `json:"started_at,omitempty"`
	FinishedAt time.Time            `json:"finished_at,omitempty"`
}

type RunsListResultDTO struct {
	Items  []RunSummaryDTO `json:"items"`
	Offset int             `json:"offset"`
	Limit  int             `json:"limit"`
	Total  int             `json:"total"`
}

type RunTaskDTO struct {
	TaskID     domain.TaskID `json:"task_id"`
	Kind       string        `json:"kind,omitempty"`
	State      string        `json:"state,omitempty"`
	Summary    string        `json:"summary,omitempty"`
	StartedAt  time.Time     `json:"started_at,omitempty"`
	FinishedAt time.Time     `json:"finished_at,omitempty"`
}

type RunActivityDTO struct {
	Seq      int64         `json:"seq,omitempty"`
	Time     time.Time     `json:"time"`
	Category string        `json:"category"`
	Type     string        `json:"type"`
	Level    string        `json:"level,omitempty"`
	TaskID   domain.TaskID `json:"task_id,omitempty"`
	Summary  string        `json:"summary,omitempty"`
}

type RunsGetResultDTO struct {
	Run   RunSummaryDTO `json:"run"`
	Tasks []RunTaskDTO  `json:"tasks,omitempty"`
}

type RunsActivityResultDTO struct {
	Items  []RunActivityDTO `json:"items"`
	Offset int              `json:"offset"`
	Limit  int              `json:"limit"`
	Total  int              `json:"total"`
}

type RunStateEventPayloadDTO struct {
	PreviousState string               `json:"previous_state,omitempty"`
	State         string               `json:"state"`
	Status        string               `json:"status,omitempty"`
	WaitReason    string               `json:"wait_reason,omitempty"`
	Resource      domain.ResourceKey   `json:"resource,omitempty"`
	LaneID        domain.BrowserLaneID `json:"lane_id,omitempty"`
}

type RunProgressEventPayloadDTO struct {
	CurrentChapter    int `json:"current_chapter,omitempty"`
	CompletedChapters int `json:"completed_chapters,omitempty"`
	TotalChapters     int `json:"total_chapters,omitempty"`
	TotalWordCount    int `json:"total_word_count,omitempty"`
}

type RunTaskEventPayloadDTO struct {
	TaskID  domain.TaskID `json:"task_id"`
	Kind    string        `json:"kind,omitempty"`
	State   string        `json:"state,omitempty"`
	Summary string        `json:"summary,omitempty"`
}

// NewRunControlCommand validates the frozen G05.1 identity and creates only the
// typed AppRuntime envelope. It never chooses a resource, priority or lane.
// Runtime handling for these run.* commands remains closed until its owning gate.
func NewRunControlCommand(commandID string, kind CommandKind, runID domain.RunID) (CommandRequest, error) {
	if !isRunCenterCommandKind(kind) {
		return CommandRequest{}, fmt.Errorf("unsupported run control command: %s", kind)
	}
	identity := domain.RunIdentity{RunID: runID}
	if err := identity.Validate(); err != nil {
		return CommandRequest{}, fmt.Errorf("invalid run control identity: %w", err)
	}
	return CommandRequest{
		ContractVersion: ContractVersion,
		ID:              strings.TrimSpace(commandID),
		Kind:            kind,
		RunID:           string(runID),
	}, nil
}

func isRunCenterCommandKind(kind CommandKind) bool {
	switch kind {
	case CommandRunStart, CommandRunPause, CommandRunResume, CommandRunStop, CommandRunCancel, CommandRunRetry:
		return true
	default:
		return false
	}
}
