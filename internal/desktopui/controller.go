package desktopui

import (
	"context"
	"errors"
	"fmt"

	"github.com/voocel/ainovel-cli/internal/appruntime"
)

var (
	ErrNilRuntime     = errors.New("desktopui: runtime client is nil")
	ErrNoSubscription = errors.New("desktopui: event subscription is not open")
)

// Controller owns desktop presentation controllers. G05.7 adds Run Center as
// an AppRuntime-only projection/control workspace; core packages remain hidden.
type Controller struct {
	runtime   RuntimeClient
	shell     *ShellState
	sub       appruntime.EventSubscription
	nextID    uint64
	lifecycle lifecycleControlPlane
	write     writeControlPlane
	creative  CreativeWorkspaceState
	runCenter RunCenterWorkspaceState
}

func NewController(runtime RuntimeClient, width int) *Controller {
	return &Controller{
		runtime:   runtime,
		shell:     NewShell(width),
		creative:  NewCreativeWorkspaceState(),
		runCenter: NewRunCenterWorkspaceState(),
	}
}

func (c *Controller) Shell() *ShellState                { return c.shell }
func (c *Controller) Creative() *CreativeWorkspaceState { return &c.creative }

func (c *Controller) Bootstrap(ctx context.Context) error {
	if c.runtime == nil {
		c.shell.Error = runtimeUnavailableViewError()
		c.shell.Load = LoadRuntimeError
		return ErrNilRuntime
	}
	if err := c.Refresh(ctx); err != nil {
		return err
	}

	sub, err := c.runtime.Subscribe(ctx, appruntime.EventCursor{
		ContractVersion: appruntime.ContractVersion,
		AfterSeq:        c.shell.LastEventSeq,
	})
	if err != nil {
		c.shell.Error = viewErrorFromError(err)
		c.shell.Load = LoadRuntimeError
		c.deactivateLifecycleControls()
		return err
	}
	if c.sub != nil {
		_ = c.sub.Close()
	}
	c.sub = sub
	return nil
}

func (c *Controller) Refresh(ctx context.Context) error {
	if c.runtime == nil {
		c.shell.Error = runtimeUnavailableViewError()
		c.shell.Load = LoadRuntimeError
		return ErrNilRuntime
	}
	requestID := c.nextRequestID()
	c.shell.BeginSnapshot(requestID)
	snapshot, err := c.runtime.Snapshot(ctx)
	if err != nil {
		c.shell.FailSnapshot(requestID, viewErrorFromError(err))
		c.deactivateLifecycleControls()
		return err
	}
	if !c.shell.AcceptSnapshot(requestID, snapshot) && c.shell.Error != nil {
		c.deactivateLifecycleControls()
		return errors.New(c.shell.Error.Message)
	}
	c.reconcileLifecycleFromSnapshot()
	return nil
}

// PumpEvent consumes exactly one projected AppRuntime event. RUN events refresh
// Run Center only when that workspace is open; all reconciliation still flows
// through fresh AppRuntime reads rather than GUI-owned state.
func (c *Controller) PumpEvent(ctx context.Context) error {
	if c.sub == nil {
		return ErrNoSubscription
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case event, ok := <-c.sub.Events():
		if !ok {
			return ErrNoSubscription
		}
		if !c.shell.ApplyEvent(event) {
			return nil
		}
		if c.observeLifecycleEvent(event) {
			return c.Refresh(ctx)
		}
		if event.Category == appruntime.RunEventCategory && c.shell.Route == RouteRunCenter {
			return c.RefreshRunCenter(ctx)
		}
		return nil
	}
}

func (c *Controller) Close(ctx context.Context) error {
	var errs []error
	if c.sub != nil {
		if err := c.sub.Close(); err != nil {
			errs = append(errs, err)
		}
		c.sub = nil
	}
	if c.runtime != nil {
		if err := c.runtime.Close(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (c *Controller) nextRequestID() string {
	c.nextID++
	return fmt.Sprintf("shell-request-%d", c.nextID)
}

func viewErrorFromError(err error) *ErrorView {
	if err == nil {
		return nil
	}
	var appErr *appruntime.AppError
	if errors.As(err, &appErr) {
		return viewErrorFromAppError(appErr)
	}
	return runtimeUnavailableViewError()
}
