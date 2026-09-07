package sites

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type scriptedEvaluator struct {
	responses []json.RawMessage
	exprs     []string
	clicks    int
	clickX    float64
	clickY    float64
	clickErr  error
}

func (s *scriptedEvaluator) Eval(_ context.Context, expression string) (json.RawMessage, error) {
	s.exprs = append(s.exprs, expression)
	if len(s.responses) == 0 {
		return nil, nil
	}
	out := s.responses[0]
	s.responses = s.responses[1:]
	return out, nil
}

func (s *scriptedEvaluator) Click(_ context.Context, x, y float64) error {
	s.clicks++
	s.clickX = x
	s.clickY = y
	return s.clickErr
}

func TestGeminiConversationDecodesSanitizedAckSnapshot(t *testing.T) {
	e := &scriptedEvaluator{responses: []json.RawMessage{json.RawMessage(`{"busy":false,"response_count":2,"user_message_count":3,"composer_present":true,"composer_empty":false,"composer_length":17,"submit_action":" native-button ","last_response":" final ","truncated":false}`)}}
	got, err := (Gemini{}).Conversation(context.Background(), e)
	if err != nil {
		t.Fatal(err)
	}
	if got.Busy || got.ResponseCount != 2 || got.UserMessageCount != 3 || !got.ComposerPresent || got.ComposerEmpty || got.ComposerLength != 17 || got.SubmitAction != "native-button" || got.LastResponse != "final" || got.Truncated {
		t.Fatalf("unexpected snapshot: %+v", got)
	}
}

func TestGeminiConversationExpressionExposesAckSignalsWithoutPromptText(t *testing.T) {
	e := &scriptedEvaluator{responses: []json.RawMessage{json.RawMessage(`{"busy":false,"response_count":0,"user_message_count":0,"composer_present":true,"composer_empty":false,"composer_length":8,"submit_action":"native-button","last_response":"","truncated":false}`)}}
	if _, err := (Gemini{}).Conversation(context.Background(), e); err != nil {
		t.Fatal(err)
	}
	if len(e.exprs) != 1 {
		t.Fatalf("expressions = %d, want 1", len(e.exprs))
	}
	expr := e.exprs[0]
	for _, want := range []string{"user-query", "user_message_count", "composer_present", "composer_empty", "composer_length", "submit_action"} {
		if !strings.Contains(expr, want) {
			t.Fatalf("conversation expression missing SEND ACK marker %q", want)
		}
	}
	if strings.Contains(expr, "return {prompt") || strings.Contains(expr, "prompt_text") {
		t.Fatal("conversation snapshot must not return prompt content")
	}
}

func TestGeminiConversationRejectsNegativeComposerLength(t *testing.T) {
	e := &scriptedEvaluator{responses: []json.RawMessage{json.RawMessage(`{"composer_present":true,"composer_length":-1}`)}}
	if _, err := (Gemini{}).Conversation(context.Background(), e); err == nil || !strings.Contains(err.Error(), "negative composer length") {
		t.Fatalf("err = %v, want negative composer length rejection", err)
	}
}

func TestGeminiSubmitJSONEscapesPrompt(t *testing.T) {
	e := &scriptedEvaluator{responses: []json.RawMessage{
		json.RawMessage(`{"ok":true,"reason":"","composer_length":32}`),
		json.RawMessage(`{"ok":true,"retry":false,"reason":"","action":"native-button","x":321.5,"y":654.25}`),
	}}
	prompt := "line 1\n`quoted` </script> \"x\""
	if err := (Gemini{}).Submit(context.Background(), e, prompt); err != nil {
		t.Fatal(err)
	}
	if len(e.exprs) != 2 {
		t.Fatalf("expressions = %d, want prepare + resolve", len(e.exprs))
	}
	encoded, _ := json.Marshal(prompt)
	if !strings.Contains(e.exprs[0], string(encoded)) {
		t.Fatalf("prompt was not JSON encoded in prepare expression")
	}
	if e.clicks != 1 || e.clickX != 321.5 || e.clickY != 654.25 {
		t.Fatalf("trusted clicks=%d at %.2f,%.2f, want exactly one at resolved point", e.clicks, e.clickX, e.clickY)
	}
}

