package sites

import "context"

// ModelOption is a sanitized model choice observed from the authenticated web
// UI. ID is session-derived and contains no provider credentials or account
// identity; Label is display-only text already visible in the model picker.
type ModelOption struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Available bool   `json:"available"`
}

// ModelCatalogSnapshot is the minimum provider-model state needed by the
// desktop preflight contract. Revision changes whenever the observed catalog or
// active model changes so prior verification can be invalidated fail-closed.
type ModelCatalogSnapshot struct {
	Models        []ModelOption `json:"models"`
	ActiveModelID string        `json:"active_model_id,omitempty"`
	Revision      string        `json:"revision,omitempty"`
}

// ModelCatalogObserver reads model-picker state through the visible web UI.
// Implementations must not call provider APIs or inspect cookies, tokens,
// localStorage, account identity, or conversation content.
type ModelCatalogObserver interface {
	Adapter
	ObserveModels(ctx context.Context, evaluator Evaluator) (ModelCatalogSnapshot, error)
}
