package main

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/voocel/ainovel-cli/internal/appruntime"
	"github.com/voocel/ainovel-cli/internal/desktopui"
)

// ProjectLifecycleRequest is the only path-bearing request accepted by the
// GUI-02C.3 project lifecycle surface. Filesystem interpretation remains in Go.
type ProjectLifecycleRequest struct {
	ProjectRoot string `json:"project_root"`
}

// ProjectLifecycleResult keeps project lifecycle on the same typed/no-raw-error
// Wails boundary as Snapshot, Query, and Dispatch.
type ProjectLifecycleResult struct {
	ContractVersion string                      `json:"contract_version"`
	ProjectRoot     string                      `json:"project_root,omitempty"`
	Data            *appruntime.DesktopSnapshot `json:"data,omitempty"`
	Error           *appruntime.AppError        `json:"error,omitempty"`
}

type projectRuntimeFactory func(context.Context, string, bool) (desktopui.RuntimeClient, error)

func newGatewayWithRuntimeFactory(runtime desktopui.RuntimeClient, emit desktopEventEmitter, factory projectRuntimeFactory) *Gateway {
	gateway := newGateway(runtime, emit)
	if factory != nil {
		gateway.runtimeFactory = factory
	}
	return gateway
}

// CreateProject creates a new project directory through the Go-owned runtime
// factory and activates the resulting AppRuntime as the sole project session.
func (g *Gateway) CreateProject(req ProjectLifecycleRequest) ProjectLifecycleResult {
	return g.activateProject(req, true, false)
}

// OpenProject opens an existing project only when there is no active session.
// Switching an active session is intentionally a separate typed operation.
func (g *Gateway) OpenProject(req ProjectLifecycleRequest) ProjectLifecycleResult {
	return g.activateProject(req, false, false)
}

// SwitchProject prepares the replacement runtime, then stops the old event
// bridge before entering the controller's exclusive replacement operation.
func (g *Gateway) SwitchProject(req ProjectLifecycleRequest) ProjectLifecycleResult {
	return g.activateProject(req, false, true)
}

// CloseProject stops the current event bridge and then waits for the session
// controller to acquire exclusive ownership before closing the active runtime.
func (g *Gateway) CloseProject() ProjectLifecycleResult {
	result := ProjectLifecycleResult{ContractVersion: appruntime.ContractVersion}
	if g == nil {
		result.Error = runtimeUnavailableGatewayError()
		return result
	}
	g.lifecycleMu.Lock()
	defer g.lifecycleMu.Unlock()
	if !g.hasRuntime() {
		result.Error = runtimeUnavailableGatewayError()
		return result
	}
	ctx := g.requestContext()
	g.stopEventBridge(ctx)
	if err := g.replaceRuntime(ctx, nil); err != nil {
		result.Error = gatewayError(err)
		return result
	}
	g.mu.Lock()
	g.projectRoot = ""
	g.mu.Unlock()
	return result
}

func (g *Gateway) activateProject(req ProjectLifecycleRequest, create, switching bool) ProjectLifecycleResult {
	result := ProjectLifecycleResult{ContractVersion: appruntime.ContractVersion}
	if g == nil {
		result.Error = runtimeUnavailableGatewayError()
		return result
	}
	root, err := normalizeProjectRoot(req.ProjectRoot)
	if err != nil {
		result.Error = invalidArgumentGatewayError()
		return result
	}

	g.lifecycleMu.Lock()
	defer g.lifecycleMu.Unlock()
	active := g.hasRuntime()
	if switching {
		if !active {
			result.Error = runtimeUnavailableGatewayError()
			return result
		}
		g.mu.Lock()
		currentRoot := g.projectRoot
		g.mu.Unlock()
		if currentRoot != "" && currentRoot == root {
			result.Error = commandNotAllowedGatewayError()
			return result
		}
	} else if active {
		result.Error = commandNotAllowedGatewayError()
		return result
	}

	factory := g.runtimeFactory
	if factory == nil {
		factory = defaultProjectRuntimeFactory
	}
	ctx := g.requestContext()
	replacement, err := factory(ctx, root, create)
	if err != nil {
		result.Error = gatewayError(err)
		return result
	}
	if replacement == nil {
		result.Error = internalGatewayError()
		return result
	}

	// The old session's event stream is cut before exclusive runtime replacement.
	// replaceRuntime then waits for all in-flight Snapshot/Query/Dispatch leases.
	g.stopEventBridge(ctx)
	if err := g.replaceRuntime(ctx, replacement); err != nil {
		_ = replacement.Close(ctx)
		result.Error = gatewayError(err)
		return result
	}
	if err := g.startEventBridge(replacement); err != nil {
		_ = g.replaceRuntime(ctx, nil)
		result.Error = gatewayError(err)
		return result
	}
	g.mu.Lock()
	g.projectRoot = root
	g.mu.Unlock()

	snapshot := g.Snapshot()
	if snapshot.Error != nil {
		g.stopEventBridge(ctx)
		_ = g.replaceRuntime(ctx, nil)
		g.mu.Lock()
		g.projectRoot = ""
		g.mu.Unlock()
		result.Error = snapshot.Error
		return result
	}
	result.ProjectRoot = root
	result.Data = snapshot.Data
	return result
}

func normalizeProjectRoot(root string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", filepath.ErrBadPattern
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	return filepath.Clean(absolute), nil
}
