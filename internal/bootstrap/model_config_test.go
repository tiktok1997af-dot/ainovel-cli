package bootstrap

import "testing"

func TestWebResolveContextWindowUsesLocalCompactionLimit(t *testing.T) {
	cfg := Config{
		Web:           WebAIConfig{Enabled: true, Site: WebModelName},
		ContextWindow: 300000,
	}
	cfg.FillDefaults()
	got, source := cfg.ResolveContextWindow(WebProviderName, WebModelName)
	if got != 300000 || source != CtxWindowConfig {
		t.Fatalf("WEB local context window = %d %s, want 300000/%s", got, source, CtxWindowConfig)
	}
}

func TestWebResolveContextWindowFallsBackLocally(t *testing.T) {
	cfg := Config{Web: WebAIConfig{Enabled: true, Site: WebModelName}}
	cfg.FillDefaults()
	got, source := cfg.ResolveContextWindow(WebProviderName, WebModelName)
	if got != DefaultContextWindow || source != CtxWindowDefault {
		t.Fatalf("WEB fallback context window = %d %s, want %d/%s", got, source, DefaultContextWindow, CtxWindowDefault)
	}
}
