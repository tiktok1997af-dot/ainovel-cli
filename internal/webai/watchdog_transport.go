package webai

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

const (
	defaultWatchdogStallTimeout        = 90 * time.Second
	defaultWatchdogSoftRetryLimit      = 2
	defaultWatchdogBrowserRestartLimit = 2
	defaultWatchdogRetryDelay          = 3 * time.Second
	defaultWatchdogRestartDelay        = 750 * time.Millisecond
)

// AutoRecoveryConfig controls bounded recovery around one browser-backed model
// round trip. The watchdog only replays a prompt while it is still inside the
// transport boundary, before a parsed response/tool call can reach local Tools
// or Store side effects.
type AutoRecoveryConfig struct {
	Inner               Transport
	Session             *SessionManager
	StallTimeout        time.Duration
	SoftRetryLimit      int
	BrowserRestartLimit int
	RetryDelay          time.Duration
	RestartDelay        time.Duration

	// restart is test-only injection. Production uses restartManagedSession.
	restart func(context.Context) error
}

// AutoRecoveryTransport adds explicit Gemini UI-error detection, a bounded
// progress-aware stall watchdog, same-session retries, managed Chrome restart,
// and safe prompt replay. It never restarts the ainovel-cli process itself.
type AutoRecoveryTransport struct {
	inner               Transport
	session             *SessionManager
	stallTimeout        time.Duration
	softRetryLimit      int
	browserRestartLimit int
	retryDelay          time.Duration
	restartDelay        time.Duration
	restart             func(context.Context) error
}

var _ Transport = (*AutoRecoveryTransport)(nil)

type recoverySignal struct {
	reason string
	cause  error
}

func (e *recoverySignal) Error() string {
	if e == nil {
		return "webai auto-recovery signal"
	}
	if e.cause == nil {
		return "webai auto-recovery: " + e.reason
	}
	return fmt.Sprintf("webai auto-recovery %s: %v", e.reason, e.cause)
}

func (e *recoverySignal) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func NewAutoRecoveryTransport(cfg AutoRecoveryConfig) (*AutoRecoveryTransport, error) {
	if cfg.Inner == nil {
		return nil, fmt.Errorf("webai: auto-recovery watchdog requires an inner transport")
	}

	stallTimeout := cfg.StallTimeout
	if stallTimeout == 0 {
		stallTimeout = defaultWatchdogStallTimeout
	}
	if stallTimeout < 0 {
		stallTimeout = 0
	}

	softRetries := cfg.SoftRetryLimit
	if softRetries == 0 {
		softRetries = defaultWatchdogSoftRetryLimit
	}
	if softRetries < 0 {
		softRetries = 0
	}

	browserRestarts := cfg.BrowserRestartLimit
	if browserRestarts == 0 {
		browserRestarts = defaultWatchdogBrowserRestartLimit
	}
	if browserRestarts < 0 {
		browserRestarts = 0
	}

	retryDelay := cfg.RetryDelay
	if retryDelay == 0 {
		retryDelay = defaultWatchdogRetryDelay
	}
	if retryDelay < 0 {
		retryDelay = 0
	}

	restartDelay := cfg.RestartDelay
	if restartDelay == 0 {
		restartDelay = defaultWatchdogRestartDelay
	}
	if restartDelay < 0 {
		restartDelay = 0
	}

	w := &AutoRecoveryTransport{
		inner:               cfg.Inner,
		session:             cfg.Session,
		stallTimeout:        stallTimeout,
		softRetryLimit:      softRetries,
		browserRestartLimit: browserRestarts,
		retryDelay:          retryDelay,
		restartDelay:        restartDelay,
		restart:             cfg.restart,
	}
	if w.restart == nil {
		w.restart = w.restartManagedSession
	}
	return w, nil
}

