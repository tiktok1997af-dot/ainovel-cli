package bootstrap

import (
	"errors"
	"strings"
	"testing"

	"github.com/voocel/ainovel-cli/internal/errs"
	"github.com/voocel/ainovel-cli/internal/notify"
)

func webConfig() Config {
	return Config{Web: WebAIConfig{Enabled: true, Site: WebModelName}}
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

func TestDetectLegacyAPIConfigWithMigrationHint(t *testing.T) {
	legacy := []byte(`{"provider":"openai","model":"gpt-5","providers":{"openai":{"api_key":"secret","base_url":"https://api.example/v1"}}}`)
	err := detectLegacyAPIConfig(legacy)
	if !errors.Is(err, errs.ErrConfig) {
		t.Fatalf("legacy config must wrap ErrConfig: %v", err)
	}
	for _, want := range []string{"legacy AI provider/API", "web.enabled=true", "gemini-web", "provider"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("migration error missing %q: %v", want, err)
		}
	}
}

func TestDetectLegacyRoleRouting(t *testing.T) {
	legacy := []byte(`{"web":{"enabled":true},"roles":{"writer":{"provider":"openai","model":"gpt-5","fallbacks":[]}}}`)
	err := detectLegacyAPIConfig(legacy)
	if !errors.Is(err, errs.ErrConfig) || !strings.Contains(err.Error(), "roles.writer") {
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
