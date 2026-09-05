package webai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/voocel/agentcore"
	"github.com/voocel/agentcore/subagent"
	"github.com/voocel/ainovel-cli/internal/llmcontract"
)

type fakeTransport struct {
	mu        sync.Mutex
	responses []string
	prompts   []string
	fn        func(context.Context, string) (string, error)
}

func (f *fakeTransport) RoundTrip(ctx context.Context, prompt string) (string, error) {
	f.mu.Lock()
	f.prompts = append(f.prompts, prompt)
	fn := f.fn
	if fn == nil {
		if len(f.responses) == 0 {
			f.mu.Unlock()
			return "", fmt.Errorf("fake transport: no response left")
		}
		response := f.responses[0]
		f.responses = f.responses[1:]
		f.mu.Unlock()
		return response, nil
	}
	f.mu.Unlock()
	return fn(ctx, prompt)
}

func (f *fakeTransport) promptSnapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.prompts...)
}

func mustModel(t *testing.T, transport Transport) *Model {
	t.Helper()
	m, err := NewModel(ModelConfig{Site: "fake-web", Model: "fake-model", Transport: transport})
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}
	return m
}

func TestModelGenerateText(t *testing.T) {
	transport := &fakeTransport{responses: []string{wrappedResponse(`{"kind":"text","text":"hello from web"}`)}}
	model := mustModel(t, transport)
	resp, err := model.Generate(context.Background(), []agentcore.Message{agentcore.UserMsg("hello")}, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got := resp.Message.TextContent(); got != "hello from web" {
		t.Fatalf("text = %q", got)
	}
	if len(transport.promptSnapshot()) != 1 {
		t.Fatal("expected exactly one web round trip")
	}
}

func TestModelGenerateStreamEmitsAuthoritativeDone(t *testing.T) {
	transport := &fakeTransport{responses: []string{wrappedResponse(`{"kind":"text","text":"complete"}`)}}
	model := mustModel(t, transport)
	stream, err := model.GenerateStream(context.Background(), []agentcore.Message{agentcore.UserMsg("go")}, nil)
	if err != nil {
		t.Fatalf("GenerateStream: %v", err)
	}
	events := make([]agentcore.StreamEvent, 0, 1)
	for event := range stream {
		events = append(events, event)
	}
	if len(events) != 1 || events[0].Type != agentcore.StreamEventDone {
		t.Fatalf("events = %+v", events)
	}
	if events[0].Message.TextContent() != "complete" || events[0].StopReason != agentcore.StopReasonStop {
		t.Fatalf("unexpected done event: %+v", events[0])
	}
}

func TestModelCancellationPropagatesContext(t *testing.T) {
	started := make(chan struct{})
	transport := &fakeTransport{fn: func(ctx context.Context, _ string) (string, error) {
		close(started)
		<-ctx.Done()
		return "", ctx.Err()
	}}
	model := mustModel(t, transport)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := model.Generate(ctx, []agentcore.Message{agentcore.UserMsg("wait")}, nil)
		done <- err
	}()
	<-started
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestModelForcesPromptContractForStructuredCalls(t *testing.T) {
	model := mustModel(t, &fakeTransport{})
	_, resolution := llmcontract.Plan(model, llmcontract.Contract{
		Name: "decision",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{"choice": map[string]any{"type": "string"}},
			"required": []string{"choice"},
		},
	})
	if resolution.Mode != llmcontract.ModePromptContract {
		t.Fatalf("mode = %s, want prompt_contract", resolution.Mode)
	}
	if resolution.Provider != "web" || resolution.Model != "fake-model" {
		t.Fatalf("identity = %s/%s", resolution.Provider, resolution.Model)
	}
}

