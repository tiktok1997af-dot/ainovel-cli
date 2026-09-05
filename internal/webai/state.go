package webai

import "time"

// SessionState is the browser/login lifecycle exposed to the host and UI.
type SessionState string

const (
	SessionStarting     SessionState = "STARTING"
	SessionAuthRequired SessionState = "AUTH_REQUIRED"
	SessionReady        SessionState = "READY"
	SessionBusy         SessionState = "BUSY"
	SessionDegraded     SessionState = "DEGRADED"
	SessionFailed       SessionState = "FAILED"
	SessionStopped      SessionState = "STOPPED"
)

// SessionSnapshot is a read-only view of the current browser session.
// It intentionally contains no cookies, passwords, tokens or browser storage.
type SessionSnapshot struct {
	State       SessionState `json:"state"`
	Site        string       `json:"site,omitempty"`
	BrowserPath string       `json:"browser_path,omitempty"`
	ProfileDir  string       `json:"profile_dir,omitempty"`
	PID         int          `json:"pid,omitempty"`
	StartedAt   time.Time    `json:"started_at,omitempty"`
	ChangedAt   time.Time    `json:"changed_at"`
	Reason      string       `json:"reason,omitempty"`
}

func validSessionState(state SessionState) bool {
	switch state {
	case SessionStarting, SessionAuthRequired, SessionReady, SessionBusy, SessionDegraded, SessionFailed, SessionStopped:
		return true
	default:
		return false
	}
}
