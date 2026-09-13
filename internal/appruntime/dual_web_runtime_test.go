package appruntime

import (
	"encoding/json"
	"testing"
)

func TestD02DualWebProviderRuntimeStartsFailClosed(t *testing.T) {
	rt := newDualWebProviderRuntime()

	providers := rt.providersResult()
	if len(providers.Providers) != 2 {
		t.Fatalf("providers = %d, want 2", len(providers.Providers))
	}
	for _, provider := range providers.Providers {
		if provider.Authenticated || provider.Ready || provider.ActiveModelID != "" {
			t.Fatalf("provider %s unexpectedly ready: %+v", provider.Provider, provider)
		}
	}

	preflight, err := rt.preflight([]WebAIProvider{ProviderGeminiWeb})
	if err != nil {
		t.Fatal(err)
	}
	if preflight.StartAllowed {
		t.Fatal("start allowed before model selection and readiness")
	}
}

func TestD02ModelSelectionVerifiesOnlyExactObservedActiveModel(t *testing.T) {
	rt := newDualWebProviderRuntime()
	mustUpdateD02Provider(t, rt, webAIProviderSnapshot{
		Provider:      ProviderGeminiWeb,
		LaneID:        "gemini-web",
		Authenticated: true,
		Ready:         true,
		ActiveModelID: "gemini-model-a",
		Catalog: WebAIModelCatalogDTO{
			Provider: ProviderGeminiWeb,
			Revision: "gemini-rev-1",
			Models: []WebAIModelOptionDTO{
				{ID: "gemini-model-a", Label: "Gemini Model A", Available: true},
				{ID: "gemini-model-b", Label: "Gemini Model B", Available: true},
			},
		},
	})

	selection, err := rt.selectModel(ProviderGeminiWeb, "gemini-model-b")
	if err != nil {
		t.Fatal(err)
	}
	if selection.Verified {
		t.Fatal("selection verified despite observed active model mismatch")
	}

	selection, err = rt.selectModel(ProviderGeminiWeb, "gemini-model-a")
	if err != nil {
		t.Fatal(err)
	}
	if !selection.Verified {
		t.Fatalf("exact observed model was not verified: %+v", selection)
	}
}

func TestD02PreflightRequiresEveryRequiredProviderVerified(t *testing.T) {
	rt := newDualWebProviderRuntime()
	mustUpdateD02Provider(t, rt, d02ReadySnapshot(ProviderGeminiWeb, "gemini-web", "gemini-model", "g-rev"))
	mustUpdateD02Provider(t, rt, d02ReadySnapshot(ProviderChatGPTWeb, "chatgpt-web", "chatgpt-model", "c-rev"))

	if _, err := rt.selectModel(ProviderGeminiWeb, "gemini-model"); err != nil {
		t.Fatal(err)
	}
	preflight, err := rt.preflight([]WebAIProvider{ProviderGeminiWeb, ProviderChatGPTWeb})
	if err != nil {
		t.Fatal(err)
	}
	if preflight.StartAllowed {
		t.Fatal("start allowed with ChatGPT required but unselected")
	}

	if _, err := rt.selectModel(ProviderChatGPTWeb, "chatgpt-model"); err != nil {
		t.Fatal(err)
	}
	preflight, err = rt.preflight([]WebAIProvider{ProviderGeminiWeb, ProviderChatGPTWeb})
	if err != nil {
		t.Fatal(err)
	}
	if !preflight.StartAllowed {
		t.Fatalf("start blocked after both exact models verified: %+v", preflight)
	}
	if err := preflight.Validate(); err != nil {
		t.Fatalf("preflight contract validation failed: %v", err)
	}
}