func TestWebToolCallExecutesOnlyThroughLocalAgentcore(t *testing.T) {
	var executed atomic.Int32
	tool := agentcore.NewFuncTool("save", "save locally", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"value": map[string]any{"type": "string"},
		},
		"required":             []string{"value"},
		"additionalProperties": false,
	}, func(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
		executed.Add(1)
		if string(args) != `{"value":"ok"}` {
			return nil, fmt.Errorf("unexpected args: %s", args)
		}
		return json.RawMessage(`{"saved":true}`), nil
	})
	transport := &fakeTransport{responses: []string{
		wrappedResponse(`{"kind":"tool_calls","tool_calls":[{"name":"save","arguments":{"value":"ok"}}]}`),
		wrappedResponse(`{"kind":"text","text":"finished"}`),
	}}
	model := mustModel(t, transport)
	runner := subagent.NewRunner(subagent.Config{
		Name: "writer", Description: "w1 contract", Model: model,
		SystemPrompt: "test", Tools: []agentcore.Tool{tool}, MaxTurns: 4,
	})
	result, err := runner.Run(context.Background(), "writer", "test local web tool protocol")
	if err != nil {
		t.Fatalf("Runner.Run: %v", err)
	}
	if executed.Load() != 1 {
		t.Fatalf("local tool executions = %d, want 1", executed.Load())
	}
	if result.Output != "finished" {
		t.Fatalf("output = %q", result.Output)
	}
	prompts := transport.promptSnapshot()
	if len(prompts) != 2 {
		t.Fatalf("round trips = %d, want 2", len(prompts))
	}
	if !strings.Contains(prompts[1], `"role":"tool"`) || !strings.Contains(prompts[1], `saved`) {
		t.Fatal("second web request did not contain the local tool result")
	}
}

func TestSchemaInvalidWebToolCallNeverExecutes(t *testing.T) {
	var executed atomic.Int32
	tool := agentcore.NewFuncTool("save", "save locally", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"value": map[string]any{"type": "string"},
		},
		"required": []string{"value"},
	}, func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
		executed.Add(1)
		return json.RawMessage(`"should-not-run"`), nil
	})
	transport := &fakeTransport{responses: []string{
		wrappedResponse(`{"kind":"tool_calls","tool_calls":[{"name":"save","arguments":{}}]}`),
		wrappedResponse(`{"kind":"text","text":"corrected"}`),
	}}
	model := mustModel(t, transport)
	runner := subagent.NewRunner(subagent.Config{
		Name: "writer", Description: "w1 validation", Model: model,
		SystemPrompt: "test", Tools: []agentcore.Tool{tool}, MaxTurns: 4,
	})
	result, err := runner.Run(context.Background(), "writer", "reject invalid tool arguments")
	if err != nil {
		t.Fatalf("Runner.Run: %v", err)
	}
	if executed.Load() != 0 {
		t.Fatalf("invalid tool executed %d times", executed.Load())
	}
	if result.Output != "corrected" {
		t.Fatalf("output = %q", result.Output)
	}
	prompts := transport.promptSnapshot()
	if len(prompts) != 2 || !strings.Contains(prompts[1], "InputValidationError") {
		t.Fatalf("expected validation error to be returned to web model; prompts=%d", len(prompts))
	}
}

func TestWebErrorRetryContracts(t *testing.T) {
	retry := &Error{Kind: ErrorTransport, Op: "capture", Cause: errors.New("page reload"), Retry: true, RetryDelay: 2 * time.Second}
	if !errors.Is(retry, agentcore.ErrProviderNetwork) {
		t.Fatalf("transport error must map to provider-neutral network sentinel: %v", retry)
	}
	var retryable agentcore.RetryableError
	if !errors.As(retry, &retryable) || !retryable.Retryable() {
		t.Fatalf("retryable contract missing: %v", retry)
	}
	var hinter agentcore.RetryHinter
	if !errors.As(retry, &hinter) || hinter.RetryAfter() != 2*time.Second {
		t.Fatalf("retry hint missing: %v", retry)
	}

	auth := &Error{Kind: ErrorAuthRequired, Op: "readiness", Cause: errors.New("login required")}
	if !errors.Is(auth, agentcore.ErrProviderAuth) || auth.Retryable() {
		t.Fatalf("auth error must be non-retryable and map to auth sentinel: %v", auth)
	}
}
