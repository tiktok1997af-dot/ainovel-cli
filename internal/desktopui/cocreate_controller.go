package desktopui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/voocel/ainovel-cli/internal/appruntime"
)

var ErrCoCreateWorkspaceUnavailable = errors.New("desktopui: CoCreate workspace is unavailable")

func (c *Controller) CoCreate() *CoCreateWorkspaceState {
	if c == nil {
		return nil
	}
	return &c.cocreate
}

// ResetCoCreateColdStart resets presentation-only history. It does not mutate
// project truth and is rejected while a Host-backed stage session is active.
func (c *Controller) ResetCoCreateColdStart() error {
	if c == nil {
		return ErrCoCreateWorkspaceUnavailable
	}
	state := &c.cocreate
	if state.Pending || state.StageActive {
		return c.failCoCreateValidation(appruntime.ErrorCodeCommandNotAllowed, appruntime.ErrorCategoryConflict)
	}
	state.resetConversation(appruntime.CoCreateModeColdStart, false)
	return nil
}

// BeginCoCreateStage opens the Host-backed stage CoCreate occupancy window via
// the already-locked AppRuntime contract. No UI-owned scheduler or browser
// authority is introduced.
func (c *Controller) BeginCoCreateStage(ctx context.Context) (appruntime.CommandResult, error) {
	if c == nil || c.runtime == nil {
		return appruntime.CommandResult{}, ErrNilRuntime
	}
	state := &c.cocreate
	if state.Pending || state.StageActive {
		return appruntime.CommandResult{}, c.failCoCreateValidation(appruntime.ErrorCodeCommandNotAllowed, appruntime.ErrorCategoryConflict)
	}

	commandID := c.beginCoCreateAction(CoCreateActionBeginStage)
	result, err := c.runtime.Dispatch(ctx, appruntime.CommandRequest{
		ContractVersion: appruntime.ContractVersion,
		ID:              commandID,
		Kind:            appruntime.CommandCoCreateStageBegin,
	})
	if err := c.validateCoCreateDispatch(commandID, result, err); err != nil {
		return result, err
	}
	var dto appruntime.CoCreateStageStateResultDTO
	if err := decodeCoCreateResult(result.Data, &dto); err != nil || dto.Session.Mode != appruntime.CoCreateModeStage || !dto.Session.StageActive || dto.Session.HistoryCount != 0 {
		protocolErr := coCreateProtocolError()
		c.failCoCreateAction(commandID, protocolErr)
		return result, protocolErr
	}

	state.resetConversation(appruntime.CoCreateModeStage, true)
	c.completeCoCreateAction(commandID)
	return result, c.Refresh(ctx)
}

// SendCoCreate sends one user turn in the workspace's current mode. The local
// history is appended only after AppRuntime accepts and validates a sanitized
// result, so failed commands never create presentation continuity that Host did
// not observe.
func (c *Controller) SendCoCreate(ctx context.Context, message string) (appruntime.CommandResult, error) {
	if c == nil || c.runtime == nil {
		return appruntime.CommandResult{}, ErrNilRuntime
	}
	state := &c.cocreate
	message = strings.TrimSpace(message)
	if state.Pending || message == "" || len([]byte(message)) > appruntime.MaxCoCreateMessageBytes || !canAppendCoCreateTurn(state.History) {
		return appruntime.CommandResult{}, c.failCoCreateValidation(appruntime.ErrorCodeInvalidArgument, appruntime.ErrorCategoryValidation)
	}

	mode := state.Mode
	switch mode {
	case appruntime.CoCreateModeColdStart:
		if state.StageActive || state.HandoffAccepted {
			return appruntime.CommandResult{}, c.failCoCreateValidation(appruntime.ErrorCodeCommandNotAllowed, appruntime.ErrorCategoryConflict)
		}
	case appruntime.CoCreateModeStage:
		if !state.StageActive {
			return appruntime.CommandResult{}, c.failCoCreateValidation(appruntime.ErrorCodeCommandNotAllowed, appruntime.ErrorCategoryConflict)
		}
	default:
		return appruntime.CommandResult{}, c.failCoCreateValidation(appruntime.ErrorCodeInvalidArgument, appruntime.ErrorCategoryValidation)
	}

	history := cloneCoCreateHistory(state.History)
	history = append(history, appruntime.CoCreateHistoryItemDTO{
		Role:    appruntime.CoCreateRoleUser,
		Message: message,
	})
	payload, err := json.Marshal(appruntime.CoCreateTurnCommandPayload{Mode: mode, History: history})
	if err != nil {
		return appruntime.CommandResult{}, c.failCoCreateProtocol(err)
	}

	commandID := c.beginCoCreateAction(CoCreateActionSend)
	result, dispatchErr := c.runtime.Dispatch(ctx, appruntime.CommandRequest{
		ContractVersion: appruntime.ContractVersion,
		ID:              commandID,
		Kind:            appruntime.CommandCoCreateTurn,
		Payload:         payload,
	})
	if err := c.validateCoCreateDispatch(commandID, result, dispatchErr); err != nil {
		return result, err
	}

	var dto appruntime.CoCreateTurnResultDTO
	if err := decodeCoCreateResult(result.Data, &dto); err != nil || !validCoCreateTurnPresentation(dto, mode, state.StageActive, len(history)+1) {
		protocolErr := coCreateProtocolError()
		c.failCoCreateAction(commandID, protocolErr)
		return result, protocolErr
	}

	history = append(history, appruntime.CoCreateHistoryItemDTO{
		Role:        appruntime.CoCreateRoleAssistant,
		Message:     strings.TrimSpace(dto.Message),
		Draft:       strings.TrimSpace(dto.Draft),
		Ready:       dto.Ready,
		Suggestions: append([]string(nil), dto.Suggestions...),
	})
	state.History = history
	state.Draft = strings.TrimSpace(dto.Draft)
	state.Ready = dto.Ready
	state.Suggestions = append([]string(nil), dto.Suggestions...)
	state.StageActive = dto.Session.StageActive
	c.completeCoCreateAction(commandID)
	return result, nil
}

