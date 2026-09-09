package appruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/voocel/ainovel-cli/internal/host"
)

// DesktopLifecycleState is the desktop-facing run lifecycle. Transitional
// states exist at AppRuntime even though the legacy Host exposes only
// idle/running/paused/completed.
type DesktopLifecycleState string

const (
	LifecycleReady      DesktopLifecycleState = "ready"
	LifecycleStarting   DesktopLifecycleState = "starting"
	LifecycleRunning    DesktopLifecycleState = "running"
	LifecyclePausing    DesktopLifecycleState = "pausing"
	LifecyclePaused     DesktopLifecycleState = "paused"
	LifecycleResuming   DesktopLifecycleState = "resuming"
	LifecycleStopping   DesktopLifecycleState = "stopping"
	LifecycleStopped    DesktopLifecycleState = "stopped"
	LifecycleCancelling DesktopLifecycleState = "cancelling"
	LifecycleCancelled  DesktopLifecycleState = "cancelled"
	LifecycleFailed     DesktopLifecycleState = "failed"
	LifecycleRecovering DesktopLifecycleState = "recovering"
	LifecycleCompleted  DesktopLifecycleState = "completed"
)

const (
	CommandStart  CommandKind = "start"
	CommandPause  CommandKind = "pause"
	CommandResume CommandKind = "resume"
	CommandStop   CommandKind = "stop"
	CommandCancel CommandKind = "cancel"
	CommandRetry  CommandKind = "retry"
)

type StartMode string

const (
	StartModeResume StartMode = "resume"
	StartModeNew    StartMode = "new"
)

// StartCommandPayload keeps new-book creation explicit. An empty payload uses
// resume mode, which is the safe default for an already opened project.
type StartCommandPayload struct {
	Mode        StartMode `json:"mode,omitempty"`
	Requirement string    `json:"requirement,omitempty"`
}

type lifecycleEventPayload struct {
	CommandID     string                `json:"command_id,omitempty"`
	Command       CommandKind           `json:"command"`
	PreviousState DesktopLifecycleState `json:"previous_state,omitempty"`
	State         DesktopLifecycleState `json:"state"`
	Detail        string                `json:"detail,omitempty"`
}

const (
	eventTypeLifecycleState = "lifecycle_state"
	engineStopPollInterval  = 20 * time.Millisecond
	engineStopWaitLimit     = 2 * time.Minute
)

func (r *Runtime) dispatchLifecycle(ctx context.Context, cmd CommandRequest) (CommandResult, error) {
	r.commandMu.Lock()
	defer r.commandMu.Unlock()

	if err := ctx.Err(); err != nil {
		return CommandResult{}, err
	}
	cmd.ID = r.ensureCommandID(cmd.ID)
	state := r.currentLifecycleState()
	result := CommandResult{
		CommandID: cmd.ID,
		RunID:     cmd.RunID,
		TaskID:    cmd.TaskID,
		Status:    string(state),
	}

	switch cmd.Kind {
	case CommandStart:
		return r.dispatchStart(cmd, state, result)
	case CommandPause:
		return r.dispatchPause(cmd, state, result)
	case CommandResume:
		return r.dispatchResume(cmd, state, result)
	case CommandStop:
		return r.dispatchStop(cmd, state, result)
	case CommandCancel:
		return r.dispatchCancel(cmd, state, result)
	case CommandRetry:
		return r.dispatchRetry(cmd, state, result)
	default:
		return result, fmt.Errorf("%w: %q", ErrInvalidCommand, cmd.Kind)
	}
}

