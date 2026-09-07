package webai

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/voocel/ainovel-cli/internal/webai/sites"
)

// GeminiWebTransportConfig controls one WEB-ONLY prompt round trip through the
// visible, already authenticated Gemini browser session.
type GeminiWebTransportConfig struct {
	Session                   *SessionManager
	ResponseTimeout           time.Duration
	PollInterval              time.Duration
	StableWindow              time.Duration
	PreflightRetries          int
	CaptureReconnects         int
	AuthRequiredGrace         time.Duration
	ReadinessPollInterval     time.Duration
	SubmitConfirmTimeout      time.Duration
	SubmitConfirmPollInterval time.Duration

	adapter          sites.InteractionAdapter
	evaluatorFactory func(context.Context, SessionSnapshot, sites.Adapter) (interactionEvaluator, error)
}

// GeminiWebTransport is the W3 browser transport used by WebChatModel. It does
// not call a Gemini/Google AI HTTP API; all prompt/response work happens through
// the logged-in visible web page over loopback Chrome DevTools.
type GeminiWebTransport struct {
	session                   *SessionManager
	adapter                   sites.InteractionAdapter
	responseTimeout           time.Duration
	pollInterval              time.Duration
	stableWindow              time.Duration
	preflightRetries          int
	captureReconnects         int
	authRequiredGrace         time.Duration
	readinessPollInterval     time.Duration
	submitConfirmTimeout      time.Duration
	submitConfirmPollInterval time.Duration
	evaluatorFactory          func(context.Context, SessionSnapshot, sites.Adapter) (interactionEvaluator, error)
}

type interactionEvaluator interface {
	sites.Evaluator
	Close() error
}

var _ Transport = (*GeminiWebTransport)(nil)

func NewGeminiWebTransport(cfg GeminiWebTransportConfig) (*GeminiWebTransport, error) {
	if cfg.Session == nil {
		return nil, fmt.Errorf("webai: Gemini web transport requires a browser session")
	}
	adapter := cfg.adapter
	if adapter == nil {
		adapter = sites.Gemini{}
	}
	responseTimeout := cfg.ResponseTimeout
	if responseTimeout <= 0 {
		responseTimeout = 5 * time.Minute
	}
	pollInterval := cfg.PollInterval
	if pollInterval <= 0 {
		pollInterval = 300 * time.Millisecond
	}
	stableWindow := cfg.StableWindow
	if stableWindow <= 0 {
		stableWindow = 1200 * time.Millisecond
	}
	preflightRetries := cfg.PreflightRetries
	if preflightRetries < 0 {
		preflightRetries = 0
	}
	if preflightRetries == 0 {
		preflightRetries = 2
	}
	captureReconnects := cfg.CaptureReconnects
	if captureReconnects < 0 {
		captureReconnects = 0
	}
	if captureReconnects == 0 {
		captureReconnects = 2
	}
	authRequiredGrace := cfg.AuthRequiredGrace
	if authRequiredGrace <= 0 {
		authRequiredGrace = 15 * time.Second
	}
	readinessPollInterval := cfg.ReadinessPollInterval
	if readinessPollInterval <= 0 {
		readinessPollInterval = 500 * time.Millisecond
	}
	submitConfirmTimeout := cfg.SubmitConfirmTimeout
	if submitConfirmTimeout <= 0 {
		submitConfirmTimeout = 5 * time.Second
	}
	submitConfirmPollInterval := cfg.SubmitConfirmPollInterval
	if submitConfirmPollInterval <= 0 {
		submitConfirmPollInterval = 150 * time.Millisecond
	}
	factory := cfg.evaluatorFactory
	if factory == nil {
		factory = openInteractionEvaluator
	}
	return &GeminiWebTransport{
		session:                   cfg.Session,
		adapter:                   adapter,
		responseTimeout:           responseTimeout,
		pollInterval:              pollInterval,
		stableWindow:              stableWindow,
		preflightRetries:          preflightRetries,
		captureReconnects:         captureReconnects,
		authRequiredGrace:         authRequiredGrace,
		readinessPollInterval:     readinessPollInterval,
		submitConfirmTimeout:      submitConfirmTimeout,
		submitConfirmPollInterval: submitConfirmPollInterval,
		evaluatorFactory:          factory,
	}, nil
}

