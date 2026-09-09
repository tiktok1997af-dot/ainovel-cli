package webai

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/voocel/ainovel-cli/internal/webai/sites"
)

type pendingRecoveryTestAdapter struct {
	mu           sync.Mutex
	conversation int
	postRetry    int
	submitN      int
	retryN       int
}

func (a *pendingRecoveryTestAdapter) Name() string           { return "pending-recovery-test" }
func (a *pendingRecoveryTestAdapter) TargetScore(string) int { return 100 }
func (a *pendingRecoveryTestAdapter) Probe(context.Context, sites.Evaluator) (sites.Result, error) {
	return sites.Result{State: sites.ReadinessReady}, nil
}
func (a *pendingRecoveryTestAdapter) Submit(context.Context, sites.Evaluator, string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.submitN++
	return nil
}
func (a *pendingRecoveryTestAdapter) Cancel(context.Context, sites.Evaluator) (bool, error) {
	return true, nil
}
func (a *pendingRecoveryTestAdapter) RetryPendingSubmit(context.Context, sites.Evaluator, string) (bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.retryN++
	return true, nil
}
func (a *pendingRecoveryTestAdapter) Conversation(context.Context, sites.Evaluator) (sites.ConversationSnapshot, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.conversation++
	if a.retryN == 0 {
		if a.conversation == 1 {
			return sites.ConversationSnapshot{
				ResponseCount: 1, UserMessageCount: 1,
				ComposerPresent: true, ComposerEmpty: true, LastResponse: "old",
			}, nil
		}
		return sites.ConversationSnapshot{
			ResponseCount: 1, UserMessageCount: 1,
			ComposerPresent: true, ComposerEmpty: false, ComposerLength: 6, LastResponse: "old",
		}, nil
	}

	a.postRetry++
	if a.postRetry == 1 {
		return sites.ConversationSnapshot{
			Busy: true, ResponseCount: 1, UserMessageCount: 2,
			ComposerPresent: true, ComposerEmpty: true, LastResponse: "old",
		}, nil
	}
	return sites.ConversationSnapshot{
		Busy: false, ResponseCount: 2, UserMessageCount: 2,
		ComposerPresent: true, ComposerEmpty: true, LastResponse: "final",
	}, nil
}

func TestPendingComposerSubmitFailureIsStrict(t *testing.T) {
	exact := &Error{Kind: ErrorTransport, Op: "confirm Gemini web prompt submit", Cause: errors.New(pendingComposerAckDiagnostic)}
	if !pendingComposerSubmitFailure(exact) {
		t.Fatal("exact pending-composer failure must be recoverable")
	}
	joined := &Error{Kind: ErrorTransport, Op: "confirm Gemini web prompt submit", Cause: errors.Join(errors.New("ambiguous submit"), errors.New(pendingComposerAckDiagnostic))}
	if pendingComposerSubmitFailure(joined) {
		t.Fatal("joined/ambiguous submit failure must never enter recovery")
	}
	cleared := &Error{Kind: ErrorTransport, Op: "confirm Gemini web prompt submit", Cause: errors.New("Gemini SEND ACK missing: composer cleared but no new user turn")}
	if pendingComposerSubmitFailure(cleared) {
		t.Fatal("composer-cleared failure must remain fail-closed")
	}
}

func TestAutoRecoveryRecoversStuckComposerWithoutPromptReplay(t *testing.T) {
	adapter := &pendingRecoveryTestAdapter{}
	session := readyTestSession()
	session.probe = readinessProbeFunc(func(context.Context, SessionSnapshot) (ReadinessResult, error) {
		return ReadinessResult{State: SessionReady}, nil
	})

	transport, err := NewGeminiWebTransport(GeminiWebTransportConfig{
		Session:                   session,
		ResponseTimeout:           100 * time.Millisecond,
		PollInterval:              time.Millisecond,
		StableWindow:              2 * time.Millisecond,
		PreflightRetries:          1,
		CaptureReconnects:         1,
		ReadinessPollInterval:     time.Millisecond,
		SubmitConfirmTimeout:      6 * time.Millisecond,
		SubmitConfirmPollInterval: time.Millisecond,
		adapter:                   adapter,
		evaluatorFactory: func(context.Context, SessionSnapshot, sites.Adapter) (interactionEvaluator, error) {
			return noopInteractionEvaluator{}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	watchdog, err := NewAutoRecoveryTransport(AutoRecoveryConfig{
		Inner:               transport,
		Session:             session,
		StallTimeout:        50 * time.Millisecond,
		SoftRetryLimit:      -1,
		BrowserRestartLimit: -1,
		RetryDelay:          time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := watchdog.RoundTrip(context.Background(), "prompt")
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	if got != "final" {
		t.Fatalf("final = %q, want final", got)
	}
	adapter.mu.Lock()
	submitN, retryN := adapter.submitN, adapter.retryN
	adapter.mu.Unlock()
	if submitN != 1 {
		t.Fatalf("initial Submit calls = %d, want exactly 1 (no prompt replay)", submitN)
	}
	if retryN != 1 {
		t.Fatalf("pending recovery clicks = %d, want exactly 1", retryN)
	}
	if state := session.Snapshot().State; state != SessionReady {
		t.Fatalf("session state = %s, want READY", state)
	}
}

func TestAutoRecoveryNeverRecoversAmbiguousSubmitFailure(t *testing.T) {
	cause := errors.Join(fmt.Errorf("renderer click result ambiguous"), errors.New(pendingComposerAckDiagnostic))
	err := &Error{Kind: ErrorTransport, Op: "confirm Gemini web prompt submit", Cause: cause, Retry: false}
	if pendingComposerSubmitFailure(err) {
		t.Fatal("ambiguous submit error must never be considered safe for recovery")
	}
}
