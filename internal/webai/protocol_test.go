package webai

import (
	"errors"
	"strings"
	"testing"

	"github.com/voocel/agentcore"
)

func wrappedResponse(body string) string {
	return responseStart + "\n" + body + "\n" + responseEnd
}

func testToolSpec() agentcore.ToolSpec {
	return agentcore.ToolSpec{
		Name:        "save_chapter",
		Description: "save a chapter",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"chapter": map[string]any{"type": "integer"},
			},
			"required": []string{"chapter"},
		},
	}
}

func TestBuildPromptDoesNotLeakProviderCredentials(t *testing.T) {
	cfg := agentcore.CallConfig{
		APIKey:         "sk-legacy-must-never-reach-web",
		SessionID:      "provider-session-secret",
		PromptCacheKey: "provider-cache-secret",
		MaxTokens:      1234,
	}
	prompt, err := BuildPrompt([]agentcore.Message{agentcore.UserMsg("hello")}, []agentcore.ToolSpec{testToolSpec()}, cfg)
	if err != nil {
		t.Fatalf("BuildPrompt: %v", err)
	}
	for _, secret := range []string{cfg.APIKey, cfg.SessionID, cfg.PromptCacheKey} {
		if strings.Contains(prompt, secret) {
			t.Fatalf("web prompt leaked provider-only value %q", secret)
		}
	}
	if !strings.Contains(prompt, `"max_tokens":1234`) {
		t.Fatal("safe call projection should preserve max_tokens intent")
	}
	if !strings.Contains(prompt, `"name":"save_chapter"`) {
		t.Fatal("tool spec was not serialized into web request")
	}
}

func TestParseResponseText(t *testing.T) {
	msg, err := ParseResponse("request", wrappedResponse(`{"kind":"text","text":"chapter ready"}`), nil)
	if err != nil {
		t.Fatalf("ParseResponse: %v", err)
	}
	if msg.StopReason != agentcore.StopReasonStop {
		t.Fatalf("stop reason = %q", msg.StopReason)
	}
	if got := msg.TextContent(); got != "chapter ready" {
		t.Fatalf("text = %q", got)
	}
}

func TestParseResponseToolCallIsDeterministicAndRegistryBound(t *testing.T) {
	raw := wrappedResponse(`{"kind":"tool_calls","tool_calls":[{"name":"save_chapter","arguments":{"chapter":7}}]}`)
	tools := []agentcore.ToolSpec{testToolSpec()}
	first, err := ParseResponse("same-request", raw, tools)
	if err != nil {
		t.Fatalf("first ParseResponse: %v", err)
	}
	second, err := ParseResponse("same-request", raw, tools)
	if err != nil {
		t.Fatalf("second ParseResponse: %v", err)
	}
	calls := first.ToolCalls()
	if len(calls) != 1 {
		t.Fatalf("tool calls = %d", len(calls))
	}
	if calls[0].Name != "save_chapter" || string(calls[0].Args) != `{"chapter":7}` {
		t.Fatalf("unexpected tool call: %+v", calls[0])
	}
	if calls[0].ID == "" || calls[0].ID != second.ToolCalls()[0].ID {
		t.Fatalf("tool call ID must be stable for identical request/response: %q vs %q", calls[0].ID, second.ToolCalls()[0].ID)
	}
	if first.StopReason != agentcore.StopReasonToolUse {
		t.Fatalf("stop reason = %q", first.StopReason)
	}
}

func TestParseResponseRejectsUnknownTool(t *testing.T) {
	_, err := ParseResponse("request", wrappedResponse(`{"kind":"tool_calls","tool_calls":[{"name":"shell","arguments":{}}]}`), []agentcore.ToolSpec{testToolSpec()})
	if !errors.Is(err, ErrProtocol) {
		t.Fatalf("expected ErrProtocol, got %v", err)
	}
}

func TestParseResponseRejectsNonObjectToolArguments(t *testing.T) {
	_, err := ParseResponse("request", wrappedResponse(`{"kind":"tool_calls","tool_calls":[{"name":"save_chapter","arguments":[1,2]}]}`), []agentcore.ToolSpec{testToolSpec()})
	if !errors.Is(err, ErrProtocol) {
		t.Fatalf("expected ErrProtocol, got %v", err)
	}
}

func TestParseResponseRejectsContentOutsideEnvelope(t *testing.T) {
	_, err := ParseResponse("request", "commentary\n"+wrappedResponse(`{"kind":"text","text":"x"}`), nil)
	if !errors.Is(err, ErrProtocol) {
		t.Fatalf("expected ErrProtocol, got %v", err)
	}
}

func TestParseResponseRejectsUnknownJSONFields(t *testing.T) {
	_, err := ParseResponse("request", wrappedResponse(`{"kind":"text","text":"x","surprise":true}`), nil)
	if !errors.Is(err, ErrProtocol) {
		t.Fatalf("expected ErrProtocol, got %v", err)
	}
}