func (t *GeminiWebTransport) RoundTrip(ctx context.Context, prompt string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if strings.TrimSpace(prompt) == "" {
		return "", protocolError("Gemini web round trip", fmt.Errorf("prompt is empty"))
	}

	snap, err := t.ensureReady(ctx)
	if err != nil {
		return "", err
	}
	evaluator, err := t.openWithRetry(ctx, snap)
	if err != nil {
		return "", err
	}
	defer func() {
		if evaluator != nil {
			_ = evaluator.Close()
		}
	}()

	baseline, err := t.adapter.Conversation(ctx, evaluator)
	if err != nil {
		return "", readinessTransportError("read Gemini baseline", err)
	}
	if baseline.Truncated {
		return "", protocolError("read Gemini baseline", fmt.Errorf("existing response exceeds capture limit"))
	}
	if baseline.Busy {
		return "", readinessTransportError("read Gemini baseline", fmt.Errorf("Gemini conversation is still busy"))
	}

	// A successful DOM click is not delivery acknowledgement. Gemini can accept
	// text into the composer while the send action itself is dropped or stalls.
	// Execute the side effect at most once, then require an independent read-only
	// SEND ACK before entering BUSY/response capture. This prevents the old failure
	// mode where an unsubmitted prompt waited for the full response timeout.
	submitErr := t.adapter.Submit(ctx, evaluator, prompt)
	confirmed, confirmErr := t.confirmSubmit(ctx, &evaluator, baseline, submitErr != nil)
	if !confirmed {
		t.session.finishBusy(SessionDegraded, "Gemini SEND ACK missing; prompt delivery is unconfirmed")
		cause := confirmErr
		if cause == nil {
			cause = fmt.Errorf("Gemini UI did not acknowledge the submitted prompt")
		}
		if submitErr != nil {
			cause = errors.Join(submitErr, cause)
		}
		return "", &Error{Kind: ErrorTransport, Op: "confirm Gemini web prompt submit", Cause: cause, Retry: false}
	}
	if err := t.session.beginBusy("Gemini SEND ACK confirmed; response capture started"); err != nil {
		return "", err
	}

	opCtx, cancel := context.WithTimeout(ctx, t.responseTimeout)
	defer cancel()
	final, err := t.captureFinal(opCtx, ctx, &evaluator, baseline)
	if err != nil {
		return "", err
	}
	t.session.finishBusy(SessionReady, "Gemini web final response captured")
	return final, nil
}

// confirmSubmit performs only read-only DOM observations after the single submit
// attempt. It never types, presses Enter, clicks Send, or re-submits. A SEND ACK
// is accepted only when Gemini exposes a new user turn, transitions from idle to
// BUSY, or produces a new assistant response. Composer-empty by itself is kept
// only as sanitized diagnostic evidence because clearing text alone does not
// prove that Gemini accepted the request.
func (t *GeminiWebTransport) confirmSubmit(
	ctx context.Context,
	evaluator *interactionEvaluator,
	baseline sites.ConversationSnapshot,
	reconnectFirst bool,
) (bool, error) {
	confirmCtx, cancel := context.WithTimeout(ctx, t.submitConfirmTimeout)
	defer cancel()

	baselineText := strings.TrimSpace(baseline.LastResponse)
	var lastErr error
	var lastSnapshot sites.ConversationSnapshot
	haveSnapshot := false
	reconnects := 0

	reconnect := func() {
		if evaluator == nil {
			lastErr = fmt.Errorf("Gemini evaluator pointer is unavailable")
			return
		}
		if *evaluator != nil {
			_ = (*evaluator).Close()
			*evaluator = nil
		}
		next, openErr := t.evaluatorFactory(confirmCtx, t.session.Snapshot(), t.adapter)
		if openErr != nil {
			lastErr = openErr
			return
		}
		*evaluator = next
		reconnects++
	}

	// If the submit call itself returned an ambiguous CDP/renderer error, its
	// execution context may no longer be usable. Reconnect once before observing.
	if reconnectFirst {
		reconnect()
	}

	for {
		if err := confirmCtx.Err(); err != nil {
			if ctx.Err() != nil {
				return false, ctx.Err()
			}
			ackErr := missingSubmitAckError(lastSnapshot, haveSnapshot)
			if lastErr != nil {
				ackErr = errors.Join(ackErr, lastErr)
			}
			return false, ackErr
		}

		if evaluator != nil && *evaluator != nil {
			snapshot, readErr := t.adapter.Conversation(confirmCtx, *evaluator)
			if readErr == nil {
				haveSnapshot = true
				lastSnapshot = snapshot
				if snapshot.Truncated {
					return false, protocolError("confirm Gemini submit", fmt.Errorf("response exceeds capture limit"))
				}
				if submitAcknowledged(baseline, baselineText, snapshot) {
					return true, nil
				}
			} else {
				lastErr = readErr
				if reconnects < t.captureReconnects {
					reconnect()
				}
			}
		} else if reconnects < t.captureReconnects {
			reconnect()
		}

		if err := waitContext(confirmCtx, t.submitConfirmPollInterval); err != nil {
			if ctx.Err() != nil {
				return false, ctx.Err()
			}
			ackErr := missingSubmitAckError(lastSnapshot, haveSnapshot)
			if lastErr != nil {
				ackErr = errors.Join(ackErr, lastErr)
			}
			return false, ackErr
		}
	}
}

