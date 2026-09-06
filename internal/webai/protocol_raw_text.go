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

// parseResponseWithRawText preserves the strict legacy JSON protocol while
// accepting two explicit WEB-only extensions:
//   - TEXT for arbitrary assistant text without JSON-string escaping.
//   - TOOL_CALL_RAW for exactly one tool call whose one top-level string
//     argument is carried verbatim outside the small metadata JSON object.
//
// The raw tool form does not execute anything. It reconstructs one ordinary
// JSON arguments object, validates registry/name/object invariants, and returns
// the native agentcore tool-call block. Agentcore still applies the tool schema
// immediately before local execution.
func parseResponseWithRawText(requestPrompt, raw string, tools []agentcore.ToolSpec) (agentcore.Message, error) {
	msg, legacyErr := ParseResponse(requestPrompt, raw, tools)
	if legacyErr == nil {
		return msg, nil
	}
	if !errors.Is(legacyErr, ErrProtocol) {
		return agentcore.Message{}, legacyErr
	}

	body, extractErr := extractEnvelope(raw)
	if extractErr != nil {
		return agentcore.Message{}, legacyErr
	}

	if body == rawToolCallPrefix || strings.HasPrefix(body, rawToolCallPrefix+"\n") || strings.HasPrefix(body, rawToolCallPrefix+"\r\n") {
		return parseRawToolCallResponse(requestPrompt, raw, body, tools)
	}

	var text string
	switch {
	case strings.HasPrefix(body, "TEXT\r\n"):
		text = body[len("TEXT\r\n"):]
	case strings.HasPrefix(body, "TEXT\n"):
		text = body[len("TEXT\n"):]
	case body == "TEXT":
		return agentcore.Message{}, protocolError("validate raw text response", fmt.Errorf("raw TEXT response is empty"))
	default:
		return agentcore.Message{}, legacyErr
	}
	if strings.TrimSpace(text) == "" {
		return agentcore.Message{}, protocolError("validate raw text response", fmt.Errorf("raw TEXT response is empty"))
	}

	return agentcore.Message{
		Role:       agentcore.RoleAssistant,
		Content:    []agentcore.ContentBlock{agentcore.TextBlock(text)},
		StopReason: agentcore.StopReasonStop,
		Timestamp:  time.Now(),
	}, nil
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
	metadataText := strings.TrimSpace(rest[:valueStart])
	if metadataText == "" {
		return agentcore.Message{}, protocolError("parse raw tool call", fmt.Errorf("raw tool call metadata is empty"))
	}

	valueRegion := rest[valueStart+len(rawValueStart):]
	valueEndRel := strings.Index(valueRegion, rawValueEnd)
	if valueEndRel < 0 {
		return agentcore.Message{}, protocolError("parse raw tool call", fmt.Errorf("missing raw value end marker"))
	}
	rawValue := valueRegion[:valueEndRel]
	trailing := valueRegion[valueEndRel+len(rawValueEnd):]
	if strings.TrimSpace(trailing) != "" {
		return agentcore.Message{}, protocolError("parse raw tool call", fmt.Errorf("unexpected content after raw value end marker"))
	}
	if strings.Contains(rawValue, rawValueStart) || strings.Contains(rawValue, rawValueEnd) {
		return agentcore.Message{}, protocolError("parse raw tool call", fmt.Errorf("nested raw value marker"))
	}
	rawValue = trimOneProtocolLineBreak(rawValue, true)
	rawValue = trimOneProtocolLineBreak(rawValue, false)
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
