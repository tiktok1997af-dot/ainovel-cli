package desktopui

import (
	"context"
	"errors"
	"strings"

	"github.com/voocel/ainovel-cli/internal/appruntime"
)

// ModelPickerOption is a desktop-safe provider choice. D05 intentionally picks
// the Web provider (Gemini or ChatGPT) and pins that provider's currently
// observed model; marketing-model switching remains owned by the visible web
// UI until a safe activation contract exists.
type ModelPickerOption struct {
	Provider appruntime.WebAIProvider
	Label    string
	Selected bool
}

type ModelSelectionState struct {
	Selected appruntime.WebAIProvider
	Options  []ModelPickerOption
}

func NewModelSelectionState() ModelSelectionState {
	return ModelSelectionState{Options: []ModelPickerOption{
		{Provider: appruntime.ProviderGeminiWeb, Label: "Gemini"},
		{Provider: appruntime.ProviderChatGPTWeb, Label: "ChatGPT"},
	}}
}

func (s ModelSelectionState) clone() ModelSelectionState {
	out := s
	out.Options = append([]ModelPickerOption(nil), s.Options...)
	return out
}

func (c *Controller) ModelSelection() ModelSelectionState {
	if c == nil {
		return NewModelSelectionState()
	}
	return c.model.clone()
}

// SelectModelProvider owns only desktop selection state. It never starts a
// browser or mutates core state. The authoritative readiness/model gate runs at
// AppRuntime START so stale UI state cannot bypass it.
func (c *Controller) SelectModelProvider(provider appruntime.WebAIProvider) error {
	if c == nil {
		return lifecycleAppError(appruntime.ErrorCodeRuntimeUnavailable, appruntime.ErrorCategoryRuntime, "The application runtime is unavailable.", true)
	}
	if err := provider.Validate(); err != nil {
		return lifecycleAppError(appruntime.ErrorCodeInvalidArgument, appruntime.ErrorCategoryValidation, "Select Gemini or ChatGPT before starting.", false)
	}
	c.model.Selected = provider
	for i := range c.model.Options {
		c.model.Options[i].Selected = c.model.Options[i].Provider == provider
	}
	return nil
}

// ExecuteSelectedStart is the D05 START wiring. The selected provider is sent
// through CommandRequest.Resource, a field already present in desktop contract
// v1, leaving the frozen StartCommandPayload unchanged.
func (c *Controller) ExecuteSelectedStart(ctx context.Context) (appruntime.CommandResult, error) {
	if c == nil || c.runtime == nil {
		return appruntime.CommandResult{}, ErrNilRuntime
	}
	provider := c.model.Selected
	if err := provider.Validate(); err != nil {
		return appruntime.CommandResult{}, lifecycleAppError(
			appruntime.ErrorCodeMutationPrecondition,
			appruntime.ErrorCategoryValidation,
			"Select Gemini or ChatGPT before starting.",
			false,
		)
	}
	if !c.shell.HasSnapshot || c.sub == nil {
		return appruntime.CommandResult{}, lifecycleAppError(
			appruntime.ErrorCodeRuntimeUnavailable,
			appruntime.ErrorCategoryRuntime,
			"The application runtime is unavailable.",
			true,
		)
	}

	current := strings.ToLower(strings.TrimSpace(c.shell.Runtime.State))
	if !allowsLifecyclePresentation(string(LifecycleStart), current) {
		return appruntime.CommandResult{}, lifecycleAppError(
			appruntime.ErrorCodeCommandNotAllowed,
			appruntime.ErrorCategoryConflict,
			"This action is not allowed in the current state.",
			false,
		)
	}

	commandID, err := c.beginLifecycleCommand(LifecycleStart, current)
	if err != nil {
		return appruntime.CommandResult{}, err
	}
	payload, err := lifecycleCommandPayload(LifecycleStart)
	if err != nil {
		c.finishLifecycleFailure(commandID, "", viewErrorFromError(err))
		return appruntime.CommandResult{}, err
	}
	request := appruntime.CommandRequest{
		ContractVersion: appruntime.ContractVersion,
		ID:              commandID,
		Kind:            appruntime.CommandStart,
		Resource:        string(provider),
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
	if result.ContractVersion != appruntime.ContractVersion || result.CommandID != commandID || !result.Accepted {
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
