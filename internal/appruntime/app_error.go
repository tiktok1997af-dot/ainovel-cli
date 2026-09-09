package appruntime

import (
	"context"
	"errors"
	"fmt"

	"github.com/voocel/agentcore"
	apperrs "github.com/voocel/ainovel-cli/internal/errs"
	"github.com/voocel/ainovel-cli/internal/webai"
)

type ErrorCategory string
type ErrorCode string

const (
	ErrorCategoryValidation ErrorCategory = "validation"
	ErrorCategoryConflict   ErrorCategory = "conflict"
	ErrorCategoryBrowser    ErrorCategory = "browser"
	ErrorCategoryAI         ErrorCategory = "ai"
	ErrorCategoryStore      ErrorCategory = "store"
	ErrorCategoryRecovery   ErrorCategory = "recovery"
	ErrorCategoryRuntime    ErrorCategory = "runtime"
	ErrorCategoryInternal   ErrorCategory = "internal"
)

const (
	ErrorCodeInvalidArgument    ErrorCode = "invalid_argument"
	ErrorCodeContractMismatch   ErrorCode = "contract_mismatch"
	ErrorCodeRuntimeUnavailable ErrorCode = "runtime_unavailable"
	ErrorCodeRuntimeClosed      ErrorCode = "runtime_closed"
	ErrorCodeRequestCancelled   ErrorCode = "request_cancelled"
	ErrorCodeCommandNotAllowed  ErrorCode = "command_not_allowed"
	ErrorCodeCommandRejected    ErrorCode = "command_rejected"
	ErrorCodeBrowserAuth        ErrorCode = "browser_auth_required"
	ErrorCodeBrowserTransport   ErrorCode = "browser_transport"
	ErrorCodeBrowserTimeout     ErrorCode = "browser_timeout"
	ErrorCodeBrowserProtocol    ErrorCode = "browser_protocol"
	ErrorCodeAIProvider         ErrorCode = "ai_provider"
	ErrorCodeStoreRead          ErrorCode = "store_read"
	ErrorCodeStoreWrite         ErrorCode = "store_write"
	ErrorCodeRecoveryFailed     ErrorCode = "recovery_failed"
	ErrorCodeInternal           ErrorCode = "internal"
)

// AppError is the only error shape that may cross the desktop bridge. Cause is
// retained only inside Go for errors.Is/errors.As and is deliberately excluded
// from JSON so implementation errors, paths and provider internals cannot leak.
type AppError struct {
	Code      ErrorCode     `json:"code"`
	Category  ErrorCategory `json:"category"`
	Message   string        `json:"message"`
	Retryable bool          `json:"retryable"`
	Cause     error         `json:"-"`
}

func (e *AppError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Message != "" {
		return e.Message
	}
	return string(e.Code)
}

