package host

import (
	"context"
	"fmt"

	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/webai"
)

// Desktop run-orchestration methods expose only bounded G05 authorities to
// AppRuntime. Store and SessionManager remain private to Host.
func (h *Host) DesktopRunsList() ([]domain.RunRegistryRecord, error) {
	if h == nil || h.store == nil || h.store.Runtime == nil {
		return nil, fmt.Errorf("host: runtime store is unavailable")
	}
	return h.store.Runtime.ListRunsWithLegacyProjection()
}

func (h *Host) DesktopRunLoad(runID domain.RunID) (*domain.RunRegistryRecord, error) {
	if h == nil || h.store == nil || h.store.Runtime == nil {
		return nil, fmt.Errorf("host: runtime store is unavailable")
	}
	return h.store.Runtime.LoadRun(runID)
}

func (h *Host) DesktopRunSave(record domain.RunRegistryRecord) (domain.RunRegistryRecord, error) {
	if h == nil || h.store == nil || h.store.Runtime == nil {
		return record, fmt.Errorf("host: runtime store is unavailable")
	}
	return h.store.Runtime.SaveRun(record)
}

func (h *Host) DesktopRunAppendHistory(entry domain.RunHistoryRecord) (domain.RunHistoryRecord, error) {
	if h == nil || h.store == nil || h.store.Runtime == nil {
		return entry, fmt.Errorf("host: runtime store is unavailable")
	}
	return h.store.Runtime.AppendRunHistory(entry)
}

func (h *Host) DesktopRunHistory(runID domain.RunID) ([]domain.RunHistoryRecord, error) {
	if h == nil || h.store == nil || h.store.Runtime == nil {
		return nil, fmt.Errorf("host: runtime store is unavailable")
	}
	return h.store.Runtime.LoadRunHistory(runID)
}

func (h *Host) DesktopScheduledRuns() ([]domain.RunScheduleTicket, error) {
	if h == nil || h.store == nil || h.store.Runtime == nil {
		return nil, fmt.Errorf("host: runtime store is unavailable")
	}
	return h.store.Runtime.LoadScheduledRuns()
}

func (h *Host) DesktopEnqueueRun(runID domain.RunID, priority domain.RunSchedulerPriority) (domain.RunScheduleTicket, error) {
	if h == nil || h.store == nil || h.store.Runtime == nil {
		return domain.RunScheduleTicket{}, fmt.Errorf("host: runtime store is unavailable")
	}
	return h.store.Runtime.EnqueueScheduledRun(runID, priority)
}

func (h *Host) DesktopDequeueScheduledRun(runID domain.RunID) (*domain.RunScheduleTicket, error) {
	if h == nil || h.store == nil || h.store.Runtime == nil {
		return nil, fmt.Errorf("host: runtime store is unavailable")
	}
	return h.store.Runtime.DequeueScheduledRun(runID)
}

func (h *Host) DesktopRemoveScheduledRun(runID domain.RunID) error {
	if h == nil || h.store == nil || h.store.Runtime == nil {
		return fmt.Errorf("host: runtime store is unavailable")
	}
	return h.store.Runtime.RemoveScheduledRun(runID)
}

func (h *Host) DesktopStoryResourceKey() (domain.ResourceKey, error) {
	if h == nil || h.store == nil || h.store.Runtime == nil {
		return "", fmt.Errorf("host: runtime store is unavailable")
	}
	return h.store.Runtime.CanonicalStoryResourceKey()
}

func (h *Host) DesktopResourceLocks() ([]domain.ResourceLockState, error) {
	if h == nil || h.store == nil || h.store.Runtime == nil {
		return nil, fmt.Errorf("host: runtime store is unavailable")
	}
	return h.store.Runtime.LoadResourceLocks()
}

func (h *Host) DesktopRequestRunResource(runID domain.RunID, resource domain.ResourceKey) (domain.ResourceLockDecision, error) {
	if h == nil || h.store == nil || h.store.Runtime == nil {
		return domain.ResourceLockDecision{}, fmt.Errorf("host: runtime store is unavailable")
	}
	return h.store.Runtime.RequestRunResourceLock(runID, resource)
}

func (h *Host) DesktopReleaseRunResourceClaims(runID domain.RunID) error {
	if h == nil || h.store == nil || h.store.Runtime == nil {
		return fmt.Errorf("host: runtime store is unavailable")
	}
	return h.store.Runtime.ReleaseAllRunResourceClaims(runID)
}

// DesktopNewBrowserLanePool binds AppRuntime orchestration to the same
// SessionManager already used by Host models. G05.7 intentionally exposes one
// physical execution lane because one Host/Engine context owns this project.
func (h *Host) DesktopNewBrowserLanePool() (*webai.AdoptedBrowserLanePool, error) {
	if h == nil || h.webSession == nil {
		return nil, fmt.Errorf("host: WEB session is unavailable")
	}
	return webai.NewAdoptedBrowserLanePool(h.webSession)
}

// DesktopEnsureWebSession restores the adopted Host browser after a lane is
// released. A non-fatal readiness error is allowed while the visible Chrome
// process remains alive, matching normal WEB-only startup semantics.
func (h *Host) DesktopEnsureWebSession(ctx context.Context) error {
	if h == nil || h.webSession == nil {
		return fmt.Errorf("host: WEB session is unavailable")
	}
	snap := h.webSession.Snapshot()
	if snap.PID != 0 && snap.State != webai.SessionStopped && snap.State != webai.SessionFailed {
		return nil
	}
	snap, err := h.webSession.Start(ctx)
	if err != nil && (snap.PID == 0 || snap.State == webai.SessionFailed || snap.State == webai.SessionStopped) {
		return err
	}
	return nil
}

func (h *Host) DesktopRuntimeState() string {
	if h == nil {
		return ""
	}
	return h.Snapshot().RuntimeState
}
