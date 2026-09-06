package webai

import (
	"context"
	"strings"
	"testing"

	"github.com/voocel/agentcore"
)

func TestModelGenerateRawTextPreservesStructuredJSONBody(t *testing.T) {
	body := `{"planner":"architect_long","task":"write two chapters","reason":"long-form workflow"}`
	transport := &fakeTransport{responses: []string{wrappedResponse("TEXT\n" + body)}}
	model := mustModel(t, transport)

	resp, err := model.Generate(context.Background(), []agentcore.Message{agentcore.UserMsg("plan")}, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got := resp.Message.TextContent(); got != body {
		t.Fatalf("text = %q, want %q", got, body)
	}
	prompts := transport.promptSnapshot()
	if len(prompts) != 1 {
		t.Fatalf("round trips = %d, want 1", len(prompts))
	}
	if !strings.Contains(prompts[0], "AINOVEL WEB RESPONSE EXTENSION") || !strings.Contains(prompts[0], "complete answer verbatim") {
		t.Fatal("request prompt did not advertise raw TEXT response extension")
	}
	if got := strings.Count(prompts[0], responseStart); got != 1 {
		t.Fatalf("response start marker occurrences = %d, want exactly 1", got)
	}
	if got := strings.Count(prompts[0], responseEnd); got != 1 {
		t.Fatalf("response end marker occurrences = %d, want exactly 1", got)
	}
}

func TestModelProtocolRepairCanReturnRawText(t *testing.T) {
	body := `{"planner":"architect_long","task":"write two chapters","reason":"long-form workflow"}`
	transport := &fakeTransport{responses: []string{
		wrappedResponse(`{"kind":"text","text":"{"planner":"architect_long" nope}`),
		wrappedResponse("TEXT\n" + body),
	}}
	model := mustModel(t, transport)

	resp, err := model.Generate(context.Background(), []agentcore.Message{agentcore.UserMsg("plan")}, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got := resp.Message.TextContent(); got != body {
		t.Fatalf("text = %q, want repaired raw body %q", got, body)
	}
	prompts := transport.promptSnapshot()
	if len(prompts) != 2 {
		t.Fatalf("round trips = %d, want 2", len(prompts))
	}
	if !strings.Contains(prompts[1], "Do not JSON-escape") || !strings.Contains(prompts[1], "TEXT") {
		t.Fatal("repair prompt did not prefer the raw TEXT form")
	}
	if got := strings.Count(prompts[1], responseStart); got != 1 {
		t.Fatalf("repair response start marker occurrences = %d, want exactly 1", got)
	}
	if got := strings.Count(prompts[1], responseEnd); got != 1 {
		t.Fatalf("repair response end marker occurrences = %d, want exactly 1", got)
	}
}

func TestRawTextExtensionDoesNotRelaxToolCallJSONValidation(t *testing.T) {
	transport := &fakeTransport{responses: []string{
		wrappedResponse(`{"kind":"tool_calls","tool_calls":[{"name":"save_chapter","arguments":[1,2]}]}`),
		wrappedResponse(`{"kind":"tool_calls","tool_calls":[{"name":"save_chapter","arguments":[1,2]}]}`),
	}}
	model := mustModel(t, transport)

	_, err := model.Generate(context.Background(), []agentcore.Message{agentcore.UserMsg("save")}, []agentcore.ToolSpec{testToolSpec()})
	if err == nil {
		t.Fatal("expected invalid tool arguments to remain rejected")
	}
	if got := len(transport.promptSnapshot()); got != 2 {
		t.Fatalf("round trips = %d, want bounded first attempt + one repair", got)
	}
}
