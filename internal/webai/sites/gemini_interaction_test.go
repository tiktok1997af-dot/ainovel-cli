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

func TestGeminiConversationDecodesSnapshot(t *testing.T) {
	e := &scriptedEvaluator{responses: []json.RawMessage{json.RawMessage(`{"busy":false,"response_count":2,"last_response":" final ","truncated":false}`)}}
	got, err := (Gemini{}).Conversation(context.Background(), e)
	if err != nil {
		t.Fatal(err)
	}
	if got.Busy || got.ResponseCount != 2 || got.LastResponse != "final" || got.Truncated {
		t.Fatalf("unexpected snapshot: %+v", got)
	}
}

func TestGeminiSubmitJSONEscapesPrompt(t *testing.T) {
	e := &scriptedEvaluator{responses: []json.RawMessage{
		json.RawMessage(`{"ok":true,"reason":""}`),
		json.RawMessage(`{"ok":true,"retry":false,"reason":""}`),
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

func TestGeminiSubmitSupportsCurrentCustomSendControls(t *testing.T) {
	e := &scriptedEvaluator{responses: []json.RawMessage{
		json.RawMessage(`{"ok":true,"reason":""}`),
		json.RawMessage(`{"ok":true,"retry":false,"reason":""}`),
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

func TestGeminiSubmitPollsDisabledSendControlWithoutReplacingPrompt(t *testing.T) {
	e := &scriptedEvaluator{responses: []json.RawMessage{
		json.RawMessage(`{"ok":true,"reason":""}`),
		json.RawMessage(`{"ok":false,"retry":true,"reason":"send control is disabled"}`),
		json.RawMessage(`{"ok":true,"retry":false,"reason":""}`),
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

func TestGeminiCancelReportsClicked(t *testing.T) {
	e := &scriptedEvaluator{responses: []json.RawMessage{json.RawMessage(`{"clicked":true}`)}}
	clicked, err := (Gemini{}).Cancel(context.Background(), e)
	if err != nil {
		t.Fatal(err)
	}
	if !clicked {
		t.Fatal("expected cancel click")
	}
}
