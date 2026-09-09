package host

import "github.com/voocel/ainovel-cli/internal/domain"

// DesktopRuntimeQueueAfter exposes the existing durable runtime queue to
// AppRuntime without exposing Store itself to desktop callers. Returned items
// are snapshots loaded from the canonical runtime queue; AppRuntime projects
// them into desktop DTOs before they cross the GUI boundary.
func (h *Host) DesktopRuntimeQueueAfter(afterSeq int64) ([]domain.RuntimeQueueItem, error) {
	if h == nil || h.store == nil || h.store.Runtime == nil {
		return nil, nil
	}
	return h.store.Runtime.LoadQueueAfter(afterSeq)
}
