package webai

import "testing"

func TestGeminiSessionDefaultsConcreteReadinessProbe(t *testing.T) {
	manager := NewSessionManager(SessionConfig{Site: "gemini-web"})
	if manager.cfg.StartURL != "https://gemini.google.com/app" {
		t.Fatalf("Gemini start URL = %q", manager.cfg.StartURL)
	}
	if _, ok := manager.probe.(*DevToolsReadinessProbe); !ok {
		t.Fatalf("Gemini default readiness probe = %T, want *DevToolsReadinessProbe", manager.probe)
	}
}
