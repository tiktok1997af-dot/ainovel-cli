package appruntime

import "testing"

func TestD01FrozenDualWebParallelDefaults(t *testing.T) {
	cfg := FrozenDualWebParallelDefaults()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("frozen defaults invalid: %v", err)
	}
	if cfg.Mode != "balanced" || !cfg.LazyBrowserStart {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	if cfg.GeminiLaneCapacity != 1 || cfg.ChatGPTLaneCapacity != 1 || cfg.AIConcurrency != 2 || cfg.LocalWorkers != 2 || cfg.MaxActiveJobs != 4 {
		t.Fatalf("unexpected concurrency defaults: %+v", cfg)
	}
	if cfg.CheckpointChapters != 10 {
		t.Fatalf("checkpoint default = %d, want 10", cfg.CheckpointChapters)
	}
}

func TestD01ProviderVocabularyIsExactlyDualWeb(t *testing.T) {
	for _, provider := range []WebAIProvider{ProviderGeminiWeb, ProviderChatGPTWeb} {
		if err := provider.Validate(); err != nil {
			t.Fatalf("provider %q invalid: %v", provider, err)
		}
	}
	if err := WebAIProvider("other-web").Validate(); err == nil {
		t.Fatal("unknown provider accepted")
	}
}

func TestD01ModelCatalogRejectsDuplicateOrUnboundedEntries(t *testing.T) {
	valid := WebAIModelCatalogDTO{
		Provider: ProviderGeminiWeb,
		Revision: "rev-1",
		Models: []WebAIModelOptionDTO{
			{ID: "model-a", Label: "Model A", Available: true},
			{ID: "model-b", Label: "Model B", Available: true},
		},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid catalog rejected: %v", err)
	}

	duplicate := valid
	duplicate.Models = append([]WebAIModelOptionDTO(nil), valid.Models...)
	duplicate.Models[1].ID = "model-a"
	if err := duplicate.Validate(); err == nil {
		t.Fatal("duplicate model id accepted")
	}

	oversized := valid
	oversized.Models = []WebAIModelOptionDTO{{ID: string(make([]byte, MaxWebAIModelIDBytes+1)), Label: "Model", Available: true}}
	if err := oversized.Validate(); err == nil {
		t.Fatal("oversized model id accepted")
	}
}

func TestD01VerifiedSelectionMustMatchObservedExactModel(t *testing.T) {
	selection := WebAIModelSelectionDTO{
		Provider:         ProviderChatGPTWeb,
		RequestedModelID: "requested",
		RequestedLabel:   "Requested",
		ObservedModelID:  "requested",
		CatalogRevision:  "rev-7",
		Authenticated:    true,
		ProviderReady:    true,
		ModelAvailable:   true,
		Verified:         true,
	}
	if err := selection.Validate(); err != nil {
		t.Fatalf("valid selection rejected: %v", err)
	}

	selection.ObservedModelID = "different"
	if err := selection.Validate(); err == nil {
		t.Fatal("mismatched observed model accepted")
	}
}

func TestD01PreflightFailsClosedUntilEveryRequiredProviderVerified(t *testing.T) {
	verified := func(provider WebAIProvider, model string) WebAIModelSelectionDTO {
		return WebAIModelSelectionDTO{
			Provider:         provider,
			RequestedModelID: model,
			ObservedModelID:  model,
			CatalogRevision:  "catalog-rev",
			Authenticated:    true,
			ProviderReady:    true,
			ModelAvailable:   true,
			Verified:         true,
		}
	}

	ready := WebAIModelPreflightDTO{
		RequiredProviders: []WebAIProvider{ProviderGeminiWeb, ProviderChatGPTWeb},
		Selections: []WebAIModelSelectionDTO{
			verified(ProviderGeminiWeb, "g-model"),
			verified(ProviderChatGPTWeb, "c-model"),
		},
		StartAllowed: true,
	}
	if err := ready.Validate(); err != nil {
		t.Fatalf("ready preflight rejected: %v", err)
	}

	blocked := ready
	blocked.Selections = blocked.Selections[:1]
	blocked.StartAllowed = false
	if err := blocked.Validate(); err != nil {
		t.Fatalf("valid fail-closed preflight rejected: %v", err)
	}
	blocked.StartAllowed = true
	if err := blocked.Validate(); err == nil {
		t.Fatal("preflight allowed start without required provider verification")
	}
}

func TestD01ConfigRejectsUnsafeConcurrencyOrCheckpoint(t *testing.T) {
	cfg := FrozenDualWebParallelDefaults()
	cfg.AIConcurrency = 3
	if err := cfg.Validate(); err == nil {
		t.Fatal("AI concurrency above frozen dual-lane capacity accepted")
	}

	cfg = FrozenDualWebParallelDefaults()
	cfg.CheckpointChapters = 11
	if err := cfg.Validate(); err == nil {
		t.Fatal("checkpoint outside frozen 5..10 range accepted")
	}
}
