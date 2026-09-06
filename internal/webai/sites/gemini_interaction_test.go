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
		json.RawMessage(`{"ok":true,"retry":false,"reason":"","action":"native-button"}`),
	}}
	prompt := "line 1\n`quoted` </script> \"x\""
	if err := (Gemini{}).Submit(context.Background(), e, prompt); err != nil {
		t.Fatal(err)
	}
	if len(e.exprs) != 2 {
		t.Fatalf("expressions = %d, want prepare + click", len(e.exprs))
	}
	encoded, _ := json.Marshal(prompt)
	if !strings.Contains(e.exprs[0], string(encoded)) {
		t.Fatalf("prompt was not JSON encoded in prepare expression")
	}
}

func TestGeminiSubmitRequiresPreparedComposerText(t *testing.T) {
	e := &scriptedEvaluator{responses: []json.RawMessage{
		json.RawMessage(`{"ok":true,"reason":"","composer_length":0}`),
	}}
	if err := (Gemini{}).Submit(context.Background(), e, "prompt"); err == nil || !strings.Contains(err.Error(), "prepared composer is empty") {
		t.Fatalf("err = %v, want prepared-composer rejection", err)
	}
	if len(e.exprs) != 1 {
		t.Fatalf("expressions = %d, want prepare only", len(e.exprs))
	}
}

func TestGeminiSubmitSupportsCurrentCustomSendControls(t *testing.T) {
	e := &scriptedEvaluator{responses: []json.RawMessage{
		json.RawMessage(`{"ok":true,"reason":"","composer_length":6}`),
		json.RawMessage(`{"ok":true,"retry":false,"reason":"","action":"nested-native-button"}`),
	}}
	if err := (Gemini{}).Submit(context.Background(), e, "prompt"); err != nil {
		t.Fatal(err)
	}
	if len(e.exprs) != 2 {
		t.Fatalf("expressions = %d, want prepare + click", len(e.exprs))
	}
	clickExpr := e.exprs[1]
	for _, want := range []string{
		"gem-icon-button.send-button",
		"gem-icon-button.submit",
		"send-button-container",
		"arrow_upward",
	} {
		if !strings.Contains(clickExpr, want) {
			t.Fatalf("click expression missing current Gemini compatibility marker %q", want)
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
		json.RawMessage(`{"ok":true,"retry":false,"reason":"","action":"shadow-native-button"}`),
	}}
	if err := (Gemini{}).Submit(context.Background(), e, "prompt"); err != nil {
		t.Fatal(err)
	}
	clickExpr := e.exprs[1]
	for _, want := range []string{
		"candidate instanceof HTMLButtonElement",
		"candidate.shadowRoot",
		"root.querySelectorAll('button')",
		"nested-native-button",
		"shadow-native-button",
		"custom-send-host",
	} {
		if !strings.Contains(clickExpr, want) {
			t.Fatalf("native-action resolver missing %q", want)
		}
	}
	if !strings.Contains(clickExpr, "if (disabled(action.element))") {
		t.Fatal("resolved native action must be checked for disabled state")
	}
}

func TestGeminiSubmitNeverUsesContainerAsBlindClickTarget(t *testing.T) {
	e := &scriptedEvaluator{responses: []json.RawMessage{
		json.RawMessage(`{"ok":true,"reason":"","composer_length":6}`),
		json.RawMessage(`{"ok":true,"retry":false,"reason":"","action":"native-button"}`),
	}}
	if err := (Gemini{}).Submit(context.Background(), e, "prompt"); err != nil {
		t.Fatal(err)
	}
	clickExpr := e.exprs[1]
	if strings.Contains(clickExpr, "const sendButton = findSend();") || strings.Contains(clickExpr, "sendButton.click()") {
		t.Fatal("legacy blind wrapper click path must not survive")
	}
	if !strings.Contains(clickExpr, "const action = findSendAction();") || !strings.Contains(clickExpr, "action.element.click();") {
		t.Fatal("submit must click only a canonical resolved action")
	}
}

func TestGeminiSubmitPreparationUsesFrameworkInputSignalsAndReadback(t *testing.T) {
	e := &scriptedEvaluator{responses: []json.RawMessage{
		json.RawMessage(`{"ok":true,"reason":"","composer_length":6}`),
		json.RawMessage(`{"ok":true,"retry":false,"reason":"","action":"native-button"}`),
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

func TestGeminiSubmitPollsDisabledSendControlWithoutReplacingPrompt(t *testing.T) {
	e := &scriptedEvaluator{responses: []json.RawMessage{
		json.RawMessage(`{"ok":true,"reason":"","composer_length":6}`),
		json.RawMessage(`{"ok":false,"retry":true,"reason":"actionable send control is disabled"}`),
		json.RawMessage(`{"ok":true,"retry":false,"reason":"","action":"native-button"}`),
	}}
	if err := (Gemini{}).Submit(context.Background(), e, "prompt"); err != nil {
		t.Fatal(err)
	}
	if len(e.exprs) != 3 {
		t.Fatalf("expressions = %d, want one prepare + two click probes", len(e.exprs))
	}
	if strings.Contains(e.exprs[1], "const prompt =") || strings.Contains(e.exprs[2], "const prompt =") {
		t.Fatal("send-control polling must not rewrite the composer")
	}
}

func TestGeminiCancelCanonicalizesStopControls(t *testing.T) {
	e := &scriptedEvaluator{responses: []json.RawMessage{json.RawMessage(`{"clicked":true}`)}}
	clicked, err := (Gemini{}).Cancel(context.Background(), e)
	if err != nil {
		t.Fatal(err)
	}
	if !clicked {
		t.Fatal("expected cancel click")
	}
	if len(e.exprs) != 1 {
		t.Fatalf("expressions = %d, want 1", len(e.exprs))
	}
	for _, want := range []string{"candidate.shadowRoot", "root.querySelectorAll('button')", "action.click()"} {
		if !strings.Contains(e.exprs[0], want) {
			t.Fatalf("cancel native-action resolver missing %q", want)
		}
	}
}
