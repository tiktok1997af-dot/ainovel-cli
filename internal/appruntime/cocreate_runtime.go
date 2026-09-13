package appruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/voocel/ainovel-cli/internal/host"
)

// coCreateRuntimeAdapter is the single AppRuntime-to-Host seam for desktop
// CoCreate. G08.4 opens only cold-start turn execution; stage execution remains
// closed for G08.5.
type coCreateRuntimeAdapter struct {
	core          *host.Host
	coldStartTurn func(context.Context, []host.CoCreateMessage) (host.CoCreateReply, error)
}

func newCoCreateRuntimeAdapter(core *host.Host) *coCreateRuntimeAdapter {
	adapter := &coCreateRuntimeAdapter{core: core}
	if core != nil {
		adapter.coldStartTurn = func(ctx context.Context, history []host.CoCreateMessage) (host.CoCreateReply, error) {
			return core.CoCreateStream(ctx, history, nil)
		}
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
			return result, fmt.Errorf("%w: stage CoCreate execution opens in G08.5", ErrNotImplemented)
		default:
			return result, fmt.Errorf("%w: invalid CoCreate mode", ErrInvalidCommand)
		}
	case CommandCoCreateStageBegin, CommandCoCreateStageFinish, CommandCoCreateStageCancel:
		return result, fmt.Errorf("%w: stage CoCreate execution opens in G08.5", ErrNotImplemented)
	default:
		return result, fmt.Errorf("%w: invalid CoCreate route", ErrInvalidCommand)
	}
}

func (a *coCreateRuntimeAdapter) dispatchColdStartTurn(ctx context.Context, payload CoCreateTurnCommandPayload, result CommandResult) (CommandResult, error) {
	if a.coldStartTurn == nil {
		return result, ErrRuntimeUnavailable
	}

	reply, err := a.coldStartTurn(ctx, coCreateHostHistory(payload.History))
	if err != nil {
		return result, fmt.Errorf("cold-start CoCreate turn: %w", err)
	}

	dto := CoCreateTurnResultDTO{
		Session: CoCreateSessionDTO{
			Mode:         CoCreateModeColdStart,
			HistoryCount: len(payload.History) + 1,
			StageActive:  false,
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
