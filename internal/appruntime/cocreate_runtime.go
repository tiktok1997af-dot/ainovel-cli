package appruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/voocel/ainovel-cli/internal/host"
)

// coCreateRuntimeAdapter is the single AppRuntime-to-Host seam for desktop
// CoCreate. G08.5 opens the existing Host stage begin/turn/finish/cancel seams
// while keeping all browser/provider/session authority inside Host.
type coCreateRuntimeAdapter struct {
	core          *host.Host
	coldStartTurn func(context.Context, []host.CoCreateMessage) (host.CoCreateReply, error)
	stageBegin    func() bool
	stageTurn     func(context.Context, []host.CoCreateMessage) (host.CoCreateReply, error)
	stageFinish   func(string) error
	stageCancel   func()
	stageActive   bool
}

func newCoCreateRuntimeAdapter(core *host.Host) *coCreateRuntimeAdapter {
	adapter := &coCreateRuntimeAdapter{core: core}
	if core != nil {
		adapter.coldStartTurn = func(ctx context.Context, history []host.CoCreateMessage) (host.CoCreateReply, error) {
			return core.CoCreateStream(ctx, history, nil)
		}
		adapter.stageBegin = core.PauseForCoCreate
		adapter.stageTurn = func(ctx context.Context, history []host.CoCreateMessage) (host.CoCreateReply, error) {
			return core.StageCoCreateStream(ctx, history, nil)
		}
		adapter.stageFinish = core.ResumeFromCoCreate
		adapter.stageCancel = core.CancelCoCreate
	}
	return adapter
}

// dispatchCoCreate serializes CoCreate with the existing AppRuntime command
// authority. Holding commandMu across the ownership check prevents a Run Center
// command/scheduler transition from being admitted concurrently between the
// G05 managed-work check and the Host CoCreate call.
func (r *Runtime) dispatchCoCreate(ctx context.Context, cmd CommandRequest) (CommandResult, error) {
	r.commandMu.Lock()
	defer r.commandMu.Unlock()

	cmd.ID = r.ensureCommandID(cmd.ID)
	result := CommandResult{
		CommandID: cmd.ID,
		RunID:     cmd.RunID,
		TaskID:    cmd.TaskID,
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}

	route, err := decodeCoCreateContract(cmd)
	if err != nil {
		return result, err
	}

	// G05 remains the sole scheduler/resource/browser-lane authority. CoCreate
	// cannot execute while queued, blocked, active or otherwise managed Run
	// Center work exists, even when the underlying Engine is momentarily idle.
	if r.run != nil && r.run.hasManagedWork() {
		return result, fmt.Errorf("%w: Run Center managed work owns CoCreate execution", ErrCommandNotAllowed)
	}

	if r.coCreate == nil {
		return result, ErrRuntimeUnavailable
	}
	return r.coCreate.dispatch(ctx, route, result)
}

func (a *coCreateRuntimeAdapter) dispatch(ctx context.Context, route coCreateContractRoute, result CommandResult) (CommandResult, error) {
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if a == nil || a.core == nil {
		return result, ErrRuntimeUnavailable
	}

	switch route.Kind {
	case CommandCoCreateTurn:
		payload, ok := route.Request.(CoCreateTurnCommandPayload)
		if !ok {
			return result, fmt.Errorf("%w: invalid CoCreate turn route", ErrInvalidCommand)
		}
		switch payload.Mode {
		case CoCreateModeColdStart:
			return a.dispatchColdStartTurn(ctx, payload, result)
		case CoCreateModeStage:
			return a.dispatchStageTurn(ctx, payload, result)
		default:
			return result, fmt.Errorf("%w: invalid CoCreate mode", ErrInvalidCommand)
		}
	case CommandCoCreateStageBegin:
		return a.dispatchStageBegin(result)
	case CommandCoCreateStageFinish:
		payload, ok := route.Request.(CoCreateStageFinishCommandPayload)
		if !ok {
			return result, fmt.Errorf("%w: invalid stage finish route", ErrInvalidCommand)
		}
		return a.dispatchStageFinish(payload, result)
	case CommandCoCreateStageCancel:
		return a.dispatchStageCancel(result)
	default:
		return result, fmt.Errorf("%w: invalid CoCreate route", ErrInvalidCommand)
	}
}

func (a *coCreateRuntimeAdapter) dispatchColdStartTurn(ctx context.Context, payload CoCreateTurnCommandPayload, result CommandResult) (CommandResult, error) {
	if a.stageActive {
		return result, fmt.Errorf("%w: stage CoCreate is active", ErrCommandNotAllowed)
	}
	if a.coldStartTurn == nil {
		return result, ErrRuntimeUnavailable
	}

	reply, err := a.coldStartTurn(ctx, coCreateHostHistory(payload.History))
	if err != nil {
		return result, fmt.Errorf("cold-start CoCreate turn: %w", err)
	}

	return projectCoCreateTurnResult(payload, reply, false, result)
}

