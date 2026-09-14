package host

import (
	"fmt"

	"github.com/voocel/ainovel-cli/internal/bootstrap"
	"github.com/voocel/ainovel-cli/internal/flow"
)

// DesktopBindReviewCheckpoint binds the process-effective D07 checkpoint
// cadence to this Host's Store. Persisted Settings remain next-process only;
// AppRuntime supplies the already-frozen runtime value from h.cfg.
func (h *Host) DesktopBindReviewCheckpoint(interval int) error {
	if h == nil || h.store == nil {
		return fmt.Errorf("host review checkpoint binding is unavailable")
	}
	if interval < bootstrap.PipelineCheckpointMin || interval > bootstrap.PipelineCheckpointMax {
		return fmt.Errorf("review checkpoint interval must be within %d..%d", bootstrap.PipelineCheckpointMin, bootstrap.PipelineCheckpointMax)
	}
	return flow.BindRuntimeReviewInterval(h.store, interval)
}

// DesktopUnbindReviewCheckpoint releases only the process-scoped flow binding.
func (h *Host) DesktopUnbindReviewCheckpoint() {
	if h == nil {
		return
	}
	flow.UnbindRuntimeReviewInterval(h.store)
}
