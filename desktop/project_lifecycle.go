package main

import "github.com/voocel/ainovel-cli/internal/appruntime"

// ProjectLifecycleRequest is the typed Wails request for project-scoped
// lifecycle transitions. ProjectRoot is always resolved and validated in Go;
// the frontend never receives filesystem authority.
type ProjectLifecycleRequest struct {
	ProjectRoot string `json:"project_root"`
}

// ProjectLifecycleResult is the only project lifecycle result shape exposed by
// the Gateway. Implementation errors and filesystem details stay inside Go.
type ProjectLifecycleResult struct {
	ContractVersion string               `json:"contract_version"`
	ProjectRoot     string               `json:"project_root,omitempty"`
	Active          bool                 `json:"active"`
	Error           *appruntime.AppError `json:"error,omitempty"`
}

func (g *Gateway) CreateProject(ProjectLifecycleRequest) ProjectLifecycleResult {
	return pendingProjectLifecycleResult()
}

func (g *Gateway) OpenProject(ProjectLifecycleRequest) ProjectLifecycleResult {
	return pendingProjectLifecycleResult()
}

func (g *Gateway) SwitchProject(ProjectLifecycleRequest) ProjectLifecycleResult {
	return pendingProjectLifecycleResult()
}

func (g *Gateway) CloseProject() ProjectLifecycleResult {
	return pendingProjectLifecycleResult()
}

func pendingProjectLifecycleResult() ProjectLifecycleResult {
	return ProjectLifecycleResult{
		ContractVersion: appruntime.ContractVersion,
		Error: &appruntime.AppError{
			Code:     appruntime.ErrorCodeUnsupportedOperation,
			Category: appruntime.ErrorCategoryValidation,
			Message:  "This project action is not supported.",
		},
	}
}
