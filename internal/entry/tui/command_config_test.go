package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/voocel/ainovel-cli/internal/host"
	"github.com/voocel/ainovel-cli/internal/webai"
)

func webConfigTestState() *modelConfigState {
	return &modelConfigState{
		web: host.WebConfigurationSnapshot{
			Enabled:     true,
			Site:        "gemini-web",
			Model:       "gemini-web",
			ProfileName: "default",
			ConfigPath:  "/tmp/ainovel/config.json",
			HasSession:  true,
			Session: webai.SessionSnapshot{
				State:  webai.SessionReady,
				PID:    4242,
				Reason: "authenticated Gemini composer is ready",
			},
		},
		editing:     -1,
		profileName: "default",
	}
}

func TestWebConfigModalContainsBrowserOnlyControls(t *testing.T) {
	state := webConfigTestState()
	plain := ansi.Strip(renderModelConfigModal(120, state))
	for _, want := range []string{"Gemini Web", "WEB-only", "READY", "Chrome", "Hồ sơ Chrome", "default", "Lưu cấu hình"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("WEB-only /config missing %q:\n%s", want, plain)
		}
	}
	for _, forbidden := range []string{"API Key", "Base URL", "Provider", "Endpoint", "Ollama", "OpenRouter", "Anthropic", "OpenAI"} {
		if strings.Contains(plain, forbidden) {
			t.Fatalf("WEB-only /config leaked legacy control %q:\n%s", forbidden, plain)
		}
	}
}

func TestWebConfigChromePathCanBeClearedForAutoDetect(t *testing.T) {
	state := webConfigTestState()
	state.browserPath = `C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe`
	state.cursor = webConfigFieldChrome
	m := Model{modelConfig: state}

	m.handleModelConfigKey(tea.KeyMsg{Type: tea.KeyEnter})
	if state.editing != webConfigFieldChrome {
		t.Fatalf("Chrome field did not enter edit mode: %d", state.editing)
	}
	state.input.SetValue("")
	m.handleModelConfigKey(tea.KeyMsg{Type: tea.KeyEnter})
	if state.editing != -1 || state.browserPath != "" {
		t.Fatalf("empty Chrome path must select auto-detect, editing=%d path=%q", state.editing, state.browserPath)
	}
	if got := state.fieldValue(webConfigFieldChrome); got != "Tự động phát hiện" {
		t.Fatalf("Chrome display = %q", got)
	}
}

func TestWebConfigProfileDefaultsWhenBlank(t *testing.T) {
	state := webConfigTestState()
	state.cursor = webConfigFieldProfile
	m := Model{modelConfig: state}

	m.handleModelConfigKey(tea.KeyMsg{Type: tea.KeyEnter})
	state.input.SetValue("   ")
	m.handleModelConfigKey(tea.KeyMsg{Type: tea.KeyEnter})
	if state.profileName != "default" {
		t.Fatalf("blank profile = %q, want default", state.profileName)
	}
}

func TestWebConfigAuthRequiredShowsManualLoginGuidance(t *testing.T) {
	state := webConfigTestState()
	state.web.Session.State = webai.SessionAuthRequired
	state.web.Session.Reason = "login required"
	plain := ansi.Strip(renderModelConfigModal(120, state))
	if !strings.Contains(plain, "đăng nhập Gemini") || !strings.Contains(plain, "AUTH_REQUIRED") {
		t.Fatalf("AUTH_REQUIRED guidance missing:\n%s", plain)
	}
}

func TestWebConfigLegacyModeCannotSaveProviderCredentials(t *testing.T) {
	state := webConfigTestState()
	state.web.Enabled = false
	state.cursor = webConfigFieldSave
	m := Model{modelConfig: state}
	m.handleModelConfigKey(tea.KeyMsg{Type: tea.KeyEnter})
	if state.saving {
		t.Fatal("legacy config must not enter browser save path")
	}
	if !strings.Contains(state.message, "legacy") && !strings.Contains(state.message, "WEB-only") {
		t.Fatalf("missing migration refusal: %q", state.message)
	}
}