func (r *Runtime) dispatchStart(cmd CommandRequest, state DesktopLifecycleState, result CommandResult) (CommandResult, error) {
	payload, err := decodeStartPayload(cmd.Payload)
	if err != nil {
		return result, err
	}
	if !allowsStart(state) {
		return result, commandStateError(CommandStart, state)
	}
	if r.core.DesktopEngineRunning() {
		return result, fmt.Errorf("%w: engine is still running", ErrCommandNotAllowed)
	}

	switch payload.Mode {
	case StartModeNew:
		requirement := strings.TrimSpace(payload.Requirement)
		if requirement == "" {
			return result, fmt.Errorf("%w: start new requires requirement", ErrInvalidCommand)
		}
		if err := r.core.PrepareUserRules(requirement); err != nil {
			r.failLifecycle(cmd, state, err)
			result.Status = string(LifecycleFailed)
			return result, err
		}
		if err := r.core.StartPrepared(requirement); err != nil {
			r.failLifecycle(cmd, state, err)
			result.Status = string(LifecycleFailed)
			return result, err
		}
	case StartModeResume:
		label, err := r.core.Resume()
		if err != nil {
			r.failLifecycle(cmd, state, err)
			result.Status = string(LifecycleFailed)
			return result, err
		}
		if strings.TrimSpace(label) == "" {
			return result, fmt.Errorf("%w: no resumable project state", ErrCommandRejected)
		}
	default:
		return result, fmt.Errorf("%w: unsupported start mode %q", ErrInvalidCommand, payload.Mode)
	}

	r.emitAcceptedTransition(cmd, state, LifecycleStarting, "start accepted by core")
	r.emitAcceptedTransition(cmd, LifecycleStarting, LifecycleRunning, "engine running")
	result.Accepted = true
	result.Status = string(LifecycleRunning)
	return result, nil
}

func (r *Runtime) dispatchPause(cmd CommandRequest, state DesktopLifecycleState, result CommandResult) (CommandResult, error) {
	if state != LifecycleRunning {
		return result, commandStateError(CommandPause, state)
	}
	if !r.core.Abort() {
		return result, fmt.Errorf("%w: core did not accept pause", ErrCommandRejected)
	}

	r.emitAcceptedTransition(cmd, state, LifecyclePausing, "pause accepted; checkpoint/progress retained")
	r.watchEngineStopped(cmd, LifecyclePausing, LifecyclePaused)
	result.Accepted = true
	result.Status = string(LifecyclePausing)
	return result, nil
}

func (r *Runtime) dispatchResume(cmd CommandRequest, state DesktopLifecycleState, result CommandResult) (CommandResult, error) {
	if state != LifecyclePaused {
		return result, commandStateError(CommandResume, state)
	}
	if r.core.DesktopEngineRunning() {
		return result, fmt.Errorf("%w: engine is still finishing pause", ErrCommandNotAllowed)
	}
	label, err := r.core.Resume()
	if err != nil {
		r.failLifecycle(cmd, state, err)
		result.Status = string(LifecycleFailed)
		return result, err
	}
	if strings.TrimSpace(label) == "" {
		return result, fmt.Errorf("%w: no resumable project state", ErrCommandRejected)
	}

	r.emitAcceptedTransition(cmd, state, LifecycleResuming, label)
	r.emitAcceptedTransition(cmd, LifecycleResuming, LifecycleRunning, "engine running")
	result.Accepted = true
	result.Status = string(LifecycleRunning)
	return result, nil
}

func (r *Runtime) dispatchStop(cmd CommandRequest, state DesktopLifecycleState, result CommandResult) (CommandResult, error) {
	switch state {
	case LifecycleRunning:
		if !r.core.Abort() {
			return result, fmt.Errorf("%w: core did not accept stop", ErrCommandRejected)
		}
		r.emitAcceptedTransition(cmd, state, LifecycleStopping, "run stop accepted; durable checkpoint/log retained")
		r.watchEngineStopped(cmd, LifecycleStopping, LifecycleStopped)
		result.Accepted = true
		result.Status = string(LifecycleStopping)
		return result, nil
	case LifecyclePaused:
		if r.core.DesktopEngineRunning() {
			return result, fmt.Errorf("%w: engine is still finishing pause", ErrCommandNotAllowed)
		}
		r.emitAcceptedTransition(cmd, state, LifecycleStopping, "paused run closing")
		r.emitAcceptedTransition(cmd, LifecycleStopping, LifecycleStopped, "run stopped; durable state retained")
		result.Accepted = true
		result.Status = string(LifecycleStopped)
		return result, nil
	default:
		return result, commandStateError(CommandStop, state)
	}
}

