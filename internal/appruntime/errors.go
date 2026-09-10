package appruntime

import "errors"

var (
	// ErrNilHost protects the single-core invariant at construction time.
	ErrNilHost = errors.New("appruntime: host is nil")
	// ErrRuntimeUnavailable reports an invalid zero-value Runtime.
	ErrRuntimeUnavailable = errors.New("appruntime: runtime unavailable")
	// ErrClosed reports calls made after AppRuntime has been closed.
	ErrClosed = errors.New("appruntime: runtime closed")
	// ErrNotImplemented is used only by staged desktop planes whose behavior is
	// deliberately implemented in a later locked roadmap step.
	ErrNotImplemented = errors.New("appruntime: not implemented")
	// ErrInvalidQuery reports an unknown read query or malformed typed payload.
	ErrInvalidQuery = errors.New("appruntime: invalid query")
	// ErrInvalidCommand reports an unknown lifecycle command or malformed payload.
	ErrInvalidCommand = errors.New("appruntime: invalid command")
	// ErrCommandNotAllowed reports a valid command used from an invalid state.
	ErrCommandNotAllowed = errors.New("appruntime: command not allowed")
	// ErrCommandRejected reports a command that passed AppRuntime validation but
	// could not be accepted by the underlying Host/runtime facts.
	ErrCommandRejected = errors.New("appruntime: command rejected")

	// G03 mutation-plane sentinels keep desktop conflict semantics stable without
	// exposing Store/domain implementation errors across AppRuntime.
	ErrInvalidMutation        = errors.New("appruntime: invalid mutation")
	ErrUnsupportedMutation    = errors.New("appruntime: unsupported mutation")
	ErrMutationTargetNotFound = errors.New("appruntime: mutation target not found")
	ErrMutationPrecondition   = errors.New("appruntime: mutation precondition conflict")
	ErrMutationStale          = errors.New("appruntime: stale mutation")
)
