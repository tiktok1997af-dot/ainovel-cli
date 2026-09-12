package sites

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type recoveryKeyEvaluator struct {
	*scriptedEvaluator
	enterPresses int
	enterErr     error
}

func (r *recoveryKeyEvaluator) PressEnter(context.Context) error {
	r.enterPresses++
	return r.enterErr
}

func TestGeminiRetryPendingSubmitNeverRetypesAndUsesTrustedEnterRecovery(t *testing.T) {
	base := &scriptedEvaluator{responses: []json.RawMessage{
		json.RawMessage(`{"ok":true,"reason":"","composer_length":6,"expected_length":6,"composer_kind":"ql-editor","focused":false}`),
		json.RawMessage(`{"found":true,"x":40,"y":50,"kind":"ql-editor"}`),
		json.RawMessage(`{"ok":true,"reason":"","composer_length":6,"expected_length":6,"composer_kind":"ql-editor","focused":true}`),
	}}
	e := &recoveryKeyEvaluator{scriptedEvaluator: base}
	submitted, err := (Gemini{}).RetryPendingSubmit(context.Background(), e, "prompt")
	if err != nil {
		t.Fatal(err)
	}
	if !submitted {
		t.Fatal("expected one pending-submit recovery action")
	}
	if base.replacements != 0 {
		t.Fatalf("replacements = %d, recovery must never retype/replay prompt", base.replacements)
	}
	if base.clicks != 1 || base.clickX != 40 || base.clickY != 50 {
		t.Fatalf("trusted clicks=%d last=%.2f,%.2f, want only composer refocus at 40,50", base.clicks, base.clickX, base.clickY)
	}
	if e.enterPresses != 1 {
		t.Fatalf("trusted Enter presses = %d, want exactly one", e.enterPresses)
	}
	for _, expr := range base.exprs {
		if strings.Contains(expr, geminiResolveSendExpression) {
			t.Fatal("recovery must not repeat the failed Send-button resolver/click path")
		}
	}
}

func TestGeminiRetryPendingSubmitFailsClosedWhenExactPromptIsGone(t *testing.T) {
	mismatch := json.RawMessage(`{"ok":false,"reason":"prompt composer did not retain trusted input","composer_length":5,"expected_length":6,"composer_kind":"ql-editor","focused":true}`)
	base := &scriptedEvaluator{responses: []json.RawMessage{mismatch}, repeatLast: mismatch}
	e := &recoveryKeyEvaluator{scriptedEvaluator: base}
	submitted, err := (Gemini{}).RetryPendingSubmit(context.Background(), e, "prompt")
	if err != nil {
		t.Fatal(err)
	}
	if submitted {
		t.Fatal("recovery must not submit when exact prompt is no longer present")
	}
	if base.replacements != 0 || base.clicks != 0 || e.enterPresses != 0 {
		t.Fatalf("replacements=%d clicks=%d enter=%d, fail-closed recovery must be read-only", base.replacements, base.clicks, e.enterPresses)
	}
}

func TestGeminiRetryPendingSubmitEnterFailureIsReturnedWithoutReplay(t *testing.T) {
	base := &scriptedEvaluator{responses: []json.RawMessage{
		json.RawMessage(`{"ok":true,"reason":"","composer_length":6,"expected_length":6,"composer_kind":"ql-editor","focused":false}`),
		json.RawMessage(`{"found":true,"x":40,"y":50,"kind":"ql-editor"}`),
		json.RawMessage(`{"ok":true,"reason":"","composer_length":6,"expected_length":6,"composer_kind":"ql-editor","focused":true}`),
	}}
	e := &recoveryKeyEvaluator{scriptedEvaluator: base, enterErr: errors.New("key failed")}
	submitted, err := (Gemini{}).RetryPendingSubmit(context.Background(), e, "prompt")
	if err == nil || !strings.Contains(err.Error(), "trusted Enter submit") {
		t.Fatalf("err = %v, want trusted Enter failure", err)
	}
	if submitted {
		t.Fatal("failed trusted Enter must not be reported as submitted")
	}
	if base.replacements != 0 || base.clicks != 1 || e.enterPresses != 1 {
		t.Fatalf("replacements=%d clicks=%d enter=%d, recovery must remain single-action and never replay prompt", base.replacements, base.clicks, e.enterPresses)
	}
}
