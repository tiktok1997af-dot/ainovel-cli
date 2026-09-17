package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/voocel/ainovel-cli/assets"
	"github.com/voocel/ainovel-cli/internal/appruntime"
	"github.com/voocel/ainovel-cli/internal/bootstrap"
	"github.com/voocel/ainovel-cli/internal/desktopui"
	"github.com/voocel/ainovel-cli/internal/host"
)

func defaultProjectRuntimeFactory(ctx context.Context, projectRoot string, create bool) (desktopui.RuntimeClient, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root, err := filepath.Abs(projectRoot)
	if err != nil {
		return nil, invalidProjectRootError(err)
	}
	if create {
		if _, statErr := os.Stat(root); statErr == nil {
			return nil, projectConflictError()
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return nil, internalProjectLifecycleError(statErr)
		}
		if err := os.MkdirAll(root, 0o755); err != nil {
			return nil, internalProjectLifecycleError(err)
		}
	} else {
		info, statErr := os.Stat(root)
		if errors.Is(statErr, os.ErrNotExist) {
			return nil, projectNotFoundError(statErr)
		}
		if statErr != nil {
			return nil, internalProjectLifecycleError(statErr)
		}
		if !info.IsDir() {
			return nil, invalidProjectRootError(nil)
		}
	}

	cfg, err := bootstrap.LoadConfigForProject(root)
	if err != nil {
		return nil, internalProjectLifecycleError(err)
	}
	cfg.OutputDir = root
	cfg.FillDefaults()
	bundle := assets.LoadWithLanguage(cfg.NormalizedLanguage(), cfg.Style, assets.DefaultLoadOptions(cfg.OutputDir))
	configPath, err := bootstrap.ProjectConfigPath(root)
	if err != nil {
		return nil, invalidProjectRootError(err)
	}
	core, err := host.New(cfg, bundle, host.WithConfigPath(configPath))
	if err != nil {
		slog.Error("GUI-02C.4 project Host initialization failed", "module", "desktop.project_runtime_factory", "project_root", root, "err", err)
		return nil, internalProjectLifecycleError(err)
	}
	runtime, err := appruntime.New(core)
	if err != nil {
		slog.Error("GUI-02C.4 project AppRuntime initialization failed", "module", "desktop.project_runtime_factory", "project_root", root, "err", err)
		core.Close()
		return nil, err
	}
	return runtime, nil
}

func invalidProjectRootError(cause error) *appruntime.AppError {
	return &appruntime.AppError{
		Code:     appruntime.ErrorCodeInvalidArgument,
		Category: appruntime.ErrorCategoryValidation,
		Message:  "The request is invalid.",
		Cause:    cause,
	}
}

func projectNotFoundError(cause error) *appruntime.AppError {
	return &appruntime.AppError{
		Code:     appruntime.ErrorCodeTargetNotFound,
		Category: appruntime.ErrorCategoryStore,
		Message:  "The requested project target was not found.",
		Cause:    cause,
	}
}

func projectConflictError() *appruntime.AppError {
	return &appruntime.AppError{
		Code:     appruntime.ErrorCodePreconditionConflict,
		Category: appruntime.ErrorCategoryConflict,
		Message:  "The project changed state before this action could be applied.",
	}
}

func internalProjectLifecycleError(cause error) *appruntime.AppError {
	return &appruntime.AppError{
		Code:     appruntime.ErrorCodeInternal,
		Category: appruntime.ErrorCategoryInternal,
		Message:  "An internal runtime error occurred.",
		Cause:    cause,
	}
}
