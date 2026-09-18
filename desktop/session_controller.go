package main

import (
	"context"
	"errors"
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

// transitionRuntime owns the complete session handoff under one exclusive
// controller lock. The old runtime is hidden and closed first. prepare then
// runs while no runtime is visible to Gateway calls; only after prepare succeeds
// is replacement published. activate runs while the same lock is still held so
// event/session ownership can be committed before readers are released.
func (c *projectSessionController) transitionRuntime(
	ctx context.Context,
	replacement desktopui.RuntimeClient,
	prepare func() error,
	activate func(),
) error {
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
	if prepare != nil {
		if err := prepare(); err != nil {
			return err
		}
	}
	c.runtime = replacement
	if activate != nil {
		activate()
	}
	return nil
}

// handoffRuntime performs a rollback-capable runtime replacement while holding
// the controller's exclusive ownership lock. The old runtime is hidden from
// Gateway readers before build runs, but it is not closed until the replacement
// is fully constructed. If build fails, rollback must restore any temporarily
// released external ownership (for example the persistent WEB browser profile)
// before the old runtime is republished.
func (c *projectSessionController) handoffRuntime(
	ctx context.Context,
	build func(desktopui.RuntimeClient) (desktopui.RuntimeClient, error),
	rollback func(desktopui.RuntimeClient) error,
	prepare func(desktopui.RuntimeClient) error,
	activate func(),
) error {
	if c == nil {
		return errors.New("project session controller is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	old := c.runtime
	if old == nil {
		return errors.New("project session runtime is unavailable")
	}

	c.runtime = nil
	replacement, err := build(old)
	if err != nil {
		if rollback != nil {
			if rollbackErr := rollback(old); rollbackErr != nil {
				_ = old.Close(ctx)
				return errors.Join(err, rollbackErr)
			}
		}
		c.runtime = old
		return err
	}
	if replacement == nil {
		if rollback != nil {
			if rollbackErr := rollback(old); rollbackErr != nil {
				_ = old.Close(ctx)
				return errors.Join(errors.New("replacement runtime is nil"), rollbackErr)
			}
		}
		c.runtime = old
		return errors.New("replacement runtime is nil")
	}
	if replacement == old {
		c.runtime = old
		return errors.New("replacement runtime must differ from active runtime")
	}

	if err := old.Close(ctx); err != nil {
		_ = replacement.Close(ctx)
		return err
	}
	if prepare != nil {
		if err := prepare(replacement); err != nil {
			_ = replacement.Close(ctx)
			return err
		}
	}

	c.runtime = replacement
	if activate != nil {
		activate()
	}
	return nil
}

// replaceRuntime waits for all in-flight runtime leases, deactivates and closes
// the old runtime while holding exclusive ownership, then activates replacement.
// If closing the old runtime fails, the controller remains fail-closed with no
// active runtime.
func (c *projectSessionController) replaceRuntime(ctx context.Context, replacement desktopui.RuntimeClient) error {
	return c.transitionRuntime(ctx, replacement, nil, nil)
}

func (c *projectSessionController) closeRuntime(ctx context.Context) error {
	return c.replaceRuntime(ctx, nil)
}
