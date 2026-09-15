package sites

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type chatGPTSendSeamEvaluator struct {
	responses    []json.RawMessage
	exprs        []string
	replacements int
	clicks       int
	enters       int
	clickErr     error
	enterErr     error
}

func (e *chatGPTSendSeamEvaluator) Eval(_ context.Context, expression string) (json.RawMessage, error) {
	e.exprs = append(e.exprs, expression)
	if len(e.responses) == 0 {
		return json.RawMessage(`{"ok":false,"retry":true,"found":false,"reason":"send button not found","action":""}`), nil
	}
	out := e.responses[0]
	e.responses = e.responses[1:]
	return out, nil
}

func (e *chatGPTSendSeamEvaluator) Click(context.Context, float64, float64) error {
	e.clicks++
	return e.clickErr
}

func (e *chatGPTSendSeamEvaluator) ReplaceText(context.Context, float64, float64, string) error {
	e.replacements++
	return nil
}

func (e *chatGPTSendSeamEvaluator) PressEnter(context.Context) error {
	e.enters++
	return e.enterErr
}

type chatGPTPersistentMismatchEvaluator struct {
	exprs        []string
	replacements int
	clicks       int
	enters       int
	conversation int
}

func (e *chatGPTPersistentMismatchEvaluator) Eval(_ context.Context, expression string) (json.RawMessage, error) {
	e.exprs = append(e.exprs, expression)
	switch {
	case expression == chatGPTResolveComposerExpression:
		return json.RawMessage(`{"found":true,"x":40,"y":50,"kind":"prosemirror"}`), nil
	case expression == chatGPTResolveSendExpression:
		return json.RawMessage(`{"ok":true,"retry":false,"found":true,"reason":"","x":100,"y":200,"action":"button"}`), nil
	case expression == chatGPTConversationExpression:
		e.conversation++
		if e.conversation == 1 {
			return json.RawMessage(`{"busy":false,"response_count":1,"user_message_count":1,"composer_present":true,"composer_empty":false,"composer_length":128,"submit_action":"button","last_response":"old","truncated":false}`), nil
		}
		return json.RawMessage(`{"busy":true,"response_count":1,"user_message_count":2,"composer_present":true,"composer_empty":true,"composer_length":0,"submit_action":"","last_response":"old","truncated":false}`), nil
	case strings.Contains(expression, "const prompt ="):
		return json.RawMessage(`{"ok":false,"reason":"composer text mismatch","composer_length":128,"expected_length":140,"composer_line_breaks":2,"expected_line_breaks":3,"composer_kind":"prosemirror","focused":true}`), nil
	default:
		return nil, errors.New("unexpected ChatGPT expression")
	}
}

func (e *chatGPTPersistentMismatchEvaluator) Click(context.Context, float64, float64) error {
	e.clicks++
	return nil
}

func (e *chatGPTPersistentMismatchEvaluator) ReplaceText(context.Context, float64, float64, string) error {
	e.replacements++
	return nil
}

func (e *chatGPTPersistentMismatchEvaluator) PressEnter(context.Context) error {
	e.enters++
	return nil
}

func TestD08ChatGPTVerifiedPromptUsesExactlyOneVisibleSendClick(t *testing.T) {
	e := &chatGPTSendSeamEvaluator{responses: []json.RawMessage{
		json.RawMessage(`{"found":true,"x":40,"y":50,"kind":"prosemirror"}`),
		json.RawMessage(`{"ok":true,"reason":"","composer_length":6,"expected_length":6,"composer_line_breaks":0,"expected_line_breaks":0,"composer_kind":"prosemirror","focused":true}`),
		json.RawMessage(`{"ok":true,"retry":false,"found":true,"reason":"","x":100,"y":200,"action":"button"}`),
	}}

	if err := submitVerifiedChatGPTPrompt(context.Background(), e, e, 0); err != nil {
		t.Fatalf("submitVerifiedChatGPTPrompt: %v", err)
	}
	if e.clicks != 1 || e.enters != 0 {
		t.Fatalf("clicks=%d enters=%d, want exactly one click and zero Enter fallbacks", e.clicks, e.enters)
	}
}

func TestD08ChatGPTVerifiedPromptFallsBackToExactlyOneTrustedEnterWhenSendAbsent(t *testing.T) {
	e := &chatGPTSendSeamEvaluator{responses: []json.RawMessage{
		json.RawMessage(`{"ok":false,"retry":true,"found":false,"reason":"send button not found","action":""}`),
	}}

	if err := submitVerifiedChatGPTPrompt(context.Background(), e, e, 0); err != nil {
		t.Fatalf("submitVerifiedChatGPTPrompt: %v", err)
	}
	if e.clicks != 0 || e.enters != 1 {
		t.Fatalf("clicks=%d enters=%d, want zero clicks and exactly one trusted Enter fallback", e.clicks, e.enters)
	}
}

