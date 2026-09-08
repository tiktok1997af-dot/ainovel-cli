package webai

import (
	"context"
	"testing"
	"time"

	"github.com/voocel/ainovel-cli/internal/webai/sites"
)

func newWatchdogGeminiTransport(t *testing.T, adapter *fakeInteractionAdapter) *GeminiWebTransport {
	t.Helper()
	transport, err := NewGeminiWebTransport(GeminiWebTransportConfig{
		Session:                   readyTestSession(),
		ResponseTimeout:           100 * time.Millisecond,
		PollInterval:              time.Millisecond,
		StableWindow:              2 * time.Millisecond,
		PreflightRetries:          1,
		CaptureReconnects:         1,
		AuthRequiredGrace:         8 * time.Millisecond,
		ReadinessPollInterval:     time.Millisecond,
		SubmitConfirmTimeout:      8 * time.Millisecond,
		SubmitConfirmPollInterval: time.Millisecond,
		adapter:                   adapter,
		evaluatorFactory: func(context.Context, SessionSnapshot, sites.Adapter) (interactionEvaluator, error) {
			return noopInteractionEvaluator{}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return transport
}

func TestGeminiWatchdogTreatsRenderedProgressAsActivity(t *testing.T) {
	adapter := &fakeInteractionAdapter{snapshots: []sites.ConversationSnapshot{
		{ResponseCount: 1, UserMessageCount: 1, LastResponse: "old"},
		{Busy: true, ResponseCount: 1, UserMessageCount: 2, LastResponse: "old"},
		{Busy: true, ResponseCount: 1, UserMessageCount: 2, LastResponse: "part 1"},
		{Busy: true, ResponseCount: 1, UserMessageCount: 2, LastResponse: "part 2"},
		{Busy: false, ResponseCount: 2, UserMessageCount: 2, LastResponse: "final"},
		{Busy: false, ResponseCount: 2, UserMessageCount: 2, LastResponse: "final"},
		{Busy: false, ResponseCount: 2, UserMessageCount: 2, LastResponse: "final"},
	}}
	inner := newWatchdogGeminiTransport(t, adapter)
	watchdog, err := NewAutoRecoveryTransport(AutoRecoveryConfig{
		Inner:               inner,
		Session:             inner.session,
		StallTimeout:        5 * time.Millisecond,
		SoftRetryLimit:      -1,
		BrowserRestartLimit: -1,
		RetryDelay:          -1,
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := watchdog.RoundTrip(context.Background(), "prompt")
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	if got != "final" {
		t.Fatalf("response = %q, want final", got)
	}
	if adapter.cancelN != 0 {
		t.Fatalf("cancel count = %d, want 0", adapter.cancelN)
	}
}

func TestGeminiWatchdogCancelsAfterNoObservableProgress(t *testing.T) {
	adapter := &fakeInteractionAdapter{snapshots: []sites.ConversationSnapshot{
		{ResponseCount: 1, UserMessageCount: 1, LastResponse: "old"},
		{Busy: true, ResponseCount: 1, UserMessageCount: 2, LastResponse: "old"},
		{Busy: true, ResponseCount: 1, UserMessageCount: 2, LastResponse: "old"},
	}}
	inner := newWatchdogGeminiTransport(t, adapter)
	watchdog, err := NewAutoRecoveryTransport(AutoRecoveryConfig{
		Inner:               inner,
		Session:             inner.session,
		StallTimeout:        5 * time.Millisecond,
		SoftRetryLimit:      -1,
		BrowserRestartLimit: -1,
		RetryDelay:          -1,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = watchdog.RoundTrip(context.Background(), "prompt")
	if err == nil {
		t.Fatal("expected watchdog exhaustion")
	}
	if adapter.cancelN == 0 {
		t.Fatal("stalled Gemini generation must be cancelled before replay/exhaustion")
	}
	if state := inner.session.Snapshot().State; state != SessionDegraded {
		t.Fatalf("session state = %s, want DEGRADED", state)
	}
}
