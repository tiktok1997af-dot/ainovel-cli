package bootstrap

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/voocel/ainovel-cli/internal/errs"
	"github.com/voocel/ainovel-cli/internal/notify"
	"github.com/voocel/ainovel-cli/internal/utils"
)

const (
	DefaultContextWindow = 200000
	CompactRatio         = 0.85
	MinCompactReserve    = 8000
	WebProviderName      = "web"
	WebModelName         = "gemini-web"

	PipelineModeBalanced      = "balanced"
	PipelineModeTurbo         = "turbo"
	PipelineCheckpointMin     = 5
	PipelineCheckpointMax     = 10
	PipelineCheckpointDefault = 10
)

func CompactReserveTokens(window int) int {
	if window <= 0 {
		return 0
	}
	reserve := window - int(float64(window)*CompactRatio)
	if reserve < MinCompactReserve {
		return MinCompactReserve
	}
	return reserve
}

type WebAIConfig struct {
	Enabled     bool   `json:"enabled,omitempty"`
	Site        string `json:"site,omitempty"`
	BrowserPath string `json:"browser_path,omitempty"`
	ProfileName string `json:"profile_name,omitempty"`
	StartURL    string `json:"start_url,omitempty"`
}

func (w *WebAIConfig) fillDefaults() {
	if strings.TrimSpace(w.Site) == "" {
		w.Site = WebModelName
	}
	if strings.TrimSpace(w.ProfileName) == "" {
		w.ProfileName = "default"
	}
}

type PipelineConfig struct {
	Mode               string `json:"mode,omitempty"`
	CheckpointChapters int    `json:"checkpoint_chapters,omitempty"`
}

func (p *PipelineConfig) fillDefaults() {
	if strings.TrimSpace(p.Mode) == "" {
		p.Mode = PipelineModeBalanced
	} else {
		p.Mode = strings.ToLower(strings.TrimSpace(p.Mode))
	}
	if p.CheckpointChapters == 0 {
		p.CheckpointChapters = PipelineCheckpointDefault
	}
}

func (p PipelineConfig) validate() error {
	mode := strings.ToLower(strings.TrimSpace(p.Mode))
	if mode == "" {
		mode = PipelineModeBalanced
	}
	if mode != PipelineModeBalanced && mode != PipelineModeTurbo {
		return fmt.Errorf("pipeline.mode must be balanced or turbo: %w", errs.ErrConfig)
	}
	checkpoint := p.CheckpointChapters
	if checkpoint == 0 {
		checkpoint = PipelineCheckpointDefault
	}
	if checkpoint < PipelineCheckpointMin || checkpoint > PipelineCheckpointMax {
		return fmt.Errorf("pipeline.checkpoint_chapters must be between %d and %d: %w", PipelineCheckpointMin, PipelineCheckpointMax, errs.ErrConfig)
	}
	return nil
}

type RoleConfig struct {
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
}

var knownRoles = map[string]bool{
	"architect":         true,
	"writer":            true,
	"editor":            true,
	"repair":            true,
	"import_segment":    true,
	"import_analyze":    true,
	"import_synthesize": true,
}

type Config struct {
	OutputDir string `json:"-"`

	Web      WebAIConfig    `json:"web,omitzero"`
	Pipeline PipelineConfig `json:"pipeline,omitzero"`

	Provider  string `json:"-"`
	ModelName string `json:"-"`

	ReasoningEffort string                `json:"reasoning_effort,omitempty"`
	Roles           map[string]RoleConfig `json:"roles,omitempty"`

	Style         string `json:"style,omitempty"`
	Language      string `json:"language,omitempty"`
	ContextWindow int    `json:"context_window,omitempty"`

	Notify NotifyConfig `json:"notify,omitzero"`
}

type NotifyConfig struct {
	Enabled *bool    `json:"enabled,omitempty"`
	Command string   `json:"command,omitempty"`
	Events  []string `json:"events,omitempty"`
}

func (n NotifyConfig) IsEnabled() bool { return n.Enabled == nil || *n.Enabled }

const LegacyAPIMigrationHint = "legacy AI provider/API configuration is no longer supported; set web.enabled=true and web.site=gemini-web, then remove provider/providers/api_key/base_url, budget, and role provider/model/fallback routing"

