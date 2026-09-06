package bootstrap

import (
	"errors"
	"strings"
	"testing"

	"github.com/voocel/ainovel-cli/internal/errs"
)

func TestWebOnlyConfigNeedsNoAPICredential(t *testing.T) {
	cfg := Config{
		Web:   WebAIConfig{Enabled: true},
		Roles: map[string]RoleConfig{"writer": {ReasoningEffort: "high"}},
	}
	cfg.FillDefaults()
	if err := cfg.ValidateBase(); err != nil {
		t.Fatalf("ValidateBase: %v", err)
	}
	if cfg.Provider != WebProviderName || cfg.ModelName != WebModelName {
		t.Fatalf("identity = %s/%s", cfg.Provider, cfg.ModelName)
	}
}

func TestDetectLegacyAPIConfigRejectsRemovedProviderSchema(t *testing.T) {
	cases := []string{
		`{"provider":"openai","web":{"enabled":true}}`,
		`{"model":"gpt-5","web":{"enabled":true}}`,
		`{"providers":{"openai":{"api_key":"must-not-be-used"}},"web":{"enabled":true}}`,
		`{"roles":{"writer":{"provider":"openai"}},"web":{"enabled":true}}`,
		`{"roles":{"writer":{"model":"gpt-5"}},"web":{"enabled":true}}`,
		`{"roles":{"writer":{"fallbacks":[{"provider":"openai","model":"gpt-5"}]}},"web":{"enabled":true}}`,
	}
	for _, raw := range cases {
		err := detectLegacyAPIConfig([]byte(raw))
		if !errors.Is(err, errs.ErrConfig) {
			t.Fatalf("legacy config must return ErrConfig, raw=%s err=%v", raw, err)
		}
		if !strings.Contains(strings.ToLower(err.Error()), "web.enabled=true") {
			t.Fatalf("migration guidance missing for raw=%s: %v", raw, err)
		}
	}
}

func TestDetectLegacyAPIConfigAllowsWebOnlyRoleIntent(t *testing.T) {
	raw := `{"web":{"enabled":true,"site":"gemini-web"},"roles":{"writer":{"reasoning_effort":"high"}}}`
	if err := detectLegacyAPIConfig([]byte(raw)); err != nil {
		t.Fatalf("WEB-only config was rejected: %v", err)
	}
}
