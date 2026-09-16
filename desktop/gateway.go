package main

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/voocel/ainovel-cli/internal/appruntime"
	"github.com/voocel/ainovel-cli/internal/desktopui"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const desktopEventTopic = "ainovel:desktop:event"

type desktopEventEmitter func(context.Context, string, appruntime.DesktopEvent)

// GatewaySnapshotResult keeps Snapshot on the same typed, no-raw-error desktop
// boundary as QueryResult and CommandResult.
type GatewaySnapshotResult struct {
	ContractVersion string                       `json:"contract_version"`
	Data            *appruntime.DesktopSnapshot `json:"data,omitempty"`
	Error           *appruntime.AppError         `json:"error,omitempty"`
}

// Gateway remains the only Wails-bound authority adapter. GUI-02C.2 replaces
// the raw runtime pointer with a serialized project session so every runtime
// call holds a read lease for its full duration. No project lifecycle methods
// are exported in this gate; GUI-02C.3 will build typed lifecycle operations on
// top of the same session authority.
type Gateway struct {
	mu      sync.Mutex
	session *projectSessionController
	emit    desktopEventEmitter
	ctx     context.Context
	cancel  context.CancelFunc
	sub     appruntime.EventSubscription
	done    chan struct{}
}

func newGateway(runtime desktopui.RuntimeClient, emit desktopEventEmitter) *Gateway {
	return &Gateway{session: newProjectSessionController(runtime), emit: emit}
}

// Snapshot returns authoritative desktop state through a typed envelope. It
// deliberately has no arbitrary method name or path argument.
func (g *Gateway) Snapshot() GatewaySnapshotResult {
	result := GatewaySnapshotResult{ContractVersion: appruntime.ContractVersion}
	if g == nil || g.session == nil {
		result.Error = runtimeUnavailableGatewayError()
		return result
	}

	ok := g.session.withRuntime(func(runtime desktopui.RuntimeClient) {
		snapshot, err := runtime.Snapshot(g.requestContext())
		if err != nil {
			result.Error = gatewayError(err)
			return
		}
		if snapshot.Contract.Version != appruntime.ContractVersion || snapshot.Contract.SchemaVersion != appruntime.SchemaVersion {
			result.Error = contractMismatchGatewayError()
			return
		}
		result.Data = &snapshot
	})
	if !ok {
		result.Error = runtimeUnavailableGatewayError()
	}
	return result
}

// Query delegates only the typed AppRuntime query envelope. Blank contract
// versions are upgraded to the current desktop contract; explicit mismatches
// fail closed before reaching runtime authority.
func (g *Gateway) Query(req appruntime.QueryRequest) appruntime.QueryResult {
	result := appruntime.QueryResult{
		ContractVersion: appruntime.ContractVersion,
		Kind:            req.Kind,
	}
	if !normalizeRequestContract(&req.ContractVersion) {
		result.Error = contractMismatchGatewayError()
		return result
	}
	if g == nil || g.session == nil {
		result.Error = runtimeUnavailableGatewayError()
		return result
	}

	ok := g.session.withRuntime(func(runtime desktopui.RuntimeClient) {
		out, err := runtime.Query(g.requestContext(), req)
		if err != nil {
			if out.Error != nil {
				result.Error = cloneGatewayError(out.Error)
			} else {
				result.Error = gatewayError(err)
			}
			return
		}
		if out.ContractVersion != "" && out.ContractVersion != appruntime.ContractVersion {
			result.Error = contractMismatchGatewayError()
			return
		}
		if out.Kind != "" && out.Kind != req.Kind {
			result.Error = internalGatewayError()
			return
		}
		out.ContractVersion = appruntime.ContractVersion
		out.Kind = req.Kind
		out.Error = cloneGatewayError(out.Error)
		result = out
	})
	if !ok {
		result.Error = runtimeUnavailableGatewayError()
	}
	return result
}

// Dispatch delegates only typed command intents. State-machine authority stays
// inside AppRuntime; the gateway neither predicts nor duplicates transitions.
func (g *Gateway) Dispatch(cmd appruntime.CommandRequest) appruntime.CommandResult {
	result := appruntime.CommandResult{
		ContractVersion: appruntime.ContractVersion,
		CommandID:       cmd.ID,
		RunID:           cmd.RunID,
		TaskID:          cmd.TaskID,
		Resource:        cmd.Resource,
	}
	if !normalizeRequestContract(&cmd.ContractVersion) {
		result.Error = contractMismatchGatewayError()
		return result
	}
	if g == nil || g.session == nil {
		result.Error = runtimeUnavailableGatewayError()
		return result
	}

	ok := g.session.withRuntime(func(runtime desktopui.RuntimeClient) {
		out, err := runtime.Dispatch(g.requestContext(), cmd)
		if err != nil {
			if out.Error != nil {
				result.Error = cloneGatewayError(out.Error)
			} else {
				result.Error = gatewayError(err)
			}
			return
		}
		if out.ContractVersion != "" && out.ContractVersion != appruntime.ContractVersion {
			result.Error = contractMismatchGatewayError()
			return
		}
		out.ContractVersion = appruntime.ContractVersion
		if out.CommandID == "" {
			out.CommandID = cmd.ID
		}
		out.Error = cloneGatewayError(out.Error)
		result = out
	})
	if !ok {
		result.Error = runtimeUnavailableGatewayError()
	}
	return result
}

