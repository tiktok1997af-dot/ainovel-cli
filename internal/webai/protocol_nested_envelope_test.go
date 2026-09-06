package webai

import (
	"errors"
	"testing"

	"github.com/voocel/agentcore"
)

func nestedOnce(body string) string {
	return wrappedResponse(wrappedResponse(body))
}

func TestParseResponseAcceptsOneRedundantNestedTextEnvelope(t *testing.T) {
	msg, err := parseResponseWithRawText("request", nestedOnce("TEXT\nhello"), nil)
	if err != nil {
		t.Fatalf("parse nested raw text: %v", err)
	}
	if got := msg.TextContent(); got != "hello" {
		t.Fatalf("text = %q, want hello", got)
	}
}

func TestParseResponseAcceptsOneRedundantNestedStrictToolEnvelope(t *testing.T) {
	raw := nestedOnce(`{"kind":"tool_calls","tool_calls":[{"name":"save_chapter","arguments":{"chapter":7}}]}`)
	msg, err := parseResponseWithRawText("request", raw, []agentcore.ToolSpec{testToolSpec()})
	if err != nil {
		t.Fatalf("parse nested tool call: %v", err)
	}
	calls := msg.ToolCalls()
	if len(calls) != 1 || calls[0].Name != "save_chapter" || string(calls[0].Args) != `{"chapter":7}` {
		t.Fatalf("unexpected tool calls: %+v", calls)
	}
}

func TestParseResponseRejectsNestedEnvelopeWithMixedOuterContent(t *testing.T) {
	raw := wrappedResponse("commentary\n" + wrappedResponse("TEXT\nhello"))
	_, err := parseResponseWithRawText("request", raw, nil)
	if !errors.Is(err, ErrProtocol) {
		t.Fatalf("err = %v, want ErrProtocol", err)
	}
}

func TestParseResponseRejectsMoreThanOneRedundantWrapper(t *testing.T) {
	raw := wrappedResponse(nestedOnce("TEXT\nhello"))
	_, err := parseResponseWithRawText("request", raw, nil)
	if !errors.Is(err, ErrProtocol) {
		t.Fatalf("err = %v, want ErrProtocol", err)
	}
}

func TestNestedEnvelopeDoesNotRelaxUnknownToolValidation(t *testing.T) {
	raw := nestedOnce(`{"kind":"tool_calls","tool_calls":[{"name":"shell","arguments":{}}]}`)
	_, err := parseResponseWithRawText("request", raw, []agentcore.ToolSpec{testToolSpec()})
	if !errors.Is(err, ErrProtocol) {
		t.Fatalf("err = %v, want ErrProtocol", err)
	}
}
