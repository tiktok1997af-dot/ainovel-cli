package sites

import (
	"context"
	"encoding/json"
	"testing"
)

func TestGeminiRetryPendingSubmitNeverRetypesAndUsesOneRecoverySendClick(t *testing.T) {
	e := &scriptedEvaluator{responses: []json.RawMessage{
		json.RawMessage(`{"ok":true,"reason":"","composer_length":6,"expected_length":6,"composer_kind":"ql-editor","focused":false}`),
		json.RawMessage(`{"found":true,"x":40,"y":50,"kind":"ql-editor"}`),
		json.RawMessage(`{"ok":true,"reason":"","composer_length":6,"expected_length":6,"composer_kind":"ql-editor","focused":true}`),
		json.RawMessage(`{"ok":true,"retry":false,"reason":"","action":"native-button","x":100,"y":200}`),
	}}
	clicked, err := (Gemini{}).RetryPendingSubmit(context.Background(), e, "prompt")
	if err != nil {
		t.Fatal(err)
	}
	if !clicked {
		t.Fatal("expected one pending-submit recovery click")
	}
	if e.replacements != 0 {
		t.Fatalf("replacements = %d, recovery must never retype/replay prompt", e.replacements)
	}
	if e.clicks != 2 {
		t.Fatalf("trusted clicks = %d, want composer refocus + one send click", e.clicks)
	}
	if e.clickX != 100 || e.clickY != 200 {
		t.Fatalf("last click = %.2f,%.2f, want recovery Send at 100,200", e.clickX, e.clickY)
	}
}

func TestGeminiRetryPendingSubmitFailsClosedWhenExactPromptIsGone(t *testing.T) {
	mismatch := json.RawMessage(`{"ok":false,"reason":"prompt composer did not retain trusted input","composer_length":5,"expected_length":6,"composer_kind":"ql-editor","focused":true}`)
	e := &scriptedEvaluator{responses: []json.RawMessage{mismatch}, repeatLast: mismatch}
	clicked, err := (Gemini{}).RetryPendingSubmit(context.Background(), e, "prompt")
	if err != nil {
		t.Fatal(err)
	}
	if clicked {
		t.Fatal("recovery must not click Send when exact prompt is no longer present")
	}
	if e.replacements != 0 || e.clicks != 0 {
		t.Fatalf("replacements=%d clicks=%d, fail-closed recovery must be read-only", e.replacements, e.clicks)
	}
}
