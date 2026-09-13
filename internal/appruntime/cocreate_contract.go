package appruntime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

const (
	// MaxCoCreatePayloadBytes bounds one desktop CoCreate command envelope before
	// any Host/WebAI work is allowed to start.
	MaxCoCreatePayloadBytes = 1 << 20
	// MaxCoCreateHistoryItems bounds presentation/session continuity carried by
	// the desktop. CoCreate history is transient transport state, not story truth.
	MaxCoCreateHistoryItems = 24
	// MaxCoCreateMessageBytes bounds one user or assistant presentation message.
	MaxCoCreateMessageBytes = 16 << 10
	// MaxCoCreateDraftBytes bounds the cumulative creative brief/draft.
	MaxCoCreateDraftBytes = 32 << 10
	// MaxCoCreateSuggestions mirrors the Host-supported suggestion set.
	MaxCoCreateSuggestions = 3
	// MaxCoCreateSuggestionBytes bounds one presentation suggestion.
	MaxCoCreateSuggestionBytes = 2 << 10
)

const (
	CommandCoCreateTurn        CommandKind = "cocreate.turn"
	CommandCoCreateStageBegin  CommandKind = "cocreate.stage.begin"
	CommandCoCreateStageFinish CommandKind = "cocreate.stage.finish"
	CommandCoCreateStageCancel CommandKind = "cocreate.stage.cancel"
)

// G08CoCreateCommandKinds returns the exact G08.2 desktop CoCreate command
// vocabulary. Runtime/Host execution is intentionally staged to later G08
// steps; this catalog owns only typed transport and validation authority.
func G08CoCreateCommandKinds() []CommandKind {
	return []CommandKind{
		CommandCoCreateTurn,
		CommandCoCreateStageBegin,
		CommandCoCreateStageFinish,
		CommandCoCreateStageCancel,
	}
}

func isCoCreateCommandKind(kind CommandKind) bool {
	switch kind {
	case CommandCoCreateTurn, CommandCoCreateStageBegin, CommandCoCreateStageFinish, CommandCoCreateStageCancel:
		return true
	default:
		return false
	}
}

type CoCreateMode string

const (
	CoCreateModeColdStart CoCreateMode = "cold_start"
	CoCreateModeStage     CoCreateMode = "stage"
)

type CoCreateRole string

const (
	CoCreateRoleUser      CoCreateRole = "user"
	CoCreateRoleAssistant CoCreateRole = "assistant"
)

// CoCreateHistoryItemDTO carries only sanitized conversation state. Assistant
// history preserves the visible message plus cumulative draft/ready/suggestions
// so a later AppRuntime adapter can reconstruct the Host protocol internally
// without granting the GUI Host raw XML or provider response authority.
type CoCreateHistoryItemDTO struct {
	Role        CoCreateRole `json:"role"`
	Message     string       `json:"message"`
	Draft       string       `json:"draft,omitempty"`
	Ready       bool         `json:"ready,omitempty"`
	Suggestions []string     `json:"suggestions,omitempty"`
}

type CoCreateTurnCommandPayload struct {
	Mode    CoCreateMode             `json:"mode"`
	History []CoCreateHistoryItemDTO `json:"history"`
}

type CoCreateStageFinishCommandPayload struct {
	Draft string `json:"draft"`
}

// CoCreateSessionDTO is presentation metadata only. It does not identify a
// browser conversation, profile, process, persisted story object or scheduler
// resource.
type CoCreateSessionDTO struct {
	Mode         CoCreateMode `json:"mode"`
	HistoryCount int          `json:"history_count"`
	StageActive  bool         `json:"stage_active"`
}

type CoCreateTurnResultDTO struct {
	Session     CoCreateSessionDTO `json:"session"`
	Message     string             `json:"message"`
	Draft       string             `json:"draft,omitempty"`
	Ready       bool               `json:"ready"`
	Suggestions []string           `json:"suggestions,omitempty"`
}

type CoCreateStageStateResultDTO struct {
	Session CoCreateSessionDTO `json:"session"`
}

type coCreateContractRoute struct {
	Kind    CommandKind
	Request any
}

// decodeCoCreateContract validates only the G08.2 typed transport boundary.
// It performs no Host call, no browser allocation and no project mutation.
func decodeCoCreateContract(cmd CommandRequest) (coCreateContractRoute, error) {
	if strings.TrimSpace(cmd.RunID) != "" || strings.TrimSpace(cmd.TaskID) != "" || strings.TrimSpace(cmd.Resource) != "" {
		return coCreateContractRoute{}, invalidCoCreateCommand("CoCreate commands cannot claim run/task/resource authority")
	}

	route := coCreateContractRoute{Kind: cmd.Kind}
	switch cmd.Kind {
	case CommandCoCreateTurn:
		var payload CoCreateTurnCommandPayload
		if err := decodeCoCreatePayload(cmd.Payload, &payload, true); err != nil {
			return coCreateContractRoute{}, err
		}
		if err := normalizeAndValidateCoCreateTurn(&payload); err != nil {
			return coCreateContractRoute{}, err
		}
		route.Request = payload
	case CommandCoCreateStageBegin, CommandCoCreateStageCancel:
		if err := decodeCoCreateEmptyPayload(cmd.Payload); err != nil {
			return coCreateContractRoute{}, err
		}
		route.Request = struct{}{}
	case CommandCoCreateStageFinish:
		var payload CoCreateStageFinishCommandPayload
		if err := decodeCoCreatePayload(cmd.Payload, &payload, true); err != nil {
			return coCreateContractRoute{}, err
		}
		payload.Draft = strings.TrimSpace(payload.Draft)
		if payload.Draft == "" || len([]byte(payload.Draft)) > MaxCoCreateDraftBytes {
			return coCreateContractRoute{}, invalidCoCreateCommand("stage finish draft is invalid")
		}
		route.Request = payload
	default:
		return coCreateContractRoute{}, unsupportedCoCreateCommand()
	}
	return route, nil
}

