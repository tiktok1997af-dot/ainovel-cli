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
	Protocol string         `json:"protocol"`
	Messages []wireMessage  `json:"messages"`
	Tools    []wireToolSpec `json:"tools,omitempty"`
	Call     callProjection `json:"call,omitempty"`
}

type wireMessage struct {
	Role       agentcore.Role        `json:"role"`
	Text       string                `json:"text,omitempty"`
	ToolCalls  []wireHistoryToolCall `json:"tool_calls,omitempty"`
	ToolCallID string                `json:"tool_call_id,omitempty"`
	ToolName   string                `json:"tool_name,omitempty"`
	IsError    bool                  `json:"is_error,omitempty"`
}

type wireHistoryToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type wireToolSpec struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}

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

func BuildPrompt(messages []agentcore.Message, tools []agentcore.ToolSpec, cfg agentcore.CallConfig) (string, error) {
	if err := agentcore.AssertMessageSequence(messages); err != nil {
		return "", protocolError("validate message sequence", err)
	}
	projectedMessages, err := projectMessages(messages)
	if err != nil {
		return "", err
	}
	projectedTools, err := projectTools(tools)
	if err != nil {
		return "", err
	}
	payload := requestPayload{
		Protocol: protocolVersion,
		Messages: projectedMessages,
		Tools:    projectedTools,
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
	seenToolCallIDs := make(map[string]struct{})
	for i, msg := range messages {
		if !supportedRole(msg.Role) {
			return nil, protocolError("project messages", fmt.Errorf("message %d has unsupported role %q", i, msg.Role))
		}
		projected := wireMessage{Role: msg.Role}
		for j, block := range msg.Content {
			switch block.Type {
			case agentcore.ContentText:
				projected.Text += block.Text
			case agentcore.ContentThinking:
				continue
			case agentcore.ContentToolCall:
				if msg.Role != agentcore.RoleAssistant {
					return nil, protocolError("project messages", fmt.Errorf("message %d block %d has tool call on role %q", i, j, msg.Role))
				}
				if block.ToolCall == nil {
					return nil, protocolError("project messages", fmt.Errorf("message %d block %d has nil tool call", i, j))
				}
				call := *block.ToolCall
				if strings.TrimSpace(call.ID) == "" || strings.TrimSpace(call.Name) == "" {
					return nil, protocolError("project messages", fmt.Errorf("message %d contains tool call without id/name", i))
				}
				if call.ID != strings.TrimSpace(call.ID) || call.Name != strings.TrimSpace(call.Name) {
					return nil, protocolError("project messages", fmt.Errorf("message %d contains tool call with surrounding whitespace in id/name", i))
				}
				if call.ArgsInvalid {
					return nil, protocolError("project messages", fmt.Errorf("message %d tool %q contains malformed historical arguments", i, call.Name))
				}
				if _, exists := seenToolCallIDs[call.ID]; exists {
					return nil, protocolError("project messages", fmt.Errorf("duplicate historical tool_call_id %q", call.ID))
				}
				if err := validateJSONObject(call.Args); err != nil {
					return nil, protocolError("project messages", fmt.Errorf("message %d tool %q arguments: %w", i, call.Name, err))
				}
				seenToolCallIDs[call.ID] = struct{}{}
				projected.ToolCalls = append(projected.ToolCalls, wireHistoryToolCall{ID: call.ID, Name: call.Name, Arguments: append(json.RawMessage(nil), call.Args...)})
			case agentcore.ContentImage, agentcore.ContentToolRef:
				return nil, protocolError("project messages", fmt.Errorf("message %d block %d uses unsupported web content type %q", i, j, block.Type))
			default:
				return nil, protocolError("project messages", fmt.Errorf("message %d block %d has unknown content type %q", i, j, block.Type))
			}
		}
		if projected.Text == "" && len(projected.ToolCalls) == 0 {
			return nil, protocolError("project messages", fmt.Errorf("message %d has no web-transferable content after projection", i))
		}
		if msg.Role == agentcore.RoleTool {
			projected.ToolCallID, _ = msg.Metadata["tool_call_id"].(string)
			projected.ToolName, _ = msg.Metadata["tool_name"].(string)
			projected.IsError, _ = msg.Metadata["is_error"].(bool)
			if strings.TrimSpace(projected.ToolCallID) == "" || projected.ToolCallID != strings.TrimSpace(projected.ToolCallID) {
				return nil, protocolError("project messages", fmt.Errorf("tool result message %d has invalid tool_call_id", i))
			}
			if projected.ToolName != "" && projected.ToolName != strings.TrimSpace(projected.ToolName) {
				return nil, protocolError("project messages", fmt.Errorf("tool result message %d has invalid tool_name", i))
			}
		}
		out = append(out, projected)
	}
	return out, nil
}

func supportedRole(role agentcore.Role) bool {
	switch role {
	case agentcore.RoleSystem, agentcore.RoleUser, agentcore.RoleAssistant, agentcore.RoleTool:
		return true
	default:
		return false
	}
}

func projectTools(tools []agentcore.ToolSpec) ([]wireToolSpec, error) {
	if err := validateToolRegistry(tools); err != nil {
		return nil, err
	}
	out := make([]wireToolSpec, 0, len(tools))
	for _, tool := range tools {
		out = append(out, wireToolSpec{Name: tool.Name, Description: tool.Description, Parameters: tool.Parameters})
	}
	return out, nil
}

func validateToolRegistry(tools []agentcore.ToolSpec) error {
	seen := make(map[string]struct{}, len(tools))
	for i, tool := range tools {
		name := strings.TrimSpace(tool.Name)
		if name == "" {
			return protocolError("validate tools", fmt.Errorf("tool %d has empty name", i))
		}
		if name != tool.Name {
			return protocolError("validate tools", fmt.Errorf("tool %d name %q has surrounding whitespace", i, tool.Name))
		}
		if _, exists := seen[name]; exists {
			return protocolError("validate tools", fmt.Errorf("duplicate tool name %q", name))
		}
		seen[name] = struct{}{}
	}
	return nil
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

func ParseResponse(requestPrompt, raw string, tools []agentcore.ToolSpec) (agentcore.Message, error) {
	if err := validateToolRegistry(tools); err != nil {
		return agentcore.Message{}, err
	}
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
		return agentcore.Message{Role: agentcore.RoleAssistant, Content: []agentcore.ContentBlock{agentcore.TextBlock(env.Text)}, StopReason: agentcore.StopReasonStop, Timestamp: time.Now()}, nil
	case "tool_calls":
		if env.Text != "" {
			return agentcore.Message{}, protocolError("validate response", fmt.Errorf("tool_calls response must not contain text"))
		}
		if len(env.ToolCalls) == 0 || len(env.ToolCalls) > maxToolCalls {
			return agentcore.Message{}, protocolError("validate response", fmt.Errorf("tool_calls count must be between 1 and %d", maxToolCalls))
		}
		allowed := make(map[string]struct{}, len(tools))
		for _, tool := range tools {
			allowed[tool.Name] = struct{}{}
		}
		blocks := make([]agentcore.ContentBlock, 0, len(env.ToolCalls))
		for i, call := range env.ToolCalls {
			name := strings.TrimSpace(call.Name)
			if name == "" {
				return agentcore.Message{}, protocolError("validate response", fmt.Errorf("tool call %d has empty name", i))
			}
			if name != call.Name {
				return agentcore.Message{}, protocolError("validate response", fmt.Errorf("tool call %d name %q has surrounding whitespace", i, call.Name))
			}
			if _, ok := allowed[name]; !ok {
				return agentcore.Message{}, protocolError("validate response", fmt.Errorf("tool %q is not available in this request", name))
			}
			if err := validateJSONObject(call.Arguments); err != nil {
				return agentcore.Message{}, protocolError("validate response", fmt.Errorf("tool %q arguments: %w", name, err))
			}
			blocks = append(blocks, agentcore.ToolCallBlock(agentcore.ToolCall{ID: stableToolCallID(requestPrompt, raw, i, name), Name: name, Args: append(json.RawMessage(nil), call.Arguments...)}))
		}
		return agentcore.Message{Role: agentcore.RoleAssistant, Content: blocks, StopReason: agentcore.StopReasonToolUse, Timestamp: time.Now()}, nil
	default:
		return agentcore.Message{}, protocolError("validate response", fmt.Errorf("unknown response kind %q", env.Kind))
	}
}

func validateJSONObject(raw json.RawMessage) error {
	if strings.TrimSpace(string(raw)) == "" {
		return fmt.Errorf("arguments are empty")
	}
	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil {
		return err
	}
	if object == nil {
		return fmt.Errorf("arguments must be a JSON object")
	}
	return nil
}

// extractEnvelope accepts the canonical single envelope and exactly one narrow
// browser-formatting quirk: an otherwise empty outer envelope may contain one
// complete duplicate response envelope. Framing is resolved from the outermost
// prefix/suffix first, so the inner end marker cannot be mistaken for the outer
// end marker. Mixed content, deeper nesting, and commentary outside remain hard
// protocol errors.
func extractEnvelope(raw string) (string, error) {
	body, err := stripOutermostEnvelope(raw)
	if err != nil {
		return "", err
	}
	if !strings.Contains(body, responseStart) && !strings.Contains(body, responseEnd) {
		return body, nil
	}

	inner, err := stripOutermostEnvelope(body)
	if err != nil {
		return "", protocolError("extract response", fmt.Errorf("nested response marker"))
	}
	if strings.Contains(inner, responseStart) || strings.Contains(inner, responseEnd) {
		return "", protocolError("extract response", fmt.Errorf("nested response marker depth exceeds one redundant wrapper"))
	}
	return inner, nil
}

func stripOutermostEnvelope(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(trimmed, responseStart) {
		if strings.Contains(trimmed, responseStart) {
			return "", protocolError("extract response", fmt.Errorf("unexpected content before start marker"))
		}
		return "", protocolError("extract response", fmt.Errorf("missing start marker"))
	}
	if !strings.HasSuffix(trimmed, responseEnd) {
		if strings.Contains(trimmed, responseEnd) {
			return "", protocolError("extract response", fmt.Errorf("unexpected content after end marker"))
		}
		return "", protocolError("extract response", fmt.Errorf("missing end marker"))
	}
	body := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(trimmed, responseStart), responseEnd))
	if body == "" {
		return "", protocolError("extract response", fmt.Errorf("empty response envelope"))
	}
	return body, nil
}

func stableToolCallID(requestPrompt, raw string, index int, name string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%d\x00%s", requestPrompt, raw, index, name)))
	return "web-" + hex.EncodeToString(sum[:8])
}
