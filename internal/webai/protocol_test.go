package webai

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/voocel/agentcore"
)

func wrappedResponse(body string) string {
	return responseStart + "\n" + body + "\n" + responseEnd
}

func testToolSpec() agentcore.ToolSpec {
	strict := true
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
		DeferLoading: true,
		Strict:       &strict,
	}
}

func TestBuildPromptDoesNotLeakProviderCredentials(t *testing.T) {
	cfg := agentcore.CallConfig{
		APIKey:         "sk-legacy-must-never-reach-web",
		SessionID:      "provider-session-secret",
		PromptCacheKey: "provider-cache-secret",
		MaxTokens:      1234,
		ToolChoice:     map[string]any{"provider_selector_secret": "do-not-send"},
	}
	prompt, err := BuildPrompt([]agentcore.Message{agentcore.UserMsg("hello")}, []agentcore.ToolSpec{testToolSpec()}, cfg)
	if err != nil {
		t.Fatalf("BuildPrompt: %v", err)
	}
	for _, secret := range []string{cfg.APIKey, cfg.SessionID, cfg.PromptCacheKey, "provider_selector_secret", "do-not-send"} {
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

func TestBuildPromptStripsMessageTelemetryAndInternalMetadata(t *testing.T) {
	assistant := agentcore.Message{
		Role: agentcore.RoleAssistant,
		Content: []agentcore.ContentBlock{
			agentcore.TextBlock("calling local tool"),
			agentcore.ThinkingBlock("private-provider-thinking-secret"),
			agentcore.ToolCallBlock(agentcore.ToolCall{
				ID:               "tc-1",
				Name:             "save_chapter",
				Args:             json.RawMessage(`{"chapter":9}`),
				ThoughtSignature: "provider-thought-signature-secret",
			}),
		},
		Usage: &agentcore.Usage{Provider: "provider-telemetry-secret", Model: "legacy-api-model"},
		Metadata: map[string]any{
			"internal_secret": "message-metadata-secret",
		},
	}
	toolResult := agentcore.ToolResultMsg("tc-1", json.RawMessage(`{"saved":true}`), false)
	toolResult.Metadata["tool_name"] = "save_chapter"
	toolResult.Metadata["internal_secret"] = "tool-metadata-secret"
	toolResult.Usage = &agentcore.Usage{Provider: "tool-provider-secret"}

	prompt, err := BuildPrompt([]agentcore.Message{assistant, toolResult}, []agentcore.ToolSpec{testToolSpec()}, agentcore.CallConfig{})
	if err != nil {
		t.Fatalf("BuildPrompt: %v", err)
	}
	for _, secret := range []string{
		"provider-telemetry-secret",
		"legacy-api-model",
		"message-metadata-secret",
		"tool-metadata-secret",
		"tool-provider-secret",
		"private-provider-thinking-secret",
		"provider-thought-signature-secret",
	} {
		if strings.Contains(prompt, secret) {
			t.Fatalf("web prompt leaked internal/provider message data %q", secret)
		}
	}
	for _, required := range []string{`"id":"tc-1"`, `"name":"save_chapter"`, `"tool_call_id":"tc-1"`, `"saved":true`} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("web prompt lost required local tool transcript field %s", required)
		}
	}
}

func TestBuildPromptStripsProviderToolFlags(t *testing.T) {
	prompt, err := BuildPrompt([]agentcore.Message{agentcore.UserMsg("hello")}, []agentcore.ToolSpec{testToolSpec()}, agentcore.CallConfig{})
	if err != nil {
		t.Fatalf("BuildPrompt: %v", err)
	}
	if strings.Contains(prompt, `"defer_loading"`) || strings.Contains(prompt, `"strict"`) {
		t.Fatal("provider/local tool flags must not cross the browser boundary")
	}
	if !strings.Contains(prompt, `"parameters"`) {
		t.Fatal("tool JSON schema must remain available to the web model")
	}
}

func TestBuildPromptKeepsPortableToolChoiceOnly(t *testing.T) {
	for _, value := range []string{"auto", "required", "none"} {
		prompt, err := BuildPrompt(nil, nil, agentcore.CallConfig{ToolChoice: value})
		if err != nil {
			t.Fatalf("BuildPrompt(%s): %v", value, err)
		}
		if !strings.Contains(prompt, `"tool_choice":"`+value+`"`) {
			t.Fatalf("portable tool choice %q was not projected", value)
		}
	}
}

func TestBuildPromptRejectsToolResultWithoutCorrelationID(t *testing.T) {
	msg := agentcore.Message{Role: agentcore.RoleTool, Content: []agentcore.ContentBlock{agentcore.TextBlock(`{"ok":true}`)}}
	_, err := BuildPrompt([]agentcore.Message{msg}, nil, agentcore.CallConfig{})
	if !errors.Is(err, ErrProtocol) {
		t.Fatalf("expected ErrProtocol, got %v", err)
	}
}

