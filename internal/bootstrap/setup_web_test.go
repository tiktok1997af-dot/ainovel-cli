package bootstrap

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNewWebSetupConfigCreatesBrowserOnlyRuntime(t *testing.T) {
	cfg := NewWebSetupConfig("vi", "", "")
	if !cfg.Web.Enabled {
		t.Fatal("web.enabled must be true")
	}
	if cfg.Web.Site != "gemini-web" {
		t.Fatalf("web.site = %q", cfg.Web.Site)
	}
	if cfg.Web.ProfileName != "default" {
		t.Fatalf("web.profile_name = %q", cfg.Web.ProfileName)
	}
	if cfg.Provider != "web" || cfg.ModelName != "gemini-web" {
		t.Fatalf("runtime identity = %s/%s", cfg.Provider, cfg.ModelName)
	}
	if len(cfg.Providers) != 0 {
		t.Fatalf("first-run WEB config created legacy providers: %#v", cfg.Providers)
	}
	if err := cfg.ValidateBase(); err != nil {
		t.Fatalf("ValidateBase: %v", err)
	}
}

func TestNewWebSetupConfigSerializedFormHasNoAICredentialFields(t *testing.T) {
	cfg := NewWebSetupConfig("vi", `C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe`, "novel-login")
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	serialized := strings.ToLower(string(data))
	for _, forbidden := range []string{"api_key", "base_url", "extra_body", "stream_idle_timeout", "\"providers\":"} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("WEB first-run config contains forbidden API-era field %q: %s", forbidden, serialized)
		}
	}
	if !strings.Contains(serialized, `"web"`) || !strings.Contains(serialized, `"browser_path"`) || !strings.Contains(serialized, `"profile_name"`) {
		t.Fatalf("WEB browser fields missing: %s", serialized)
	}
}

func TestEmbeddedExampleContainsNoCredentialPlaceholder(t *testing.T) {
	lower := strings.ToLower(exampleConfig)
	for _, forbidden := range []string{"sk-or-", "sk-ant-", "your_open", "your_gemini", "\"providers\":"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("WEB-only example still contains credential/provider example %q", forbidden)
		}
	}
	if !strings.Contains(lower, `"enabled": true`) || !strings.Contains(lower, `"site": "gemini-web"`) {
		t.Fatal("WEB-only example does not enable Gemini Web")
	}
}