func (w *AutoRecoveryTransport) RoundTrip(ctx context.Context, prompt string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	softUsed := 0
	restartsUsed := 0
	attempt := 0
	var lastErr error

	for {
		attempt++
		raw, err := w.roundTripAttempt(ctx, prompt)
		if err == nil && looksLikeGeminiTransientUIError(raw) {
			err = &recoverySignal{reason: "Gemini explicit transient UI error", cause: fmt.Errorf("%s", summarizeTransientUIError(raw))}
			raw = ""
		}
		if err == nil {
			if attempt > 1 {
				slog.Info("Gemini Web auto-recovery succeeded", "module", "webai", "attempt", attempt, "soft_retries", softUsed, "browser_restarts", restartsUsed)
			}
			return raw, nil
		}
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if !watchdogReplaySafe(err) {
			return "", err
		}
		lastErr = err

		if softUsed < w.softRetryLimit {
			softUsed++
			slog.Warn("Gemini Web watchdog soft retry", "module", "webai", "attempt", attempt, "retry", softUsed, "err", err)
			if waitErr := waitContext(ctx, w.retryDelay); waitErr != nil {
				return "", waitErr
			}
			continue
		}

		if restartsUsed < w.browserRestartLimit {
			restartsUsed++
			slog.Warn("Gemini Web watchdog restarting managed browser", "module", "webai", "attempt", attempt, "restart", restartsUsed, "err", err)
			if restartErr := w.restart(ctx); restartErr != nil {
				lastErr = errors.Join(lastErr, restartErr)
				if restartsUsed < w.browserRestartLimit {
					continue
				}
				break
			}
			continue
		}
		break
	}

	return "", &Error{
		Kind:  ErrorTransport,
		Op:    "auto-recovery watchdog exhausted",
		Cause: lastErr,
		Retry: false,
	}
}

func (w *AutoRecoveryTransport) roundTripAttempt(parent context.Context, prompt string) (string, error) {
	if w.stallTimeout <= 0 {
		return w.inner.RoundTrip(parent, prompt)
	}
	if gemini, ok := w.inner.(*GeminiWebTransport); ok {
		return gemini.roundTripWithIdleWatchdog(parent, prompt, w.stallTimeout)
	}

	// Generic transports cannot expose browser DOM progress. Tests and future
	// non-Gemini adapters therefore fall back to a bounded whole-attempt timeout.
	attemptCtx, cancel := context.WithTimeout(parent, w.stallTimeout)
	defer cancel()

	raw, err := w.inner.RoundTrip(attemptCtx, prompt)
	if errors.Is(attemptCtx.Err(), context.DeadlineExceeded) && parent.Err() == nil {
		return "", &recoverySignal{
			reason: "no final response before watchdog deadline",
			cause:  fmt.Errorf("no completed transport response for %s", w.stallTimeout),
		}
	}
	return raw, err
}

func (w *AutoRecoveryTransport) restartManagedSession(ctx context.Context) error {
	if w.session == nil {
		return fmt.Errorf("managed browser session is unavailable")
	}
	if err := w.session.Stop(); err != nil {
		return fmt.Errorf("stop managed Gemini browser: %w", err)
	}
	if err := waitContext(ctx, w.restartDelay); err != nil {
		return err
	}
	snap, err := w.session.Start(ctx)
	if err != nil {
		return fmt.Errorf("restart managed Gemini browser (%s): %w", snap.State, err)
	}
	if snap.State != SessionReady {
		return fmt.Errorf("restart managed Gemini browser did not reach READY (state=%s)", snap.State)
	}
	return nil
}

func watchdogReplaySafe(err error) bool {
	if err == nil {
		return false
	}
	var signal *recoverySignal
	if errors.As(err, &signal) {
		return true
	}

	// GeminiWebTransport cancels generation before returning ErrorTimeout and
	// returns from the transport boundary before any local tool can be executed.
	// Replaying this timeout is therefore safe with respect to local Store/tool
	// side effects. Ambiguous submit errors remain ErrorTransport and are never
	// replayed here.
	var webErr *Error
	return errors.As(err, &webErr) && webErr.Kind == ErrorTimeout
}

func looksLikeGeminiTransientUIError(raw string) bool {
	text := strings.ToLower(strings.TrimSpace(raw))
	if text == "" || len([]rune(text)) > 700 {
		return false
	}

	checks := []struct {
		primary string
		retry   string
	}{
		{"something went wrong", "try"},
		{"sorry, something went wrong", ""},
		{"đã xảy ra lỗi", "thử lại"},
		{"có lỗi xảy ra", "thử lại"},
		{"出了点问题", "重试"},
		{"出错了", "重试"},
	}
	for _, check := range checks {
		if !strings.Contains(text, check.primary) {
			continue
		}
		if check.retry == "" || strings.Contains(text, check.retry) {
			return true
		}
	}
	return false
}

func summarizeTransientUIError(raw string) string {
	text := strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
	const max = 180
	if len([]rune(text)) <= max {
		return text
	}
	runes := []rune(text)
	return string(runes[:max]) + "…"
}