func TestGeminiSubmitRequiresTrustedPointerCapability(t *testing.T) {
	type evalOnly struct{ scriptedEvaluator }
	var evaluator Evaluator = &evalOnly{}
	if _, ok := evaluator.(PointerEvaluator); ok {
		t.Fatal("test evaluator unexpectedly implements PointerEvaluator")
	}
	if err := (Gemini{}).Submit(context.Background(), evaluator, "prompt"); err == nil || !strings.Contains(err.Error(), "trusted pointer input") {
		t.Fatalf("err = %v, want trusted-pointer requirement", err)
	}
}

func TestGeminiSubmitRequiresPreparedComposerText(t *testing.T) {
	e := &scriptedEvaluator{responses: []json.RawMessage{
		json.RawMessage(`{"ok":true,"reason":"","composer_length":0}`),
	}}
	if err := (Gemini{}).Submit(context.Background(), e, "prompt"); err == nil || !strings.Contains(err.Error(), "prepared composer is empty") {
		t.Fatalf("err = %v, want prepared-composer rejection", err)
	}
	if len(e.exprs) != 1 || e.clicks != 0 {
		t.Fatalf("expressions=%d clicks=%d, want prepare only and no click", len(e.exprs), e.clicks)
	}
}

func TestGeminiSubmitSupportsCurrentCustomSendControls(t *testing.T) {
	e := &scriptedEvaluator{responses: []json.RawMessage{
		json.RawMessage(`{"ok":true,"reason":"","composer_length":6}`),
		json.RawMessage(`{"ok":true,"retry":false,"reason":"","action":"nested-native-button","x":100,"y":200}`),
	}}
	if err := (Gemini{}).Submit(context.Background(), e, "prompt"); err != nil {
		t.Fatal(err)
	}
	if len(e.exprs) != 2 || e.clicks != 1 {
		t.Fatalf("expressions=%d clicks=%d, want prepare + resolve + one trusted click", len(e.exprs), e.clicks)
	}
	resolveExpr := e.exprs[1]
	for _, want := range []string{
		"gem-icon-button.send-button",
		"gem-icon-button.submit",
		"send-button-container",
		"arrow_upward",
	} {
		if !strings.Contains(resolveExpr, want) {
			t.Fatalf("resolve expression missing current Gemini compatibility marker %q", want)
		}
	}
	for i, expr := range e.exprs {
		if strings.Contains(expr, "async ()") || strings.Contains(expr, "new Promise") {
			t.Fatalf("submit expression %d contains browser-side async Promise: %s", i, expr)
		}
	}
}

func TestGeminiSubmitCanonicalizesCustomHostsToNativeButtonsFirst(t *testing.T) {
	e := &scriptedEvaluator{responses: []json.RawMessage{
		json.RawMessage(`{"ok":true,"reason":"","composer_length":6}`),
		json.RawMessage(`{"ok":true,"retry":false,"reason":"","action":"shadow-native-button","x":10,"y":20}`),
	}}
	if err := (Gemini{}).Submit(context.Background(), e, "prompt"); err != nil {
		t.Fatal(err)
	}
	resolveExpr := e.exprs[1]
	for _, want := range []string{
		"candidate instanceof HTMLButtonElement",
		"candidate.shadowRoot",
		"root.querySelectorAll('button')",
		"nested-native-button",
		"shadow-native-button",
		"custom-send-host",
	} {
		if !strings.Contains(resolveExpr, want) {
			t.Fatalf("native-action resolver missing %q", want)
		}
	}
	if !strings.Contains(resolveExpr, "if (disabled(action.element))") {
		t.Fatal("resolved native action must be checked for disabled state")
	}
}

func TestGeminiSubmitResolverHasNoBrowserSideClick(t *testing.T) {
	e := &scriptedEvaluator{responses: []json.RawMessage{
		json.RawMessage(`{"ok":true,"reason":"","composer_length":6}`),
		json.RawMessage(`{"ok":true,"retry":false,"reason":"","action":"native-button","x":10,"y":20}`),
	}}
	if err := (Gemini{}).Submit(context.Background(), e, "prompt"); err != nil {
		t.Fatal(err)
	}
	resolveExpr := e.exprs[1]
	if strings.Contains(resolveExpr, ".click()") || strings.Contains(resolveExpr, "dispatchEvent(new MouseEvent") || strings.Contains(resolveExpr, "dispatchEvent(new PointerEvent") {
		t.Fatal("send resolver must be read-only with respect to click side effects")
	}
	for _, want := range []string{"const action = findSendAction();", "getBoundingClientRect", "return {ok: true", "x: point.x", "y: point.y"} {
		if !strings.Contains(resolveExpr, want) {
			t.Fatalf("send coordinate resolver missing %q", want)
		}
	}
}

