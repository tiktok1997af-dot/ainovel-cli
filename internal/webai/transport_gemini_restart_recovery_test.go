package webai

import (
	"context"
	"errors"
	"testing"

	"github.com/voocel/ainovel-cli/internal/webai/sites"
)

// W5E-R1 regression: after a restart boundary the first CDP evaluator can be
// stale while the newly launched Chrome target is still settling. A capture
// reconnect must retry opening the evaluator without ever resubmitting the
// already-acknowledged prompt.
func TestGeminiWebTransportCaptureReconnectRetriesTransientOpenFailure(t *testing.T) {
	adapter := &fakeInteractionAdapter{
		snapshots: []sites.ConversationSnapshot{
			{ResponseCount: 0, UserMessageCount: 0, ComposerPresent: true, ComposerEmpty: true},
			{Busy: true, ResponseCount: 0, UserMessageCount: 1, ComposerPresent: true, ComposerEmpty: true},
			{Busy: false, ResponseCount: 1, UserMessageCount: 1, ComposerPresent: true, ComposerEmpty: true, LastResponse: "final"},
			{Busy: false, ResponseCount: 1, UserMessageCount: 1, ComposerPresent: true, ComposerEmpty: true, LastResponse: "final"},
			{Busy: false, ResponseCount: 1, UserMessageCount: 1, ComposerPresent: true, ComposerEmpty: true, LastResponse: "final"},
		},
		snapErrs: []error{nil, nil, errors.New("temporary CDP write timeout after restart"), nil, nil, nil},
	}
	session := readyTestSession()
	transport := testTransport(t, session, adapter)
	transport.captureReconnects = 1
	transport.preflightRetries = 1

	opens := 0
	transport.evaluatorFactory = func(context.Context, SessionSnapshot, sites.Adapter) (interactionEvaluator, error) {
		opens++
		if opens == 2 {
			return nil, errors.New("new Chrome target is still settling")
		}
		return noopInteractionEvaluator{}, nil
	}

	got, err := transport.RoundTrip(context.Background(), "one prompt")
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	if got != "final" {
		t.Fatalf("final = %q, want final", got)
	}
	if opens < 3 {
		t.Fatalf("evaluator opens = %d, want bounded retry after transient reconnect-open failure", opens)
	}
	if adapter.submitN != 1 {
		t.Fatalf("submit count = %d, want exactly 1", adapter.submitN)
	}
	if state := session.Snapshot().State; state != SessionReady {
		t.Fatalf("session state = %s, want READY", state)
	}
}

// W5E-R1 regression: a post-capture DEGRADED state is expected to be transient
// while the restarted Chrome/Gemini renderer settles. Readiness must stay
// fail-closed for prompt submission while allowing bounded read-only probes to
// converge back to READY.
func TestGeminiWebTransportDegradedSessionSettlesReadyBeforeNextPrompt(t *testing.T) {
	session := readyTestSession()
	session.snapshot.State = SessionDegraded
	calls := 0
	session.probe = readinessProbeFunc(func(context.Context, SessionSnapshot) (ReadinessResult, error) {
		calls++
		if calls < 3 {
			return ReadinessResult{State: SessionDegraded, Reason: "restarted renderer still settling"}, nil
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
	if calls < 3 {
		t.Fatalf("probe calls = %d, want bounded DEGRADED recovery probes", calls)
	}
}
