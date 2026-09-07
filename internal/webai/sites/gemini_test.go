package sites

import (
	"context"
	"encoding/json"
	"testing"
)

type staticEvaluator struct {
	payload json.RawMessage
	err     error
}

func (s staticEvaluator) Eval(context.Context, string) (json.RawMessage, error) {
	return s.payload, s.err
}

func probeGeminiPayload(t *testing.T, payload string) Result {
	t.Helper()
	result, err := (Gemini{}).Probe(context.Background(), staticEvaluator{payload: json.RawMessage(payload)})
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	return result
}

func TestGeminiProbeCleanProfileSignInWins(t *testing.T) {
	result := probeGeminiPayload(t, `{
		"host":"gemini.google.com",
		"path":"/app",
		"has_composer":true,
		"has_sign_in":true,
		"candidate_accounts_iframe":true,
		"candidate_ogs_iframe":true
	}`)
	if result.State != ReadinessAuthRequired {
		t.Fatalf("state = %q, want %q", result.State, ReadinessAuthRequired)
	}
}

func TestGeminiProbeIframeAccountShellReady(t *testing.T) {
	result := probeGeminiPayload(t, `{
		"host":"gemini.google.com",
		"path":"/app",
		"has_composer":true,
		"has_sign_in":false,
		"candidate_accounts_iframe":true,
		"candidate_ogs_iframe":true
	}`)
	if result.State != ReadinessReady {
		t.Fatalf("state = %q, want %q (reason %q)", result.State, ReadinessReady, result.Reason)
	}
}

func TestGeminiProbeComposerWithoutAuthenticatedSignalStaysAuthRequired(t *testing.T) {
	result := probeGeminiPayload(t, `{
		"host":"gemini.google.com",
		"path":"/app",
		"has_composer":true,
		"has_sign_in":false
	}`)
	if result.State != ReadinessAuthRequired {
		t.Fatalf("state = %q, want %q", result.State, ReadinessAuthRequired)
	}
}

func TestGeminiProbeClassicAccountControlReady(t *testing.T) {
	result := probeGeminiPayload(t, `{
		"host":"gemini.google.com",
		"path":"/app",
		"has_account_control":true,
		"has_composer":true,
		"has_sign_in":false
	}`)
	if result.State != ReadinessReady {
		t.Fatalf("state = %q, want %q", result.State, ReadinessReady)
	}
}
