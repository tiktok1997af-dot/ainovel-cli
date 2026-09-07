package webai

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/voocel/agentcore"
)

func TestWebModelPromptUsesDOMMessageBoundaryWithoutOuterMarkers(t *testing.T) {
	transport := &fakeTransport{responses: []string{"TEXT\nhello"}}
	model := mustModel(t, transport)
	resp, err := model.Generate(context.Background(), []agentcore.Message{agentcore.UserMsg("hello")}, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got := resp.Message.TextContent(); got != "hello" {
		t.Fatalf("text = %q", got)
	}
	prompts := transport.promptSnapshot()
	if len(prompts) != 1 {
		t.Fatalf("round trips = %d, want 1", len(prompts))
	}
	prompt := prompts[0]
	if strings.Contains(prompt, responseStart) || strings.Contains(prompt, responseEnd) {
		t.Fatal("current Gemini WEB prompt must not request legacy outer response markers")
	}
	for _, want := range []string{"assistant-message boundary", "TEXT", rawToolCallPrefix, rawValueStart} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("current Gemini WEB prompt missing DOM-boundary contract marker %q", want)
		}
	}
}

func TestWebModelAcceptsDOMDelimitedToolBody(t *testing.T) {
	transport := &fakeTransport{responses: []string{
		`{"kind":"tool_calls","tool_calls":[{"name":"draft_chapter","arguments":{"chapter":1,"content":"short","mode":"write"}}]}`,
	}}
	model := mustModel(t, transport)
	resp, err := model.Generate(context.Background(), []agentcore.Message{agentcore.UserMsg("draft")}, []agentcore.ToolSpec{rawContentToolSpec()})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	calls := resp.Message.ToolCalls()
	if len(calls) != 1 || calls[0].Name != "draft_chapter" {
		t.Fatalf("unexpected tool calls: %+v", calls)
	}
}

func TestWebModelDOMBoundaryRepairStaysBoundedAndMarkerFree(t *testing.T) {
	transport := &fakeTransport{responses: []string{
		"not a protocol body",
		"still invalid",
		"also invalid",
	}}
	model := mustModel(t, transport)
	_, err := model.Generate(context.Background(), []agentcore.Message{agentcore.UserMsg("hello")}, nil)
	if !errors.Is(err, ErrProtocol) {
		t.Fatalf("err = %v, want ErrProtocol", err)
	}
	prompts := transport.promptSnapshot()
	if len(prompts) != 1+maxProtocolFormatRepairs {
		t.Fatalf("round trips = %d, want %d", len(prompts), 1+maxProtocolFormatRepairs)
	}
	for i, prompt := range prompts[1:] {
		if strings.Contains(prompt, responseStart) || strings.Contains(prompt, responseEnd) {
			t.Fatalf("repair prompt %d reintroduced legacy outer markers", i+1)
		}
		if !strings.Contains(prompt, "whole-message body") || !strings.Contains(prompt, rawToolCallPrefix) {
			t.Fatalf("repair prompt %d missing current DOM-boundary contract", i+1)
		}
	}
}

func TestWebModelPartialLegacyWrapperStillRepairsInsteadOfExecuting(t *testing.T) {
	transport := &fakeTransport{responses: []string{
		responseStart + "\n" + `{"kind":"tool_calls","tool_calls":[{"name":"draft_chapter","arguments":{"chapter":1,"content":"must not execute","mode":"write"}}]}`,
		"TEXT\nrepaired safely",
	}}
	model := mustModel(t, transport)
	resp, err := model.Generate(context.Background(), []agentcore.Message{agentcore.UserMsg("draft")}, []agentcore.ToolSpec{rawContentToolSpec()})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(resp.Message.ToolCalls()) != 0 || resp.Message.TextContent() != "repaired safely" {
		t.Fatalf("unexpected repaired response: %+v", resp.Message)
	}
	if len(transport.promptSnapshot()) != 2 {
		t.Fatal("partial legacy wrapper should require exactly one format repair")
	}
}
