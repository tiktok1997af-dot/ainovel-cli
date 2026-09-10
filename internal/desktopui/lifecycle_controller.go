package desktopui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/voocel/ainovel-cli/internal/appruntime"
)

const lifecycleStateEventType = "lifecycle_state"

type LifecycleAction string

const (
	LifecycleStart  LifecycleAction = "start"
	LifecyclePause  LifecycleAction = "pause"
	LifecycleResume LifecycleAction = "resume"
	LifecycleStop   LifecycleAction = "stop"
	LifecycleCancel LifecycleAction = "cancel"
	LifecycleRetry  LifecycleAction = "retry"
)

// LifecycleCommandState is transient UI state for the last command request.
// It does not own lifecycle truth: ObservedState is copied only from an
// AppRuntime CommandResult, lifecycle event, or fresh DesktopSnapshot.
type LifecycleCommandState struct {
	Pending       bool
	CommandID     string
	Action        LifecycleAction
	Accepted      bool
	ResultStatus  string
	ObservedState string
	Error         *ErrorView
}

type lifecycleControlPlane struct {
	mu     sync.Mutex
	nextID uint64
	state  LifecycleCommandState
}

type lifecycleEventPayloadView struct {
	CommandID string                           `json:"command_id,omitempty"`
	Command   appruntime.CommandKind           `json:"command"`
	State     appruntime.DesktopLifecycleState `json:"state"`
}

func (c *Controller) Lifecycle() LifecycleCommandState {
	c.lifecycle.mu.Lock()
	defer c.lifecycle.mu.Unlock()
	out := c.lifecycle.state
	if out.Error != nil {
		copyErr := *out.Error
		out.Error = &copyErr
	}
	return out
}

// ExecuteLifecycle binds only the six G02 lifecycle commands. The global Start
// control uses G02's safe resume mode for an already opened project; creating a
// new project/workspace is intentionally not opened by G04.6.
func (c *Controller) ExecuteLifecycle(ctx context.Context, action LifecycleAction) (appruntime.CommandResult, error) {
	if c.runtime == nil {
		return appruntime.CommandResult{}, ErrNilRuntime
	}
	if !c.shell.HasSnapshot || c.sub == nil {
		return appruntime.CommandResult{}, lifecycleAppError(
			appruntime.ErrorCodeRuntimeUnavailable,
			appruntime.ErrorCategoryRuntime,
			"The application runtime is unavailable.",
			true,
		)
	}

	kind, ok := lifecycleCommandKind(action)
	if !ok {
		return appruntime.CommandResult{}, lifecycleAppError(
			appruntime.ErrorCodeInvalidArgument,
			appruntime.ErrorCategoryValidation,
			"The request is invalid.",
			false,
		)
	}

	current := strings.ToLower(strings.TrimSpace(c.shell.Runtime.State))
	if !allowsLifecyclePresentation(string(action), current) {
		return appruntime.CommandResult{}, lifecycleAppError(
			appruntime.ErrorCodeCommandNotAllowed,
			appruntime.ErrorCategoryConflict,
			"This action is not allowed in the current state.",
			false,
		)
	}

	commandID, err := c.beginLifecycleCommand(action, current)
	if err != nil {
		return appruntime.CommandResult{}, err
	}

	payload, err := lifecycleCommandPayload(action)
	if err != nil {
		c.finishLifecycleFailure(commandID, "", viewErrorFromError(err))
		return appruntime.CommandResult{}, err
	}
	request := appruntime.CommandRequest{
		ContractVersion: appruntime.ContractVersion,
		ID:              commandID,
		Kind:            kind,
		Payload:         payload,
	}

	result, dispatchErr := c.runtime.Dispatch(ctx, request)
	if dispatchErr != nil || result.Error != nil {
		failure := result.Error
		if failure == nil {
			var appErr *appruntime.AppError
			if errors.As(dispatchErr, &appErr) {
				failure = appErr
			}
		}
		viewErr := viewErrorFromError(dispatchErr)
		if failure != nil {
			viewErr = viewErrorFromAppError(failure)
		}
		c.finishLifecycleFailure(commandID, result.Status, viewErr)
		refreshErr := c.Refresh(ctx)
		if dispatchErr != nil {
			return result, errors.Join(dispatchErr, refreshErr)
		}
		if result.Error != nil {
			return result, errors.Join(result.Error, refreshErr)
		}
		return result, refreshErr
	}

	if result.ContractVersion != appruntime.ContractVersion {
		protocolErr := lifecycleAppError(
			appruntime.ErrorCodeContractMismatch,
			appruntime.ErrorCategoryValidation,
			"The desktop and core contract versions are incompatible.",
			false,
		)
		c.finishLifecycleFailure(commandID, result.Status, viewErrorFromAppError(protocolErr))
		refreshErr := c.Refresh(ctx)
		return result, errors.Join(protocolErr, refreshErr)
	}
	if result.CommandID != commandID {
		protocolErr := lifecycleAppError(
			appruntime.ErrorCodeInternal,
			appruntime.ErrorCategoryInternal,
			"An internal runtime error occurred.",
			false,
		)
		c.finishLifecycleFailure(commandID, result.Status, viewErrorFromAppError(protocolErr))
		refreshErr := c.Refresh(ctx)
		return result, errors.Join(protocolErr, refreshErr)
	}
	if !result.Accepted {
		rejected := lifecycleAppError(
			appruntime.ErrorCodeCommandRejected,
			appruntime.ErrorCategoryRuntime,
			"The runtime rejected this action.",
			false,
		)
		c.finishLifecycleFailure(commandID, result.Status, viewErrorFromAppError(rejected))
		refreshErr := c.Refresh(ctx)
		return result, errors.Join(rejected, refreshErr)
	}

	c.finishLifecycleAccepted(commandID, result.Status)
	if err := c.Refresh(ctx); err != nil {
		return result, err
	}
	return result, nil
}