// FinishCoCreateStage treats the explicit Finish action as acceptance of the
// currently rendered cumulative draft and delegates it to the locked AppRuntime
// stage.finish command.
func (c *Controller) FinishCoCreateStage(ctx context.Context) (appruntime.CommandResult, error) {
	if c == nil || c.runtime == nil {
		return appruntime.CommandResult{}, ErrNilRuntime
	}
	state := &c.cocreate
	draft := strings.TrimSpace(state.Draft)
	if state.Pending || !state.StageActive || state.Mode != appruntime.CoCreateModeStage || draft == "" || len([]byte(draft)) > appruntime.MaxCoCreateDraftBytes {
		return appruntime.CommandResult{}, c.failCoCreateValidation(appruntime.ErrorCodeCommandNotAllowed, appruntime.ErrorCategoryConflict)
	}
	payload, err := json.Marshal(appruntime.CoCreateStageFinishCommandPayload{Draft: draft})
	if err != nil {
		return appruntime.CommandResult{}, c.failCoCreateProtocol(err)
	}

	commandID := c.beginCoCreateAction(CoCreateActionFinish)
	result, dispatchErr := c.runtime.Dispatch(ctx, appruntime.CommandRequest{
		ContractVersion: appruntime.ContractVersion,
		ID:              commandID,
		Kind:            appruntime.CommandCoCreateStageFinish,
		Payload:         payload,
	})
	if err := c.validateCoCreateDispatch(commandID, result, dispatchErr); err != nil {
		return result, err
	}
	if err := validateCoCreateStageTerminalResult(result.Data); err != nil {
		c.failCoCreateAction(commandID, err)
		return result, err
	}
	state.StageActive = false
	c.completeCoCreateAction(commandID)
	return result, c.Refresh(ctx)
}

func (c *Controller) CancelCoCreateStage(ctx context.Context) (appruntime.CommandResult, error) {
	if c == nil || c.runtime == nil {
		return appruntime.CommandResult{}, ErrNilRuntime
	}
	state := &c.cocreate
	if state.Pending || !state.StageActive || state.Mode != appruntime.CoCreateModeStage {
		return appruntime.CommandResult{}, c.failCoCreateValidation(appruntime.ErrorCodeCommandNotAllowed, appruntime.ErrorCategoryConflict)
	}

	commandID := c.beginCoCreateAction(CoCreateActionCancel)
	result, dispatchErr := c.runtime.Dispatch(ctx, appruntime.CommandRequest{
		ContractVersion: appruntime.ContractVersion,
		ID:              commandID,
		Kind:            appruntime.CommandCoCreateStageCancel,
	})
	if err := c.validateCoCreateDispatch(commandID, result, dispatchErr); err != nil {
		return result, err
	}
	if err := validateCoCreateStageTerminalResult(result.Data); err != nil {
		c.failCoCreateAction(commandID, err)
		return result, err
	}
	state.StageActive = false
	c.completeCoCreateAction(commandID)
	return result, c.Refresh(ctx)
}

// StartNewFromCoCreate hands the explicitly accepted cold-start draft to the
// existing lifecycle start(mode=new) command. Model Ready is presentation data;
// the user's Start action is the actual acceptance authority.
func (c *Controller) StartNewFromCoCreate(ctx context.Context) (appruntime.CommandResult, error) {
	if c == nil || c.runtime == nil {
		return appruntime.CommandResult{}, ErrNilRuntime
	}
	state := &c.cocreate
	draft := strings.TrimSpace(state.Draft)
	if state.Pending || state.StageActive || state.Mode != appruntime.CoCreateModeColdStart || state.HandoffAccepted || draft == "" || len([]byte(draft)) > appruntime.MaxCoCreateDraftBytes {
		return appruntime.CommandResult{}, c.failCoCreateValidation(appruntime.ErrorCodeCommandNotAllowed, appruntime.ErrorCategoryConflict)
	}
	payload, err := json.Marshal(appruntime.StartCommandPayload{
		Mode:        appruntime.StartModeNew,
		Requirement: draft,
	})
	if err != nil {
		return appruntime.CommandResult{}, c.failCoCreateProtocol(err)
	}

	commandID := c.beginCoCreateAction(CoCreateActionStartNew)
	result, dispatchErr := c.runtime.Dispatch(ctx, appruntime.CommandRequest{
		ContractVersion: appruntime.ContractVersion,
		ID:              commandID,
		Kind:            appruntime.CommandStart,
		Payload:         payload,
	})
	if err := c.validateCoCreateDispatch(commandID, result, dispatchErr); err != nil {
		return result, err
	}
	state.HandoffAccepted = true
	c.completeCoCreateAction(commandID)
	return result, c.Refresh(ctx)
}

