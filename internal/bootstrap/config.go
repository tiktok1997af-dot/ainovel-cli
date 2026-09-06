package bootstrap

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/voocel/agentcore/llm"
	"github.com/voocel/ainovel-cli/internal/errs"
	"github.com/voocel/ainovel-cli/internal/models"
	"github.com/voocel/ainovel-cli/internal/notify"
	"github.com/voocel/ainovel-cli/internal/utils"
)

// DefaultContextWindow 模型未在 registry 登记时的兜底窗口大小。
const DefaultContextWindow = 200000

// CompactRatio 触发上下文压缩的相对阈值：tokens >= window * CompactRatio 时压缩。
// 0.85 是经验值，给"下一轮 prompt + 大工具结果"留 15% 头部空间，同时让大窗口
// 模型也能在 85% 主动压缩，避免在 1M 名义窗口下吃满才压（注意力衰退区）。
const CompactRatio = 0.85

// MinCompactReserve 是 ReserveTokens 的下限。
const MinCompactReserve = 8000

// CompactReserveTokens 按 CompactRatio 反算 ReserveTokens 并应用 MinCompactReserve floor。
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

// ProviderConfig 是 W5C 删除前暂时保留的 legacy API provider 配置。
// WEB-only 模式明确拒绝此配置进入 active runtime。
type ProviderConfig struct {
	Type              string        `json:"type,omitempty"`
	API               string        `json:"api,omitempty"`
	APIKey            string        `json:"api_key,omitempty"`
	BaseURL           string        `json:"base_url,omitempty"`
	Models            []ModelConfig `json:"models,omitempty"`
	ExtraBody         map[string]any `json:"extra_body,omitempty"`
	Extra             map[string]any `json:"extra,omitempty"`
	StreamIdleTimeout string        `json:"stream_idle_timeout,omitempty"`
}

// ModelConfig 描述 legacy provider 模型；WEB-only 模式只保留固定网页会话标签。
type ModelConfig struct {
	Name          string `json:"name"`
	ContextWindow int    `json:"context_window,omitempty"`
	JSONSchema    *bool  `json:"json_schema,omitempty"`
}

func (m *ModelConfig) UnmarshalJSON(data []byte) error {
	var legacy string
	if err := json.Unmarshal(data, &legacy); err == nil {
		m.Name = legacy
		m.ContextWindow = 0
		m.JSONSchema = nil
		return nil
	}
	type modelConfigAlias ModelConfig
	var decoded modelConfigAlias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return fmt.Errorf("model config must be a string or object: %w", err)
	}
	*m = ModelConfig(decoded)
	return nil
}

func (pc ProviderConfig) ModelConfig(name string) (ModelConfig, bool) {
	name = strings.TrimSpace(name)
	for _, model := range pc.Models {
		if strings.TrimSpace(model.Name) == name {
			return model, true
		}
	}
	return ModelConfig{}, false
}

func (c Config) ModelJSONSchema(provider, model string) *bool {
	if pc, ok := c.Providers[provider]; ok {
		if mc, ok := pc.ModelConfig(model); ok {
			return mc.JSONSchema
		}
	}
	return nil
}

const defaultStreamIdleTimeout = 5 * time.Minute

func (pc ProviderConfig) StreamIdleTimeoutValue() (time.Duration, error) {
	s := strings.TrimSpace(pc.StreamIdleTimeout)
	if s == "" {
		return defaultStreamIdleTimeout, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q (use Go duration like \"900s\" / \"15m\")", s)
	}
	if d <= 0 {
		return 0, fmt.Errorf("must be positive, got %q", s)
	}
	return d, nil
}

func (pc ProviderConfig) RequiresAPIKey(name string) bool {
	switch name {
	case "ollama", "bedrock":
		return false
	}
	return pc.Type == ""
}

func (pc ProviderConfig) ProviderType(name string) (string, error) {
	if pc.Type != "" {
		return pc.Type, nil
	}
	if llm.IsProviderRegistered(name) {
		return name, nil
	}
	return "", fmt.Errorf("provider %q 缺少 type，且不在 litellm 已知 provider 列表中: %w", name, errs.ErrConfig)
}

type ModelRef struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

type RoleConfig struct {
	Provider        string     `json:"provider,omitempty"`
	Model           string     `json:"model,omitempty"`
	Fallbacks       []ModelRef `json:"fallbacks,omitempty"`
	ReasoningEffort string     `json:"reasoning_effort,omitempty"`
}

var knownRoles = map[string]bool{
	"architect":         true,
	"writer":            true,
	"editor":            true,
	"import_segment":    true,
	"import_analyze":    true,
	"import_synthesize": true,
}