func (r *Runtime) dispatchCancel(cmd CommandRequest, state DesktopLifecycleState, result CommandResult) (CommandResult, error) {
	switch state {
	case LifecycleRunning:
		if !r.core.Abort() {
			return result, fmt.Errorf("%w: core did not accept cancel", ErrCommandRejected)
		}
		r.emitAcceptedTransition(cmd, state, LifecycleCancelling, "current attempt cancelled; already committed facts are not rolled back")
		r.watchEngineStopped(cmd, LifecycleCancelling, LifecycleCancelled)
		result.Accepted = true
		result.Status = string(LifecycleCancelling)
		return result, nil
	case LifecyclePaused:
		if r.core.DesktopEngineRunning() {
			return result, fmt.Errorf("%w: engine is still finishing pause", ErrCommandNotAllowed)
		}
		r.emitAcceptedTransition(cmd, state, LifecycleCancelling, "paused attempt cancelled")
		r.emitAcceptedTransition(cmd, LifecycleCancelling, LifecycleCancelled, "attempt cancelled")
		result.Accepted = true
		result.Status = string(LifecycleCancelled)
		return result, nil
	default:
		return result, commandStateError(CommandCancel, state)
	}
}

func (r *Runtime) dispatchRetry(cmd CommandRequest, state DesktopLifecycleState, result CommandResult) (CommandResult, error) {
	if state != LifecycleFailed && state != LifecycleCancelled && state != LifecycleStopped {
		return result, commandStateError(CommandRetry, state)
	}
	if r.core.DesktopEngineRunning() {
		return result, fmt.Errorf("%w: engine is still running", ErrCommandNotAllowed)
	}
	label, err := r.core.Resume()
	if err != nil {
		r.failLifecycle(cmd, state, err)
		result.Status = string(LifecycleFailed)
		return result, err
	}
	if strings.TrimSpace(label) == "" {
		return result, fmt.Errorf("%w: no resumable project state", ErrCommandRejected)
	}

	r.emitAcceptedTransition(cmd, state, LifecycleRecovering, label)
	r.emitAcceptedTransition(cmd, LifecycleRecovering, LifecycleRunning, "retry resumed from durable facts")
	result.Accepted = true
	result.Status = string(LifecycleRunning)
	return result, nil
}

func decodeStartPayload(raw json.RawMessage) (StartCommandPayload, error) {
	payload := StartCommandPayload{Mode: StartModeResume}
	if len(raw) == 0 {
		return payload, nil
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return StartCommandPayload{}, fmt.Errorf("%w: start payload: %v", ErrInvalidCommand, err)
	}
	if payload.Mode == "" {
		payload.Mode = StartModeResume
	}
	return payload, nil
}

func allowsStart(state DesktopLifecycleState) bool {
	switch state {
	case LifecycleReady, LifecycleStopped, LifecycleCancelled, LifecycleFailed:
		return true
	default:
		return false
	}
}

func commandStateError(kind CommandKind, state DesktopLifecycleState) error {
	return fmt.Errorf("%w: command %s is not allowed from %s", ErrCommandNotAllowed, kind, state)
}

func (r *Runtime) ensureCommandID(id string) string {
	if id = strings.TrimSpace(id); id != "" {
		return id
	}
	return fmt.Sprintf("cmd-%d", r.nextCommandID.Add(1))
}

func (r *Runtime) currentLifecycleState() DesktopLifecycleState {
	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()
	if r.lifecycleState == "" {
		r.lifecycleState = lifecycleFromCore(r.core.Snapshot().RuntimeState)
	}
	return r.lifecycleState
}

func (r *Runtime) setLifecycleState(state DesktopLifecycleState) DesktopLifecycleState {
	r.lifecycleMu.Lock()
	previous := r.lifecycleState
	r.lifecycleState = state
	r.lifecycleMu.Unlock()
	return previous
}