func (a *coCreateRuntimeAdapter) dispatchStageBegin(result CommandResult) (CommandResult, error) {
	if a.stageActive {
		return result, fmt.Errorf("%w: stage CoCreate is already active", ErrCommandNotAllowed)
	}
	if a.stageBegin == nil {
		return result, ErrRuntimeUnavailable
	}
	if !a.stageBegin() {
		return result, fmt.Errorf("%w: Host rejected stage CoCreate begin", ErrCommandRejected)
	}

	a.stageActive = true
	return projectCoCreateStageState(true, result)
}

func (a *coCreateRuntimeAdapter) dispatchStageTurn(ctx context.Context, payload CoCreateTurnCommandPayload, result CommandResult) (CommandResult, error) {
	if !a.stageActive {
		return result, fmt.Errorf("%w: stage CoCreate is not active", ErrCommandNotAllowed)
	}
	if a.stageTurn == nil {
		return result, ErrRuntimeUnavailable
	}

	reply, err := a.stageTurn(ctx, coCreateHostHistory(payload.History))
	if err != nil {
		return result, fmt.Errorf("stage CoCreate turn: %w", err)
	}
	return projectCoCreateTurnResult(payload, reply, true, result)
}

func (a *coCreateRuntimeAdapter) dispatchStageFinish(payload CoCreateStageFinishCommandPayload, result CommandResult) (CommandResult, error) {
	if !a.stageActive {
		return result, fmt.Errorf("%w: stage CoCreate is not active", ErrCommandNotAllowed)
	}
	if a.stageFinish == nil {
		return result, ErrRuntimeUnavailable
	}

	// Host.ResumeFromCoCreate clears Host occupancy before it calls Continue.
	// Mirror that terminal transition after the Host call even on error so the
	// AppRuntime presentation flag cannot claim a stage that Host already left.
	err := a.stageFinish(payload.Draft)
	a.stageActive = false
	if err != nil {
		return result, fmt.Errorf("finish stage CoCreate: %w", err)
	}
	return projectCoCreateStageState(false, result)
}

func (a *coCreateRuntimeAdapter) dispatchStageCancel(result CommandResult) (CommandResult, error) {
	if !a.stageActive {
		return result, fmt.Errorf("%w: stage CoCreate is not active", ErrCommandNotAllowed)
	}
	if a.stageCancel == nil {
		return result, ErrRuntimeUnavailable
	}

	a.stageCancel()
	a.stageActive = false
	return projectCoCreateStageState(false, result)
}

func projectCoCreateTurnResult(payload CoCreateTurnCommandPayload, reply host.CoCreateReply, stageActive bool, result CommandResult) (CommandResult, error) {
	dto := CoCreateTurnResultDTO{
		Session: CoCreateSessionDTO{
			Mode:         payload.Mode,
			HistoryCount: len(payload.History) + 1,
			StageActive:  stageActive,
		},
		Message:     strings.TrimSpace(reply.Message),
		Draft:       strings.TrimSpace(reply.Prompt),
		Ready:       reply.Ready,
		Suggestions: append([]string(nil), reply.Suggestions...),
	}
	if err := validateCoCreateTurnResult(dto); err != nil {
		return result, fmt.Errorf("%w: Host returned invalid CoCreate turn result", ErrCommandRejected)
	}

	data, err := json.Marshal(dto)
	if err != nil {
		return result, fmt.Errorf("encode CoCreate turn result: %w", err)
	}
	result.Accepted = true
	result.Data = data
	return result, nil
}

func projectCoCreateStageState(active bool, result CommandResult) (CommandResult, error) {
	dto := CoCreateStageStateResultDTO{
		Session: CoCreateSessionDTO{
			Mode:         CoCreateModeStage,
			HistoryCount: 0,
			StageActive:  active,
		},
	}
	data, err := json.Marshal(dto)
	if err != nil {
		return result, fmt.Errorf("encode stage CoCreate state: %w", err)
	}
	result.Accepted = true
	result.Data = data
	return result, nil
}

// coCreateHostHistory reconstructs only the Host-visible CoCreate protocol
// continuity needed for the next model turn. Raw provider/model responses never
// cross the AppRuntime boundary.
func coCreateHostHistory(items []CoCreateHistoryItemDTO) []host.CoCreateMessage {
	history := make([]host.CoCreateMessage, 0, len(items))
	for _, item := range items {
		content := item.Message
		if item.Role == CoCreateRoleAssistant {
			content = coCreateAssistantProtocol(item)
		}
		history = append(history, host.CoCreateMessage{
			Role:    string(item.Role),
			Content: content,
		})
	}
	return history
}

func coCreateAssistantProtocol(item CoCreateHistoryItemDTO) string {
	var b strings.Builder
	b.WriteString("<reply>\n")
	b.WriteString(item.Message)
	b.WriteString("\n</reply>\n\n<draft>\n")
	b.WriteString(item.Draft)
	b.WriteString("\n</draft>\n\n<ready>")
	if item.Ready {
		b.WriteString("true")
	} else {
		b.WriteString("false")
	}
	b.WriteString("</ready>\n\n<suggestions>\n")
	for _, suggestion := range item.Suggestions {
		b.WriteString("- ")
		b.WriteString(suggestion)
		b.WriteByte('\n')
	}
	b.WriteString("</suggestions>")
	return b.String()
}