// WebAIConfig is the only AI transport configuration used by the W5 product.
// It contains browser/session metadata only; credentials remain inside Chrome.
type WebAIConfig struct {
	Enabled     bool   `json:"enabled,omitempty"`
	Site        string `json:"site,omitempty"`
	BrowserPath string `json:"browser_path,omitempty"`
	ProfileName string `json:"profile_name,omitempty"`
	ProfileDir  string `json:"profile_dir,omitempty"`
	StartURL    string `json:"start_url,omitempty"`
}

// Config 小说应用配置。
type Config struct {
	OutputDir string `json:"-"`

	// Provider/ModelName remain during W5A for store/session metadata compatibility.
	// In WEB-only mode FillDefaults pins them to web/gemini-web; they are not API selectors.
	Provider        string `json:"provider,omitempty"`
	ModelName       string `json:"model,omitempty"`
	ReasoningEffort string `json:"reasoning_effort,omitempty"`

	// Web is the active W5 AI configuration.
	Web WebAIConfig `json:"web,omitzero"`

	// Legacy API fields are retained only until W5C. WEB-only validation rejects them.
	Providers map[string]ProviderConfig `json:"providers,omitempty"`
	Roles     map[string]RoleConfig     `json:"roles,omitempty"`

	Style         string `json:"style,omitempty"`
	Language      string `json:"language,omitempty"`
	ContextWindow int    `json:"context_window,omitempty"`
	Budget        BudgetConfig `json:"budget,omitzero"`
	Notify        NotifyConfig `json:"notify,omitzero"`
}

type BudgetConfig struct {
	BookUSD   float64 `json:"book_usd,omitempty"`
	WarnRatio float64 `json:"warn_ratio,omitempty"`
	HardStop  bool    `json:"hard_stop,omitempty"`
}

func (b BudgetConfig) Enabled() bool { return b.BookUSD > 0 }

type NotifyConfig struct {
	Enabled *bool    `json:"enabled,omitempty"`
	Command string   `json:"command,omitempty"`
	Events  []string `json:"events,omitempty"`
}

func (n NotifyConfig) IsEnabled() bool { return n.Enabled == nil || *n.Enabled }