// startup/shutdown are Wails lifecycle callbacks, intentionally unexported so
// they cannot become JavaScript bindings.
func (g *Gateway) startup(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	bridgeCtx, cancel := context.WithCancel(ctx)

	g.mu.Lock()
	g.ctx = bridgeCtx
	g.cancel = cancel
	emit := g.emit
	g.mu.Unlock()

	if emit == nil || g.session == nil {
		return
	}
	var sub appruntime.EventSubscription
	var subErr error
	ok := g.session.withRuntime(func(runtime desktopui.RuntimeClient) {
		sub, subErr = runtime.Subscribe(bridgeCtx, appruntime.EventCursor{ContractVersion: appruntime.ContractVersion})
	})
	if !ok {
		return
	}
	if subErr != nil {
		emit(bridgeCtx, desktopEventTopic, gatewayBridgeErrorEvent(subErr))
		return
	}
	done := make(chan struct{})
	g.mu.Lock()
	g.sub = sub
	g.done = done
	g.mu.Unlock()
	go g.pumpEvents(bridgeCtx, sub, done, emit)
}

func (g *Gateway) shutdown(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	g.mu.Lock()
	cancel := g.cancel
	sub := g.sub
	done := g.done
	g.cancel = nil
	g.sub = nil
	g.done = nil
	g.ctx = nil
	g.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if sub != nil {
		_ = sub.Close()
	}
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
		}
	}
	if g.session != nil {
		g.session.mutate(func(runtime desktopui.RuntimeClient) desktopui.RuntimeClient {
			if runtime != nil {
				_ = runtime.Close(ctx)
			}
			return nil
		})
	}
}

func (g *Gateway) pumpEvents(ctx context.Context, sub appruntime.EventSubscription, done chan struct{}, emit desktopEventEmitter) {
	defer close(done)
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-sub.Events():
			if !ok {
				return
			}
			if event.ContractVersion == "" {
				event.ContractVersion = appruntime.ContractVersion
			}
			if event.ContractVersion != appruntime.ContractVersion {
				continue
			}
			emit(ctx, desktopEventTopic, event)
		}
	}
}

func (g *Gateway) requestContext() context.Context {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.ctx != nil {
		return g.ctx
	}
	return context.Background()
}

func normalizeRequestContract(version *string) bool {
	if *version == "" {
		*version = appruntime.ContractVersion
		return true
	}
	return *version == appruntime.ContractVersion
}

func gatewayError(err error) *appruntime.AppError {
	if err == nil {
		return nil
	}
	var appErr *appruntime.AppError
	if errors.As(err, &appErr) {
		return cloneGatewayError(appErr)
	}
	return internalGatewayError()
}

func cloneGatewayError(err *appruntime.AppError) *appruntime.AppError {
	if err == nil {
		return nil
	}
	return &appruntime.AppError{
		Code:      err.Code,
		Category:  err.Category,
		Message:   err.Message,
		Retryable: err.Retryable,
	}
}

func runtimeUnavailableGatewayError() *appruntime.AppError {
	return &appruntime.AppError{
		Code:      appruntime.ErrorCodeRuntimeUnavailable,
		Category:  appruntime.ErrorCategoryRuntime,
		Message:   "The application runtime is unavailable.",
		Retryable: true,
	}
}

func contractMismatchGatewayError() *appruntime.AppError {
	return &appruntime.AppError{
		Code:     appruntime.ErrorCodeContractMismatch,
		Category: appruntime.ErrorCategoryValidation,
		Message:  "The desktop and core contract versions are incompatible.",
	}
}

func internalGatewayError() *appruntime.AppError {
	return &appruntime.AppError{
		Code:     appruntime.ErrorCodeInternal,
		Category: appruntime.ErrorCategoryInternal,
		Message:  "An internal runtime error occurred.",
	}
}

func gatewayBridgeErrorEvent(err error) appruntime.DesktopEvent {
	appErr := gatewayError(err)
	return appruntime.DesktopEvent{
		ContractVersion: appruntime.ContractVersion,
		Time:            time.Now().UTC(),
		Category:        "ERROR",
		Type:            "gateway_subscription_error",
		Level:           "error",
		Summary:         appErr.Message,
		Error:           appErr,
	}
}

func emitWailsDesktopEvent(ctx context.Context, topic string, event appruntime.DesktopEvent) {
	wailsruntime.EventsEmit(ctx, topic, event)
}