func (c *Controller) beginLifecycleCommand(action LifecycleAction, observed string) (string, error) {
	c.lifecycle.mu.Lock()
	defer c.lifecycle.mu.Unlock()
	if c.lifecycle.state.Pending {
		return "", lifecycleAppError(
			appruntime.ErrorCodeCommandNotAllowed,
			appruntime.ErrorCategoryConflict,
			"This action is not allowed in the current state.",
			false,
		)
	}
	c.lifecycle.nextID++
	commandID := fmt.Sprintf("desktop-lifecycle-%d", c.lifecycle.nextID)
	c.lifecycle.state = LifecycleCommandState{
		Pending:       true,
		CommandID:     commandID,
		Action:        action,
		ObservedState: observed,
	}
	c.shell.Controls = activatedLifecycleControls(observed, true)
	return commandID, nil
}

func (c *Controller) finishLifecycleAccepted(commandID, status string) {
	c.lifecycle.mu.Lock()
	defer c.lifecycle.mu.Unlock()
	if c.lifecycle.state.CommandID != commandID {
		return
	}
	c.lifecycle.state.Pending = false
	c.lifecycle.state.Accepted = true
	c.lifecycle.state.ResultStatus = strings.ToLower(strings.TrimSpace(status))
	if isKnownLifecycleState(c.lifecycle.state.ResultStatus) {
		c.lifecycle.state.ObservedState = c.lifecycle.state.ResultStatus
		c.shell.Runtime.State = c.lifecycle.state.ResultStatus
	}
	c.lifecycle.state.Error = nil
	c.shell.Controls = activatedLifecycleControls(c.shell.Runtime.State, false)
}

func (c *Controller) finishLifecycleFailure(commandID, status string, viewErr *ErrorView) {
	c.lifecycle.mu.Lock()
	defer c.lifecycle.mu.Unlock()
	if c.lifecycle.state.CommandID != commandID {
		return
	}
	c.lifecycle.state.Pending = false
	c.lifecycle.state.Accepted = false
	c.lifecycle.state.ResultStatus = strings.ToLower(strings.TrimSpace(status))
	c.lifecycle.state.Error = viewErr
	c.shell.Controls = activatedLifecycleControls(c.shell.Runtime.State, false)
}

func (c *Controller) reconcileLifecycleFromSnapshot() {
	state := strings.ToLower(strings.TrimSpace(c.shell.Runtime.State))
	c.lifecycle.mu.Lock()
	defer c.lifecycle.mu.Unlock()
	if c.lifecycle.state.CommandID != "" {
		c.lifecycle.state.ObservedState = state
	}
	c.shell.Controls = activatedLifecycleControls(state, c.lifecycle.state.Pending)
}

