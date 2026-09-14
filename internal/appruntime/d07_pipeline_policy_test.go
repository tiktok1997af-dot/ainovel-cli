package appruntime

import "testing"

func TestD07BalancedDefaultsPreserveSafetyAndCheckpointTen(t *testing.T) {
	policy, err := NewD07PipelinePolicy("", 0)
	if err != nil {
		t.Fatal(err)
	}
	cfg := policy.Config
	if cfg.Mode != "balanced" || !cfg.StrictRole || !cfg.PipelineEnabled || !cfg.AutoReview || !cfg.AutoQA || cfg.AutoRepair != "bounded" {
		t.Fatalf("unsafe balanced defaults: %+v", cfg)
	}
	if cfg.CheckpointChapters != 10 || !policy.CheckpointDue(10) || policy.CheckpointDue(9) {
		t.Fatalf("checkpoint policy mismatch: %+v", cfg)
	}
	if policy.AllowsIndependentDraftReviewOverlap() {
		t.Fatal("balanced unexpectedly enabled turbo overlap")
	}
}

func TestD07TurboChangesThroughputPermissionNotSafety(t *testing.T) {
	policy, err := NewD07PipelinePolicy("turbo", 5)
	if err != nil {
		t.Fatal(err)
	}
	cfg := policy.Config
	if !policy.AllowsIndependentDraftReviewOverlap() {
		t.Fatal("turbo did not enable independent draft/review overlap")
	}
	if !cfg.StrictRole || !cfg.AutoReview || !cfg.AutoQA || cfg.AutoRepair != "bounded" {
		t.Fatalf("turbo weakened safety: %+v", cfg)
	}
	if !policy.CheckpointDue(5) || !policy.CheckpointDue(10) {
		t.Fatal("turbo checkpoint cadence mismatch")
	}
}

func TestD07CheckpointRangeFailsClosed(t *testing.T) {
	for _, value := range []int{4, 11} {
		if _, err := NewD07PipelinePolicy("balanced", value); err == nil {
			t.Fatalf("checkpoint %d accepted outside 5..10", value)
		}
	}
}

func TestD07StrictRoleProviderMapping(t *testing.T) {
	policy, err := NewD07PipelinePolicy("balanced", 10)
	if err != nil {
		t.Fatal(err)
	}
	gemini := []D07PipelineRole{D07RoleWriter, D07RoleCoCreate}
	for _, role := range gemini {
		provider, err := policy.ProviderForRole(role)
		if err != nil || provider != ProviderGeminiWeb {
			t.Fatalf("role %q provider=%q err=%v", role, provider, err)
		}
	}
	chatgpt := []D07PipelineRole{D07RoleArchitect, D07RoleReview, D07RoleContinuity, D07RoleRepair, D07RoleQA}
	for _, role := range chatgpt {
		provider, err := policy.ProviderForRole(role)
		if err != nil || provider != ProviderChatGPTWeb {
			t.Fatalf("role %q provider=%q err=%v", role, provider, err)
		}
	}
	if _, err := policy.ProviderForRole("unknown"); err == nil {
		t.Fatal("unknown role silently fell back to a provider")
	}
}

func TestD07AutoRepairIsBounded(t *testing.T) {
	policy, err := NewD07PipelinePolicy("balanced", 10)
	if err != nil {
		t.Fatal(err)
	}
	if !policy.AutoRepairAllowed(0) {
		t.Fatal("first bounded repair attempt was rejected")
	}
	if policy.AutoRepairAllowed(1) || policy.AutoRepairAllowed(2) {
		t.Fatal("bounded repair allowed an extra automatic attempt")
	}
}
