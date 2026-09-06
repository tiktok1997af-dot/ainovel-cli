package webai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/voocel/ainovel-cli/internal/webai/sites"
)

type noopInteractionEvaluator struct{}

func (noopInteractionEvaluator) Eval(context.Context, string) (json.RawMessage, error) {
	return json.RawMessage(`null`), nil
}
func (noopInteractionEvaluator) Close() error { return nil }

type readinessProbeFunc func(context.Context, SessionSnapshot) (ReadinessResult, error)

func (f readinessProbeFunc) Probe(ctx context.Context, snap SessionSnapshot) (ReadinessResult, error) {
	return f(ctx, snap)
}

type fakeInteractionAdapter struct {
	mu        sync.Mutex
	snapshots []sites.ConversationSnapshot
	snapErrs  []error
	submitN   int
	cancelN   int
}

func (f *fakeInteractionAdapter) Name() string           { return "fake" }
func (f *fakeInteractionAdapter) TargetScore(string) int { return 100 }
func (f *fakeInteractionAdapter) Probe(context.Context, sites.Evaluator) (sites.Result, error) {
	return sites.Result{State: sites.ReadinessReady}, nil
}
func (f *fakeInteractionAdapter) Conversation(context.Context, sites.Evaluator) (sites.ConversationSnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.snapErrs) > 0 {
		err := f.snapErrs[0]
		f.snapErrs = f.snapErrs[1:]
		if err != nil {
			return sites.ConversationSnapshot{}, err
		}
	}
	if len(f.snapshots) == 0 {
		return sites.ConversationSnapshot{}, nil
	}
	out := f.snapshots[0]
	if len(f.snapshots) > 1 {
		f.snapshots = f.snapshots[1:]
	}
	return out, nil
}
func (f *fakeInteractionAdapter) Submit(context.Context, sites.Evaluator, string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.submitN++
	return nil
}
func (f *fakeInteractionAdapter) Cancel(context.Context, sites.Evaluator) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cancelN++
	return true, nil
}

func readyTestSession() *SessionManager {
	return &SessionManager{
		process: newFakeBrowserProcess(1234),
		snapshot: SessionSnapshot{
			State:      SessionReady,
			ProfileDir: "test-profile",
			ChangedAt:  time.Now(),
		},
	}
}

