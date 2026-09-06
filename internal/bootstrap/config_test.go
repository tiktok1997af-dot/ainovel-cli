package bootstrap

import (
	"errors"
	"strings"
	"testing"

	"github.com/voocel/ainovel-cli/internal/errs"
	"github.com/voocel/ainovel-cli/internal/notify"
)

func webConfig() Config {
	return Config{Web: WebAIConfig{Enabled: true, Site: "gemini-web"}}
}

func TestConfigResolveReasoningEffort(t *testing.T) {
	cfg := webConfig()
	cfg.ReasoningEffort = "low"
	cfg.Roles = map[string]RoleConfig{
		"writer":    {ReasoningEffort: "high"},
		"architect": {},
	}
	for role, want := range map[string]string{
		"writer": "high", "architect": "low", "editor": "low", "": "low", "default": "low", "arbiter": "low",
	} {
		if got := cfg.ResolveReasoningEffort(role); got != want {
			t.Errorf("ResolveReasoningEffort(%q) = %q, want %q", role, got, want)
		}
	}
}

func TestValidateBaseRequiresWebOnlyRuntime(t *testing.T) {
	cfg := Config{}
	err := cfg.ValidateBase()
	if !errors.Is(err, errs.ErrConfig) || !strings.Contains(err.Error(), "web.enabled=true") {
		t.Fatalf("missing WEB-only requirement: %v", err)
	}
}

func TestValidateBaseRejectsLegacyAPIConfigWithMigrationHint(t *testing.T) {
	cfg := Config{
		Provider: "openai", ModelName: "gpt-5",
		Providers: map[string]ProviderConfig{"openai": {APIKey: "secret", BaseURL: "https://api.example/v1"}},
	}
	err := cfg.ValidateBase()
	if !errors.Is(err, errs.ErrConfig) {
		t.Fatalf("legacy config must wrap ErrConfig: %v", err)
	}
	for _, want := range []string{"legacy AI provider/API", "web.enabled=true", "gemini-web", "api_key", "base_url"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("migration error missing %q: %v", want, err)
		}
	}
}

func TestValidateBaseRejectsLegacyRoleRoutingInsideWebMode(t *testing.T) {
	cfg := webConfig()
	cfg.Roles = map[string]RoleConfig{"writer": {Provider: "openai", Model: "gpt-5"}}
	if err := cfg.ValidateBase(); !errors.Is(err, errs.ErrConfig) || !strings.Contains(err.Error(), "legacy provider/model/fallback") {
		t.Fatalf("legacy role routing must fail closed: %v", err)
	}
}

func TestValidateBaseRejectsNonConfigurableRoles(t *testing.T) {
	for _, role := range []string{"coordinator", "arbiter"} {
		cfg := webConfig()
		cfg.Roles = map[string]RoleConfig{role: {ReasoningEffort: "low"}}
		if err := cfg.ValidateBase(); !errors.Is(err, errs.ErrConfig) {
			t.Fatalf("roles.%s must be rejected, got %v", role, err)
		}
	}
}

func TestValidateBaseNotifyEventsMatchRuntimeContract(t *testing.T) {
	cfg := webConfig()
	cfg.Notify = NotifyConfig{Events: notify.Kinds()}
	if err := cfg.ValidateBase(); err != nil {
		t.Fatalf("current notify kinds must validate: %v", err)
	}
	cfg.Notify = NotifyConfig{Events: []string{"repeat"}}
	if err := cfg.ValidateBase(); !errors.Is(err, errs.ErrConfig) {
		t.Fatalf("legacy repeat event must be rejected: %v", err)
	}
}
