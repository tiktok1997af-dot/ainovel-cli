package host

import "github.com/voocel/ainovel-cli/internal/webai"

// DesktopChatGPTLane returns the single ChatGPT lane owned by the Host model
// graph. AppRuntime adopts this exact lane instead of creating another browser
// profile/session. The lane exposes no credentials or browser storage.
func (h *Host) DesktopChatGPTLane() *webai.ChatGPTLane {
	if h == nil || h.models == nil {
		return nil
	}
	return h.models.ChatGPTLane()
}