func (e *AppError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func contractError(got string) *AppError {
	return &AppError{
		Code:     ErrorCodeContractMismatch,
		Category: ErrorCategoryValidation,
		Message:  fmt.Sprintf("Desktop contract mismatch: got %q, want %q", got, ContractVersion),
	}
}

func validateContractVersion(version string) error {
	if version == "" || version == ContractVersion {
		return nil
	}
	return contractError(version)
}

func recoveryError(err error) *AppError {
	if err == nil {
		return nil
	}
	app := normalizeAppError(err)
	if app.Category == ErrorCategoryInternal || app.Category == ErrorCategoryRuntime {
		app.Code = ErrorCodeRecoveryFailed
		app.Category = ErrorCategoryRecovery
		app.Retryable = true
		app.Message = safeErrorMessage(app.Code)
	}
	return app
}

func safeErrorMessage(code ErrorCode) string {
	switch code {
	case ErrorCodeInvalidArgument:
		return "The request is invalid."
	case ErrorCodeContractMismatch:
		return "The desktop and core contract versions are incompatible."
	case ErrorCodeRuntimeUnavailable:
		return "The application runtime is unavailable."
	case ErrorCodeRuntimeClosed:
		return "The application runtime is closed."
	case ErrorCodeRequestCancelled:
		return "The request was cancelled."
	case ErrorCodeCommandNotAllowed:
		return "This action is not allowed in the current state."
	case ErrorCodeCommandRejected:
		return "The runtime rejected this action."
	case ErrorCodeBrowserAuth:
		return "Browser login is required."
	case ErrorCodeBrowserTransport:
		return "The browser connection failed."
	case ErrorCodeBrowserTimeout:
		return "The browser operation timed out."
	case ErrorCodeBrowserProtocol:
		return "The browser page or protocol is not supported."
	case ErrorCodeAIProvider:
		return "The AI execution failed."
	case ErrorCodeStoreRead:
		return "Project data could not be read."
	case ErrorCodeStoreWrite:
		return "Project data could not be saved."
	case ErrorCodeRecoveryFailed:
		return "The run could not be recovered from durable state."
	default:
		return "An internal runtime error occurred."
	}
}

func normalizeAppError(err error) *AppError {
	if err == nil {
		return nil
	}
	var existing *AppError
	if errors.As(err, &existing) {
		return existing
	}

	app := &AppError{Code: ErrorCodeInternal, Category: ErrorCategoryInternal, Cause: err}
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		app.Code, app.Category = ErrorCodeRequestCancelled, ErrorCategoryRuntime
	case errors.Is(err, ErrNilHost), errors.Is(err, ErrRuntimeUnavailable):
		app.Code, app.Category = ErrorCodeRuntimeUnavailable, ErrorCategoryRuntime
	case errors.Is(err, ErrClosed):
		app.Code, app.Category = ErrorCodeRuntimeClosed, ErrorCategoryRuntime
	case errors.Is(err, ErrInvalidCommand):
		app.Code, app.Category = ErrorCodeInvalidArgument, ErrorCategoryValidation
	case errors.Is(err, ErrCommandNotAllowed):
		app.Code, app.Category = ErrorCodeCommandNotAllowed, ErrorCategoryConflict
	case errors.Is(err, ErrCommandRejected):
		app.Code, app.Category = ErrorCodeCommandRejected, ErrorCategoryRuntime
	case errors.Is(err, apperrs.ErrStoreRead):
		app.Code, app.Category, app.Retryable = ErrorCodeStoreRead, ErrorCategoryStore, true
	case errors.Is(err, apperrs.ErrStoreWrite):
		app.Code, app.Category, app.Retryable = ErrorCodeStoreWrite, ErrorCategoryStore, true
	case errors.Is(err, apperrs.ErrConfig), errors.Is(err, apperrs.ErrToolArgs):
		app.Code, app.Category = ErrorCodeInvalidArgument, ErrorCategoryValidation
	case errors.Is(err, apperrs.ErrToolConflict), errors.Is(err, apperrs.ErrToolPrecondition), errors.Is(err, apperrs.ErrPhaseTransition), errors.Is(err, apperrs.ErrFlowTransition):
		app.Code, app.Category = ErrorCodeCommandNotAllowed, ErrorCategoryConflict
	case errors.Is(err, apperrs.ErrProvider), errors.Is(err, agentcore.ErrProviderAuth), errors.Is(err, agentcore.ErrProviderNetwork), errors.Is(err, agentcore.ErrProviderTimeout):
		app.Code, app.Category = ErrorCodeAIProvider, ErrorCategoryAI
		app.Retryable = !errors.Is(err, agentcore.ErrProviderAuth)
	}

	var webErr *webai.Error
	if errors.As(err, &webErr) {
		app.Category = ErrorCategoryBrowser
		app.Retryable = webErr.Retryable()
		switch webErr.Kind {
		case webai.ErrorAuthRequired, webai.ErrorSecurityChallenge:
			app.Code = ErrorCodeBrowserAuth
		case webai.ErrorTimeout:
			app.Code = ErrorCodeBrowserTimeout
		case webai.ErrorProtocol, webai.ErrorUnsupportedSite:
			app.Code = ErrorCodeBrowserProtocol
		default:
			app.Code = ErrorCodeBrowserTransport
		}
	}
	app.Message = safeErrorMessage(app.Code)
	return app
}
