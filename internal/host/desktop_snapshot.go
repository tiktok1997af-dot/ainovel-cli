package host

import "github.com/voocel/ainovel-cli/internal/webai"

// WebSessionSnapshot exposes a read-only browser/session projection for
// AppRuntime. It intentionally returns only webai.SessionSnapshot, which does
// not contain credentials, cookies, tokens, or browser storage.
func (h *Host) WebSessionSnapshot() webai.SessionSnapshot {
	if h == nil {
		return webai.SessionSnapshot{State: webai.SessionStopped}
	}

	h.mu.Lock()
	session := h.webSession
	h.mu.Unlock()
	if session == nil {
		return webai.SessionSnapshot{State: webai.SessionStopped}
	}
	return session.Snapshot()
}
