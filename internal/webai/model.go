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

const maxProtocolFormatRepairs = 2

// rawTextProtocolInstruction replaces the legacy response-format instruction
// before a request crosses the browser boundary. It deliberately contains only
// one response marker pair so a browser model is not encouraged to nest an
// example envelope inside another envelope. The request payload format itself
// remains ainovel-web/1 and legacy JSON envelopes remain accepted by the parser
// for backward compatibility.
const rawTextProtocolInstruction = `You are the AI backend for ainovel-cli. The browser is only a transport layer.
Return exactly one response envelope and nothing else.

AINOVEL WEB RESPONSE EXTENSION:
<<<AINOVEL_WEB_RESPONSE>>>
<body>
<<<END_AINOVEL_WEB_RESPONSE>>>

Replace <body> with exactly one of these forms:
- Normal assistant text: first line is the literal word TEXT, then a newline, then the complete answer verbatim. In shorthand: TEXT\n<the complete answer verbatim>. The body after TEXT may itself be JSON, Markdown, prose, quotes, or multiple lines. Do not JSON-escape or wrap normal text in a {"kind":"text"} object.
- Ordinary local tool request with small/simple arguments: one strict JSON object of the form {"kind":"tool_calls","tool_calls":[{"name":"exact_tool_name","arguments":{}}]}.
- One local tool request containing one long top-level string argument (for example a chapter body in content): use TOOL_CALL_RAW so that long string is not JSON-escaped. The body form is:
TOOL_CALL_RAW
{"name":"exact_tool_name","arguments":{"all_other_arguments":"go here"},"raw_string_field":"content"}
<<<AINOVEL_RAW_VALUE>>>
<the complete raw string value verbatim>
<<<END_AINOVEL_RAW_VALUE>>>
The metadata arguments object MUST omit the field named by raw_string_field. The runtime inserts the raw value into that top-level field, reconstructs a normal JSON arguments object, and validates the tool/schema locally before execution.

Rules:
- A response is exactly one of TEXT, strict JSON tool_calls, or one TOOL_CALL_RAW; never combine forms.
- Use TOOL_CALL_RAW only for exactly one tool call and exactly one long top-level string argument. Keep all other arguments in the small metadata JSON object.
- The raw value must not contain the reserved strings <<<AINOVEL_RAW_VALUE>>> or <<<END_AINOVEL_RAW_VALUE>>>.
- Use only tool names and argument fields present in the request tool schema.
- Never represent a tool request as TEXT.
- Do not claim that a tool ran; the local ainovel runtime executes tools only after validating the call.
- Do not write Markdown fences around the response envelope or raw value.
- Do not emit commentary outside the response markers.
- Emit the response markers exactly once each.`

// protocolRepairPrompt intentionally does NOT repeat the literal outer response
// markers. Gemini already has the full ainovel request immediately earlier in
// the same browser conversation; repeating the marker example in a repair turn
// can itself encourage an extra echoed/nested wrapper. This prompt asks only for
// a format correction of the already-intended answer. No local tool is executed
// until a later parse succeeds.
const protocolRepairPrompt = `Your previous answer could not be parsed as an ainovel-web/1 response. No local tool from that answer has been executed.
Do not redo the user's task, do not change the intended answer, and do not add commentary.
Re-emit only the same intended answer using the exact AINOVEL response framing and body forms already defined earlier in this same conversation.
Emit exactly one outer response envelope. Never quote, explain, duplicate, or place an AINOVEL response start/end marker inside the envelope body.
For normal assistant text, the body starts with the literal word TEXT, then one newline, then the complete intended text verbatim.
For a small local tool request, the body is one strict valid JSON object with kind tool_calls and only exact tool names/argument fields from the original request.
For one local tool request containing one long top-level string argument, use TOOL_CALL_RAW exactly as already defined in the original request and preserve the intended raw string verbatim.
Do not claim a tool ran. Do not use Markdown fences. This is a transport-format repair only.`

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
	if !strings.HasPrefix(prompt, protocolInstruction) {
		return nil, protocolError("prepare raw text response protocol", fmt.Errorf("legacy protocol instruction prefix is missing"))
	}
	prompt = rawTextProtocolInstruction + strings.TrimPrefix(prompt, protocolInstruction)

	raw, err := m.transport.RoundTrip(ctx, prompt)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	msg, parseErr := parseResponseWithRawText(prompt, raw, tools)
	if parseErr == nil {
		return &agentcore.LLMResponse{Message: msg}, nil
	}
	if !errors.Is(parseErr, ErrProtocol) {
		return nil, parseErr
	}

	// Browser models can occasionally return a syntactically malformed response
	// envelope even after following the contract. No local tool has executed at
	// this point, so a very small bounded number of format-only repair turns in
	// the same web conversation is safe. This is deliberately not a general retry
	// loop: transport/auth/timeout errors return immediately, and after the second
	// format repair another protocol violation fails hard.
	lastProtocolErr := parseErr
	for attempt := 0; attempt < maxProtocolFormatRepairs; attempt++ {
		repairedRaw, repairErr := m.transport.RoundTrip(ctx, protocolRepairPrompt)
		if repairErr != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, repairErr
		}
		repaired, repairErr := parseResponseWithRawText(prompt, repairedRaw, tools)
		if repairErr == nil {
			return &agentcore.LLMResponse{Message: repaired}, nil
		}
		if !errors.Is(repairErr, ErrProtocol) {
			return nil, repairErr
		}
		lastProtocolErr = repairErr
	}
	return nil, lastProtocolErr
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
