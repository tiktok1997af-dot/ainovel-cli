package webai

import (
	"errors"
	"fmt"
	"time"

	"github.com/voocel/agentcore"
)

// ErrorKind is a stable classification for browser/web transport failures.
type ErrorKind string

const (
	ErrorAuthRequired      ErrorKind = "auth_required"
	ErrorSecurityChallenge ErrorKind = "security_challenge"
	ErrorTransport         ErrorKind = "transport"
	ErrorTimeout           ErrorKind = "timeout"
	ErrorProtocol          ErrorKind = "protocol"
	ErrorUnsupportedSite   ErrorKind = "unsupported_site"
)

// ErrProtocol marks an invalid response envelope or unsupported protocol payload.
var ErrProtocol = errors.New("webai protocol error")

// Error preserves web-specific failure semantics while integrating with the
// retry/error contracts already used by agentcore and ainovel-cli.
type Error struct {
	Kind       ErrorKind
	Op         string
	Cause      error
	Retry      bool
	RetryDelay time.Duration
}

var (
	_ agentcore.RetryableError = (*Error)(nil)
	_ agentcore.RetryHinter    = (*Error)(nil)
)

func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	prefix := "webai"
	if e.Op != "" {
		prefix += " " + e.Op
	}
	if e.Kind != "" {
		prefix += " [" + string(e.Kind) + "]"
	}
	if e.Cause == nil {
		return prefix
	}
	return fmt.Sprintf("%s: %v", prefix, e.Cause)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// Retryable implements agentcore.RetryableError.
func (e *Error) Retryable() bool { return e != nil && e.Retry }

// RetryAfter implements agentcore.RetryHinter.
func (e *Error) RetryAfter() time.Duration {
	if e == nil {
		return 0
	}
	return e.RetryDelay
}

// Is maps web transport failures onto the provider-neutral sentinels already
// understood by host/engine/retry code. This does not mean an HTTP provider is
// involved; it only reuses the existing stable error taxonomy.
func (e *Error) Is(target error) bool {
	if e == nil {
		return false
	}
	switch e.Kind {
	case ErrorAuthRequired, ErrorSecurityChallenge:
		return target == agentcore.ErrProviderAuth
	case ErrorTransport:
		return target == agentcore.ErrProviderNetwork
	case ErrorTimeout:
		return target == agentcore.ErrProviderTimeout
	case ErrorProtocol:
		return target == ErrProtocol
	}
	return false
}

func protocolError(op string, cause error) error {
	return &Error{Kind: ErrorProtocol, Op: op, Cause: cause}
}
