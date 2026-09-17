package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/voocel/ainovel-cli/internal/appruntime"
)

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

func (g *Gateway) CreateProject(request ProjectLifecycleRequest) ProjectLifecycleResult {
	root, result, ok := resolveProjectRoot(request.ProjectRoot)
	if !ok {
		return result
	}
	if result, ok := preflightCreateProject(root); !ok {
		return result
	}
	return pendingProjectLifecycleResult()
}

func (g *Gateway) OpenProject(request ProjectLifecycleRequest) ProjectLifecycleResult {
	root, result, ok := resolveProjectRoot(request.ProjectRoot)
	if !ok {
		return result
	}
	if result, ok := preflightOpenProject(root); !ok {
		return result
	}
	return pendingProjectLifecycleResult()
}

func (g *Gateway) SwitchProject(request ProjectLifecycleRequest) ProjectLifecycleResult {
	root, result, ok := resolveProjectRoot(request.ProjectRoot)
	if !ok {
		return result
	}
	if result, ok := preflightOpenProject(root); !ok {
		return result
	}
	return pendingProjectLifecycleResult()
}

func (g *Gateway) CloseProject() ProjectLifecycleResult {
	return pendingProjectLifecycleResult()
}

func resolveProjectRoot(projectRoot string) (string, ProjectLifecycleResult, bool) {
	projectRoot = strings.TrimSpace(projectRoot)
	if projectRoot == "" {
		return "", lifecycleErrorResult(appruntime.ErrorCodeInvalidArgument, appruntime.ErrorCategoryValidation), false
	}
	root, err := filepath.Abs(projectRoot)
	if err != nil {
		return "", lifecycleErrorResult(appruntime.ErrorCodeInvalidArgument, appruntime.ErrorCategoryValidation), false
	}
	return filepath.Clean(root), ProjectLifecycleResult{}, true
}

func preflightCreateProject(root string) (ProjectLifecycleResult, bool) {
	info, err := os.Stat(root)
	if os.IsNotExist(err) {
		return ProjectLifecycleResult{}, true
	}
	if err != nil {
		return lifecycleErrorResult(appruntime.ErrorCodeStoreRead, appruntime.ErrorCategoryStore), false
	}
	if !info.IsDir() {
		return lifecycleErrorResult(appruntime.ErrorCodePreconditionConflict, appruntime.ErrorCategoryConflict), false
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return lifecycleErrorResult(appruntime.ErrorCodeStoreRead, appruntime.ErrorCategoryStore), false
	}
	if len(entries) != 0 {
		return lifecycleErrorResult(appruntime.ErrorCodePreconditionConflict, appruntime.ErrorCategoryConflict), false
	}
	return ProjectLifecycleResult{}, true
}

func preflightOpenProject(root string) (ProjectLifecycleResult, bool) {
	info, err := os.Stat(root)
	if os.IsNotExist(err) {
		return lifecycleErrorResult(appruntime.ErrorCodeTargetNotFound, appruntime.ErrorCategoryValidation), false
	}
	if err != nil {
		return lifecycleErrorResult(appruntime.ErrorCodeStoreRead, appruntime.ErrorCategoryStore), false
	}
	if !info.IsDir() {
		return lifecycleErrorResult(appruntime.ErrorCodeInvalidArgument, appruntime.ErrorCategoryValidation), false
	}
	return ProjectLifecycleResult{}, true
}

func lifecycleErrorResult(code appruntime.ErrorCode, category appruntime.ErrorCategory) ProjectLifecycleResult {
	message := "An internal runtime error occurred."
	switch code {
	case appruntime.ErrorCodeInvalidArgument:
		message = "The request is invalid."
	case appruntime.ErrorCodeTargetNotFound:
		message = "The requested project target was not found."
	case appruntime.ErrorCodePreconditionConflict:
		message = "The project changed state before this action could be applied."
	case appruntime.ErrorCodeStoreRead:
		message = "Project data could not be read."
	}
	return ProjectLifecycleResult{
		ContractVersion: appruntime.ContractVersion,
		Error: &appruntime.AppError{
			Code:     code,
			Category: category,
			Message:  message,
		},
	}
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
