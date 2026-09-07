package webai

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/voocel/agentcore"
	"github.com/voocel/agentcore/subagent"
)

func TestModelToolRequiredMarkerRepairsTextIntoLocalToolCall(t *testing.T) {
	var executed atomic.Int32
	tool := agentcore.NewFuncTool("save", "save locally", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"value": map[string]any{"type": "string"},
		},
		"required":             []string{"value"},
		"additionalProperties": false,
	}, func(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
		executed.Add(1)
		if string(args) != `{"value":"ok"}` {
			t.Fatalf("unexpected tool args: %s", args)
		}
		return json.RawMessage(`{"saved":true}`), nil
	})

	transport := &fakeTransport{responses: []string{
		"TEXT\nI will save next.",
		"TEXT\nI should save now.",
		`{"kind":"tool_calls","tool_calls":[{"name":"save","arguments":{"value":"ok"}}]}`,
	}}
	model := mustModel(t, transport)
	runner := subagent.NewRunner(subagent.Config{
		Name:           "writer",
		Description:    "tool-required recovery",
		Model:          model,
		SystemPrompt:   "test",
		Tools:          []agentcore.Tool{tool},
		MaxTurns:       5,
		StopAfterTools: []string{"save"},
		StopGuardFactory: func(_, _ string) agentcore.StopGuard {
			return func(_ context.Context, _ agentcore.StopInfo) agentcore.StopDecision {
				if executed.Load() > 0 {
					return agentcore.StopDecision{Allow: true}
				}
				return agentcore.StopDecision{
					Allow:         false,
					InjectMessage: localToolRequiredMarker + "\nCall save now.",
				}
			}
		},
	})

	if _, err := runner.Run(context.Background(), "writer", "persist the artifact"); err != nil {
		t.Fatalf("Runner.Run: %v", err)
	}
	if got := executed.Load(); got != 1 {
		t.Fatalf("local tool executions = %d, want 1", got)
	}

	prompts := transport.promptSnapshot()
	if len(prompts) != 3 {
		t.Fatalf("web round trips = %d, want 3", len(prompts))
	}
	if !strings.Contains(prompts[1], localToolRequiredMarker) {
		t.Fatalf("blocked worker turn did not carry marker: %q", prompts[1])
	}
	if !strings.Contains(prompts[2], localToolRequiredMarker) ||
		!strings.Contains(prompts[2], "Do not") ||
		!strings.Contains(prompts[2], "TEXT") {
		t.Fatalf("tool-required repair prompt is not explicit enough: %q", prompts[2])
	}
}

func TestModelToolRequiredMarkerWithoutToolsKeepsTextValid(t *testing.T) {
	transport := &fakeTransport{responses: []string{"TEXT\nplain response"}}
	model := mustModel(t, transport)
	resp, err := model.Generate(
		context.Background(),
		[]agentcore.Message{agentcore.UserMsg(localToolRequiredMarker + "\nno tools available")},
		nil,
	)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got := resp.Message.TextContent(); got != "plain response" {
		t.Fatalf("text = %q, want plain response", got)
	}
	if got := len(transport.promptSnapshot()); got != 1 {
		t.Fatalf("web round trips = %d, want 1", got)
	}
}