func (r *Runtime) emitAcceptedTransition(cmd CommandRequest, previous, next DesktopLifecycleState, detail string) {
	r.setLifecycleState(next)
	payload, _ := json.Marshal(lifecycleEventPayload{
		CommandID:     cmd.ID,
		Command:       cmd.Kind,
		PreviousState: previous,
		State:         next,
		Detail:        detail,
	})
	r.broadcast(DesktopEvent{
		Time:     time.Now().UTC(),
		Category: "LIFECYCLE",
		Type:     eventTypeLifecycleState,
		Level:    "info",
		RunID:    cmd.RunID,
		TaskID:   cmd.TaskID,
		Summary:  string(next),
		Payload:  payload,
	})
}

func (r *Runtime) failLifecycle(cmd CommandRequest, previous DesktopLifecycleState, cause error) {
	r.setLifecycleState(LifecycleFailed)
	payload, _ := json.Marshal(lifecycleEventPayload{
		CommandID:     cmd.ID,
		Command:       cmd.Kind,
		PreviousState: previous,
		State:         LifecycleFailed,
		Detail:        cause.Error(),
	})
	r.broadcast(DesktopEvent{
		Time:     time.Now().UTC(),
		Category: "LIFECYCLE",
		Type:     eventTypeLifecycleState,
		Level:    "error",
		RunID:    cmd.RunID,
		TaskID:   cmd.TaskID,
		Summary:  string(LifecycleFailed),
		Payload:  payload,
	})
}

func (r *Runtime) watchEngineStopped(cmd CommandRequest, transition, final DesktopLifecycleState) {
	r.commandWG.Add(1)
	go func() {
		defer r.commandWG.Done()
		ticker := time.NewTicker(engineStopPollInterval)
		defer ticker.Stop()
		timer := time.NewTimer(engineStopWaitLimit)
		defer timer.Stop()

		for {
			if r.closed.Load() {
				return
			}
			if !r.core.DesktopEngineRunning() {
				if r.currentLifecycleState() == transition {
					r.emitAcceptedTransition(cmd, transition, final, "engine stopped")
				}
				return
			}
			select {
			case <-ticker.C:
			case <-timer.C:
				if r.currentLifecycleState() == transition {
					r.failLifecycle(cmd, transition, fmt.Errorf("engine did not stop within lifecycle guard window"))
				}
				return
			}
		}
	}()
}

func lifecycleFromCore(state string) DesktopLifecycleState {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "running":
		return LifecycleRunning
	case "paused":
		return LifecyclePaused
	case "completed":
		return LifecycleCompleted
	case "idle", "":
		return LifecycleReady
	default:
		return LifecycleReady
	}
}

func (r *Runtime) reconcileLifecycle(src host.UISnapshot) {
	current := r.currentLifecycleState()
	if current == LifecycleStarting || current == LifecyclePausing || current == LifecycleResuming ||
		current == LifecycleStopping || current == LifecycleCancelling || current == LifecycleRecovering {
		return
	}
	if current == LifecycleStopped || current == LifecycleCancelled || current == LifecycleFailed {
		return
	}
	coreState := lifecycleFromCore(src.RuntimeState)
	switch current {
	case LifecycleRunning:
		if coreState == LifecycleCompleted || coreState == LifecyclePaused || coreState == LifecycleReady {
			r.setLifecycleState(coreState)
		}
	case LifecycleReady:
		if coreState == LifecycleRunning || coreState == LifecyclePaused || coreState == LifecycleCompleted {
			r.setLifecycleState(coreState)
		}
	case LifecyclePaused:
		if coreState == LifecycleCompleted {
			r.setLifecycleState(coreState)
		}
	case LifecycleCompleted:
		// Completed stays terminal until an explicit reopen/new-project path.
	}
}

func (r *Runtime) applyLifecycleProjection(snapshot *DesktopSnapshot) {
	if snapshot == nil {
		return
	}
	state := r.currentLifecycleState()
	snapshot.Runtime.State = string(state)
	snapshot.Runtime.IsRunning = r.core.DesktopEngineRunning()
	switch state {
	case LifecycleStarting, LifecyclePausing, LifecycleResuming, LifecycleStopping,
		LifecycleCancelling, LifecycleRecovering, LifecycleStopped, LifecycleCancelled, LifecycleFailed:
		snapshot.Runtime.Status = string(state)
	}
}
