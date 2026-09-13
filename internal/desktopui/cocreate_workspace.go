package desktopui

import (
	"strings"

	"github.com/voocel/ainovel-cli/internal/appruntime"
)

type CoCreateAction string

const (
	CoCreateActionBeginStage CoCreateAction = "begin_stage"
	CoCreateActionSend       CoCreateAction = "send"
	CoCreateActionFinish     CoCreateAction = "finish"
	CoCreateActionCancel     CoCreateAction = "cancel"
	CoCreateActionStartNew   CoCreateAction = "start_new"
)

type CoCreateControlView struct {
	Enabled bool
	Reason  string
}

type CoCreateControlsView struct {
	BeginStage CoCreateControlView
	Send       CoCreateControlView
	Finish     CoCreateControlView
	Cancel     CoCreateControlView
	StartNew   CoCreateControlView
}

// CoCreateWorkspaceState is transient desktop presentation state only. History
// mirrors the sanitized AppRuntime DTO and is never persisted as project truth.
type CoCreateWorkspaceState struct {
	Load            LoadState
	Mode            appruntime.CoCreateMode
	History         []appruntime.CoCreateHistoryItemDTO
	Draft           string
	Ready           bool
	Suggestions     []string
	StageActive     bool
	Pending         bool
	PendingAction   CoCreateAction
	LastCommandID   string
	HandoffAccepted bool
	Error           *ErrorView
}

func NewCoCreateWorkspaceState() CoCreateWorkspaceState {
	return CoCreateWorkspaceState{
		Load: LoadReady,
		Mode: appruntime.CoCreateModeColdStart,
	}
}

func (s *CoCreateWorkspaceState) Controls() CoCreateControlsView {
	if s == nil {
		return disabledCoCreateControls("CoCreate workspace is unavailable.")
	}
	if s.Pending {
		return disabledCoCreateControls("A CoCreate command is pending.")
	}

	controls := CoCreateControlsView{}
	if s.StageActive {
		controls.BeginStage = CoCreateControlView{Reason: "A stage CoCreate session is already active."}
		controls.Send = CoCreateControlView{Enabled: canAppendCoCreateTurn(s.History), Reason: "CoCreate history has reached its bounded limit."}
		if controls.Send.Enabled {
			controls.Send.Reason = ""
		}
		controls.Finish = CoCreateControlView{Enabled: strings.TrimSpace(s.Draft) != "", Reason: "A non-empty accepted draft is required."}
		if controls.Finish.Enabled {
			controls.Finish.Reason = ""
		}
		controls.Cancel = CoCreateControlView{Enabled: true}
		controls.StartNew = CoCreateControlView{Reason: "Cold-start handoff is unavailable during stage CoCreate."}
		return controls
	}

	controls.BeginStage = CoCreateControlView{Enabled: true}
	controls.Send = CoCreateControlView{Enabled: s.Mode == appruntime.CoCreateModeColdStart && canAppendCoCreateTurn(s.History), Reason: "CoCreate history has reached its bounded limit."}
	if controls.Send.Enabled {
		controls.Send.Reason = ""
	}
	controls.Finish = CoCreateControlView{Reason: "No stage CoCreate session is active."}
	controls.Cancel = CoCreateControlView{Reason: "No stage CoCreate session is active."}
	controls.StartNew = CoCreateControlView{
		Enabled: s.Mode == appruntime.CoCreateModeColdStart && strings.TrimSpace(s.Draft) != "" && !s.HandoffAccepted,
		Reason:  "A non-empty accepted cold-start draft is required.",
	}
	if controls.StartNew.Enabled {
		controls.StartNew.Reason = ""
	}
	return controls
}

func disabledCoCreateControls(reason string) CoCreateControlsView {
	control := CoCreateControlView{Reason: reason}
	return CoCreateControlsView{
		BeginStage: control,
		Send:       control,
		Finish:     control,
		Cancel:     control,
		StartNew:   control,
	}
}

func canAppendCoCreateTurn(history []appruntime.CoCreateHistoryItemDTO) bool {
	// One Send appends a user item before dispatch and one assistant item after
	// acceptance. Reserve both slots so the sanitized history never exceeds the
	// AppRuntime transport bound.
	return len(history)+2 <= appruntime.MaxCoCreateHistoryItems
}

func cloneCoCreateHistory(in []appruntime.CoCreateHistoryItemDTO) []appruntime.CoCreateHistoryItemDTO {
	out := make([]appruntime.CoCreateHistoryItemDTO, len(in))
	for i := range in {
		out[i] = in[i]
		out[i].Suggestions = append([]string(nil), in[i].Suggestions...)
	}
	return out
}

func (s *CoCreateWorkspaceState) resetConversation(mode appruntime.CoCreateMode, stageActive bool) {
	s.Mode = mode
	s.History = nil
	s.Draft = ""
	s.Ready = false
	s.Suggestions = nil
	s.StageActive = stageActive
	s.HandoffAccepted = false
	s.Error = nil
	s.Load = LoadReady
}