func decodeCoCreatePayload(payload json.RawMessage, dst any, required bool) error {
	if len(payload) == 0 || string(payload) == "null" {
		if required {
			return invalidCoCreateCommand("CoCreate payload is required")
		}
		return nil
	}
	if len(payload) > MaxCoCreatePayloadBytes || !json.Valid(payload) {
		return invalidCoCreateCommand("CoCreate payload is invalid or too large")
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return invalidCoCreateCommand("CoCreate payload does not match typed contract")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return invalidCoCreateCommand("CoCreate payload contains trailing values")
	}
	return nil
}

func decodeCoCreateEmptyPayload(payload json.RawMessage) error {
	if len(payload) == 0 || string(payload) == "null" {
		return nil
	}
	var empty struct{}
	return decodeCoCreatePayload(payload, &empty, false)
}

func normalizeAndValidateCoCreateTurn(payload *CoCreateTurnCommandPayload) error {
	if payload == nil || !validCoCreateMode(payload.Mode) {
		return invalidCoCreateCommand("CoCreate mode is invalid")
	}
	if len(payload.History) == 0 || len(payload.History) > MaxCoCreateHistoryItems {
		return invalidCoCreateCommand("CoCreate history count is invalid")
	}
	if payload.History[0].Role != CoCreateRoleUser || payload.History[len(payload.History)-1].Role != CoCreateRoleUser {
		return invalidCoCreateCommand("CoCreate history must start and end with user input")
	}

	for i := range payload.History {
		item := &payload.History[i]
		expected := CoCreateRoleUser
		if i%2 == 1 {
			expected = CoCreateRoleAssistant
		}
		if item.Role != expected {
			return invalidCoCreateCommand("CoCreate history roles must alternate user/assistant")
		}
		item.Message = strings.TrimSpace(item.Message)
		if item.Message == "" || len([]byte(item.Message)) > MaxCoCreateMessageBytes {
			return invalidCoCreateCommand("CoCreate history message is invalid")
		}

		switch item.Role {
		case CoCreateRoleUser:
			if strings.TrimSpace(item.Draft) != "" || item.Ready || len(item.Suggestions) != 0 {
				return invalidCoCreateCommand("user history cannot carry assistant result fields")
			}
		case CoCreateRoleAssistant:
			item.Draft = strings.TrimSpace(item.Draft)
			if len([]byte(item.Draft)) > MaxCoCreateDraftBytes {
				return invalidCoCreateCommand("assistant draft exceeds CoCreate bound")
			}
			if err := normalizeAndValidateSuggestions(&item.Suggestions); err != nil {
				return err
			}
		default:
			return invalidCoCreateCommand("CoCreate history role is invalid")
		}
	}
	return nil
}

func validateCoCreateTurnResult(result CoCreateTurnResultDTO) error {
	if !validCoCreateMode(result.Session.Mode) || result.Session.HistoryCount < 0 || result.Session.HistoryCount > MaxCoCreateHistoryItems {
		return invalidCoCreateCommand("CoCreate result session metadata is invalid")
	}
	result.Message = strings.TrimSpace(result.Message)
	if result.Message == "" || len([]byte(result.Message)) > MaxCoCreateMessageBytes {
		return invalidCoCreateCommand("CoCreate result message is invalid")
	}
	result.Draft = strings.TrimSpace(result.Draft)
	if len([]byte(result.Draft)) > MaxCoCreateDraftBytes {
		return invalidCoCreateCommand("CoCreate result draft exceeds bound")
	}
	return normalizeAndValidateSuggestions(&result.Suggestions)
}

func normalizeAndValidateSuggestions(values *[]string) error {
	if values == nil {
		return nil
	}
	if len(*values) > MaxCoCreateSuggestions {
		return invalidCoCreateCommand("too many CoCreate suggestions")
	}
	for i := range *values {
		(*values)[i] = strings.TrimSpace((*values)[i])
		if (*values)[i] == "" || len([]byte((*values)[i])) > MaxCoCreateSuggestionBytes {
			return invalidCoCreateCommand("CoCreate suggestion is invalid")
		}
	}
	return nil
}

func validCoCreateMode(mode CoCreateMode) bool {
	return mode == CoCreateModeColdStart || mode == CoCreateModeStage
}

func invalidCoCreateCommand(_ string) error {
	return &AppError{
		Code:     ErrorCodeInvalidArgument,
		Category: ErrorCategoryValidation,
		Message:  safeErrorMessage(ErrorCodeInvalidArgument),
		Cause:    ErrInvalidCommand,
	}
}

func unsupportedCoCreateCommand() error {
	return &AppError{
		Code:     ErrorCodeUnsupportedOperation,
		Category: ErrorCategoryValidation,
		Message:  safeErrorMessage(ErrorCodeUnsupportedOperation),
		Cause:    fmt.Errorf("%w: unsupported CoCreate command", ErrInvalidCommand),
	}
}
