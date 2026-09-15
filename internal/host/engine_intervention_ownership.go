package host

import (
	"log/slog"
	"time"

	"github.com/voocel/ainovel-cli/internal/flow"
)

// completeOwnedInterventionDispatch releases the replay obligation only after
// the exact intervention dispatch has completed successfully. A different
// routed instruction must never consume this ownership token.
func (e *engine) completeOwnedInterventionDispatch(inst *flow.Instruction) {
	if inst == nil {
		return
	}
	key := instructionKey(inst)
	e.mu.Lock()
	if e.ownedDispatchKey == key {
		e.ownedDispatchKey = ""
		e.ownedDispatchText = ""
	}
	e.mu.Unlock()
}

// restoreOwnedInterventionDispatch closes the pending -> next -> takeNext exit
// window. Host clears PendingSteer after enqueue succeeds, so once a dispatch
// leaves e.pending the Engine owns the obligation to either finish that exact
// dispatch or put the original steer back for Resume/Continue replay.
func (e *engine) restoreOwnedInterventionDispatch() {
	e.mu.Lock()
	text := e.ownedDispatchText
	e.ownedDispatchKey = ""
	e.ownedDispatchText = ""
	e.mu.Unlock()
	if text == "" {
		return
	}
	if err := e.store.RunMeta.SetPendingSteer(text); err != nil {
		slog.Warn("在途干预回存失败", "module", "engine", "err", err)
		if e.emitEvent != nil {
			e.emitEvent(Event{Time: time.Now(), Category: "ERROR", Level: "error",
				Summary: "引擎退出时在途干预回存失败: " + err.Error()})
		}
		return
	}
	if e.emitEvent != nil {
		e.emitEvent(Event{Time: time.Now(), Category: "SYSTEM", Level: "warn",
			Summary: "引擎已停,在途裁定派单未完成;干预已保留,继续创作时自动重新裁定"})
	}
}
