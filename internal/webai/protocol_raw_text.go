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

	body, wrapped, err := responseBodyForExtensions(normalizedRaw)
	if err != nil {
		// A response that mentions a legacy wrapper must satisfy that wrapper
		// exactly. Never fall back to DOM-body parsing for a partial wrapper.
		return agentcore.Message{}, legacyErr
	}
	if body == "" {
		return agentcore.Message{}, legacyErr
	}

	if body == rawToolCallPrefix || strings.HasPrefix(body, rawToolCallPrefix+"\n") || strings.HasPrefix(body, rawToolCallPrefix+"\r\n") {
		return parseRawToolCallResponse(requestPrompt, normalizedRaw, body, tools)
	}

	if text, ok, err := parseBareTextBody(body); ok || err != nil {
		return text, err
	}

	// Reuse the locked legacy JSON validator by supplying the envelope locally
	// for the current bare-body path. A complete legacy wrapper already failed
	// that same validator above, so returning its original error is equivalent and
	// avoids accidentally normalizing a malformed legacy body a second time.
	if strings.HasPrefix(body, "{") {
		if wrapped {
			return agentcore.Message{}, legacyErr
		}
		localWrapped := responseStart + "\n" + body + "\n" + responseEnd
		parsed, parseErr := ParseResponse(requestPrompt, localWrapped, tools)
		if parseErr == nil {
			return parsed, nil
		}
		return agentcore.Message{}, parseErr
	}

	return agentcore.Message{}, legacyErr
}

// responseBodyForExtensions returns the body for TEXT/TOOL_CALL_RAW parsing.
// If either legacy outer marker is present, the entire response must be one
// valid complete legacy envelope. This keeps missing/stray/nested wrapper cases
// strict while allowing old wrapped TEXT and TOOL_CALL_RAW browser sessions to
// continue working.
func responseBodyForExtensions(raw string) (body string, wrapped bool, err error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", false, nil
	}
	mentionsStart := strings.Contains(trimmed, responseStart)
	mentionsEnd := strings.Contains(trimmed, responseEnd)
	if !mentionsStart && !mentionsEnd {
		return trimmed, false, nil
	}
	if !mentionsStart || !mentionsEnd {
		return "", true, protocolError("extract legacy response body", fmt.Errorf("partial legacy response wrapper"))
	}
	extracted, extractErr := extractEnvelope(trimmed)
	if extractErr != nil {
		return "", true, extractErr
	}
	if strings.Contains(extracted, responseStart) || strings.Contains(extracted, responseEnd) {
		return "", true, protocolError("extract legacy response body", fmt.Errorf("nested legacy response wrapper"))
	}
	return strings.TrimSpace(extracted), true, nil
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
	hasLegacyRawEnd := false
	if valueEndRel := strings.Index(valueRegion, rawValueEnd); valueEndRel >= 0 {
		hasLegacyRawEnd = true
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
	if hasLegacyRawEnd {
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
