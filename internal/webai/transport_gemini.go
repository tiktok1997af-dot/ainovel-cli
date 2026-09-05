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
	Session           *SessionManager
	ResponseTimeout   time.Duration
	PollInterval      time.Duration
	StableWindow      time.Duration
	PreflightRetries  int
	CaptureReconnects int

	adapter          sites.InteractionAdapter
	evaluatorFactory func(context.Context, SessionSnapshot, sites.Adapter) (interactionEvaluator, error)
}

// GeminiWebTransport is the W3 browser transport used by WebChatModel. It does
// not call a Gemini/Google AI HTTP API; all prompt/response work happens through
// the logged-in visible web page over loopback Chrome DevTools.
type GeminiWebTransport struct {
	session           *SessionManager
	adapter           sites.InteractionAdapter
	responseTimeout   time.Duration
	pollInterval      time.Duration
	stableWindow      time.Duration
	preflightRetries  int
	captureReconnects int
	evaluatorFactory  func(context.Context, SessionSnapshot, sites.Adapter) (interactionEvaluator, error)
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
	factory := cfg.evaluatorFactory
	if factory == nil {
		factory = openInteractionEvaluator
	}
	return &GeminiWebTransport{
		session:           cfg.Session,
		adapter:           adapter,
		responseTimeout:   responseTimeout,
		pollInterval:      pollInterval,
		stableWindow:      stableWindow,
		preflightRetries:  preflightRetries,
		captureReconnects: captureReconnects,
		evaluatorFactory:  factory,
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
	defer func() { _ = evaluator.Close() }()

	baseline, err := t.adapter.Conversation(ctx, evaluator)
	if err != nil {
		return "", readinessTransportError("read Gemini baseline", err)
	}
	if baseline.Truncated {
		return "", protocolError("read Gemini baseline", fmt.Errorf("existing response exceeds capture limit"))
	}
	if err := t.adapter.Submit(ctx, evaluator, prompt); err != nil {
		return "", &Error{Kind: ErrorTransport, Op: "submit Gemini web prompt", Cause: err, Retry: false}
	}
	if err := t.session.beginBusy("Gemini web prompt submitted"); err != nil {
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

func (t *GeminiWebTransport) ensureReady(ctx context.Context) (SessionSnapshot, error) {
	var lastErr error
	for attempt := 0; attempt <= t.preflightRetries; attempt++ {
		snap := t.session.Snapshot()
		if snap.State == SessionReady {
			return snap, nil
		}
		if snap.State == SessionAuthRequired {
			return snap, &Error{Kind: ErrorAuthRequired, Op: "prepare Gemini web session", Cause: fmt.Errorf("manual web login is required")}
		}
		refreshed, err := t.session.Refresh(ctx)
		if err == nil && refreshed.State == SessionReady {
			return refreshed, nil
		}
		if refreshed.State == SessionAuthRequired {
			return refreshed, &Error{Kind: ErrorAuthRequired, Op: "prepare Gemini web session", Cause: fmt.Errorf("manual web login is required")}
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
			clicked := t.bestEffortCancel(*evaluator)
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
