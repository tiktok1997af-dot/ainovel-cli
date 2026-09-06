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
	if !strings.Contains(prompts[1], "previous answer") || !strings.Contains(prompts[1], "strict valid JSON") {
		t.Fatalf("repair prompt missing bounded reformat contract: %q", prompts[1])
	}
	if strings.Contains(prompts[1], responseStart) || strings.Contains(prompts[1], responseEnd) {
		t.Fatalf("repair prompt must not echo literal outer response markers: %q", prompts[1])
	}
}

func TestModelUsesSecondBoundedFormatRepair(t *testing.T) {
	transport := &fakeTransport{responses: []string{
		wrappedResponse(`{"kind":"text","text":"broken" nope}`),
		wrappedResponse(`{"kind":"text","text":"still broken" nope}`),
		wrappedResponse("TEXT\nrepaired-on-second-format-turn"),
	}}
	model := mustModel(t, transport)

	resp, err := model.Generate(context.Background(), []agentcore.Message{agentcore.UserMsg("hello")}, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got := resp.Message.TextContent(); got != "repaired-on-second-format-turn" {
		t.Fatalf("text = %q", got)
	}
	if got := len(transport.promptSnapshot()); got != 3 {
		t.Fatalf("round trips = %d, want 3", got)
	}
}

func TestModelProtocolRepairFailsAfterBoundedRepairs(t *testing.T) {
	transport := &fakeTransport{responses: []string{
		wrappedResponse(`{"kind":"text","text":"broken" nope}`),
		wrappedResponse(`{"kind":"text","text":"still broken" nope}`),
		wrappedResponse(`{"kind":"text","text":"third broken" nope}`),
	}}
	model := mustModel(t, transport)

	_, err := model.Generate(context.Background(), []agentcore.Message{agentcore.UserMsg("hello")}, nil)
	if !errors.Is(err, ErrProtocol) {
		t.Fatalf("err = %v, want ErrProtocol", err)
	}
	if got := len(transport.promptSnapshot()); got != 1+maxProtocolFormatRepairs {
		t.Fatalf("round trips = %d, want %d", got, 1+maxProtocolFormatRepairs)
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
