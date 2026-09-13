package appruntime

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
)

const (
	QueryWebAIProviders QueryKind = "webai.providers"
	QueryWebAIModels    QueryKind = "webai.models"
	QueryWebAIPreflight QueryKind = "webai.preflight"

	CommandWebAIModelSelect CommandKind = "webai.model.select"
)

type WebAIProviderRuntimeDTO struct {
	Provider      WebAIProvider `json:"provider"`
	LaneID        string        `json:"lane_id"`
	Authenticated bool          `json:"authenticated"`
	Ready         bool          `json:"ready"`
	ActiveModelID string        `json:"active_model_id,omitempty"`
	Catalog       WebAIModelCatalogDTO `json:"catalog"`
}

type WebAIProvidersResultDTO struct {
	Providers []WebAIProviderRuntimeDTO `json:"providers"`
}

type WebAIModelsQuery struct {
	Provider WebAIProvider `json:"provider"`
}

type WebAIPreflightQuery struct {
	RequiredProviders []WebAIProvider `json:"required_providers"`
}

type WebAIModelSelectCommandPayload struct {
	Provider WebAIProvider `json:"provider"`
	ModelID  string        `json:"model_id"`
}

type WebAIModelSelectResultDTO struct {
	Selection WebAIModelSelectionDTO `json:"selection"`
}

type webAIProviderSnapshot struct {
	Provider      WebAIProvider
	LaneID        string
	Authenticated bool
	Ready         bool
	ActiveModelID string
	Catalog       WebAIModelCatalogDTO
}

type dualWebProviderRuntime struct {
	mu         sync.RWMutex
	providers  map[WebAIProvider]webAIProviderSnapshot
	selections map[WebAIProvider]WebAIModelSelectionDTO
}

func newDualWebProviderRuntime() *dualWebProviderRuntime {
	return &dualWebProviderRuntime{
		providers: map[WebAIProvider]webAIProviderSnapshot{
			ProviderGeminiWeb: {
				Provider: ProviderGeminiWeb,
				LaneID:   "gemini-web",
				Catalog: WebAIModelCatalogDTO{
					Provider: ProviderGeminiWeb,
				},
			},
			ProviderChatGPTWeb: {
				Provider: ProviderChatGPTWeb,
				LaneID:   "chatgpt-web",
				Catalog: WebAIModelCatalogDTO{
					Provider: ProviderChatGPTWeb,
				},
			},
		},
		selections: make(map[WebAIProvider]WebAIModelSelectionDTO),
	}
}

func isDualWebQueryKind(kind QueryKind) bool {
	switch kind {
	case QueryWebAIProviders, QueryWebAIModels, QueryWebAIPreflight:
		return true
	default:
		return false
	}
}

func isDualWebCommandKind(kind CommandKind) bool {
	return kind == CommandWebAIModelSelect
}

func (r *dualWebProviderRuntime) updateProvider(snapshot webAIProviderSnapshot) error {
	if r == nil {
		return ErrRuntimeUnavailable
	}
	if err := snapshot.Provider.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(snapshot.LaneID) == "" {
		return fmt.Errorf("web AI provider lane id is required")
	}
	if snapshot.Catalog.Provider != snapshot.Provider {
		return fmt.Errorf("web AI catalog provider mismatch")
	}
	if err := snapshot.Catalog.Validate(); err != nil {
		return err
	}
	active := strings.TrimSpace(snapshot.ActiveModelID)
	if active != "" && !catalogContainsModel(snapshot.Catalog, active, false) {
		return fmt.Errorf("active web AI model is not present in provider catalog")
	}
	snapshot.ActiveModelID = active
	snapshot.Catalog = cloneWebAIModelCatalog(snapshot.Catalog)

	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[snapshot.Provider] = snapshot
	if selected, ok := r.selections[snapshot.Provider]; ok {
		r.selections[snapshot.Provider] = evaluateWebAISelection(snapshot, selected.RequestedModelID)
	}
	return nil
}

func (r *dualWebProviderRuntime) providersResult() WebAIProvidersResultDTO {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ordered := []WebAIProvider{ProviderGeminiWeb, ProviderChatGPTWeb}
	result := WebAIProvidersResultDTO{Providers: make([]WebAIProviderRuntimeDTO, 0, len(ordered))}
	for _, provider := range ordered {
		snapshot, ok := r.providers[provider]
		if !ok {
			continue
		}
		result.Providers = append(result.Providers, providerSnapshotDTO(snapshot))
	}
	return result
}