func (c *Controller) deactivateLifecycleControls() {
	c.lifecycle.mu.Lock()
	defer c.lifecycle.mu.Unlock()
	controls := ReservedLifecycleControls(c.shell.Runtime.State)
	for i := range controls {
		controls[i].Enabled = false
		controls[i].Reason = "Lifecycle controls require a current AppRuntime snapshot and subscription."
	}
	c.shell.Controls = controls
}

func (c *Controller) observeLifecycleEvent(event appruntime.DesktopEvent) bool {
	if !strings.EqualFold(strings.TrimSpace(event.Category), "LIFECYCLE") ||
		strings.TrimSpace(event.Type) != lifecycleStateEventType {
		return false
	}

	payload := lifecycleEventPayloadView{}
	_ = json.Unmarshal(event.Payload, &payload)
	state := strings.ToLower(strings.TrimSpace(string(payload.State)))
	if !isKnownLifecycleState(state) {
		state = strings.ToLower(strings.TrimSpace(event.Summary))
	}
	if !isKnownLifecycleState(state) {
		return false
	}

	c.lifecycle.mu.Lock()
	defer c.lifecycle.mu.Unlock()
	c.shell.Runtime.State = state
	if c.lifecycle.state.CommandID != "" &&
		(payload.CommandID == "" || payload.CommandID == c.lifecycle.state.CommandID) {
		c.lifecycle.state.ObservedState = state
	}
	c.shell.Controls = activatedLifecycleControls(state, c.lifecycle.state.Pending)
	return true
}

func activatedLifecycleControls(state string, pending bool) []LifecycleControlView {
	state = strings.ToLower(strings.TrimSpace(state))
	controls := ReservedLifecycleControls(state)
	for i := range controls {
		switch {
		case pending:
			controls[i].Enabled = false
			controls[i].Reason = "A lifecycle command is pending."
		case controls[i].Eligible:
			controls[i].Enabled = true
			controls[i].Reason = ""
		case state == "":
			controls[i].Enabled = false
			controls[i].Reason = "Authoritative runtime state is not available."
		default:
			controls[i].Enabled = false
			controls[i].Reason = "Unavailable in the current runtime state."
		}
	}
	return controls
}

func lifecycleCommandKind(action LifecycleAction) (appruntime.CommandKind, bool) {
	switch action {
	case LifecycleStart:
		return appruntime.CommandStart, true
	case LifecyclePause:
		return appruntime.CommandPause, true
	case LifecycleResume:
		return appruntime.CommandResume, true
	case LifecycleStop:
		return appruntime.CommandStop, true
	case LifecycleCancel:
		return appruntime.CommandCancel, true
	case LifecycleRetry:
		return appruntime.CommandRetry, true
	default:
		return "", false
	}
}

func lifecycleCommandPayload(action LifecycleAction) (json.RawMessage, error) {
	if action != LifecycleStart {
		return nil, nil
	}
	payload, err := json.Marshal(appruntime.StartCommandPayload{Mode: appruntime.StartModeResume})
	if err != nil {
		return nil, lifecycleAppError(
			appruntime.ErrorCodeInvalidArgument,
			appruntime.ErrorCategoryValidation,
			"The request is invalid.",
			false,
		)
	}
	return payload, nil
}

func isKnownLifecycleState(state string) bool {
	switch appruntime.DesktopLifecycleState(strings.ToLower(strings.TrimSpace(state))) {
	case appruntime.LifecycleReady,
		appruntime.LifecycleStarting,
		appruntime.LifecycleRunning,
		appruntime.LifecyclePausing,
		appruntime.LifecyclePaused,
		appruntime.LifecycleResuming,
		appruntime.LifecycleStopping,
		appruntime.LifecycleStopped,
		appruntime.LifecycleCancelling,
		appruntime.LifecycleCancelled,
		appruntime.LifecycleFailed,
		appruntime.LifecycleRecovering,
		appruntime.LifecycleCompleted:
		return true
	default:
		return false
	}
}

func lifecycleAppError(code appruntime.ErrorCode, category appruntime.ErrorCategory, message string, retryable bool) *appruntime.AppError {
	return &appruntime.AppError{
		Code:      code,
		Category:  category,
		Message:   message,
		Retryable: retryable,
	}
}
