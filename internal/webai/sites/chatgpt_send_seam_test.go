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

func TestD08ChatGPTVerifiedPromptUsesExactlyOneVisibleSendClick(t *testing.T) {
	e := &chatGPTSendSeamEvaluator{responses: []json.RawMessage{
		json.RawMessage(`{"found":true,"x":40,"y":50,"kind":"prosemirror"}`),
		json.RawMessage(`{"ok":true,"reason":"","composer_length":6,"expected_length":6,"composer_line_breaks":0,"expected_line_breaks":0,"composer_kind":"prosemirror","focused":true}`),
		json.RawMessage(`{"ok":true,"retry":false,"found":true,"reason":"","x":100,"y":200,"action":"button"}`),
	}}

	if err := (ChatGPT{}).Submit(context.Background(), e, "prompt"); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if e.replacements != 1 {
		t.Fatalf("trusted replacements = %d, want exactly 1", e.replacements)
	}
	if e.clicks != 1 || e.enters != 0 {
		t.Fatalf("clicks=%d enters=%d, want exactly one click and zero Enter fallbacks", e.clicks, e.enters)
	}
}

func TestD08ChatGPTVerifiedPromptFallsBackToExactlyOneTrustedEnterOnlyWhenSendAbsent(t *testing.T) {
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

func TestD08ChatGPTDisabledExplicitSendNeverFallsBackToEnter(t *testing.T) {
	e := &chatGPTSendSeamEvaluator{responses: []json.RawMessage{
		json.RawMessage(`{"ok":false,"retry":true,"found":true,"reason":"send button disabled","x":100,"y":200,"action":"button"}`),
	}}

	err := submitVerifiedChatGPTPrompt(context.Background(), e, e, 0)
	if err == nil || !strings.Contains(err.Error(), "send button disabled") {
		t.Fatalf("error = %v, want disabled-send fail-closed error", err)
	}
	if e.clicks != 0 || e.enters != 0 {
		t.Fatalf("clicks=%d enters=%d, disabled explicit Send must not click or Enter", e.clicks, e.enters)
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
		"found: true",
	} {
		if !strings.Contains(chatGPTResolveSendExpression, want) {
			t.Fatalf("ChatGPT send resolver missing %q", want)
		}
	}
}