func (c *Config) ValidateBase() error {
	if !c.Web.Enabled {
		return fmt.Errorf("WEB-only runtime requires web.enabled=true and web.site=gemini-web; login is completed manually in the visible Chrome session: %w", errs.ErrConfig)
	}
	return c.validateWebOnly()
}

func (c *Config) validateWebOnly() error {
	c.Web.fillDefaults()
	site := strings.ToLower(strings.TrimSpace(c.Web.Site))
	if site == "gemini" {
		site = WebModelName
		c.Web.Site = site
	}
	if site != WebModelName {
		return fmt.Errorf("web.site %q is not supported; W5 currently supports gemini-web only: %w", c.Web.Site, errs.ErrConfig)
	}
	if err := c.Pipeline.validate(); err != nil {
		return err
	}

	for _, field := range []struct{ name, value string }{
		{"web.site", c.Web.Site},
		{"web.browser_path", c.Web.BrowserPath},
		{"web.profile_name", c.Web.ProfileName},
		{"web.start_url", c.Web.StartURL},
		{"reasoning_effort", c.ReasoningEffort},
	} {
		if err := validateConfigText(field.name, field.value); err != nil {
			return err
		}
	}
	profile := strings.TrimSpace(c.Web.ProfileName)
	if profile == "." || profile == ".." || strings.ContainsAny(profile, `/\\`) {
		return fmt.Errorf("web.profile_name %q is invalid: %w", c.Web.ProfileName, errs.ErrConfig)
	}

	for role, rc := range c.Roles {
		if !knownRoles[role] {
			return fmt.Errorf("unknown role %q in roles config: %w", role, errs.ErrConfig)
		}
		if err := validateConfigText(fmt.Sprintf("role %q reasoning_effort", role), rc.ReasoningEffort); err != nil {
			return err
		}
	}

	if c.ContextWindow < 0 {
		return fmt.Errorf("context_window must be >= 0: %w", errs.ErrConfig)
	}
	if err := validateConfigText("notify.command", c.Notify.Command); err != nil {
		return err
	}
	for _, ev := range c.Notify.Events {
		if !notify.IsKnownKind(ev) {
			return fmt.Errorf("unknown notify event %q (valid: %s): %w", ev, strings.Join(notify.Kinds(), "/"), errs.ErrConfig)
		}
	}
	return nil
}

func validateConfigText(name, value string) error {
	if utils.ContainsControl(value) {
		return fmt.Errorf("%s contains control character: %w", name, errs.ErrConfig)
	}
	return nil
}

func (c *Config) FillDefaults() {
	if c.OutputDir == "" {
		c.OutputDir = filepath.Join("output", "novel")
	}
	if c.Web.Enabled {
		c.Web.fillDefaults()
		c.Provider = WebProviderName
		c.ModelName = WebModelName
	}
	c.Pipeline.fillDefaults()
	if c.Roles == nil {
		c.Roles = make(map[string]RoleConfig)
	}
	if c.Style == "" {
		c.Style = "default"
	}
	if c.Language == "" {
		c.Language = "vi"
	} else {
		c.Language = strings.ToLower(strings.TrimSpace(c.Language))
	}
}

func (c Config) NormalizedLanguage() string {
	lang := strings.ToLower(strings.TrimSpace(c.Language))
	if lang == "zh" || lang == "chinese" || lang == "cn" {
		return "zh"
	}
	return "vi"
}

type ContextWindowSource string

const (
	CtxWindowConfig  ContextWindowSource = "config"
	CtxWindowDefault ContextWindowSource = "default"
)

func (c Config) ResolveContextWindow(_, _ string) (int, ContextWindowSource) {
	if c.ContextWindow > 0 {
		return c.ContextWindow, CtxWindowConfig
	}
	return DefaultContextWindow, CtxWindowDefault
}

func (c Config) ResolveReasoningEffort(role string) string {
	if role != "" && role != "default" {
		if rc, ok := c.Roles[role]; ok && rc.ReasoningEffort != "" {
			return rc.ReasoningEffort
		}
	}
	return c.ReasoningEffort
}

func LogContextWindowChoice(role, model string, window int, source ContextWindowSource) {
	attrs := []any{"module", "context", "role", role, "model", model, "window", window, "source", source}
	if source == CtxWindowConfig {
		slog.Info("上下文窗口（来自本地配置 context_window）", attrs...)
		return
	}
	slog.Warn("未配置 context_window，WEB-only 使用本地兜底窗口", attrs...)
}