func TestGeminiSubmitPreparationUsesFrameworkInputSignalsAndReadback(t *testing.T) {
	e := &scriptedEvaluator{responses: []json.RawMessage{
		json.RawMessage(`{"ok":true,"reason":"","composer_length":6}`),
		json.RawMessage(`{"ok":true,"retry":false,"reason":"","action":"native-button","x":10,"y":20}`),
	}}
	if err := (Gemini{}).Submit(context.Background(), e, "prompt"); err != nil {
		t.Fatal(err)
	}
	prepareExpr := e.exprs[0]
	for _, want := range []string{"beforeinput", "document.execCommand('insertText'", "samePrompt", "composer_length"} {
		if !strings.Contains(prepareExpr, want) {
			t.Fatalf("composer preparation missing %q", want)
		}
	}
}

func TestGeminiSubmitPollsDisabledSendControlWithoutReplacingPromptOrClicking(t *testing.T) {
	e := &scriptedEvaluator{responses: []json.RawMessage{
		json.RawMessage(`{"ok":true,"reason":"","composer_length":6}`),
		json.RawMessage(`{"ok":false,"retry":true,"reason":"actionable send control is disabled"}`),
		json.RawMessage(`{"ok":true,"retry":false,"reason":"","action":"native-button","x":10,"y":20}`),
	}}
	if err := (Gemini{}).Submit(context.Background(), e, "prompt"); err != nil {
		t.Fatal(err)
	}
	if len(e.exprs) != 3 || e.clicks != 1 {
		t.Fatalf("expressions=%d clicks=%d, want one prepare + two read-only probes + one click", len(e.exprs), e.clicks)
	}
	if strings.Contains(e.exprs[1], "const prompt =") || strings.Contains(e.exprs[2], "const prompt =") {
		t.Fatal("send-control polling must not rewrite the composer")
	}
}

func TestGeminiSubmitPointerFailureIsNotRetried(t *testing.T) {
	e := &scriptedEvaluator{
		responses: []json.RawMessage{
			json.RawMessage(`{"ok":true,"reason":"","composer_length":6}`),
			json.RawMessage(`{"ok":true,"retry":false,"reason":"","action":"native-button","x":10,"y":20}`),
		},
		clickErr: context.DeadlineExceeded,
	}
	err := (Gemini{}).Submit(context.Background(), e, "prompt")
	if err == nil || !strings.Contains(err.Error(), "trusted send click") {
		t.Fatalf("err = %v, want trusted-click failure", err)
	}
	if e.clicks != 1 || len(e.exprs) != 2 {
		t.Fatalf("clicks=%d expressions=%d, ambiguous pointer failure must never resubmit", e.clicks, len(e.exprs))
	}
}

func TestGeminiCancelUsesTrustedPointerAndReadOnlyResolver(t *testing.T) {
	e := &scriptedEvaluator{responses: []json.RawMessage{json.RawMessage(`{"found":true,"x":42,"y":84}`)}}
	clicked, err := (Gemini{}).Cancel(context.Background(), e)
	if err != nil {
		t.Fatal(err)
	}
	if !clicked || e.clicks != 1 || e.clickX != 42 || e.clickY != 84 {
		t.Fatalf("clicked=%v pointer clicks=%d at %.2f,%.2f", clicked, e.clicks, e.clickX, e.clickY)
	}
	if len(e.exprs) != 1 {
		t.Fatalf("expressions = %d, want 1", len(e.exprs))
	}
	if strings.Contains(e.exprs[0], ".click()") {
		t.Fatal("cancel resolver must not click inside browser JavaScript")
	}
	for _, want := range []string{"candidate.shadowRoot", "root.querySelectorAll('button')", "getBoundingClientRect", "found: true"} {
		if !strings.Contains(e.exprs[0], want) {
			t.Fatalf("cancel coordinate resolver missing %q", want)
		}
	}
}
