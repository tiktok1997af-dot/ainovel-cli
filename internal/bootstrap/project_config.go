package bootstrap

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/voocel/ainovel-cli/internal/errs"
)

// ProjectConfigPath resolves the project-scoped config target without
// consulting or mutating the process working directory.
func ProjectConfigPath(projectRoot string) (string, error) {
	if strings.TrimSpace(projectRoot) == "" {
		return "", fmt.Errorf("project root is required: %w", errs.ErrConfig)
	}
	root, err := filepath.Abs(projectRoot)
	if err != nil {
		return "", fmt.Errorf("resolve project root: %w", err)
	}
	return filepath.Join(root, configDirName, "config.json"), nil
}

// LoadConfigForProject loads global configuration overlaid by the config rooted
// at projectRoot without consulting or mutating the process working directory.
// It preserves LoadConfig's migration semantics while making desktop project
// selection explicit and safe for multiple project sessions in one process.
func LoadConfigForProject(projectRoot string) (Config, error) {
	projectPath, err := ProjectConfigPath(projectRoot)
	if err != nil {
		return Config{}, err
	}

	var cfg Config
	var globalErr error
	if p := DefaultConfigPath(); p != "" {
		global, found, err := loadOptionalJSON(p)
		switch {
		case err != nil:
			globalErr = err
		case found:
			cfg = global
		}
	}

	project, found, err := loadOptionalJSON(projectPath)
	if err != nil {
		return cfg, fmt.Errorf("project config %s is invalid: %w", projectPath, err)
	}
	if found {
		return mergeConfig(cfg, project), nil
	}

	if globalErr != nil && errors.Is(globalErr, errs.ErrConfig) {
		return cfg, fmt.Errorf("global config requires WEB-only migration: %w", globalErr)
	}
	return cfg, nil
}
