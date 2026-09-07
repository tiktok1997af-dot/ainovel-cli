package webai

import (
	"context"
	"encoding/json"
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

const (
	maxProtocolFormatRepairs = 2
	maxToolRequiredRepairs   = 2
	localToolRequiredMarker  = "AINOVEL_LOCAL_TOOL_REQUIRED"
)

// rawTextProtocolInstruction replaces the legacy response-format instruction
// before a request crosses the browser boundary. Gemini's rendered assistant
// message is already a reliable outer boundary, so the model is intentionally
// NOT asked to reproduce another start/end wrapper inside the message. Legacy
// wrapped responses remain parser-compatible for already-existing sessions.
const rawTextProtocolInstruction = `You are the AI backend for ainovel-cli. The browser is only a transport layer.
Return exactly one AINOVEL response body as the entire assistant message and nothing else. The assistant-message boundary is the response boundary; do not add an outer response wrapper, start marker, end marker, preface, epilogue, or Markdown fence.

Use exactly one of these whole-message forms:
- Normal assistant text: first line is the literal word TEXT, then a newline, then the complete answer verbatim. In shorthand: TEXT\n<the complete answer verbatim>. The text after TEXT may itself be JSON, Markdown, prose, quotes, or multiple lines. Do not JSON-escape or wrap normal text in a {"kind":"text"} object.
- Ordinary local tool request with small/simple arguments: one strict JSON object of the form {"kind":"tool_calls","tool_calls":[{"name":"exact_tool_name","arguments":{}}]}.
- One local tool request containing one long top-level string argument (for example a chapter body in content): use this whole-message form:
TOOL_CALL_RAW
{"name":"exact_tool_name","arguments":{"all_other_arguments":"go here"},"raw_string_field":"content"}
<<<AINOVEL_RAW_VALUE>>>
<the complete raw string value verbatim until the end of this assistant message>
The metadata arguments object MUST omit the field named by raw_string_field. The assistant-message boundary terminates the raw string; do not append a closing delimiter. The runtime inserts the raw value into that top-level field, reconstructs a normal JSON arguments object, and validates the tool/schema locally before execution.

Rules:
- A response is exactly one of TEXT, strict JSON tool_calls, or one TOOL_CALL_RAW; never combine forms.
- Use TOOL_CALL_RAW only for exactly one tool call and exactly one long top-level string argument. Keep all other arguments in the small metadata JSON object.
- The raw value must not contain the reserved string <<<AINOVEL_RAW_VALUE>>>.
- Use only tool names and argument fields present in the request tool schema.
- Never represent a tool request as TEXT.
- If the conversation contains the exact marker AINOVEL_LOCAL_TOOL_REQUIRED, the local runtime has blocked a TEXT-only worker turn. While that marker remains in the current task history, TEXT is forbidden: respond with a valid available local tool request that advances the persisted artifact instead.
- Do not claim that a tool ran; the local ainovel runtime executes tools only after validating the call.
- Do not write Markdown fences around any response form.
- Do not emit commentary before or after the selected response form.`

// protocolRepairPrompt asks only for a format correction of the already-intended
// answer. No local tool from the malformed response has executed. The repair
// also uses the DOM-delimited whole-message body so it cannot recreate the old
// missing-outer-end-marker failure mode.
const protocolRepairPrompt = `Your previous answer could not be parsed as an ainovel-web/1 response. No local tool from that answer has been executed.
Do not redo the user's task, do not change the intended answer, and do not add commentary.
Re-emit only the same intended answer using one exact AINOVEL whole-message body already defined earlier in this conversation. Do not add any outer response wrapper or Markdown fence.
For normal assistant text, start with the literal word TEXT, then one newline, then the complete intended text verbatim.
For a small local tool request, emit only one strict valid JSON object with kind tool_calls and exact tool names/argument fields from the original request.
For one local tool request containing one long top-level string argument, use TOOL_CALL_RAW exactly as already defined; after the raw-value start delimiter, preserve the complete intended raw string until the end of the assistant message and do not append a closing delimiter.
Do not claim a tool ran. This is a transport-format repair only.`

// jsonSyntaxRepairPrompt is used only when the strict response decoder reports
// a JSON syntax error. The malformed response has not executed a local tool, so
// this remains a side-effect-free correction inside the existing repair budget.
const jsonSyntaxRepairPrompt = `Your previous answer was rejected because it contained invalid JSON syntax. No local tool from that answer has been executed.
Do not redo the user's task, change the intended answer, change the intended tool, change any intended argument value, summarize, or add commentary.
Re-emit the same intended answer using one exact AINOVEL whole-message body already defined earlier in this conversation.
If the intended response is normal assistant text, use TEXT followed by one newline and the same intended text verbatim.
If the intended response is a small local tool request, emit one strict JSON object only. Use double-quoted JSON keys and string values, put commas only between object fields or array elements, use no comments or trailing commas, emit no unquoted prose inside JSON, and escape embedded quotes, backslashes, and control characters correctly.
If the intended response is one local tool request with one long top-level string argument, use TOOL_CALL_RAW exactly as already defined so the long raw value is not embedded into JSON metadata.
Do not add an outer response wrapper or Markdown fence and do not claim a tool ran. This is a JSON-syntax transport repair only.`

// rawStringFieldRepairPrompt is used only when a TOOL_CALL_RAW response reached
// the strict raw-tool parser but its raw_string_field metadata was invalid.
// The malformed call has not executed, so this remains a side-effect-free,
// format-only correction inside the existing bounded protocol-repair budget.
const rawStringFieldRepairPrompt = `Your previous TOOL_CALL_RAW answer was rejected because raw_string_field was invalid. No local tool from that answer has been executed.
Do not redo the user's task, change the intended tool, change any intended argument value, summarize, or add commentary.
Re-emit the same intended tool call and the same raw string value using one valid TOOL_CALL_RAW body.
Set raw_string_field to one exact top-level string field name from the requested tool schema, with no leading or trailing whitespace, newline, carriage return, or tab.
The metadata arguments must omit that same field; keep every other intended small argument in the metadata arguments object.
After the raw-value start delimiter, preserve the complete same raw string value verbatim until the end of the assistant message. Do not append a closing delimiter, outer response wrapper, or Markdown fence.
Do not claim the tool ran. This is a transport-format repair only.`

// toolRequiredRepairPrompt is narrower than the generic format repair. It is
// used only after a StopGuard-injected machine marker proves that the worker
// still owes a persisted artifact, yet the web model returned a valid TEXT-only
// answer. No local tool has executed from that TEXT answer, so asking the same
// web conversation to emit one advancing local tool call is side-effect safe.
const toolRequiredRepairPrompt = `AINOVEL_LOCAL_TOOL_REQUIRED
The local runtime rejected the previous TEXT-only answer because this worker still owes a persisted artifact. No local tool from that rejected answer has executed.
Do not repeat, explain, summarize, promise, or return TEXT.
Continue the same task by issuing exactly one available local tool call that advances the required artifact.
Use one strict tool_calls JSON object for small/simple arguments, or TOOL_CALL_RAW for exactly one call containing one long top-level string argument.
Use only exact tool names and argument fields from the request tool schema already present in this conversation. Do not claim the tool ran and do not add commentary or Markdown fences.`

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

func localToolRequired(messages []agentcore.Message, tools []agentcore.ToolSpec) bool {
	if len(tools) == 0 {
		return false
	}
	for i := len(messages) - 1; i >= 0; i-- {
		if strings.Contains(messages[i].TextContent(), localToolRequiredMarker) {
			return true
		}
	}
	return false
}

func invalidRawStringFieldProtocolError(err error) bool {
	var webErr *Error
	if !errors.As(err, &webErr) || webErr == nil {
		return false
	}
	return webErr.Kind == ErrorProtocol &&
		webErr.Op == "validate raw tool call" &&
		webErr.Cause != nil &&
		webErr.Cause.Error() == "raw_string_field is invalid"
}

func jsonSyntaxProtocolError(err error) bool {
	var webErr *Error
	if !errors.As(err, &webErr) || webErr == nil || webErr.Kind != ErrorProtocol || webErr.Op != "decode response" {
		return false
	}
	var syntaxErr *json.SyntaxError
	return errors.As(webErr.Cause, &syntaxErr)
}

func repairPromptForProtocolError(err error) string {
	if invalidRawStringFieldProtocolError(err) {
		return rawStringFieldRepairPrompt
	}
	if jsonSyntaxProtocolError(err) {
		return jsonSyntaxRepairPrompt
	}
	return protocolRepairPrompt
}

func (m *Model) repairRequiredLocalTool(ctx context.Context, requestPrompt string, tools []agentcore.ToolSpec) (*agentcore.LLMResponse, error) {
	lastErr := protocolError("enforce required local tool call", fmt.Errorf("assistant returned TEXT while a local tool was required"))
	for attempt := 0; attempt < maxToolRequiredRepairs; attempt++ {
		raw, err := m.transport.RoundTrip(ctx, toolRequiredRepairPrompt)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, err
		}
		msg, parseErr := parseResponseWithRawText(requestPrompt, raw, tools)
		if parseErr == nil {
			if msg.StopReason == agentcore.StopReasonToolUse {
				return &agentcore.LLMResponse{Message: msg}, nil
			}
			lastErr = protocolError(
				"enforce required local tool call",
				fmt.Errorf("repair attempt %d returned stop reason %q instead of a local tool call", attempt+1, msg.StopReason),
			)
			continue
		}
		if !errors.Is(parseErr, ErrProtocol) {
			return nil, parseErr
		}
		lastErr = parseErr
	}
	return nil, lastErr
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
	mustUseLocalTool := localToolRequired(messages, tools)

	raw, err := m.transport.RoundTrip(ctx, prompt)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	msg, parseErr := parseResponseWithRawText(prompt, raw, tools)
	if parseErr == nil {
		if mustUseLocalTool && msg.StopReason != agentcore.StopReasonToolUse {
			return m.repairRequiredLocalTool(ctx, prompt, tools)
		}
		return &agentcore.LLMResponse{Message: msg}, nil
	}
	if !errors.Is(parseErr, ErrProtocol) {
		return nil, parseErr
	}

	// Browser models can occasionally return a malformed response body even after
	// following the contract. No local tool has executed at this point, so a very
	// small bounded number of format-only repair turns in the same web conversation
	// is safe. Transport/auth/timeout failures return immediately; another protocol
	// violation after the second repair fails hard.
	lastProtocolErr := parseErr
	for attempt := 0; attempt < maxProtocolFormatRepairs; attempt++ {
		repairPrompt := repairPromptForProtocolError(lastProtocolErr)
		repairedRaw, repairErr := m.transport.RoundTrip(ctx, repairPrompt)
		if repairErr != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, repairErr
		}
		repaired, repairErr := parseResponseWithRawText(prompt, repairedRaw, tools)
		if repairErr == nil {
			if mustUseLocalTool && repaired.StopReason != agentcore.StopReasonToolUse {
				return m.repairRequiredLocalTool(ctx, prompt, tools)
			}
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
		Provider: m.site,
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
		Provider: m.site,
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
