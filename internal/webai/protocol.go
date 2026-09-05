package webai

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/voocel/agentcore"
)

const (
	protocolVersion = "ainovel-web/1"
	responseStart   = "<<<AINOVEL_WEB_RESPONSE>>>"
	responseEnd     = "<<<END_AINOVEL_WEB_RESPONSE>>>"
	maxToolCalls    = 8
)

type requestPayload struct {
	Protocol string               `json:"protocol"`
	Messages []wireMessage        `json:"messages"`
	Tools    []agentcore.ToolSpec `json:"tools,omitempty"`
	Call     callProjection       `json:"call,omitempty"`
}

// wireMessage is the minimum conversation state allowed to cross the browser
// boundary. Provider telemetry, timestamps and arbitrary Message.Metadata are
// deliberately excluded. Tool-result correlation keeps only the three fields
// required by the local agent loop transcript.
type wireMessage struct {
	Role       agentcore.Role       `json:"role"`
	Text       string               `json:"text,omitempty"`
	ToolCalls  []wireHistoryToolCall `json:"tool_calls,omitempty"`
	ToolCallID string               `json:"tool_call_id,omitempty"`
	ToolName   string               `json:"tool_name,omitempty"`
	IsError    bool                 `json:"is_error,omitempty"`
}

type wireHistoryToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// callProjection intentionally excludes API keys, cache routing identifiers and
// other provider-only transport fields. A browser prompt must never become a
// side channel for credentials inherited from legacy call options.
type callProjection struct {
	ThinkingLevel  agentcore.ThinkingLevel   `json:"thinking_level,omitempty"`
	ThinkingBudget int                       `json:"thinking_budget,omitempty"`
	MaxTokens      int                       `json:"max_tokens,omitempty"`
	ToolChoice     string                    `json:"tool_choice,omitempty"`
	ResponseFormat *agentcore.ResponseFormat `json:"response_format,omitempty"`
}

type responseEnvelope struct {
	Kind      string         `json:"kind"`
	Text      string         `json:"text,omitempty"`
	ToolCalls []wireToolCall `json:"tool_calls,omitempty"`
}

type wireToolCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

const protocolInstruction = `You are the AI backend for ainovel-cli. The browser is only a transport layer.
Return exactly one machine-readable envelope and nothing else.

For a normal assistant answer:
<<<AINOVEL_WEB_RESPONSE>>>
{"kind":"text","text":"your complete answer"}
<<<END_AINOVEL_WEB_RESPONSE>>>

When a local tool is required:
<<<AINOVEL_WEB_RESPONSE>>>
{"kind":"tool_calls","tool_calls":[{"name":"exact_tool_name","arguments":{}}]}
<<<END_AINOVEL_WEB_RESPONSE>>>

Rules:
- Use only tool names present in the request.
- Tool arguments must be one JSON object matching that tool schema.
- Do not claim that a tool ran; the local ainovel runtime executes tools after validating the call.
- Do not write Markdown fences around the envelope.
- Do not emit commentary outside the response markers.
- A response is either text or tool_calls, never both.`