func (c *Controller) beginCoCreateAction(action CoCreateAction) string {
	commandID := c.nextRequestID()
	state := &c.cocreate
	state.Pending = true
	state.PendingAction = action
	state.LastCommandID = commandID
	state.Error = nil
	state.Load = LoadCommandPending
	return commandID
}

func (c *Controller) completeCoCreateAction(commandID string) {
	state := &c.cocreate
	if state.LastCommandID != commandID {
		return
	}
	state.Pending = false
	state.PendingAction = ""
	state.Error = nil
	state.Load = LoadReady
}

func (c *Controller) failCoCreateAction(commandID string, err error) {
	state := &c.cocreate
	if state.LastCommandID != commandID {
		return
	}
	state.Pending = false
	state.PendingAction = ""
	state.Error = viewErrorFromError(err)
	state.Load = loadStateForWorkspaceError(state.Error)
}

func (c *Controller) validateCoCreateDispatch(commandID string, result appruntime.CommandResult, dispatchErr error) error {
	if dispatchErr != nil {
		c.failCoCreateAction(commandID, dispatchErr)
		return dispatchErr
	}
	if result.Error != nil {
		c.failCoCreateAction(commandID, result.Error)
		return result.Error
	}
	if result.ContractVersion != appruntime.ContractVersion || result.CommandID != commandID {
		err := coCreateProtocolError()
		c.failCoCreateAction(commandID, err)
		return err
	}
	if !result.Accepted {
		err := &appruntime.AppError{
			Code:     appruntime.ErrorCodeCommandRejected,
			Category: appruntime.ErrorCategoryRuntime,
			Message:  "The runtime rejected this action.",
		}
		c.failCoCreateAction(commandID, err)
		return err
	}
	return nil
}

func (c *Controller) failCoCreateValidation(code appruntime.ErrorCode, category appruntime.ErrorCategory) error {
	err := &appruntime.AppError{
		Code:     code,
		Category: category,
		Message:  "This CoCreate action is not available in the current state.",
	}
	c.cocreate.Error = viewErrorFromAppError(err)
	c.cocreate.Load = loadStateForWorkspaceError(c.cocreate.Error)
	return err
}

func (c *Controller) failCoCreateProtocol(_ error) error {
	err := coCreateProtocolError()
	c.cocreate.Error = viewErrorFromAppError(err)
	c.cocreate.Load = LoadRuntimeError
	return err
}

func coCreateProtocolError() *appruntime.AppError {
	return &appruntime.AppError{
		Code:     appruntime.ErrorCodeInternal,
		Category: appruntime.ErrorCategoryInternal,
		Message:  "An internal runtime error occurred.",
	}
}

func decodeCoCreateResult(data json.RawMessage, dst any) error {
	if len(data) == 0 || !json.Valid(data) {
		return coCreateProtocolError()
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return coCreateProtocolError()
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return coCreateProtocolError()
	}
	return nil
}

func validCoCreateTurnPresentation(dto appruntime.CoCreateTurnResultDTO, mode appruntime.CoCreateMode, stageActive bool, historyCount int) bool {
	if dto.Session.Mode != mode || dto.Session.StageActive != stageActive || dto.Session.HistoryCount != historyCount ||
		historyCount < 0 || historyCount > appruntime.MaxCoCreateHistoryItems {
		return false
	}
	message := strings.TrimSpace(dto.Message)
	if message == "" || len([]byte(message)) > appruntime.MaxCoCreateMessageBytes {
		return false
	}
	if len([]byte(strings.TrimSpace(dto.Draft))) > appruntime.MaxCoCreateDraftBytes || len(dto.Suggestions) > appruntime.MaxCoCreateSuggestions {
		return false
	}
	for _, suggestion := range dto.Suggestions {
		suggestion = strings.TrimSpace(suggestion)
		if suggestion == "" || len([]byte(suggestion)) > appruntime.MaxCoCreateSuggestionBytes {
			return false
		}
	}
	return true
}

func validateCoCreateStageTerminalResult(data json.RawMessage) error {
	var dto appruntime.CoCreateStageStateResultDTO
	if err := decodeCoCreateResult(data, &dto); err != nil || dto.Session.Mode != appruntime.CoCreateModeStage || dto.Session.StageActive || dto.Session.HistoryCount != 0 {
		return coCreateProtocolError()
	}
	return nil
}
