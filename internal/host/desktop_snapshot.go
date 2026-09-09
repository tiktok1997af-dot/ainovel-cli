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

// DesktopEngineRunning exposes only the physical Engine goroutine state needed
// by AppRuntime to finish PAUSING / STOPPING / CANCELLING transitions. It does
// not expose the engine pointer or any mutation surface to desktop callers.
func (h *Host) DesktopEngineRunning() bool {
	if h == nil || h.engine == nil {
		return false
	}
	return h.engine.isRunning()
}