func TestD02CatalogDriftInvalidatesPreviousVerification(t *testing.T) {
	rt := newDualWebProviderRuntime()
	mustUpdateD02Provider(t, rt, d02ReadySnapshot(ProviderGeminiWeb, "gemini-web", "gemini-model", "rev-1"))
	selection, err := rt.selectModel(ProviderGeminiWeb, "gemini-model")
	if err != nil || !selection.Verified {
		t.Fatalf("initial selection not verified: %+v err=%v", selection, err)
	}

	mustUpdateD02Provider(t, rt, webAIProviderSnapshot{
		Provider:      ProviderGeminiWeb,
		LaneID:        "gemini-web",
		Authenticated: true,
		Ready:         true,
		ActiveModelID: "gemini-model-new",
		Catalog: WebAIModelCatalogDTO{
			Provider: ProviderGeminiWeb,
			Revision: "rev-2",
			Models: []WebAIModelOptionDTO{
				{ID: "gemini-model-new", Label: "Gemini Model New", Available: true},
			},
		},
	})

	preflight, err := rt.preflight([]WebAIProvider{ProviderGeminiWeb})
	if err != nil {
		t.Fatal(err)
	}
	if preflight.StartAllowed {
		t.Fatal("catalog drift failed to invalidate previous model verification")
	}
	if len(preflight.Selections) != 1 || preflight.Selections[0].Verified {
		t.Fatalf("selection remained verified after drift: %+v", preflight.Selections)
	}
}

func TestD02ModelSelectCommandRejectsUnavailableAndUnknownFields(t *testing.T) {
	rt := newDualWebProviderRuntime()
	mustUpdateD02Provider(t, rt, webAIProviderSnapshot{
		Provider: ProviderGeminiWeb,
		LaneID:   "gemini-web",
		Catalog: WebAIModelCatalogDTO{
			Provider: ProviderGeminiWeb,
			Revision: "rev-1",
			Models: []WebAIModelOptionDTO{
				{ID: "disabled", Label: "Disabled", Available: false},
			},
		},
	})

	payload, _ := json.Marshal(WebAIModelSelectCommandPayload{Provider: ProviderGeminiWeb, ModelID: "disabled"})
	result, err := rt.dispatch(CommandRequest{ID: "cmd-1", Kind: CommandWebAIModelSelect, Payload: payload})
	if err == nil || result.Accepted {
		t.Fatalf("unavailable model selection accepted: result=%+v err=%v", result, err)
	}

	badPayload := json.RawMessage(`{"provider":"gemini-web","model_id":"disabled","unexpected":true}`)
	result, err = rt.dispatch(CommandRequest{ID: "cmd-2", Kind: CommandWebAIModelSelect, Payload: badPayload})
	if err == nil || result.Accepted {
		t.Fatalf("unknown command payload field accepted: result=%+v err=%v", result, err)
	}
}

func TestD02TypedQueriesExposeOnlySanitizedCatalogState(t *testing.T) {
	rt := newDualWebProviderRuntime()
	mustUpdateD02Provider(t, rt, d02ReadySnapshot(ProviderChatGPTWeb, "chatgpt-web", "chatgpt-model", "rev-1"))

	payload, _ := json.Marshal(WebAIModelsQuery{Provider: ProviderChatGPTWeb})
	raw, err := rt.query(QueryRequest{Kind: QueryWebAIModels, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	var catalog WebAIModelCatalogDTO
	if err := json.Unmarshal(raw, &catalog); err != nil {
		t.Fatal(err)
	}
	if catalog.Provider != ProviderChatGPTWeb || len(catalog.Models) != 1 || catalog.Models[0].ID != "chatgpt-model" {
		t.Fatalf("unexpected catalog projection: %+v", catalog)
	}
}

func d02ReadySnapshot(provider WebAIProvider, laneID, modelID, revision string) webAIProviderSnapshot {
	return webAIProviderSnapshot{
		Provider:      provider,
		LaneID:        laneID,
		Authenticated: true,
		Ready:         true,
		ActiveModelID: modelID,
		Catalog: WebAIModelCatalogDTO{
			Provider: provider,
			Revision: revision,
			Models: []WebAIModelOptionDTO{
				{ID: modelID, Label: modelID, Available: true},
			},
		},
	}
}

func mustUpdateD02Provider(t *testing.T, rt *dualWebProviderRuntime, snapshot webAIProviderSnapshot) {
	t.Helper()
	if err := rt.updateProvider(snapshot); err != nil {
		t.Fatal(err)
	}
}
