package webai

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/voocel/agentcore"
)

func TestModelRepairsOneMalformedProtocolResponse(t *testing.T) {
	transport := &fakeTransport{responses: []string{
		wrappedResponse(`{"kind":"text","text":"broken" nope}`),
		wrappedResponse(`{"kind":"text","text":"repaired"}`),
	}}
	model := mustModel(t, transport)

	resp, err := model.Generate(context.Background(), []agentcore.Message{agentcore.UserMsg("hello")}, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got := resp.Message.TextContent(); got != "repaired" {
		t.Fatalf("text = %q, want repaired", got)
	}
	prompts := transport.promptSnapshot()
	if len(prompts) != 2 {
		t.Fatalf("round trips = %d, want 2", len(prompts))
	}
	if !strings.Contains(prompts[1], "immediately previous answer") || !strings.Contains(prompts[1], "strict valid JSON") {
		t.Fatalf("repair prompt missing bounded reformat contract: %q", prompts[1])
	}
}

func TestModelProtocolRepairFailsAfterSecondMalformedResponse(t *testing.T) {
	transport := &fakeTransport{responses: []string{
		wrappedResponse(`{"kind":"text","text":"broken" nope}`),
		wrappedResponse(`{"kind":"text","text":"still broken" nope}`),
	}}
	model := mustModel(t, transport)

	_, err := model.Generate(context.Background(), []agentcore.Message{agentcore.UserMsg("hello")}, nil)
	if !errors.Is(err, ErrProtocol) {
		t.Fatalf("err = %v, want ErrProtocol", err)
	}
	if got := len(transport.promptSnapshot()); got != 2 {
		t.Fatalf("round trips = %d, want exactly 2", got)
	}
}

func TestModelDoesNotRepairTransportFailure(t *testing.T) {
	transport := &fakeTransport{fn: func(context.Context, string) (string, error) {
		return "", &Error{Kind: ErrorTransport, Op: "submit", Cause: errors.New("network")}
	}}
	model := mustModel(t, transport)

	_, err := model.Generate(context.Background(), []agentcore.Message{agentcore.UserMsg("hello")}, nil)
	var webErr *Error
	if !errors.As(err, &webErr) || webErr.Kind != ErrorTransport {
		t.Fatalf("err = %v, want transport error", err)
	}
	if got := len(transport.promptSnapshot()); got != 1 {
		t.Fatalf("round trips = %d, want 1", got)
	}
}
