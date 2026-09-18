package host

import (
	"context"
	"fmt"

	"github.com/voocel/ainovel-cli/internal/webai"
)

// DesktopSuspendWebSessionForProjectHandoff temporarily releases the persistent
// Gemini WEB browser profile before a desktop project runtime replacement is
// constructed. The Host itself remains alive so a failed replacement can roll
// back to the same project/runtime without rebuilding project state.
func (h *Host) DesktopSuspendWebSessionForProjectHandoff(ctx context.Context) error {
	if h == nil {
		return fmt.Errorf("desktop project handoff host is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	h.mu.Lock()
	session := h.webSession
	h.mu.Unlock()
	if session == nil {
		return nil
	}
	return session.Stop()
}

// DesktopResumeWebSessionForProjectHandoff restarts the same persistent Gemini
// WEB session after a failed desktop project replacement. AUTH_REQUIRED and
// transient DEGRADED readiness are valid live-browser states; only fatal start
// failure prevents rollback from republishing the old runtime.
func (h *Host) DesktopResumeWebSessionForProjectHandoff(ctx context.Context) error {
	if h == nil {
		return fmt.Errorf("desktop project handoff host is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	h.mu.Lock()
	session := h.webSession
	h.mu.Unlock()
	if session == nil {
		return nil
	}

	before := session.Snapshot()
	if before.PID != 0 && before.State != webai.SessionStopped && before.State != webai.SessionFailed {
		return nil
	}

	snap, err := session.Start(ctx)
	if err == nil {
		return nil
	}
	if snap.PID != 0 && snap.State != webai.SessionStopped && snap.State != webai.SessionFailed {
		return nil
	}
	return err
}