// BuildPrompt serializes one agentcore model request into a deterministic text
// payload suitable for submission through a logged-in web conversation.
func BuildPrompt(messages []agentcore.Message, tools []agentcore.ToolSpec, cfg agentcore.CallConfig) (string, error) {
	projected, err := projectMessages(messages)
	if err != nil {
		return "", err
	}
	payload := requestPayload{
		Protocol: protocolVersion,
		Messages: projected,
		Tools:    tools,
		Call: callProjection{
			ThinkingLevel:  cfg.ThinkingLevel,
			ThinkingBudget: cfg.ThinkingBudget,
			MaxTokens:      cfg.MaxTokens,
			ToolChoice:     portableToolChoice(cfg.ToolChoice),
			ResponseFormat: cfg.ResponseFormat,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", protocolError("encode request", err)
	}
	return protocolInstruction + "\n\n<<<AINOVEL_WEB_REQUEST>>>\n" + string(body) + "\n<<<END_AINOVEL_WEB_REQUEST>>>", nil
}

func projectMessages(messages []agentcore.Message) ([]wireMessage, error) {
	out := make([]wireMessage, 0, len(messages))
	for i, msg := range messages {
		projected := wireMessage{Role: msg.Role, Text: msg.TextContent()}
		for _, call := range msg.ToolCalls() {
			if strings.TrimSpace(call.ID) == "" || strings.TrimSpace(call.Name) == "" {
				return nil, protocolError("project messages", fmt.Errorf("message %d contains tool call without id/name", i))
			}
			projected.ToolCalls = append(projected.ToolCalls, wireHistoryToolCall{
				ID:        call.ID,
				Name:      call.Name,
				Arguments: append(json.RawMessage(nil), call.Args...),
			})
		}
		if msg.Role == agentcore.RoleTool {
			projected.ToolCallID, _ = msg.Metadata["tool_call_id"].(string)
			projected.ToolName, _ = msg.Metadata["tool_name"].(string)
			projected.IsError, _ = msg.Metadata["is_error"].(bool)
			if strings.TrimSpace(projected.ToolCallID) == "" {
				return nil, protocolError("project messages", fmt.Errorf("tool result message %d is missing tool_call_id", i))
			}
		}
		out = append(out, projected)
	}
	return out, nil
}

func portableToolChoice(choice any) string {
	value, ok := choice.(string)
	if !ok {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "auto", "required", "none":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

// ParseResponse validates a captured web answer and converts it to the native
// agentcore message shape. Tool schema validation itself remains enforced by
// agentcore immediately before local execution; this layer additionally rejects
// unknown tool names and non-object JSON arguments before they enter the loop.
func ParseResponse(requestPrompt, raw string, tools []agentcore.ToolSpec) (agentcore.Message, error) {
	payload, err := extractEnvelope(raw)
	if err != nil {
		return agentcore.Message{}, err
	}

	var env responseEnvelope
	dec := json.NewDecoder(strings.NewReader(payload))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&env); err != nil {
		return agentcore.Message{}, protocolError("decode response", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("multiple JSON values in response envelope")
		}
		return agentcore.Message{}, protocolError("decode response", err)
	}

	switch env.Kind {
	case "text":
		if len(env.ToolCalls) != 0 {
			return agentcore.Message{}, protocolError("validate response", fmt.Errorf("text response must not contain tool_calls"))
		}
		if strings.TrimSpace(env.Text) == "" {
			return agentcore.Message{}, protocolError("validate response", fmt.Errorf("text response is empty"))
		}
		return agentcore.Message{
			Role:       agentcore.RoleAssistant,
			Content:    []agentcore.ContentBlock{agentcore.TextBlock(env.Text)},
			StopReason: agentcore.StopReasonStop,
			Timestamp:  time.Now(),
		}, nil

	case "tool_calls":
		if env.Text != "" {
			return agentcore.Message{}, protocolError("validate response", fmt.Errorf("tool_calls response must not contain text"))
		}
		if len(env.ToolCalls) == 0 || len(env.ToolCalls) > maxToolCalls {
			return agentcore.Message{}, protocolError("validate response", fmt.Errorf("tool_calls count must be between 1 and %d", maxToolCalls))
		}
		allowed := make(map[string]agentcore.ToolSpec, len(tools))
		for _, tool := range tools {
			allowed[tool.Name] = tool
		}
		blocks := make([]agentcore.ContentBlock, 0, len(env.ToolCalls))
		for i, call := range env.ToolCalls {
			name := strings.TrimSpace(call.Name)
			if name == "" {
				return agentcore.Message{}, protocolError("validate response", fmt.Errorf("tool call %d has empty name", i))
			}
			if _, ok := allowed[name]; !ok {
				return agentcore.Message{}, protocolError("validate response", fmt.Errorf("tool %q is not available in this request", name))
			}
			args := strings.TrimSpace(string(call.Arguments))
			if args == "" {
				return agentcore.Message{}, protocolError("validate response", fmt.Errorf("tool %q has empty arguments", name))
			}
			var object map[string]any
			if err := json.Unmarshal(call.Arguments, &object); err != nil || object == nil {
				if err == nil {
					err = fmt.Errorf("arguments must be a JSON object")
				}
				return agentcore.Message{}, protocolError("validate response", fmt.Errorf("tool %q arguments: %w", name, err))
			}
			blocks = append(blocks, agentcore.ToolCallBlock(agentcore.ToolCall{
				ID:   stableToolCallID(requestPrompt, raw, i, name),
				Name: name,
				Args: append(json.RawMessage(nil), call.Arguments...),
			}))
		}
		return agentcore.Message{
			Role:       agentcore.RoleAssistant,
			Content:    blocks,
			StopReason: agentcore.StopReasonToolUse,
			Timestamp:  time.Now(),
		}, nil
	default:
		return agentcore.Message{}, protocolError("validate response", fmt.Errorf("unknown response kind %q", env.Kind))
	}
}

func extractEnvelope(raw string) (string, error) {
	start := strings.Index(raw, responseStart)
	if start < 0 {
		return "", protocolError("extract response", fmt.Errorf("missing start marker"))
	}
	if strings.TrimSpace(raw[:start]) != "" {
		return "", protocolError("extract response", fmt.Errorf("unexpected content before start marker"))
	}
	bodyStart := start + len(responseStart)
	endRel := strings.Index(raw[bodyStart:], responseEnd)
	if endRel < 0 {
		return "", protocolError("extract response", fmt.Errorf("missing end marker"))
	}
	end := bodyStart + endRel
	if strings.TrimSpace(raw[end+len(responseEnd):]) != "" {
		return "", protocolError("extract response", fmt.Errorf("unexpected content after end marker"))
	}
	if strings.Contains(raw[bodyStart:end], responseStart) || strings.Contains(raw[bodyStart:end], responseEnd) {
		return "", protocolError("extract response", fmt.Errorf("nested response marker"))
	}
	body := strings.TrimSpace(raw[bodyStart:end])
	if body == "" {
		return "", protocolError("extract response", fmt.Errorf("empty response envelope"))
	}
	return body, nil
}

func stableToolCallID(requestPrompt, raw string, index int, name string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%d\x00%s", requestPrompt, raw, index, name)))
	return "web-" + hex.EncodeToString(sum[:8])
}