func (r *dualWebProviderRuntime) models(provider WebAIProvider) (WebAIModelCatalogDTO, error) {
	if err := provider.Validate(); err != nil {
		return WebAIModelCatalogDTO{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	snapshot, ok := r.providers[provider]
	if !ok {
		return WebAIModelCatalogDTO{}, fmt.Errorf("web AI provider is not registered")
	}
	return cloneWebAIModelCatalog(snapshot.Catalog), nil
}

func (r *dualWebProviderRuntime) selectModel(provider WebAIProvider, modelID string) (WebAIModelSelectionDTO, error) {
	if err := provider.Validate(); err != nil {
		return WebAIModelSelectionDTO{}, err
	}
	modelID = strings.TrimSpace(modelID)
	if modelID == "" || len(modelID) > MaxWebAIModelIDBytes {
		return WebAIModelSelectionDTO{}, fmt.Errorf("invalid requested web AI model id")
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	snapshot, ok := r.providers[provider]
	if !ok {
		return WebAIModelSelectionDTO{}, fmt.Errorf("web AI provider is not registered")
	}
	if !catalogContainsModel(snapshot.Catalog, modelID, true) {
		return WebAIModelSelectionDTO{}, fmt.Errorf("requested web AI model is not available")
	}
	selection := evaluateWebAISelection(snapshot, modelID)
	r.selections[provider] = selection
	return selection, nil
}

func (r *dualWebProviderRuntime) preflight(required []WebAIProvider) (WebAIModelPreflightDTO, error) {
	if len(required) == 0 || len(required) > DualWebAIConcurrency {
		return WebAIModelPreflightDTO{}, fmt.Errorf("invalid required provider count")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	seen := make(map[WebAIProvider]struct{}, len(required))
	result := WebAIModelPreflightDTO{
		RequiredProviders: append([]WebAIProvider(nil), required...),
		Selections:        make([]WebAIModelSelectionDTO, 0, len(required)),
	}
	for _, provider := range required {
		if err := provider.Validate(); err != nil {
			return WebAIModelPreflightDTO{}, err
		}
		if _, ok := seen[provider]; ok {
			return WebAIModelPreflightDTO{}, fmt.Errorf("duplicate required provider")
		}
		seen[provider] = struct{}{}
		selection, ok := r.selections[provider]
		if !ok {
			selection = WebAIModelSelectionDTO{Provider: provider}
		}
		result.Selections = append(result.Selections, selection)
	}
	sort.Slice(result.Selections, func(i, j int) bool {
		return result.Selections[i].Provider < result.Selections[j].Provider
	})
	result.StartAllowed = true
	for _, selection := range result.Selections {
		if !selection.Verified {
			result.StartAllowed = false
			break
		}
	}
	return result, nil
}

func (r *dualWebProviderRuntime) query(req QueryRequest) (json.RawMessage, error) {
	switch req.Kind {
	case QueryWebAIProviders:
		if len(req.Payload) != 0 && string(req.Payload) != "null" && string(req.Payload) != "{}" {
			return nil, invalidQuery("web AI providers query does not accept payload")
		}
		return json.Marshal(r.providersResult())
	case QueryWebAIModels:
		var payload WebAIModelsQuery
		if err := decodeQueryPayload(req.Payload, &payload); err != nil {
			return nil, err
		}
		catalog, err := r.models(payload.Provider)
		if err != nil {
			return nil, invalidQuery("web AI provider is invalid")
		}
		return json.Marshal(catalog)
	case QueryWebAIPreflight:
		var payload WebAIPreflightQuery
		if err := decodeQueryPayload(req.Payload, &payload); err != nil {
			return nil, err
		}
		preflight, err := r.preflight(payload.RequiredProviders)
		if err != nil {
			return nil, invalidQuery("web AI preflight request is invalid")
		}
		return json.Marshal(preflight)
	default:
		return nil, ErrNotImplemented
	}
}

func (r *dualWebProviderRuntime) dispatch(cmd CommandRequest) (CommandResult, error) {
	result := CommandResult{
		ContractVersion: ContractVersion,
		CommandID:       cmd.ID,
		RunID:           cmd.RunID,
		TaskID:          cmd.TaskID,
	}
	if cmd.Kind != CommandWebAIModelSelect {
		return result, ErrNotImplemented
	}
	var payload WebAIModelSelectCommandPayload
	if err := decodeCommandPayload(cmd.Payload, &payload); err != nil {
		return result, err
	}
	selection, err := r.selectModel(payload.Provider, payload.ModelID)
	if err != nil {
		return result, ErrMutationPrecondition
	}
	data, err := json.Marshal(WebAIModelSelectResultDTO{Selection: selection})
	if err != nil {
		return result, err
	}
	result.Accepted = true
	result.Status = "MODEL_SELECTED"
	result.Data = data
	return result, nil
}

func providerSnapshotDTO(snapshot webAIProviderSnapshot) WebAIProviderRuntimeDTO {
	return WebAIProviderRuntimeDTO{
		Provider:      snapshot.Provider,
		LaneID:        snapshot.LaneID,
		Authenticated: snapshot.Authenticated,
		Ready:         snapshot.Ready,
		ActiveModelID: snapshot.ActiveModelID,
		Catalog:       cloneWebAIModelCatalog(snapshot.Catalog),
	}
}

func cloneWebAIModelCatalog(in WebAIModelCatalogDTO) WebAIModelCatalogDTO {
	out := in
	out.Models = append([]WebAIModelOptionDTO(nil), in.Models...)
	return out
}

func catalogContainsModel(catalog WebAIModelCatalogDTO, modelID string, requireAvailable bool) bool {
	modelID = strings.TrimSpace(modelID)
	for _, model := range catalog.Models {
		if strings.TrimSpace(model.ID) != modelID {
			continue
		}
		return !requireAvailable || model.Available
	}
	return false
}

func evaluateWebAISelection(snapshot webAIProviderSnapshot, modelID string) WebAIModelSelectionDTO {
	modelID = strings.TrimSpace(modelID)
	selection := WebAIModelSelectionDTO{
		Provider:         snapshot.Provider,
		RequestedModelID: modelID,
		ObservedModelID:  snapshot.ActiveModelID,
		CatalogRevision:  snapshot.Catalog.Revision,
		Authenticated:    snapshot.Authenticated,
		ProviderReady:    snapshot.Ready,
		ModelAvailable:   catalogContainsModel(snapshot.Catalog, modelID, true),
	}
	for _, model := range snapshot.Catalog.Models {
		if strings.TrimSpace(model.ID) == modelID {
			selection.RequestedLabel = strings.TrimSpace(model.Label)
			break
		}
	}
	selection.Verified = selection.Authenticated &&
		selection.ProviderReady &&
		selection.ModelAvailable &&
		strings.TrimSpace(selection.CatalogRevision) != "" &&
		selection.ObservedModelID == selection.RequestedModelID
	return selection
}
