package bootstrap

import (
	"testing"

	"github.com/voocel/ainovel-cli/internal/webai"
)

func TestNewWebModelSetUsesSingleBrowserModelForAllRoles(t *testing.T) {
	cfg := Config{Web: WebAIConfig{Enabled: true}}
	cfg.FillDefaults()
	session := webai.NewSessionManager(webai.SessionConfig{Site: WebModelName})
	models, err := NewWebModelSet(cfg, session)
	if err != nil {
		t.Fatalf("NewWebModelSet: %v", err)
	}
	if got := models.Summary(); got != "default=web/gemini-web" {
		t.Fatalf("Summary = %q", got)
	}
	architect := models.ForRole("architect")
	writer := models.ForRole("writer")
	editor := models.ForRole("editor")
	if architect == nil || writer == nil || editor == nil {
		t.Fatal("WEB-only role model must not be nil")
	}
	if architect != writer || writer != editor {
		t.Fatal("WEB-only roles must share the same browser-backed model")
	}
}
