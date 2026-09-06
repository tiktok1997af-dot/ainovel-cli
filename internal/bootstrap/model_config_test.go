package bootstrap

import (
	"strings"
	"testing"
)

func TestLegacyAPIModelConstructorFailsClosed(t *testing.T) {
	cfg := Config{Provider: "legacy-provider", ModelName: "legacy-model"}
	models, err := NewModelSet(cfg)
	if err == nil {
		t.Fatal("legacy API constructor must fail closed")
	}
	if models != nil {
		t.Fatalf("legacy API constructor returned a usable ModelSet: %#v", models)
	}
	message := strings.ToLower(err.Error())
	for _, want := range []string{"removed", "web.enabled=true", "gemini-web"} {
		if !strings.Contains(message, want) {
			t.Fatalf("migration error missing %q: %v", want, err)
		}
	}
}

func TestWebResolveContextWindowUsesLocalCompactionLimit(t *testing.T) {
	cfg := Config{
		Web:           WebAIConfig{Enabled: true, Site: "gemini-web"},
		ContextWindow: 300000,
	}
	cfg.FillDefaults()
	got, source := cfg.ResolveContextWindow("web", "gemini-web")
	if got != 300000 || source != CtxWindowConfig {
		t.Fatalf("WEB local context window = %d %s, want 300000/%s", got, source, CtxWindowConfig)
	}
}
