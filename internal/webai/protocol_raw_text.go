package webai

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/voocel/agentcore"
)

const (
	rawToolCallPrefix = "TOOL_CALL_RAW"
	rawValueStart     = "<<<AINOVEL_RAW_VALUE>>>"
	rawValueEnd       = "<<<END_AINOVEL_RAW_VALUE>>>"
)

type rawToolCallMetadata struct {
	Name           string                     `json:"name"`
	Arguments      map[string]json.RawMessage `json:"arguments"`
	RawStringField string                     `json:"raw_string_field"`
}

// parseResponseWithRawText accepts the authoritative WEB message-body protocol
// while retaining the strict legacy outer-envelope protocol for compatibility.
//
// A Gemini DOM model-response already gives the runtime an exact assistant
// message boundary. Requiring the model to reproduce a second outer end marker
// inside that DOM message created a systemic truncation failure mode, so current
// WEB requests use one of these whole-message bodies instead:
//   - TEXT + newline + arbitrary assistant text.
//   - one strict legacy JSON body (normally kind=tool_calls).
//   - TOOL_CALL_RAW for exactly one tool call with one long top-level string.
//
// Malformed/partial legacy wrappers are never silently reinterpreted as a bare
// body. No local tool executes here: reconstructed calls still pass registry,
// object and downstream tool-schema validation before execution.
func parseResponseWithRawText(requestPrompt, raw string, tools []agentcore.ToolSpec) (agentcore.Message, error) {
	normalizedRaw := normalizeSingleRedundantResponseWrapper(raw)
	msg, legacyErr := ParseResponse(requestPrompt, normalizedRaw, tools)
	if legacyErr == nil {
		return msg, nil
	}
	if !errors.Is(legacyErr, ErrProtocol) {
		return agentcore.Message{}, legacyErr
	}

	body := strings.TrimSpace(normalizedRaw)
	if body == "" {
		return agentcore.Message{}, legacyErr
	}

	// A response that mentions an outer wrapper is claiming the legacy protocol.
	// Keep that path strict so a missing/duplicated marker can never be mistaken
	// for valid TEXT or a tool request merely because the DOM message ended.
	if strings.Contains(body, responseStart) || strings.Contains(body, responseEnd) {
		return agentcore.Message{}, legacyErr
	}

	if body == rawToolCallPrefix || strings.HasPrefix(body, rawToolCallPrefix+"\n") || strings.HasPrefix(body, rawToolCallPrefix+"\r\n") {
		return parseRawToolCallResponse(requestPrompt, normalizedRaw, body, tools)
	}

	if text, ok, err := parseBareTextBody(body); ok || err != nil {
		return text, err
	}

	// Reuse the locked legacy JSON validator by supplying the envelope locally.
	// The model never has to emit these redundant delimiters on the WEB path.
	if strings.HasPrefix(body, "{") {
		wrapped := responseStart + "\n" + body + "\n" + responseEnd
		msg, err := ParseResponse(requestPrompt, wrapped, tools)
		if err == nil {
			return msg, nil
		}
		if !errors.Is(err, ErrProtocol) {
			return agentcore.Message{}, err
		}
		return agentcore.Message{}, err
	}

	return agentcore.Message{}, legacyErr
}

func parseBareTextBody(body string) (agentcore.Message, bool, error) {
	var text string
	switch {
	case strings.HasPrefix(body, "TEXT\r\n"):
		text = body[len("TEXT\r\n"):]
	case strings.HasPrefix(body, "TEXT\n"):
		text = body[len("TEXT\n"):]
	case body == "TEXT":
		return agentcore.Message{}, true, protocolError("validate raw text response", fmt.Errorf("raw TEXT response is empty"))
	default:
		return agentcore.Message{}, false, nil
	}
	if strings.TrimSpace(text) == "" {
		return agentcore.Message{}, true, protocolError("validate raw text response", fmt.Errorf("raw TEXT response is empty"))
	}
	if strings.Contains(text, responseStart) || strings.Contains(text, responseEnd) {
		return agentcore.Message{}, true, protocolError("validate raw text response", fmt.Errorf("reserved response marker inside TEXT body"))
	}

	return agentcore.Message{
		Role:       agentcore.RoleAssistant,
		Content:    []agentcore.ContentBlock{agentcore.TextBlock(text)},
		StopReason: agentcore.StopReasonStop,
		Timestamp:  time.Now(),
	}, true, nil
}

