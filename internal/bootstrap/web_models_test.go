package bootstrap

import (
	"testing"

	"github.com/voocel/ainovel-cli/internal/webai"
)

func TestNewWebModelSetUsesD07StrictRoleDualWebGraph(t *testing.T) {
	cfg := Config{Web: WebAIConfig{Enabled: true}}
	cfg.FillDefaults()
	session := webai.NewSessionManager(webai.SessionConfig{Site: WebModelName})
	models, err := NewWebModelSet(cfg, session)
	if err != nil {
		t.Fatalf("NewWebModelSet: %v", err)
	}
	if got := models.Summary(); got != "default=web/gemini-web specialist=chatgpt-web/chatgpt-web" {
		t.Fatalf("Summary = %q", got)
	}

	architect := models.ForRole("architect")
	writer := models.ForRole("writer")
	editor := models.ForRole("editor")
	if architect == nil || writer == nil || editor == nil {
		t.Fatal("D07 role model must not be nil")
	}
	if architect != editor {
		t.Fatal("architect and editor must share the single ChatGPT specialist model")
	}
	if writer == architect {
		t.Fatal("writer must remain on Gemini while architect/editor use ChatGPT")
	}
	if provider, _, _ := models.CurrentSelection("writer"); provider != WebProviderName {
		t.Fatalf("writer provider = %q, want %q", provider, WebProviderName)
	}
	if provider, _, _ := models.CurrentSelection("architect"); provider != webai.ChatGPTWebSite {
		t.Fatalf("architect provider = %q, want %q", provider, webai.ChatGPTWebSite)
	}
	if provider, _, _ := models.CurrentSelection("editor"); provider != webai.ChatGPTWebSite {
		t.Fatalf("editor provider = %q, want %q", provider, webai.ChatGPTWebSite)
	}
	if models.ChatGPTLane() == nil {
		t.Fatal("D07 model graph must own exactly one ChatGPT lane")
	}
}