func testTransport(t *testing.T, session *SessionManager, adapter *fakeInteractionAdapter) *GeminiWebTransport {
	t.Helper()
	transport, err := NewGeminiWebTransport(GeminiWebTransportConfig{
		Session:               session,
		ResponseTimeout:       100 * time.Millisecond,
		PollInterval:          time.Millisecond,
		StableWindow:          2 * time.Millisecond,
		PreflightRetries:      1,
		CaptureReconnects:     1,
		AuthRequiredGrace:     8 * time.Millisecond,
		ReadinessPollInterval: time.Millisecond,
		adapter:               adapter,
		evaluatorFactory: func(context.Context, SessionSnapshot, sites.Adapter) (interactionEvaluator, error) {
			return noopInteractionEvaluator{}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return transport
}

func TestGeminiWebTransportCapturesStableFinalResponse(t *testing.T) {
	adapter := &fakeInteractionAdapter{snapshots: []sites.ConversationSnapshot{
		{ResponseCount: 1, LastResponse: "old"},
		{Busy: true, ResponseCount: 1, LastResponse: "old"},
		{Busy: false, ResponseCount: 2, LastResponse: "<<<AINOVEL_WEB_RESPONSE>>>\n{\"kind\":\"text\",\"text\":\"ok\"}\n<<<END_AINOVEL_WEB_RESPONSE>>>"},
		{Busy: false, ResponseCount: 2, LastResponse: "<<<AINOVEL_WEB_RESPONSE>>>\n{\"kind\":\"text\",\"text\":\"ok\"}\n<<<END_AINOVEL_WEB_RESPONSE>>>"},
		{Busy: false, ResponseCount: 2, LastResponse: "<<<AINOVEL_WEB_RESPONSE>>>\n{\"kind\":\"text\",\"text\":\"ok\"}\n<<<END_AINOVEL_WEB_RESPONSE>>>"},
	}}
	session := readyTestSession()
	transport := testTransport(t, session, adapter)
	raw, err := transport.RoundTrip(context.Background(), "protocol prompt")
	if err != nil {
		t.Fatal(err)
	}
	if raw == "" {
		t.Fatal("empty final response")
	}
	if got := session.Snapshot().State; got != SessionReady {
		t.Fatalf("session state = %s, want READY", got)
	}
	if adapter.submitN != 1 {
		t.Fatalf("submit count = %d, want 1", adapter.submitN)
	}
}

func TestGeminiWebTransportTimeoutCancelsWithoutAutoResubmit(t *testing.T) {
	adapter := &fakeInteractionAdapter{snapshots: []sites.ConversationSnapshot{
		{ResponseCount: 0},
		{Busy: true, ResponseCount: 0},
	}}
	session := readyTestSession()
	transport := testTransport(t, session, adapter)
	transport.responseTimeout = 8 * time.Millisecond
	_, err := transport.RoundTrip(context.Background(), "slow prompt")
	var webErr *Error
	if !errors.As(err, &webErr) || webErr.Kind != ErrorTimeout {
		t.Fatalf("err = %v, want ErrorTimeout", err)
	}
	if webErr.Retryable() {
		t.Fatal("post-submit timeout must not auto-retry")
	}
	if adapter.cancelN == 0 {
		t.Fatal("timeout did not request Stop")
	}
	if adapter.submitN != 1 {
		t.Fatalf("submit count = %d, want 1", adapter.submitN)
	}
}

func TestGeminiWebTransportParentCancellationRequestsStop(t *testing.T) {
	adapter := &fakeInteractionAdapter{snapshots: []sites.ConversationSnapshot{
		{ResponseCount: 0},
		{Busy: true, ResponseCount: 0},
	}}
	session := readyTestSession()
	transport := testTransport(t, session, adapter)
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(5*time.Millisecond, cancel)
	_, err := transport.RoundTrip(ctx, "cancel prompt")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if adapter.cancelN == 0 {
		t.Fatal("cancellation did not request Stop")
	}
}

func TestGeminiWebTransportReconnectsCaptureWithoutResubmit(t *testing.T) {
	adapter := &fakeInteractionAdapter{
		snapshots: []sites.ConversationSnapshot{
			{ResponseCount: 0},
			{Busy: false, ResponseCount: 1, LastResponse: "final"},
			{Busy: false, ResponseCount: 1, LastResponse: "final"},
			{Busy: false, ResponseCount: 1, LastResponse: "final"},
		},
		snapErrs: []error{nil, fmt.Errorf("temporary websocket read failure"), nil, nil, nil},
	}
	session := readyTestSession()
	transport := testTransport(t, session, adapter)
	opens := 0
	transport.evaluatorFactory = func(context.Context, SessionSnapshot, sites.Adapter) (interactionEvaluator, error) {
		opens++
		return noopInteractionEvaluator{}, nil
	}
	got, err := transport.RoundTrip(context.Background(), "one prompt")
	if err != nil {
		t.Fatal(err)
	}
	if got != "final" {
		t.Fatalf("final = %q", got)
	}
	if opens < 2 {
		t.Fatalf("evaluator opens = %d, want reconnect", opens)
	}
	if adapter.submitN != 1 {
		t.Fatalf("submit count = %d, want exactly 1", adapter.submitN)
	}
}

func TestGeminiWebTransportTransientAuthRequiredSettlesReady(t *testing.T) {
	session := readyTestSession()
	session.snapshot.State = SessionAuthRequired
	calls := 0
	session.probe = readinessProbeFunc(func(context.Context, SessionSnapshot) (ReadinessResult, error) {
		calls++
		if calls == 1 {
			return ReadinessResult{State: SessionAuthRequired, Reason: "page still settling"}, nil
		}
		return ReadinessResult{State: SessionReady, Reason: "ready"}, nil
	})
	transport := testTransport(t, session, &fakeInteractionAdapter{})

	snap, err := transport.ensureReady(context.Background())
	if err != nil {
		t.Fatalf("ensureReady() error = %v", err)
	}
	if snap.State != SessionReady {
		t.Fatalf("state = %s, want READY", snap.State)
	}
	if calls < 2 {
		t.Fatalf("probe calls = %d, want at least 2", calls)
	}
}

func TestGeminiWebTransportAuthRequiredDoesNotSubmit(t *testing.T) {
	adapter := &fakeInteractionAdapter{}
	session := readyTestSession()
	session.snapshot.State = SessionAuthRequired
	transport := testTransport(t, session, adapter)
	_, err := transport.RoundTrip(context.Background(), "prompt")
	var webErr *Error
	if !errors.As(err, &webErr) || webErr.Kind != ErrorAuthRequired {
		t.Fatalf("err = %v, want auth required", err)
	}
	if adapter.submitN != 0 {
		t.Fatalf("submit count = %d, want 0", adapter.submitN)
	}
}
