package bootstrap

import (
	"strings"
	"testing"
)

func TestWebOnlyConfigNeedsNoAPICredential(t *testing.T) {
	cfg := Config{
		Web: WebAIConfig{Enabled: true},
		Roles: map[string]RoleConfig{"writer": {ReasoningEffort: "high"}},
	}
	cfg.FillDefaults()
	if err := cfg.ValidateBase(); err != nil {
		t.Fatalf("ValidateBase: %v", err)
	}
	if cfg.Provider != "web" || cfg.ModelName != "gemini-web" {
		t.Fatalf("identity = %s/%s", cfg.Provider, cfg.ModelName)
	}
	if got := cfg.CandidateModels("web"); len(got) != 1 || got[0] != "gemini-web" {
		t.Fatalf("CandidateModels = %v", got)
	}
}

func TestWebOnlyConfigRejectsLegacyAPIProvider(t *testing.T) {
	cfg := Config{
		Web: WebAIConfig{Enabled: true},
		Providers: map[string]ProviderConfig{"openai": {APIKey: "must-not-be-used"}},
	}
	cfg.FillDefaults()
	err := cfg.ValidateBase()
	if err == nil || !strings.Contains(err.Error(), "legacy API providers") {
		t.Fatalf("expected legacy API rejection, got %v", err)
	}
}
