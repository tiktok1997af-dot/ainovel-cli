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

// ModelGraphSummary returns the authoritative strict-role model graph owned by
// this Host. It is intentionally read-only and contains provider/model labels
// only; browser credentials, profile contents, and session data are excluded.
func (h *Host) ModelGraphSummary() string {
	if h == nil || h.models == nil {
		return "default=unavailable"
	}
	return h.models.Summary()
}
