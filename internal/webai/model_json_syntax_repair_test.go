package webai

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/voocel/agentcore"
)

func jsonSyntaxRepairToolSpec() agentcore.ToolSpec {
	return agentcore.ToolSpec{
		Name:        "save_plan",
		Description: "save a short plan",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"chapter": map[string]any{"type": "integer"},
				"summary": map[string]any{"type": "string"},
			},
			"required":             []string{"chapter", "summary"},
			"additionalProperties": false,
		},
	}
}

func malformedJSONToolCall() string {
	return `{"kind":"tool_calls","tool_calls":[{"name":"save_plan","arguments":{"chapter":2,"summary":"resume"} commentary}]}`
}

func validJSONToolCall() string {
	return `{"kind":"tool_calls","tool_calls":[{"name":"save_plan","arguments":{"chapter":2,"summary":"resume"}}]}`
}

func TestModelRepairsDecodeResponseSyntaxErrorWithTargetedPrompt(t *testing.T) {
	transport := &fakeTransport{responses: []string{
		malformedJSONToolCall(),
		validJSONToolCall(),
	}}
	model := mustModel(t, transport)

	resp, err := model.Generate(
		context.Background(),
		[]agentcore.Message{agentcore.UserMsg("resume chapter 2")},
		[]agentcore.ToolSpec{jsonSyntaxRepairToolSpec()},
	)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	calls := resp.Message.ToolCalls()
	if len(calls) != 1 || calls[0].Name != "save_plan" {
		t.Fatalf("tool calls = %+v, want one save_plan", calls)
	}

	prompts := transport.promptSnapshot()
	if len(prompts) != 2 {
		t.Fatalf("round trips = %d, want 2", len(prompts))
	}
	repair := prompts[1]
	for _, required := range []string{
		"invalid JSON syntax",
		"same intended answer",
		"double-quoted",
		"commas",
		"trailing commas",
		"comments",
		"escape",
	} {
		if !strings.Contains(repair, required) {
			t.Fatalf("targeted JSON repair prompt missing %q: %q", required, repair)
		}
	}
	if strings.Contains(repair, "raw_string_field was invalid") {
		t.Fatalf("JSON syntax repair must not use the raw_string_field-specific prompt: %q", repair)
	}
}

func TestModelDecodeResponseSyntaxRepairRemainsBounded(t *testing.T) {
	transport := &fakeTransport{responses: []string{
		malformedJSONToolCall(),
		malformedJSONToolCall(),
		malformedJSONToolCall(),
	}}
	model := mustModel(t, transport)

	_, err := model.Generate(
		context.Background(),
		[]agentcore.Message{agentcore.UserMsg("resume chapter 2")},
		[]agentcore.ToolSpec{jsonSyntaxRepairToolSpec()},
	)
	if !errors.Is(err, ErrProtocol) {
		t.Fatalf("err = %v, want ErrProtocol", err)
	}
	prompts := transport.promptSnapshot()
	if got, want := len(prompts), 1+maxProtocolFormatRepairs; got != want {
		t.Fatalf("round trips = %d, want %d", got, want)
	}
	for i, prompt := range prompts[1:] {
		if !strings.Contains(prompt, "invalid JSON syntax") || !strings.Contains(prompt, "trailing commas") {
			t.Fatalf("repair prompt %d was not targeted: %q", i+1, prompt)
		}
	}
}
