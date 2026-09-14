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

func TestChatGPTSessionDefaultsConcreteReadinessProbe(t *testing.T) {
	manager := NewSessionManager(SessionConfig{Site: "chatgpt-web"})
	if manager.cfg.StartURL != "https://chatgpt.com/" {
		t.Fatalf("ChatGPT start URL = %q", manager.cfg.StartURL)
	}
	if _, ok := manager.probe.(*DevToolsReadinessProbe); !ok {
		t.Fatalf("ChatGPT default readiness probe = %T, want *DevToolsReadinessProbe", manager.probe)
	}
	if manager.snapshot.Site != "chatgpt-web" {
		t.Fatalf("ChatGPT snapshot site = %q", manager.snapshot.Site)
	}
}
