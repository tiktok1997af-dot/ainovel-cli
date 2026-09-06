package webai

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/voocel/agentcore"
)

func TestModelAcceptsOneImmediateDuplicateStartMarker(t *testing.T) {
	raw := responseStart + "\n" + responseStart + "\nTEXT\nplanner-ready\n" + responseEnd
	transport := &fakeTransport{responses: []string{raw}}
	model := mustModel(t, transport)

	resp, err := model.Generate(context.Background(), []agentcore.Message{agentcore.UserMsg("plan")}, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got := resp.Message.TextContent(); got != "planner-ready" {
		t.Fatalf("text = %q, want planner-ready", got)
	}
	if got := len(transport.promptSnapshot()); got != 1 {
		t.Fatalf("round trips = %d, want 1", got)
	}
}

func TestModelAcceptsOneFullyBalancedRedundantWrapper(t *testing.T) {
	inner := wrappedResponse("TEXT\nplanner-ready")
	raw := responseStart + "\n" + inner + "\n" + responseEnd
	transport := &fakeTransport{responses: []string{raw}}
	model := mustModel(t, transport)

	resp, err := model.Generate(context.Background(), []agentcore.Message{agentcore.UserMsg("plan")}, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got := resp.Message.TextContent(); got != "planner-ready" {
		t.Fatalf("text = %q, want planner-ready", got)
	}
	if got := len(transport.promptSnapshot()); got != 1 {
		t.Fatalf("round trips = %d, want 1", got)
	}
}

func TestRedundantWrapperNormalizationRejectsEmbeddedMarker(t *testing.T) {
	raw := wrappedResponse("TEXT\nfirst\n" + responseStart + "\nsecond")
	if got := normalizeSingleRedundantResponseWrapper(raw); got != raw {
		t.Fatalf("embedded marker was incorrectly normalized: %q", got)
	}

	transport := &fakeTransport{responses: []string{raw, raw}}
	model := mustModel(t, transport)
	_, err := model.Generate(context.Background(), []agentcore.Message{agentcore.UserMsg("plan")}, nil)
	if !errors.Is(err, ErrProtocol) {
		t.Fatalf("err = %v, want ErrProtocol", err)
	}
}

func TestRedundantWrapperNormalizationRejectsMoreThanOneExtraWrapper(t *testing.T) {
	raw := strings.Join([]string{
		responseStart,
		responseStart,
		responseStart,
		"TEXT",
		"planner-ready",
		responseEnd,
		responseEnd,
		responseEnd,
	}, "\n")
	if got := normalizeSingleRedundantResponseWrapper(raw); got != raw {
		t.Fatalf("multi-layer wrapper was incorrectly normalized: %q", got)
	}
}
