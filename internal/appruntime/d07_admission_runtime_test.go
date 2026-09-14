package appruntime

import (
	"context"
	"testing"
)

func TestD07BalancedSerializesProviderAIAtD04Admission(t *testing.T) {
	authority := newDualWebParallelAuthority(newFakeDualWebLanes(), newFakeRunBackend(), "balanced")

	if _, err := authority.begin(context.Background(), dualWebJobSpec{
		JobID: "balanced-gemini", RunID: "run-001", Kind: dualWebJobAI, Provider: ProviderGeminiWeb,
	}); err != nil {
		t.Fatalf("begin Gemini: %v", err)
	}
	if _, err := authority.begin(context.Background(), dualWebJobSpec{
		JobID: "balanced-chatgpt", RunID: "run-002", Kind: dualWebJobAI, Provider: ProviderChatGPTWeb,
	}); err == nil {
		t.Fatal("balanced mode must serialize provider AI jobs")
	}
	if got := authority.snapshot(); got.ActiveAI != 1 || got.GeminiActive != 1 || got.ChatGPTActive != 0 {
		t.Fatalf("balanced snapshot=%+v", got)
	}
	if err := authority.finish("balanced-gemini"); err != nil {
		t.Fatalf("finish Gemini: %v", err)
	}
}

func TestD07TurboAllowsIndependentProviderOverlap(t *testing.T) {
	authority := newDualWebParallelAuthority(newFakeDualWebLanes(), newFakeRunBackend(), "turbo")

	for _, spec := range []dualWebJobSpec{
		{JobID: "turbo-gemini", RunID: "run-001", Kind: dualWebJobAI, Provider: ProviderGeminiWeb},
		{JobID: "turbo-chatgpt", RunID: "run-002", Kind: dualWebJobAI, Provider: ProviderChatGPTWeb},
	} {
		if _, err := authority.begin(context.Background(), spec); err != nil {
			t.Fatalf("begin %s: %v", spec.JobID, err)
		}
	}
	if got := authority.snapshot(); got.ActiveAI != 2 || got.GeminiActive != 1 || got.ChatGPTActive != 1 {
		t.Fatalf("turbo snapshot=%+v", got)
	}
	if err := authority.finish("turbo-gemini"); err != nil {
		t.Fatalf("finish Gemini: %v", err)
	}
	if err := authority.finish("turbo-chatgpt"); err != nil {
		t.Fatalf("finish ChatGPT: %v", err)
	}
}

func TestD07TurboStillHonorsD04StoryResourceLock(t *testing.T) {
	backend := newFakeRunBackend()
	authority := newDualWebParallelAuthority(newFakeDualWebLanes(), backend, "turbo")

	if _, err := authority.begin(context.Background(), dualWebJobSpec{
		JobID: "turbo-writer", RunID: "run-001", Kind: dualWebJobAI, Provider: ProviderGeminiWeb, ClaimsStoryResource: true,
	}); err != nil {
		t.Fatalf("begin writer: %v", err)
	}
	if _, err := authority.begin(context.Background(), dualWebJobSpec{
		JobID: "turbo-review", RunID: "run-002", Kind: dualWebJobAI, Provider: ProviderChatGPTWeb, ClaimsStoryResource: true,
	}); err == nil {
		t.Fatal("turbo must not bypass the canonical story-resource lock")
	}
	if got := authority.snapshot(); got.ActiveAI != 1 || backend.resourceOwner != "run-001" {
		t.Fatalf("failed turbo admission disturbed D04 authority: snapshot=%+v owner=%q", got, backend.resourceOwner)
	}
	if err := authority.finish("turbo-writer"); err != nil {
		t.Fatalf("finish writer: %v", err)
	}
}
