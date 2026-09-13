package appruntime

import (
	"context"
	"fmt"

	"github.com/voocel/ainovel-cli/internal/host"
)

// coCreateRuntimeAdapter is the single AppRuntime-to-Host seam for desktop
// CoCreate. G08.3 establishes ownership/routing only; the concrete cold-start
// and stage execution semantics remain closed for G08.4 and G08.5.
type coCreateRuntimeAdapter struct {
	core *host.Host
}

func newCoCreateRuntimeAdapter(core *host.Host) *coCreateRuntimeAdapter {
	return &coCreateRuntimeAdapter{core: core}
}

// dispatchCoCreate serializes CoCreate with the existing AppRuntime command
// authority. Holding commandMu across the ownership check prevents a Run Center
// command/scheduler transition from being admitted concurrently between the
// G05 managed-work check and a future Host CoCreate call.
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

// dispatch deliberately does not invoke Host in G08.3. It proves that valid
// typed commands have reached the dedicated adapter after G05 ownership has
// been checked. The successor steps fill the already-frozen execution slots:
// G08.4 owns cold_start turn execution; G08.5 owns stage operations.
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
			return result, fmt.Errorf("%w: cold-start CoCreate execution opens in G08.4", ErrNotImplemented)
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