// ValidateBase validates either the WEB-only product mode or the temporary
// legacy configuration path. W5C will remove the latter entirely.
func (c *Config) ValidateBase() error {
	if c.Web.Enabled {
		if err := c.validateWebOnly(); err != nil {
			return err
		}
	} else {
		if err := validateConfigText("provider", c.Provider); err != nil {
			return err
		}
		if err := validateConfigText("model", c.ModelName); err != nil {
			return err
		}
		if c.Provider == "" {
			return fmt.Errorf("provider is required: %w", errs.ErrConfig)
		}
		if c.ModelName == "" {
			return fmt.Errorf("model is required: %w", errs.ErrConfig)
		}

		pc, ok := c.Providers[c.Provider]
		if !ok {
			return fmt.Errorf("provider %q 未在 providers 中配置凭证；若在 ./.ainovel/config.json 里覆盖了 provider，需同时声明 providers.%s（含 api_key/base_url），不能只改顶层 provider: %w", c.Provider, c.Provider, errs.ErrConfig)
		}
		if pc.RequiresAPIKey(c.Provider) && pc.APIKey == "" {
			return fmt.Errorf("provider %q has no api_key configured: %w", c.Provider, errs.ErrConfig)
		}
		if err := validateProviderConfigText(c.Provider, pc); err != nil {
			return err
		}
		if err := c.validateProviderAPI("default", c.Provider, pc); err != nil {
			return err
		}
		for name, provider := range c.Providers {
			if err := validateConfigText("provider name", name); err != nil {
				return err
			}
			if err := validateProviderConfigText(name, provider); err != nil {
				return err
			}
			if err := c.validateProviderAPI(fmt.Sprintf("provider %q", name), name, provider); err != nil {
				return err
			}
		}

		for role, rc := range c.Roles {
			if err := validateConfigText("role name", role); err != nil {
				return err
			}
			if err := validateConfigText(fmt.Sprintf("role %q provider", role), rc.Provider); err != nil {
				return err
			}
			if err := validateConfigText(fmt.Sprintf("role %q model", role), rc.Model); err != nil {
				return err
			}
			if !knownRoles[role] {
				return fmt.Errorf("unknown role %q in roles config (valid: architect/writer/editor/import_segment/import_analyze/import_synthesize): %w", role, errs.ErrConfig)
			}
			if rc.Provider == "" || rc.Model == "" {
				return fmt.Errorf("role %q must have both provider and model: %w", role, errs.ErrConfig)
			}
			if err := c.validateModelRef(fmt.Sprintf("role %q", role), ModelRef{Provider: rc.Provider, Model: rc.Model}); err != nil {
				return err
			}
			for i, fallback := range rc.Fallbacks {
				if err := validateConfigText(fmt.Sprintf("role %q fallback[%d] provider", role, i), fallback.Provider); err != nil {
					return err
				}
				if err := validateConfigText(fmt.Sprintf("role %q fallback[%d] model", role, i), fallback.Model); err != nil {
					return err
				}
				if err := c.validateModelRef(fmt.Sprintf("role %q fallback[%d]", role, i), fallback); err != nil {
					return err
				}
			}
		}
	}

	if c.Budget.BookUSD < 0 {
		return fmt.Errorf("budget.book_usd must be >= 0: %w", errs.ErrConfig)
	}
	if c.Budget.Enabled() && (c.Budget.WarnRatio <= 0 || c.Budget.WarnRatio >= 1) {
		return fmt.Errorf("budget.warn_ratio must be in (0, 1): %w", errs.ErrConfig)
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

func (c *Config) validateWebOnly() error {
	if strings.TrimSpace(c.Provider) != WebOnlyProvider || strings.TrimSpace(c.ModelName) != DefaultWebModel {
		return fmt.Errorf("WEB-only identity must be %s/%s: %w", WebOnlyProvider, DefaultWebModel, errs.ErrConfig)
	}
	if len(c.Providers) != 0 {
		return fmt.Errorf("legacy API providers are not allowed in WEB-only mode: %w", errs.ErrConfig)
	}
	fields := []struct{ name, value string }{
		{"web.site", c.Web.Site},
		{"web.browser_path", c.Web.BrowserPath},
		{"web.profile_name", c.Web.ProfileName},
		{"web.profile_dir", c.Web.ProfileDir},
		{"web.start_url", c.Web.StartURL},
	}
	for _, field := range fields {
		if err := validateConfigText(field.name, field.value); err != nil {
			return err
		}
	}
	if c.Web.Site != DefaultWebSite {
		return fmt.Errorf("unsupported WEB-only site %q; currently only %s is enabled: %w", c.Web.Site, DefaultWebSite, errs.ErrConfig)
	}
	for role, rc := range c.Roles {
		if !knownRoles[role] {
			return fmt.Errorf("unknown role %q in roles config: %w", role, errs.ErrConfig)
		}
		if rc.Provider != "" || rc.Model != "" || len(rc.Fallbacks) != 0 {
			return fmt.Errorf("role %q contains legacy provider/model/fallback routing in WEB-only mode: %w", role, errs.ErrConfig)
		}
	}
	return nil
}

func validateProviderConfigText(name string, pc ProviderConfig) error {
	fields := []struct {
		label string
		value string
	}{
		{label: fmt.Sprintf("provider %q type", name), value: pc.Type},
		{label: fmt.Sprintf("provider %q api", name), value: pc.API},
		{label: fmt.Sprintf("provider %q api_key", name), value: pc.APIKey},
		{label: fmt.Sprintf("provider %q base_url", name), value: pc.BaseURL},
	}
	for _, field := range fields {
		if err := validateConfigText(field.label, field.value); err != nil {
			return err
		}
	}
	seenModels := make(map[string]bool, len(pc.Models))
	for i, model := range pc.Models {
		modelName := strings.TrimSpace(model.Name)
		if err := validateConfigText(fmt.Sprintf("provider %q models[%d].name", name, i), model.Name); err != nil {
			return err
		}
		if modelName == "" {
			return fmt.Errorf("provider %q models[%d].name is required: %w", name, i, errs.ErrConfig)
		}
		if seenModels[modelName] {
			return fmt.Errorf("provider %q has duplicate model %q: %w", name, modelName, errs.ErrConfig)
		}
		seenModels[modelName] = true
		if model.ContextWindow < 0 {
			return fmt.Errorf("provider %q model %q context_window must be >= 0: %w", name, modelName, errs.ErrConfig)
		}
	}
	switch pc.API {
	case "", "chat", "responses":
	default:
		return fmt.Errorf("provider %q api must be chat or responses: %w", name, errs.ErrConfig)
	}
	if _, err := pc.StreamIdleTimeoutValue(); err != nil {
		return fmt.Errorf("provider %q stream_idle_timeout: %w: %w", name, err, errs.ErrConfig)
	}
	return nil
}

func validateConfigText(name, value string) error {
	if utils.ContainsControl(value) {
		return fmt.Errorf("%s contains control character: %w", name, errs.ErrConfig)
	}
	return nil
}

func (c *Config) DefaultProviderConfig() ProviderConfig {
	if c.Providers == nil {
		return ProviderConfig{}
	}
	return c.Providers[c.Provider]
}

// FillDefaults pins WEB-only runtime identity and supplies browser defaults.
func (c *Config) FillDefaults() {
	if c.OutputDir == "" {
		c.OutputDir = filepath.Join("output", "novel")
	}
	if c.Web.Enabled {
		c.Provider = WebOnlyProvider
		c.ModelName = DefaultWebModel
		if strings.TrimSpace(c.Web.Site) == "" || strings.EqualFold(strings.TrimSpace(c.Web.Site), "gemini") {
			c.Web.Site = DefaultWebSite
		} else {
			c.Web.Site = strings.ToLower(strings.TrimSpace(c.Web.Site))
		}
		if strings.TrimSpace(c.Web.ProfileName) == "" {
			c.Web.ProfileName = DefaultWebProfile
		}
	}
	if c.Providers == nil {
		c.Providers = make(map[string]ProviderConfig)
	}
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
	if c.Budget.Enabled() && c.Budget.WarnRatio == 0 {
		c.Budget.WarnRatio = 0.8
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
	CtxWindowModelConfig ContextWindowSource = "model_config"
	CtxWindowConfig      ContextWindowSource = "config"
	CtxWindowRegistry    ContextWindowSource = "registry"
	CtxWindowDefault     ContextWindowSource = "default"
)

func (c Config) ResolveContextWindow(provider, modelName string) (int, ContextWindowSource) {
	if c.Web.Enabled || strings.EqualFold(strings.TrimSpace(provider), WebOnlyProvider) {
		if c.ContextWindow > 0 {
			return c.ContextWindow, CtxWindowConfig
		}
		return DefaultContextWindow, CtxWindowDefault
	}
	if pc, ok := c.Providers[strings.TrimSpace(provider)]; ok {
		if model, found := pc.ModelConfig(modelName); found && model.ContextWindow > 0 {
			return model.ContextWindow, CtxWindowModelConfig
		}
	}
	if c.ContextWindow > 0 {
		return c.ContextWindow, CtxWindowConfig
	}
	if rw := models.DefaultRegistry().ResolveContextWindow(modelName); rw > 0 {
		return rw, CtxWindowRegistry
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
	switch source {
	case CtxWindowModelConfig:
		slog.Info("上下文窗口（来自 provider 模型配置）", attrs...)
	case CtxWindowDefault:
		slog.Warn("未识别的模型，使用兜底窗口（可显式配置 context_window）", attrs...)
	case CtxWindowConfig:
		slog.Info("上下文窗口（来自配置文件 context_window）", attrs...)
	default:
		slog.Info("上下文窗口", attrs...)
	}
}

func (c Config) CandidateModels(provider string) []string {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return nil
	}
	if c.Web.Enabled && provider == WebOnlyProvider {
		return []string{DefaultWebModel}
	}

	seen := make(map[string]bool)
	result := make([]string, 0, 4)
	add := func(model string) {
		model = strings.TrimSpace(model)
		if model == "" || seen[model] {
			return
		}
		seen[model] = true
		result = append(result, model)
	}
	if pc, ok := c.Providers[provider]; ok {
		for _, model := range pc.Models {
			add(model.Name)
		}
	}
	if c.Provider == provider {
		add(c.ModelName)
	}
	for _, rc := range c.Roles {
		if rc.Provider == provider {
			add(rc.Model)
		}
		for _, fallback := range rc.Fallbacks {
			if fallback.Provider == provider {
				add(fallback.Model)
			}
		}
	}
	return result
}

func (c Config) validateModelRef(owner string, ref ModelRef) error {
	if ref.Provider == "" || ref.Model == "" {
		return fmt.Errorf("%s must have both provider and model: %w", owner, errs.ErrConfig)
	}
	pc, ok := c.Providers[ref.Provider]
	if !ok {
		return fmt.Errorf("%s references provider %q which is not configured: %w", owner, ref.Provider, errs.ErrConfig)
	}
	if pc.RequiresAPIKey(ref.Provider) && pc.APIKey == "" {
		return fmt.Errorf("%s references provider %q which has no api_key: %w", owner, ref.Provider, errs.ErrConfig)
	}
	if err := c.validateProviderAPI(owner, ref.Provider, pc); err != nil {
		return err
	}
	return nil
}

func (c Config) validateProviderAPI(owner, providerName string, pc ProviderConfig) error {
	if pc.API == "" {
		return nil
	}
	providerType, err := pc.ProviderType(providerName)
	if err != nil {
		return fmt.Errorf("%s provider %q api 配置无法解析协议类型: %w", owner, providerName, err)
	}
	if strings.ToLower(strings.TrimSpace(providerType)) != "openai" {
		return fmt.Errorf("%s provider %q api 仅支持 OpenAI 协议 provider: %w", owner, providerName, errs.ErrConfig)
	}
	return nil
}
