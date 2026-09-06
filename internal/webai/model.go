package webai

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/voocel/agentcore"
	"github.com/voocel/agentcore/llm"
)

// Transport is the browser/site boundary used by WebChatModel. W1 keeps it
// abstract so protocol behavior can be proven deterministically before any
// live DOM automation is introduced.
type Transport interface {
	RoundTrip(ctx context.Context, prompt string) (string, error)
}

// ModelConfig identifies a web-backed model/session endpoint.
type ModelConfig struct {
	Site      string
	Model     string
	Transport Transport
}

// Model implements agentcore.ChatModel over a browser/web transport.
type Model struct {
	site      string
	model     string
	transport Transport
}

var (
	_ agentcore.ChatModel     = (*Model)(nil)
	_ agentcore.ProviderNamer = (*Model)(nil)
	_ agentcore.ModelNamer    = (*Model)(nil)
	_ llm.CapabilityProvider  = (*Model)(nil)
)

// rawTextProtocolPrompt is appended after the legacy ainovel-web/1 instruction.
// It removes the need to JSON-escape arbitrary assistant text (including a JSON
// object requested by a higher-level prompt contract) while keeping tool calls
// on the strict, locally validated JSON path. Legacy JSON text envelopes remain
// accepted for backward compatibility.
const rawTextProtocolPrompt = `AINOVEL WEB RESPONSE EXTENSION — this instruction takes precedence for normal assistant text.
For every normal assistant answer, do NOT put the answer inside a JSON string. Return exactly:
<<<AINOVEL_WEB_RESPONSE>>>
TEXT
<the complete answer verbatim>
<<<END_AINOVEL_WEB_RESPONSE>>>
The raw TEXT body may itself be JSON, Markdown, prose, quotes, or multiple lines; preserve it verbatim and do not add Markdown fences around the response markers.
When a local tool is required, continue to use the strict JSON tool_calls envelope from the ainovel-web/1 instruction. Never represent a tool request as TEXT.`

const protocolRepairPrompt = `Your immediately previous answer could not be parsed as an ainovel-web/1 response. No local tool from that answer has been executed.
Do not redo the user's task and do not add commentary. Re-emit the same intended answer once, correcting only the transport format.
If the intended answer is normal assistant text, return exactly:
<<<AINOVEL_WEB_RESPONSE>>>
TEXT
<the complete previous answer verbatim; it may itself be JSON, Markdown, prose, quotes, or multiple lines>
<<<END_AINOVEL_WEB_RESPONSE>>>
Do not JSON-escape or wrap a normal text answer in a {"kind":"text"} object.
If the intended answer is a local tool request, return exactly:
<<<AINOVEL_WEB_RESPONSE>>>
{"kind":"tool_calls","tool_calls":[{"name":"an exact tool name from the immediately preceding request","arguments":{}}]}
<<<END_AINOVEL_WEB_RESPONSE>>>
For tool calls only, the envelope must be strict valid JSON with one JSON object for each arguments value. Use no Markdown fence and emit no text outside the markers.`

func NewModel(cfg ModelConfig) (*Model, error) {
	if cfg.Transport == nil {
		return nil, fmt.Errorf("webai: transport is required")
	}
	site := strings.TrimSpace(cfg.Site)
	if site == "" {
		site = "web"
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = "web-session"
	}
	return &Model{site: site, model: model, transport: cfg.Transport}, nil
}

func (m *Model) Generate(ctx context.Context, messages []agentcore.Message, tools []agentcore.ToolSpec, opts ...agentcore.CallOption) (*agentcore.LLMResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	prompt, err := BuildPrompt(messages, tools, agentcore.ResolveCallConfig(opts))
	if err != nil {
		return nil, err
	}
	prompt += "\n\n" + rawTextProtocolPrompt
	raw, err := m.transport.RoundTrip(ctx, prompt)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	msg, err := parseResponseWithRawText(prompt, raw, tools)
	if err == nil {
		return &agentcore.LLMResponse{Message: msg}, nil
	}
	if !errors.Is(err, ErrProtocol) {
		return nil, err
	}

	// Browser models can occasionally return a syntactically malformed envelope
	// even after following the response markers. No local tool has executed at
	// this point, so one bounded reformat request in the same web conversation is
	// safe. This is deliberately not a general retry loop: transport/auth/timeout
	// errors are never retried here, and a second protocol violation fails hard.
	repairedRaw, repairErr := m.transport.RoundTrip(ctx, protocolRepairPrompt)
	if repairErr != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, repairErr
	}
	repaired, repairErr := parseResponseWithRawText(prompt, repairedRaw, tools)
	if repairErr != nil {
		return nil, repairErr
	}
	return &agentcore.LLMResponse{Message: repaired}, nil
}

// GenerateStream deliberately emits one terminal event in W1. True DOM delta
// streaming is deferred until the browser/session layer is stable; the existing
// agent loop still receives the authoritative final message and stop reason.
func (m *Model) GenerateStream(ctx context.Context, messages []agentcore.Message, tools []agentcore.ToolSpec, opts ...agentcore.CallOption) (<-chan agentcore.StreamEvent, error) {
	resp, err := m.Generate(ctx, messages, tools, opts...)
	if err != nil {
		return nil, err
	}
	ch := make(chan agentcore.StreamEvent, 1)
	ch <- agentcore.StreamEvent{
		Type:       agentcore.StreamEventDone,
		Message:    resp.Message,
		StopReason: resp.Message.StopReason,
	}
	close(ch)
	return ch, nil
}

func (m *Model) SupportsTools() bool { return true }

func (m *Model) ProviderName() string { return "web" }
func (m *Model) ModelName() string    { return m.model }

func (m *Model) Info() llm.ModelInfo {
	return llm.ModelInfo{
		Name:     m.model,
		Provider: "web",
		Capabilities: []string{
			string(llm.CapabilityChat),
			string(llm.CapabilityToolCalling),
			string(llm.CapabilityStreaming),
		},
	}
}

// Capabilities advertises only what the local web protocol can guarantee.
// Native structured output, usage accounting, provider-side strict tool schemas
// and controllable reasoning are intentionally reported as unsupported.
func (m *Model) Capabilities() llm.Capabilities {
	return llm.Capabilities{
		Provider: "web",
		Model:    m.model,
		Thinking: llm.ThinkingCapabilities{
			Supported: llm.SupportNo,
			Disable:   llm.SupportUnknown,
		},
		Tools: llm.ToolCapabilities{
			Calls:         llm.SupportYes,
			ParallelCalls: llm.SupportYes,
			StrictSchema:  llm.SupportNo,
			Choice:        llm.SupportNo,
		},
		Structured: llm.StructuredCapabilities{
			JSONObject: llm.SupportNo,
			JSONSchema: llm.SupportNo,
			Strict:     llm.SupportNo,
			PromptOnly: true,
		},
		Streaming: llm.StreamingCapabilities{
			Supported:       llm.SupportPartial,
			Usage:           llm.SupportNo,
			ReasoningDeltas: llm.SupportNo,
			ToolCallDeltas:  llm.SupportNo,
			NativeResponses: llm.SupportNo,
		},
		Usage: llm.UsageCapabilities{
			InputTokens:      llm.SupportNo,
			OutputTokens:     llm.SupportNo,
			TotalTokens:      llm.SupportNo,
			ReasoningTokens:  llm.SupportNo,
			CacheReadTokens:  llm.SupportNo,
			CacheWriteTokens: llm.SupportNo,
		},
	}
}
