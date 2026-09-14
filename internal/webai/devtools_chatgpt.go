package webai

import "github.com/voocel/ainovel-cli/internal/webai/sites"

// NewChatGPTDevToolsReadinessProbe reuses the existing loopback-only Chrome
// DevTools inspection boundary with the ChatGPT-specific site adapter.
func NewChatGPTDevToolsReadinessProbe() *DevToolsReadinessProbe {
	return NewDevToolsReadinessProbe(sites.ChatGPT{})
}
