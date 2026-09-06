package sites

import "context"

// ConversationSnapshot is the minimal DOM state W3 needs to distinguish an
// in-flight generation from a stable final model response and to confirm that a
// submitted prompt actually became a Gemini user turn. It intentionally
// contains no prompt text, cookies, auth tokens, localStorage values or account
// identity.
type ConversationSnapshot struct {
	Busy             bool   `json:"busy"`
	ResponseCount    int    `json:"response_count"`
	UserMessageCount int    `json:"user_message_count"`
	ComposerPresent  bool   `json:"composer_present"`
	ComposerEmpty    bool   `json:"composer_empty"`
	LastResponse     string `json:"last_response"`
	Truncated        bool   `json:"truncated"`
}

// InteractionAdapter extends the W2 read-only adapter with the minimum W3
// actions required for one prompt round trip through the already logged-in web
// UI. Implementations must never use provider HTTP APIs.
type InteractionAdapter interface {
	Adapter
	Conversation(ctx context.Context, evaluator Evaluator) (ConversationSnapshot, error)
	Submit(ctx context.Context, evaluator Evaluator, prompt string) error
	Cancel(ctx context.Context, evaluator Evaluator) (bool, error)
}
