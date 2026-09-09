package appruntime

import "errors"

var (
	// ErrNilHost protects the single-core invariant at construction time.
	ErrNilHost = errors.New("appruntime: host is nil")
	// ErrRuntimeUnavailable reports an invalid zero-value Runtime.
	ErrRuntimeUnavailable = errors.New("appruntime: runtime unavailable")
	// ErrClosed reports calls made after AppRuntime has been closed.
	ErrClosed = errors.New("appruntime: runtime closed")
	// ErrNotImplemented is used only by G02 skeleton planes whose behavior is
	// deliberately implemented in later G02 steps.
	ErrNotImplemented = errors.New("appruntime: not implemented")
	// ErrInvalidCommand reports an unknown lifecycle command or malformed payload.
	ErrInvalidCommand = errors.New("appruntime: invalid command")
	// ErrCommandNotAllowed reports a valid command used from an invalid state.
	ErrCommandNotAllowed = errors.New("appruntime: command not allowed")
	// ErrCommandRejected reports a command that passed AppRuntime validation but
	// could not be accepted by the underlying Host/runtime facts.
	ErrCommandRejected = errors.New("appruntime: command rejected")
)