// normalizeSingleRedundantResponseWrapper tolerates exactly one redundant
// leading response wrapper sometimes echoed by a browser model. It is purposely
// narrower than extractEnvelope: only an immediately nested envelope with no
// other content is normalized. Ambiguous, embedded, repeated or commentary-
// bearing marker layouts remain untouched and therefore fail the strict parser.
func normalizeSingleRedundantResponseWrapper(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(trimmed, responseStart) {
		return raw
	}

	afterOuterStart := strings.TrimSpace(strings.TrimPrefix(trimmed, responseStart))
	if !strings.HasPrefix(afterOuterStart, responseStart) {
		return raw
	}

	startCount := strings.Count(trimmed, responseStart)
	endCount := strings.Count(trimmed, responseEnd)
	if startCount != 2 {
		return raw
	}

	if endCount == 1 {
		return afterOuterStart
	}

	if endCount == 2 && strings.HasSuffix(afterOuterStart, responseEnd) {
		inner := strings.TrimSpace(strings.TrimSuffix(afterOuterStart, responseEnd))
		if strings.HasPrefix(inner, responseStart) && strings.HasSuffix(inner, responseEnd) &&
			strings.Count(inner, responseStart) == 1 && strings.Count(inner, responseEnd) == 1 {
			return inner
		}
	}
	return raw
}

