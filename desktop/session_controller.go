package main

import (
	"context"
	"sync"

	"github.com/voocel/ainovel-cli/internal/desktopui"
)

// projectSessionController serializes ownership changes of the single active
// project-scoped runtime against in-flight Gateway calls. GUI-02C.2 keeps this
// foundation unexported; typed Create/Open/Switch/Close bindings belong to the
// later lifecycle subgate.
type projectSessionController struct {
	mu      sync.RWMutex
	runtime desktopui.RuntimeClient
}

func newProjectSessionController(runtime desktopui.RuntimeClient) *projectSessionController {
	return &projectSessionController{runtime: runtime}
}

// acquireRuntime holds a read lease for the entire caller operation. The
// returned release function must be called exactly once when ok is true.
func (c *projectSessionController) acquireRuntime() (desktopui.RuntimeClient, func(), bool) {
	if c == nil {
		return nil, nil, false
	}
	c.mu.RLock()
	if c.runtime == nil {
		c.mu.RUnlock()
		return nil, nil, false
	}
	return c.runtime, c.mu.RUnlock, true
}

// replaceRuntime waits for all in-flight runtime leases, deactivates and closes
// the old runtime while holding exclusive ownership, then activates replacement.
// If closing the old runtime fails, the controller remains fail-closed with no
// active runtime.
func (c *projectSessionController) replaceRuntime(ctx context.Context, replacement desktopui.RuntimeClient) error {
	if c == nil {
		if replacement != nil {
			return replacement.Close(ctx)
		}
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	old := c.runtime
	if old == replacement {
		return nil
	}
	c.runtime = nil
	if old != nil {
		if err := old.Close(ctx); err != nil {
			return err
		}
	}
	c.runtime = replacement
	return nil
}

func (c *projectSessionController) closeRuntime(ctx context.Context) error {
	return c.replaceRuntime(ctx, nil)
}