func TestBuildPromptRejectsInvalidMessageSequence(t *testing.T) {
	assistant := agentcore.Message{
		Role: agentcore.RoleAssistant,
		Content: []agentcore.ContentBlock{
			agentcore.ToolCallBlock(agentcore.ToolCall{ID: "tc-orphan", Name: "save_chapter", Args: json.RawMessage(`{"chapter":1}`)}),
		},
	}
	_, err := BuildPrompt([]agentcore.Message{assistant}, []agentcore.ToolSpec{testToolSpec()}, agentcore.CallConfig{})
	if !errors.Is(err, ErrProtocol) {
		t.Fatalf("expected ErrProtocol for incomplete tool transcript, got %v", err)
	}
}

func TestBuildPromptRejectsUnsupportedRoleAndContent(t *testing.T) {
	_, err := BuildPrompt([]agentcore.Message{{Role: agentcore.Role("developer"), Content: []agentcore.ContentBlock{agentcore.TextBlock("x")}}}, nil, agentcore.CallConfig{})
	if !errors.Is(err, ErrProtocol) {
		t.Fatalf("expected unsupported role protocol error, got %v", err)
	}

	_, err = BuildPrompt([]agentcore.Message{{Role: agentcore.RoleUser, Content: []agentcore.ContentBlock{agentcore.ImageURLBlock("https://example.invalid/image.png")}}}, nil, agentcore.CallConfig{})
	if !errors.Is(err, ErrProtocol) {
		t.Fatalf("expected unsupported image protocol error, got %v", err)
	}
}

func TestBuildPromptRejectsThinkingOnlyMessageAfterProjection(t *testing.T) {
	msg := agentcore.Message{Role: agentcore.RoleAssistant, Content: []agentcore.ContentBlock{agentcore.ThinkingBlock("provider-only reasoning")}}
	_, err := BuildPrompt([]agentcore.Message{msg}, nil, agentcore.CallConfig{})
	if !errors.Is(err, ErrProtocol) {
		t.Fatalf("expected empty-after-projection protocol error, got %v", err)
	}
}

func TestBuildPromptRejectsMalformedOrDuplicateHistoricalToolCalls(t *testing.T) {
	invalid := agentcore.Message{
		Role: agentcore.RoleAssistant,
		Content: []agentcore.ContentBlock{agentcore.ToolCallBlock(agentcore.ToolCall{
			ID: "tc-bad", Name: "save_chapter", Args: json.RawMessage(`{}`), ArgsInvalid: true,
		})},
	}
	_, err := BuildPrompt([]agentcore.Message{invalid, agentcore.ToolResultMsg("tc-bad", json.RawMessage(`"bad"`), true)}, []agentcore.ToolSpec{testToolSpec()}, agentcore.CallConfig{})
	if !errors.Is(err, ErrProtocol) {
		t.Fatalf("expected malformed historical tool-call protocol error, got %v", err)
	}

	first := agentcore.Message{Role: agentcore.RoleAssistant, Content: []agentcore.ContentBlock{agentcore.ToolCallBlock(agentcore.ToolCall{ID: "tc-dup", Name: "save_chapter", Args: json.RawMessage(`{"chapter":1}`)})}}
	firstResult := agentcore.ToolResultMsg("tc-dup", json.RawMessage(`{"saved":true}`), false)
	second := agentcore.Message{Role: agentcore.RoleAssistant, Content: []agentcore.ContentBlock{agentcore.ToolCallBlock(agentcore.ToolCall{ID: "tc-dup", Name: "save_chapter", Args: json.RawMessage(`{"chapter":2}`)})}}
	secondResult := agentcore.ToolResultMsg("tc-dup", json.RawMessage(`{"saved":true}`), false)
	_, err = BuildPrompt([]agentcore.Message{first, firstResult, second, secondResult}, []agentcore.ToolSpec{testToolSpec()}, agentcore.CallConfig{})
	if !errors.Is(err, ErrProtocol) {
		t.Fatalf("expected duplicate historical tool_call_id protocol error, got %v", err)
	}
}

func TestBuildPromptRejectsAmbiguousToolRegistry(t *testing.T) {
	duplicate := []agentcore.ToolSpec{testToolSpec(), testToolSpec()}
	_, err := BuildPrompt(nil, duplicate, agentcore.CallConfig{})
	if !errors.Is(err, ErrProtocol) {
		t.Fatalf("expected duplicate tool registry protocol error, got %v", err)
	}

	bad := testToolSpec()
	bad.Name = " save_chapter "
	_, err = BuildPrompt(nil, []agentcore.ToolSpec{bad}, agentcore.CallConfig{})
	if !errors.Is(err, ErrProtocol) {
		t.Fatalf("expected whitespace tool-name protocol error, got %v", err)
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

func TestParseResponseRejectsWhitespaceToolNameAndAmbiguousRegistry(t *testing.T) {
	_, err := ParseResponse("request", wrappedResponse(`{"kind":"tool_calls","tool_calls":[{"name":" save_chapter ","arguments":{"chapter":7}}]}`), []agentcore.ToolSpec{testToolSpec()})
	if !errors.Is(err, ErrProtocol) {
		t.Fatalf("expected whitespace tool-name protocol error, got %v", err)
	}

	_, err = ParseResponse("request", wrappedResponse(`{"kind":"text","text":"x"}`), []agentcore.ToolSpec{testToolSpec(), testToolSpec()})
	if !errors.Is(err, ErrProtocol) {
		t.Fatalf("expected ambiguous registry protocol error, got %v", err)
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
