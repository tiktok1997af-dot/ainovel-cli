package main

import (
	"context"

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

// newGatewayWithRuntimeFactory is the testable GUI-02C.3 composition seam. The
// behavioral implementation is intentionally staged behind the RED contract.
func newGatewayWithRuntimeFactory(runtime desktopui.RuntimeClient, emit desktopEventEmitter, _ projectRuntimeFactory) *Gateway {
	return newGateway(runtime, emit)
}

func lifecycleNotImplementedResult() ProjectLifecycleResult {
	return ProjectLifecycleResult{
		ContractVersion: appruntime.ContractVersion,
		Error:           internalGatewayError(),
	}
}

func (g *Gateway) CreateProject(ProjectLifecycleRequest) ProjectLifecycleResult {
	return lifecycleNotImplementedResult()
}

func (g *Gateway) OpenProject(ProjectLifecycleRequest) ProjectLifecycleResult {
	return lifecycleNotImplementedResult()
}

func (g *Gateway) SwitchProject(ProjectLifecycleRequest) ProjectLifecycleResult {
	return lifecycleNotImplementedResult()
}

func (g *Gateway) CloseProject() ProjectLifecycleResult {
	return lifecycleNotImplementedResult()
}