func TestD08ChatGPTDisabledVisibleSendFallsBackToExactlyOneTrustedEnter(t *testing.T) {
	e := &chatGPTSendSeamEvaluator{responses: []json.RawMessage{
		json.RawMessage(`{"ok":false,"retry":true,"found":true,"reason":"send button disabled","x":100,"y":200,"action":"button"}`),
	}}

	if err := submitVerifiedChatGPTPrompt(context.Background(), e, e, 0); err != nil {
		t.Fatalf("submitVerifiedChatGPTPrompt: %v", err)
	}
	if e.clicks != 0 || e.enters != 1 {
		t.Fatalf("clicks=%d enters=%d, want disabled Send to fall back to exactly one trusted Enter", e.clicks, e.enters)
	}
}

func TestD08ChatGPTAmbiguousClickFailureNeverFallsBackToEnter(t *testing.T) {
	e := &chatGPTSendSeamEvaluator{
		responses: []json.RawMessage{
			json.RawMessage(`{"ok":true,"retry":false,"found":true,"reason":"","x":100,"y":200,"action":"button"}`),
		},
		clickErr: errors.New("ambiguous click failure"),
	}

	err := submitVerifiedChatGPTPrompt(context.Background(), e, e, 0)
	if err == nil || !strings.Contains(err.Error(), "trusted send click") {
		t.Fatalf("error = %v, want trusted send click failure", err)
	}
	if e.clicks != 1 || e.enters != 0 {
		t.Fatalf("clicks=%d enters=%d, ambiguous click must never be followed by Enter", e.clicks, e.enters)
	}
}

func TestD08ChatGPTReadbackMismatchDoesNotSuppressSendWhenComposerIsNonEmpty(t *testing.T) {
	e := &chatGPTPersistentMismatchEvaluator{}
	if err := (ChatGPT{}).Submit(context.Background(), e, "prompt with long structured content"); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if e.replacements != 1 {
		t.Fatalf("trusted replacements=%d, want exactly 1", e.replacements)
	}
	if e.clicks != 1 || e.enters != 0 {
		t.Fatalf("clicks=%d enters=%d, non-empty mismatch must still reach one normal Send", e.clicks, e.enters)
	}
}

func TestD08ChatGPTDetectsUnsentPromptAndRecoversWithExactlyOneEnter(t *testing.T) {
	baseline := ConversationSnapshot{
		ResponseCount:     1,
		UserMessageCount:  1,
		ComposerPresent:   true,
		ComposerEmpty:     false,
		ComposerLength:    128,
		LastResponse:      "old",
	}
	e := &chatGPTSendSeamEvaluator{responses: []json.RawMessage{
		json.RawMessage(`{"ok":true,"retry":false,"found":true,"reason":"","x":100,"y":200,"action":"button"}`),
		json.RawMessage(`{"busy":false,"response_count":1,"user_message_count":1,"composer_present":true,"composer_empty":false,"composer_length":128,"submit_action":"button","last_response":"old","truncated":false}`),
		json.RawMessage(`{"busy":true,"response_count":1,"user_message_count":2,"composer_present":true,"composer_empty":true,"composer_length":0,"submit_action":"","last_response":"old","truncated":false}`),
	}}

	if err := submitChatGPTWithAck(context.Background(), e, e, baseline, 0, 0, 0); err != nil {
		t.Fatalf("submitChatGPTWithAck: %v", err)
	}
	if e.clicks != 1 || e.enters != 1 {
		t.Fatalf("clicks=%d enters=%d, want one initial Send click then exactly one Enter recovery", e.clicks, e.enters)
	}
}

func TestD08ChatGPTReadbackRemainsReadOnlyAndAcceptsEquivalentProseMirrorRepresentations(t *testing.T) {
	for _, want := range []string{
		"readCandidates",
		"composer.innerText",
		"composer.textContent",
		"blocks.map",
		"\\u200b\\ufeff",
		"value === expected",
	} {
		if !strings.Contains(chatGPTVerifyPromptExpressionTemplate, want) {
			t.Fatalf("ChatGPT readback expression missing %q", want)
		}
	}
	for _, forbidden := range []string{
		".click()",
		".focus()",
		"execCommand(",
		"dispatchEvent(",
		"textContent =",
		"innerText =",
	} {
		if strings.Contains(chatGPTVerifyPromptExpressionTemplate, forbidden) {
			t.Fatalf("ChatGPT readback expression must remain read-only; found %q", forbidden)
		}
	}
}

func TestD08ChatGPTSendResolverCoversCurrentSendControls(t *testing.T) {
	for _, want := range []string{
		`button[data-testid="send-button"]`,
		`button[data-testid*="send" i]`,
		`button[aria-label*="send" i]`,
		`button[aria-label*="submit" i]`,
		`button[aria-label*="gửi" i]`,
		`form button[type="submit"]`,
		"found: true",
	} {
		if !strings.Contains(chatGPTResolveSendExpression, want) {
			t.Fatalf("ChatGPT send resolver missing %q", want)
		}
	}
}
