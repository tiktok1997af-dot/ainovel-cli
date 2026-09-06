package bootstrap

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/voocel/ainovel-cli/internal/errs"
)

const configDirName = ".ainovel"

func DefaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, configDirName, "config.json")
}

func DefaultConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, configDirName)
}

func configDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	dir := filepath.Join(home, configDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create config dir: %w", err)
	}
	return dir, nil
}

func projectConfigPath() string {
	return filepath.Join(configDirName, "config.json")
}

// EffectiveConfigPath is the browser/creative settings write target. Existing
// project configuration wins over the global file; no project file is created
// implicitly.
func EffectiveConfigPath() string {
	rel := projectConfigPath()
	if _, err := os.Stat(rel); err == nil {
		if abs, err := filepath.Abs(rel); err == nil {
			return abs
		}
		return rel
	}
	return DefaultConfigPath()
}

// LoadConfig loads global then project WEB-only configuration. A malformed or
// legacy global file may be ignored when a valid project file exists. If the
// legacy global file is the only config, its migration error is returned so the
// user receives actionable guidance rather than a generic missing-config error.
func LoadConfig() (Config, error) {
	var cfg Config
	var globalErr error

	if p := DefaultConfigPath(); p != "" {
		global, found, err := loadOptionalJSON(p)
		switch {
		case err != nil:
			globalErr = err
			slog.Warn("全局配置不可用，等待项目级配置覆盖", "module", "config", "path", p, "err", err)
		case found:
			cfg = global
		}
	}

	project, found, err := loadOptionalJSON(projectConfigPath())
	if err != nil {
		return cfg, fmt.Errorf("项目级配置 ./.ainovel/config.json 解析失败（请检查 WEB-only 配置）: %w", err)
	}
	if found {
		return mergeConfig(cfg, project), nil
	}

	if globalErr != nil && errors.Is(globalErr, errs.ErrConfig) {
		return cfg, fmt.Errorf("全局配置需要迁移为 WEB-only: %w", globalErr)
	}
	return cfg, nil
}

func loadOptionalJSON(path string) (Config, bool, error) {
	cfg, err := loadJSONFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, false, nil
		}
		return Config{}, false, err
	}
	return cfg, true, nil
}

func LoadConfigFile(path string) (Config, error) {
	return loadJSONFile(path)
}

var forbiddenLegacyTopLevelKeys = []string{
	"provider", "model", "providers", "api_key", "base_url", "api",
	"extra", "extra_body", "stream_idle_timeout",
}

var forbiddenLegacyRoleKeys = []string{"provider", "model", "fallbacks"}

// detectLegacyAPIConfig runs before decoding into Config. This is essential:
// after C4 removed API-era struct fields, encoding/json would otherwise ignore
// old keys and silently reinterpret an old file as WEB-only configuration.
func detectLegacyAPIConfig(data []byte) error {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return nil // the normal decode path will return the syntax error
	}
	for _, key := range forbiddenLegacyTopLevelKeys {
		if _, ok := root[key]; ok {
			return fmt.Errorf("%s (found legacy key %q): %w", LegacyAPIMigrationHint, key, errs.ErrConfig)
		}
	}

	rolesRaw, ok := root["roles"]
	if !ok {
		return nil
	}
	var roles map[string]map[string]json.RawMessage
	if err := json.Unmarshal(rolesRaw, &roles); err != nil {
		return nil
	}
	for role, fields := range roles {
		for _, key := range forbiddenLegacyRoleKeys {
			if _, ok := fields[key]; ok {
				return fmt.Errorf("%s (roles.%s contains legacy key %q): %w", LegacyAPIMigrationHint, role, key, errs.ErrConfig)
			}
		}
	}
	return nil
}

func loadJSONFile(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	cleaned := stripJSONComments(data)
	if err := detectLegacyAPIConfig(cleaned); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	var cfg Config
	if err := json.Unmarshal(cleaned, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return cfg, nil
}

// mergeConfig merges browser and provider-neutral creative settings only.
func mergeConfig(base, overlay Config) Config {
	if overlay.Web != (WebAIConfig{}) {
		if overlay.Web.Enabled {
			base.Web.Enabled = true
		}
		if overlay.Web.Site != "" {
			base.Web.Site = overlay.Web.Site
		}
		if overlay.Web.BrowserPath != "" {
			base.Web.BrowserPath = overlay.Web.BrowserPath
		}
		if overlay.Web.ProfileName != "" {
			base.Web.ProfileName = overlay.Web.ProfileName
		}
		if overlay.Web.StartURL != "" {
			base.Web.StartURL = overlay.Web.StartURL
		}
	}
	if overlay.ReasoningEffort != "" {
		base.ReasoningEffort = overlay.ReasoningEffort
	}
	if overlay.Style != "" {
		base.Style = overlay.Style
	}
	if overlay.Language != "" {
		base.Language = overlay.Language
	}
	if overlay.ContextWindow > 0 {
		base.ContextWindow = overlay.ContextWindow
	}

	if len(overlay.Roles) > 0 {
		if base.Roles == nil {
			base.Roles = make(map[string]RoleConfig)
		}
		for role, incoming := range overlay.Roles {
			current := base.Roles[role]
			if incoming.ReasoningEffort != "" {
				current.ReasoningEffort = incoming.ReasoningEffort
			}
			base.Roles[role] = current
		}
	}

	if overlay.Budget != (BudgetConfig{}) {
		base.Budget = overlay.Budget
	}
	if overlay.Notify.Enabled != nil || overlay.Notify.Command != "" || len(overlay.Notify.Events) > 0 {
		base.Notify = overlay.Notify
	}
	return base
}

func CloneConfig(cfg Config) Config {
	clone := cfg
	clone.Roles = make(map[string]RoleConfig, len(cfg.Roles))
	for role, rc := range cfg.Roles {
		clone.Roles[role] = rc
	}
	clone.Notify.Events = append([]string(nil), cfg.Notify.Events...)
	return clone
}

func stripJSONComments(data []byte) []byte {
	out := make([]byte, 0, len(data))
	inString := false
	escaped := false

	for i := 0; i < len(data); i++ {
		b := data[i]
		if escaped {
			out = append(out, b)
			escaped = false
			continue
		}
		if inString {
			out = append(out, b)
			if b == '\\' {
				escaped = true
			} else if b == '"' {
				inString = false
			}
			continue
		}
		if b == '"' {
			inString = true
			out = append(out, b)
			continue
		}
		if b == '/' && i+1 < len(data) && data[i+1] == '/' {
			for i < len(data) && data[i] != '\n' {
				i++
			}
			if i < len(data) {
				out = append(out, '\n')
			}
			continue
		}
		out = append(out, b)
	}
	return out
}

func WriteStartupError(msg string) string {
	dir := DefaultConfigDir()
	if dir == "" {
		return ""
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ""
	}
	path := filepath.Join(dir, "last-error.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return ""
	}
	defer f.Close()
	if _, err := fmt.Fprintf(f, "[%s] %s\n", time.Now().Format(time.RFC3339), msg); err != nil {
		return ""
	}
	return path
}

// SaveConfig persists only a validated WEB-only configuration. Runtime
// Provider/ModelName aliases are json:"-" and therefore cannot leak to disk.
func SaveConfig(path string, cfg Config) error {
	persist := CloneConfig(cfg)
	persist.FillDefaults()
	if err := persist.ValidateBase(); err != nil {
		return fmt.Errorf("refusing to persist non-WEB configuration: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(persist, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	committed := false
	defer func() {
		_ = tmp.Close()
		if !committed {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return err
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	committed = true
	return nil
}
