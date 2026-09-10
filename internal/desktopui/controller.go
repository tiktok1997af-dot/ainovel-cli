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

// Controller owns the child-gated Creative Studio presentation controllers.
// G04.3 bootstraps Snapshot + Subscribe. G04.4 may additionally issue only the
// approved Project/Chapter/Outline Query calls. Dispatch remains closed until
// its later owning gate.
type Controller struct {
	runtime RuntimeClient
	shell   *ShellState
	sub     appruntime.EventSubscription
	nextID  uint64
}

func NewController(runtime RuntimeClient, width int) *Controller {
	return &Controller{runtime: runtime, shell: NewShell(width)}
}

func (c *Controller) Shell() *ShellState { return c.shell }

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
		return err
	}
	if !c.shell.AcceptSnapshot(requestID, snapshot) && c.shell.Error != nil {
		return errors.New(c.shell.Error.Message)
	}
	return nil
}

// PumpEvent consumes exactly one projected AppRuntime event. A renderer or
// desktop transport can decide its own scheduling without the shell creating a
// competing observer or background persistence loop.
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
		c.shell.ApplyEvent(event)
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