func submitAcknowledged(baseline sites.ConversationSnapshot, baselineText string, snapshot sites.ConversationSnapshot) bool {
	if snapshot.UserMessageCount > baseline.UserMessageCount {
		return true
	}
	if !baseline.Busy && snapshot.Busy {
		return true
	}
	if snapshot.ResponseCount > baseline.ResponseCount {
		return true
	}
	text := strings.TrimSpace(snapshot.LastResponse)
	return text != "" && text != baselineText
}

func missingSubmitAckError(snapshot sites.ConversationSnapshot, haveSnapshot bool) error {
	if !haveSnapshot {
		return fmt.Errorf("Gemini SEND ACK was not observed before the bounded confirmation deadline")
	}
	switch {
	case snapshot.ComposerPresent && !snapshot.ComposerEmpty:
		return fmt.Errorf("Gemini SEND ACK missing: prompt remained in the composer")
	case snapshot.ComposerPresent && snapshot.ComposerEmpty:
		return fmt.Errorf("Gemini SEND ACK missing: composer cleared but no new user turn, BUSY transition, or response was observed")
	default:
		return fmt.Errorf("Gemini SEND ACK missing: no new user turn, BUSY transition, or response was observed")
	}
}

func (t *GeminiWebTransport) ensureReady(ctx context.Context) (SessionSnapshot, error) {
	var lastErr error
	for attempt := 0; attempt <= t.preflightRetries; attempt++ {
		snap := t.session.Snapshot()
		if snap.State == SessionReady {
			return snap, nil
		}
		if snap.State == SessionAuthRequired {
			return t.waitAuthRequiredGrace(ctx, snap)
		}
		refreshed, err := t.session.Refresh(ctx)
		if err == nil && refreshed.State == SessionReady {
			return refreshed, nil
		}
		if refreshed.State == SessionAuthRequired {
			return t.waitAuthRequiredGrace(ctx, refreshed)
		}
		lastErr = err
		if attempt < t.preflightRetries {
			if err := waitContext(ctx, 250*time.Millisecond); err != nil {
				return t.session.Snapshot(), err
			}
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("Gemini web session is not READY (%s)", t.session.Snapshot().State)
	}
	return t.session.Snapshot(), &Error{Kind: ErrorTransport, Op: "prepare Gemini web session", Cause: lastErr, Retry: true, RetryDelay: 500 * time.Millisecond}
}

func (t *GeminiWebTransport) waitAuthRequiredGrace(ctx context.Context, initial SessionSnapshot) (SessionSnapshot, error) {
	deadline := time.Now().Add(t.authRequiredGrace)
	last := initial
	var lastErr error

	for {
		if err := ctx.Err(); err != nil {
			return last, err
		}
		if last.State == SessionReady {
			return last, nil
		}
		if time.Now().After(deadline) {
			if last.State == SessionAuthRequired {
				return last, &Error{Kind: ErrorAuthRequired, Op: "prepare Gemini web session", Cause: fmt.Errorf("manual web login is required")}
			}
			if lastErr == nil {
				lastErr = fmt.Errorf("Gemini web session did not become READY during auth grace (%s)", last.State)
			}
			return last, &Error{Kind: ErrorTransport, Op: "prepare Gemini web session", Cause: lastErr, Retry: true, RetryDelay: 500 * time.Millisecond}
		}

		if err := waitContext(ctx, t.readinessPollInterval); err != nil {
			return last, err
		}
		refreshed, err := t.session.Refresh(ctx)
		last = refreshed
		if err != nil {
			lastErr = err
		}
		if refreshed.State == SessionReady {
			return refreshed, nil
		}
		if refreshed.State == SessionFailed || refreshed.State == SessionStopped {
			if err == nil {
				err = fmt.Errorf("Gemini web session entered %s while waiting for readiness", refreshed.State)
			}
			return refreshed, &Error{Kind: ErrorTransport, Op: "prepare Gemini web session", Cause: err, Retry: true, RetryDelay: 500 * time.Millisecond}
		}
	}
}

func (t *GeminiWebTransport) openWithRetry(ctx context.Context, snap SessionSnapshot) (interactionEvaluator, error) {
	var lastErr error
	for attempt := 0; attempt <= t.preflightRetries; attempt++ {
		evaluator, err := t.evaluatorFactory(ctx, snap, t.adapter)
		if err == nil {
			return evaluator, nil
		}
		lastErr = err
		if attempt < t.preflightRetries {
			if err := waitContext(ctx, 200*time.Millisecond); err != nil {
				return nil, err
			}
		}
	}
	return nil, &Error{Kind: ErrorTransport, Op: "connect Gemini web tab", Cause: lastErr, Retry: true, RetryDelay: 500 * time.Millisecond}
}

func (t *GeminiWebTransport) captureFinal(
	opCtx context.Context,
	parentCtx context.Context,
	evaluator *interactionEvaluator,
	baseline sites.ConversationSnapshot,
) (string, error) {
	ticker := time.NewTicker(t.pollInterval)
	defer ticker.Stop()

	var candidate string
	var stableSince time.Time
	reconnects := 0
	for {
		select {
		case <-opCtx.Done():
			var current interactionEvaluator
			if evaluator != nil {
				current = *evaluator
			}
			clicked := t.bestEffortCancel(current)
			reason := "Gemini web request ended; manual readiness refresh required"
			if clicked {
				reason = "Gemini web Stop requested; readiness refresh required"
			}
			t.session.finishBusy(SessionDegraded, reason)
			if parentCtx.Err() != nil {
				return "", parentCtx.Err()
			}
			return "", &Error{Kind: ErrorTimeout, Op: "wait Gemini web response", Cause: opCtx.Err(), Retry: false}
		case <-ticker.C:
			if evaluator == nil || *evaluator == nil {
				t.session.finishBusy(SessionDegraded, "lost Gemini web response capture")
				return "", &Error{Kind: ErrorTransport, Op: "capture Gemini web response", Cause: fmt.Errorf("Gemini evaluator is unavailable"), Retry: false}
			}
			snapshot, err := t.adapter.Conversation(opCtx, *evaluator)
			if err != nil {
				if reconnects < t.captureReconnects {
					_ = (*evaluator).Close()
					next, openErr := t.evaluatorFactory(opCtx, t.session.Snapshot(), t.adapter)
					if openErr == nil {
						*evaluator = next
						reconnects++
						continue
					}
					err = errors.Join(err, openErr)
				}
				t.session.finishBusy(SessionDegraded, "lost Gemini web response capture")
				return "", &Error{Kind: ErrorTransport, Op: "capture Gemini web response", Cause: err, Retry: false}
			}
			if snapshot.Truncated {
				t.bestEffortCancel(*evaluator)
				t.session.finishBusy(SessionDegraded, "Gemini web response exceeded capture limit")
				return "", protocolError("capture Gemini web response", fmt.Errorf("final response exceeds capture limit"))
			}

			text := strings.TrimSpace(snapshot.LastResponse)
			changed := snapshot.ResponseCount > baseline.ResponseCount || (text != "" && text != strings.TrimSpace(baseline.LastResponse))
			if !changed || text == "" || snapshot.Busy {
				candidate = ""
				stableSince = time.Time{}
				continue
			}
			if text != candidate {
				candidate = text
				stableSince = time.Now()
				continue
			}
			if !stableSince.IsZero() && time.Since(stableSince) >= t.stableWindow {
				return candidate, nil
			}
		}
	}
}

func (t *GeminiWebTransport) bestEffortCancel(evaluator interactionEvaluator) bool {
	if evaluator == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	clicked, _ := t.adapter.Cancel(ctx, evaluator)
	return clicked
}

func openInteractionEvaluator(ctx context.Context, session SessionSnapshot, adapter sites.Adapter) (interactionEvaluator, error) {
	profileDir := strings.TrimSpace(session.ProfileDir)
	if profileDir == "" {
		return nil, fmt.Errorf("browser profile directory is required")
	}
	port, err := waitForDevToolsPort(ctx, profileDir, 6*time.Second, 100*time.Millisecond)
	if err != nil {
		return nil, err
	}
	targets, err := listDevToolsTargets(ctx, &http.Client{Timeout: 3 * time.Second}, port)
	if err != nil {
		return nil, err
	}
	target, err := selectDevToolsTarget(targets, adapter)
	if err != nil {
		return nil, err
	}
	wsURL, err := safeDevToolsWebSocketURL(target.WebSocketDebuggerURL, port)
	if err != nil {
		return nil, err
	}
	return newCDPEvaluator(ctx, &websocket.Dialer{HandshakeTimeout: 3 * time.Second}, wsURL)
}

func (m *SessionManager) beginBusy(reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.process == nil || m.snapshot.State != SessionReady {
		return fmt.Errorf("webai: cannot begin web request while session is %s", m.snapshot.State)
	}
	m.transitionLocked(SessionBusy, reason)
	return nil
}

func (m *SessionManager) finishBusy(state SessionState, reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.snapshot.State == SessionStopped || m.snapshot.State == SessionFailed {
		return
	}
	m.transitionLocked(state, reason)
}

func waitContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
