package bootstrap

import (
	"strings"
	"testing"

	"github.com/voocel/ainovel-cli/internal/webai"
)

func TestNewWebModelSetHasNoProviderFailover(t *testing.T) {
	cfg := Config{Web: WebAIConfig{Enabled: true}}
	cfg.FillDefaults()
	session := webai.NewSessionManager(webai.SessionConfig{Site: "gemini-web"})
	models, err := NewWebModelSet(cfg, session)
	if err != nil {
		t.Fatalf("NewWebModelSet: %v", err)
	}
	if got := models.Summary(); got != "default=web/gemini-web" {
		t.Fatalf("Summary = %q", got)
	}
	if err := models.Swap("default", "openai", "gpt-anything"); err == nil || !strings.Contains(err.Error(), "WEB-only") {
		t.Fatalf("expected WEB-only swap rejection, got %v", err)
	}
}
