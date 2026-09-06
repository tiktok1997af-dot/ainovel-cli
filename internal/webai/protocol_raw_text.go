package webai

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/voocel/agentcore"
)

// parseResponseWithRawText preserves the strict legacy JSON protocol while
// accepting the explicit TEXT discriminator used by the raw-text extension.
// Tool calls never enter this path: only a response whose envelope body starts
// with the exact TEXT discriminator is converted to an assistant text message.
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