func parseRawToolCallResponse(requestPrompt, raw, body string, tools []agentcore.ToolSpec) (agentcore.Message, error) {
	if err := validateToolRegistry(tools); err != nil {
		return agentcore.Message{}, err
	}

	rest := strings.TrimPrefix(body, rawToolCallPrefix)
	rest = strings.TrimPrefix(rest, "\r\n")
	rest = strings.TrimPrefix(rest, "\n")
	if strings.TrimSpace(rest) == "" {
		return agentcore.Message{}, protocolError("parse raw tool call", fmt.Errorf("raw tool call metadata/value is missing"))
	}

	valueStart := strings.Index(rest, rawValueStart)
	if valueStart < 0 {
		return agentcore.Message{}, protocolError("parse raw tool call", fmt.Errorf("missing raw value start marker"))
	}
	if strings.Count(rest, rawValueStart) != 1 {
		return agentcore.Message{}, protocolError("parse raw tool call", fmt.Errorf("nested raw value start marker"))
	}
	metadataText := strings.TrimSpace(rest[:valueStart])
	if metadataText == "" {
		return agentcore.Message{}, protocolError("parse raw tool call", fmt.Errorf("raw tool call metadata is empty"))
	}

	valueRegion := rest[valueStart+len(rawValueStart):]
	rawValue := valueRegion
	if valueEndRel := strings.Index(valueRegion, rawValueEnd); valueEndRel >= 0 {
		// Backward compatibility with the old framed raw-value form. If the old
		// end marker appears, it must still be one exact terminal delimiter.
		if strings.Count(valueRegion, rawValueEnd) != 1 {
			return agentcore.Message{}, protocolError("parse raw tool call", fmt.Errorf("nested raw value end marker"))
		}
		rawValue = valueRegion[:valueEndRel]
		trailing := valueRegion[valueEndRel+len(rawValueEnd):]
		if strings.TrimSpace(trailing) != "" {
			return agentcore.Message{}, protocolError("parse raw tool call", fmt.Errorf("unexpected content after raw value end marker"))
		}
	}
	if strings.Contains(rawValue, rawValueStart) || strings.Contains(rawValue, rawValueEnd) {
		return agentcore.Message{}, protocolError("parse raw tool call", fmt.Errorf("nested raw value marker"))
	}
	rawValue = trimOneProtocolLineBreak(rawValue, true)
	// Only trim the trailing protocol newline when an explicit legacy end marker
	// supplied it. In the DOM-delimited form the end of the assistant message is
	// the value boundary, so prose whitespace belongs to the value.
	if strings.Contains(valueRegion, rawValueEnd) {
		rawValue = trimOneProtocolLineBreak(rawValue, false)
	}
	if strings.TrimSpace(rawValue) == "" {
		return agentcore.Message{}, protocolError("parse raw tool call", fmt.Errorf("raw string argument is empty"))
	}

	var metadata rawToolCallMetadata
	dec := json.NewDecoder(strings.NewReader(metadataText))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&metadata); err != nil {
		return agentcore.Message{}, protocolError("decode raw tool call metadata", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("multiple JSON values in raw tool metadata")
		}
		return agentcore.Message{}, protocolError("decode raw tool call metadata", err)
	}

	name := strings.TrimSpace(metadata.Name)
	if name == "" || name != metadata.Name {
		return agentcore.Message{}, protocolError("validate raw tool call", fmt.Errorf("tool name is empty or has surrounding whitespace"))
	}
	allowed := false
	for _, tool := range tools {
		if tool.Name == name {
			allowed = true
			break
		}
	}
	if !allowed {
		return agentcore.Message{}, protocolError("validate raw tool call", fmt.Errorf("tool %q is not available in this request", name))
	}

	field := strings.TrimSpace(metadata.RawStringField)
	if field == "" || field != metadata.RawStringField || strings.ContainsAny(field, "\r\n\t") {
		return agentcore.Message{}, protocolError("validate raw tool call", fmt.Errorf("raw_string_field is invalid"))
	}
	if metadata.Arguments == nil {
		return agentcore.Message{}, protocolError("validate raw tool call", fmt.Errorf("arguments must be a JSON object"))
	}
	if _, exists := metadata.Arguments[field]; exists {
		return agentcore.Message{}, protocolError("validate raw tool call", fmt.Errorf("raw string field %q must be omitted from metadata arguments", field))
	}
	encodedValue, err := json.Marshal(rawValue)
	if err != nil {
		return agentcore.Message{}, protocolError("encode raw string argument", err)
	}
	metadata.Arguments[field] = encodedValue
	arguments, err := json.Marshal(metadata.Arguments)
	if err != nil {
		return agentcore.Message{}, protocolError("encode reconstructed tool arguments", err)
	}
	if err := validateJSONObject(arguments); err != nil {
		return agentcore.Message{}, protocolError("validate raw tool call arguments", err)
	}

	return agentcore.Message{
		Role: agentcore.RoleAssistant,
		Content: []agentcore.ContentBlock{agentcore.ToolCallBlock(agentcore.ToolCall{
			ID:   stableToolCallID(requestPrompt, raw, 0, name),
			Name: name,
			Args: append(json.RawMessage(nil), arguments...),
		})},
		StopReason: agentcore.StopReasonToolUse,
		Timestamp:  time.Now(),
	}, nil
}

// trimOneProtocolLineBreak removes only the framing line break introduced
// immediately after/before a protocol delimiter. It preserves all other prose
// whitespace verbatim.
func trimOneProtocolLineBreak(value string, leading bool) string {
	if leading {
		if strings.HasPrefix(value, "\r\n") {
			return value[2:]
		}
		if strings.HasPrefix(value, "\n") {
			return value[1:]
		}
		return value
	}
	if strings.HasSuffix(value, "\r\n") {
		return value[:len(value)-2]
	}
	if strings.HasSuffix(value, "\n") {
		return value[:len(value)-1]
	}
	return value
}
