package host

import (
	"fmt"

	"github.com/voocel/ainovel-cli/internal/domain"
)

// DesktopRecoverResourceLocks exposes only the bounded G05 restart primitive to
// AppRuntime. Store remains the canonical authority and records explicit
// recovery-release facts while preserving FIFO waiters.
func (h *Host) DesktopRecoverResourceLocks() ([]domain.ResourceLockState, error) {
	if h == nil || h.store == nil || h.store.Runtime == nil {
		return nil, fmt.Errorf("host: runtime store is unavailable")
	}
	return h.store.Runtime.RecoverResourceLocks()
}
