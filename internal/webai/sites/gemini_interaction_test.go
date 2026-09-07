package sites

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type scriptedEvaluator struct {
	responses   []json.RawMessage
	exprs       []string
	clicks      int
	clickX      float64
	clickY      float64
	clickErr    error
	replacements int
	replaceX    float64
	replaceY    float64
	replaceText string
	replaceErr  error
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

func (s *scriptedEvaluator) ReplaceText(_ context.Context, x, y float64, text string) error {
	s.replacements++
	s.replaceX = x
	s.replaceY = y
	s.replaceText = text
	return s.replaceErr
}

type evalOnlyEvaluator struct{}

func (*evalOnlyEvaluator) Eval(context.Context, string) (json.RawMessage, error) {
	return json.RawMessage(`null`), nil
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

func successfulSubmitEvaluator() *scriptedEvaluator {
	return &scriptedEvaluator{responses: []json.RawMessage{
		json.RawMessage(`{"found":true,"x":40,"y":50}`),
		json.RawMessage(`{"ok":true,"reason":"","composer_length":6}`),
		json.RawMessage(`{"ok":true,"retry":false,"reason":"","action":"native-button","x":100,"y":200}`),
	}}
}

func TestGeminiSubmitUsesTrustedComposerReplacementThenOneTrustedSendClick(t *testing.T) {
	e := successfulSubmitEvaluator()
	prompt := "line 1\n`quoted` </script> \"x\""
	if err := (Gemini{}).Submit(context.Background(), e, prompt); err != nil {
		t.Fatal(err)
	}
	if len(e.exprs) != 3 {
		t.Fatalf("expressions = %d, want resolve composer + verify + resolve send", len(e.exprs))
	}
	if e.replacements != 1 || e.replaceX != 40 || e.replaceY != 50 || e.replaceText != prompt {
		t.Fatalf("trusted replacement mismatch: count=%d point=%.2f,%.2f text=%q", e.replacements, e.replaceX, e.replaceY, e.replaceText)
	}
	if e.clicks != 1 || e.clickX != 100 || e.clickY != 200 {
		t.Fatalf("trusted send clicks=%d at %.2f,%.2f, want exactly one", e.clicks, e.clickX, e.clickY)
	}
	encoded, _ := json.Marshal(prompt)
	if !strings.Contains(e.exprs[1], string(encoded)) {
		t.Fatal("read-only verification did not JSON-encode the expected prompt")
	}
}

func TestGeminiSubmitRequiresTrustedTextInputCapability(t *testing.T) {
	var evaluator Evaluator = &evalOnlyEvaluator{}
	if _, ok := evaluator.(TextInputEvaluator); ok {
		t.Fatal("test evaluator unexpectedly implements TextInputEvaluator")
	}
	if err := (Gemini{}).Submit(context.Background(), evaluator, "prompt"); err == nil || !strings.Contains(err.Error(), "trusted text input") {
		t.Fatalf("err = %v, want trusted-text-input requirement", err)
	}
}

func TestGeminiSubmitTrustedReplacementFailureNeverAttemptsSend(t *testing.T) {
	e := &scriptedEvaluator{
		responses:  []json.RawMessage{json.RawMessage(`{"found":true,"x":40,"y":50}`)},
		replaceErr: errors.New("input failed"),
	}
	err := (Gemini{}).Submit(context.Background(), e, "prompt")
	if err == nil || !strings.Contains(err.Error(), "trusted composer input") {
		t.Fatalf("err = %v, want trusted composer input failure", err)
	}
	if e.replacements != 1 || e.clicks != 0 || len(e.exprs) != 1 {
		t.Fatalf("replacement=%d clicks=%d expressions=%d, send must not run", e.replacements, e.clicks, len(e.exprs))
	}
}

func TestGeminiSubmitReadbackFailureNeverAttemptsSend(t *testing.T) {
	e := &scriptedEvaluator{responses: []json.RawMessage{
		json.RawMessage(`{"found":true,"x":40,"y":50}`),
		json.RawMessage(`{"ok":false,"reason":"prompt composer did not retain trusted input","composer_length":3}`),
	}}
	err := (Gemini{}).Submit(context.Background(), e, "prompt")
	if err == nil || !strings.Contains(err.Error(), "did not retain trusted input") {
		t.Fatalf("err = %v, want readback failure", err)
	}
	if e.replacements != 1 || e.clicks != 0 || len(e.exprs) != 2 {
		t.Fatalf("replacement=%d clicks=%d expressions=%d, send must not run", e.replacements, e.clicks, len(e.exprs))
	}
}

func TestGeminiComposerDOMPathsAreReadOnly(t *testing.T) {
	for name, expr := range map[string]string{
		"resolve": geminiResolveComposerExpression,
		"verify":  geminiVerifyPromptExpressionTemplate,
	} {
		for _, forbidden := range []string{"execCommand(", ".focus()", ".click()", "textContent = prompt", "dispatchEvent(new InputEvent", "dispatchEvent(new Event"} {
			if strings.Contains(expr, forbidden) {
				t.Fatalf("%s composer expression contains synthetic mutation %q", name, forbidden)
			}
		}
	}
	if !strings.Contains(geminiResolveComposerExpression, "getBoundingClientRect") || !strings.Contains(geminiVerifyPromptExpressionTemplate, "composer_length") {
		t.Fatal("composer resolver/readback structural invariants are missing")
	}
}

func TestGeminiSubmitSupportsCurrentCustomSendControls(t *testing.T) {
	e := successfulSubmitEvaluator()
	if err := (Gemini{}).Submit(context.Background(), e, "prompt"); err != nil {
		t.Fatal(err)
	}
	resolveExpr := e.exprs[2]
	for _, want := range []string{"gem-icon-button.send-button", "gem-icon-button.submit", "send-button-container", "arrow_upward"} {
		if !strings.Contains(resolveExpr, want) {
			t.Fatalf("send resolver missing Gemini compatibility marker %q", want)
		}
	}
	if strings.Contains(resolveExpr, ".click()") || strings.Contains(resolveExpr, "new MouseEvent") || strings.Contains(resolveExpr, "new PointerEvent") {
		t.Fatal("send resolver must remain read-only with respect to click side effects")
	}
}

func TestGeminiSubmitCanonicalizesCustomHostsToNativeButtonsFirst(t *testing.T) {
	e := successfulSubmitEvaluator()
	if err := (Gemini{}).Submit(context.Background(), e, "prompt"); err != nil {
		t.Fatal(err)
	}
	resolveExpr := e.exprs[2]
	for _, want := range []string{"candidate instanceof HTMLButtonElement", "candidate.shadowRoot", "root.querySelectorAll('button')", "nested-native-button", "shadow-native-button", "custom-send-host"} {
		if !strings.Contains(resolveExpr, want) {
			t.Fatalf("native-action resolver missing %q", want)
		}
	}
	if !strings.Contains(resolveExpr, "if (disabled(action.element))") {
		t.Fatal("resolved native action must be checked for disabled state")
	}
}

func TestGeminiSubmitPollsDisabledSendWithoutReplacingAgainOrClickingEarly(t *testing.T) {
	e := &scriptedEvaluator{responses: []json.RawMessage{
		json.RawMessage(`{"found":true,"x":40,"y":50}`),
		json.RawMessage(`{"ok":true,"reason":"","composer_length":6}`),
		json.RawMessage(`{"ok":false,"retry":true,"reason":"actionable send control is disabled"}`),
		json.RawMessage(`{"ok":true,"retry":false,"reason":"","action":"native-button","x":10,"y":20}`),
	}}
	if err := (Gemini{}).Submit(context.Background(), e, "prompt"); err != nil {
		t.Fatal(err)
	}
	if len(e.exprs) != 4 || e.replacements != 1 || e.clicks != 1 {
		t.Fatalf("expressions=%d replacements=%d clicks=%d", len(e.exprs), e.replacements, e.clicks)
	}
	if strings.Contains(e.exprs[2], "const prompt =") || strings.Contains(e.exprs[3], "const prompt =") {
		t.Fatal("send-control polling must not rewrite or re-verify the prompt")
	}
}

func TestGeminiSubmitPointerFailureIsNotRetried(t *testing.T) {
	e := successfulSubmitEvaluator()
	e.clickErr = context.DeadlineExceeded
	err := (Gemini{}).Submit(context.Background(), e, "prompt")
	if err == nil || !strings.Contains(err.Error(), "trusted send click") {
		t.Fatalf("err = %v, want trusted-click failure", err)
	}
	if e.clicks != 1 || e.replacements != 1 || len(e.exprs) != 3 {
		t.Fatalf("clicks=%d replacements=%d expressions=%d, ambiguous send must never resubmit", e.clicks, e.replacements, len(e.exprs))
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
	if len(e.exprs) != 1 || strings.Contains(e.exprs[0], ".click()") {
		t.Fatal("cancel resolver must be one read-only DOM observation before trusted click")
	}
	for _, want := range []string{"candidate.shadowRoot", "root.querySelectorAll('button')", "getBoundingClientRect", "found: true"} {
		if !strings.Contains(e.exprs[0], want) {
			t.Fatalf("cancel coordinate resolver missing %q", want)
		}
	}
}
