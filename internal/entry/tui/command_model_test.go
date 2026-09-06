package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/voocel/ainovel-cli/internal/host"
	"github.com/voocel/ainovel-cli/internal/webai"
)

type fakeWebModelRuntime struct {
	web host.WebConfigurationSnapshot
}

func (f *fakeWebModelRuntime) WebConfiguration() host.WebConfigurationSnapshot { return f.web }

func TestWebModelStatusIsBrowserOnlyAndReadOnly(t *testing.T) {
	rt := &fakeWebModelRuntime{web: host.WebConfigurationSnapshot{
		Enabled:     true,
		Site:        "gemini-web",
		Model:       "gemini-web",
		ProfileName: "default",
		HasSession:  true,
		Session: webai.SessionSnapshot{
			State: webai.SessionReady,
			PID:   4242,
		},
	}}
	st := newModelSwitchState(rt, "writer")
	plain := ansi.Strip(renderModelSwitchBar(120, st))
	for _, want := range []string{"Gemini Web", "WEB-only", "READY", "default", "4242"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("WEB-only /model missing %q:\n%s", want, plain)
		}
	}
	for _, forbidden := range []string{"Provider:", "Vai trò:", "Suy luận:", "API Key", "Base URL", "OpenAI", "Ollama"} {
		if strings.Contains(plain, forbidden) {
			t.Fatalf("WEB-only /model exposed legacy control %q:\n%s", forbidden, plain)
		}
	}
}

func TestWebModelAuthRequiredShowsManualLoginGuidance(t *testing.T) {
	st := &modelSwitchState{web: host.WebConfigurationSnapshot{
		Enabled:     true,
		Site:        "gemini-web",
		Model:       "gemini-web",
		ProfileName: "default",
		HasSession:  true,
		Session:     webai.SessionSnapshot{State: webai.SessionAuthRequired},
	}}
	plain := ansi.Strip(renderModelSwitchBar(120, st))
	if !strings.Contains(plain, "AUTH_REQUIRED") || !strings.Contains(plain, "đăng nhập Gemini") {
		t.Fatalf("manual login guidance missing:\n%s", plain)
	}
}

func TestModelRoleHintCannotCreateSwitchingSurface(t *testing.T) {
	rt := &fakeWebModelRuntime{web: host.WebConfigurationSnapshot{
		Enabled:     true,
		Site:        "gemini-web",
		Model:       "gemini-web",
		ProfileName: "novel",
	}}
	writer := ansi.Strip(renderModelSwitchBar(120, newModelSwitchState(rt, "writer")))
	architect := ansi.Strip(renderModelSwitchBar(120, newModelSwitchState(rt, "architect")))
	if writer != architect {
		t.Fatal("role hint must not change the fixed WEB-only model surface")
	}
}
