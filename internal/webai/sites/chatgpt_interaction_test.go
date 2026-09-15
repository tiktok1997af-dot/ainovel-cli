package sites

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func successfulChatGPTSubmitEvaluator() *scriptedEvaluator {
	return &scriptedEvaluator{responses: []json.RawMessage{
		json.RawMessage(`{"found":true,"x":40,"y":50,"kind":"prosemirror"}`),
		json.RawMessage(`{"ok":true,"reason":"","composer_length":13,"expected_length":13,"composer_line_breaks":1,"expected_line_breaks":1,"composer_kind":"prosemirror","focused":true}`),
		json.RawMessage(`{"busy":false,"response_count":1,"user_message_count":1,"composer_present":true,"composer_empty":false,"composer_length":13,"submit_action":"button","last_response":"old","truncated":false}`),
		json.RawMessage(`{"ok":true,"retry":false,"found":true,"reason":"","action":"button","x":100,"y":200}`),
		json.RawMessage(`{"busy":true,"response_count":1,"user_message_count":2,"composer_present":true,"composer_empty":true,"composer_length":0,"submit_action":"","last_response":"old","truncated":false}`),
	}}
}

func TestD08ChatGPTSubmitUsesTrustedProseMirrorReplacementThenOneSendClick(t *testing.T) {
	e := successfulChatGPTSubmitEvaluator()
	prompt := "line one\nline two"
	if err := (ChatGPT{}).Submit(context.Background(), e, prompt); err != nil {
		t.Fatal(err)
	}
	if len(e.exprs) != 5 {
		t.Fatalf("expressions = %d, want resolve composer + verify + baseline + resolve send + SEND ACK", len(e.exprs))
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
	if !strings.Contains(e.exprs[2], "user_message_count") || !strings.Contains(e.exprs[4], "user_message_count") {
		t.Fatal("ChatGPT submit must prove delivery with conversation-state SEND ACK")
	}
}

func TestD08ChatGPTComposerReadbackCanonicalizesVisibleProseMirrorWithoutWeakeningNonWhitespaceEquality(t *testing.T) {
	expr := chatGPTVerifyPromptExpressionTemplate
	for _, want := range []string{
		`div.ProseMirror[contenteditable="true"]`,
		`.replace(/\r\n/g, '\n')`,
		`.replace(/\r/g, '\n')`,
		`.replace(/\u00a0/g, ' ')`,
		`.normalize('NFC')`,
		`blockTags`,
		`stripLayoutWhitespace`,
		`layoutWhitespaceEquivalent`,
		`actual !== expected && !layoutWhitespaceEquivalent`,
		`composer_line_breaks`,
		`expected_line_breaks`,
		`composer_kind`,
	} {
		if !strings.Contains(expr, want) {
			t.Fatalf("ChatGPT readback invariant missing %q", want)
		}
	}
	for _, forbidden := range []string{
		"execCommand(",
		".focus()",
		".click()",
		"textContent = prompt",
		"dispatchEvent(new InputEvent",
		"dispatchEvent(new Event",
	} {
		if strings.Contains(expr, forbidden) {
			t.Fatalf("ChatGPT readback expression contains synthetic mutation %q", forbidden)
		}
	}
}

func TestD08ChatGPTComposerResolverTargetsVisibleProseMirrorAndReportsKind(t *testing.T) {
	for _, want := range []string{
		`div.ProseMirror[contenteditable="true"]`,
		`kindOf`,
		`prosemirror`,
		`window.innerHeight`,
		`window.innerWidth`,
	} {
		if !strings.Contains(chatGPTResolveComposerExpression, want) {
			t.Fatalf("ChatGPT composer resolver missing %q", want)
		}
	}
}

func TestD08ChatGPTSendResolverSupportsCurrentVisibleSendButtonWithoutDOMClick(t *testing.T) {
	for _, want := range []string{
		`button[data-testid="send-button"]`,
		`button[aria-label*="send" i]`,
		`button[aria-label*="gửi" i]`,
	} {
		if !strings.Contains(chatGPTResolveSendExpression, want) {
			t.Fatalf("ChatGPT send resolver missing %q", want)
		}
	}
	if strings.Contains(chatGPTResolveSendExpression, ".click()") || strings.Contains(chatGPTResolveSendExpression, "new MouseEvent") || strings.Contains(chatGPTResolveSendExpression, "new PointerEvent") {
		t.Fatal("ChatGPT send resolver must remain read-only with respect to click side effects")
	}
}
