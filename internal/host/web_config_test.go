package host

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/voocel/ainovel-cli/internal/bootstrap"
)

func TestSaveWebConfigurationPersistsBrowserOnlySettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := bootstrap.Config{
		Web: bootstrap.WebAIConfig{
			Enabled:     true,
			Site:        bootstrap.WebModelName,
			ProfileName: "default",
		},
		Roles:    map[string]bootstrap.RoleConfig{},
		Language: "vi",
		Style:    "default",
	}
	cfg.FillDefaults()
	h := &Host{cfg: cfg, configPath: path}

	browser := `C:\Program Files\Google\Chrome\Application\chrome.exe`
	if err := h.SaveWebConfiguration(browser, "novel-login"); err != nil {
		t.Fatalf("SaveWebConfiguration: %v", err)
	}
	loaded, err := bootstrap.LoadConfigFile(path)
	if err != nil {
		t.Fatalf("LoadConfigFile: %v", err)
	}
	loaded.FillDefaults()
	if err := loaded.ValidateBase(); err != nil {
		t.Fatalf("saved config invalid: %v", err)
	}
	if !loaded.Web.Enabled || loaded.Web.Site != bootstrap.WebModelName {
		t.Fatalf("saved web config = %#v", loaded.Web)
	}
	if loaded.Web.BrowserPath != browser || loaded.Web.ProfileName != "novel-login" {
		t.Fatalf("saved browser settings = %#v", loaded.Web)
	}
	if loaded.Provider != bootstrap.WebProviderName || loaded.ModelName != bootstrap.WebModelName {
		t.Fatalf("saved runtime identity = %s/%s", loaded.Provider, loaded.ModelName)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read saved config: %v", err)
	}
	serialized := strings.ToLower(string(raw))
	for _, forbidden := range []string{`"provider":`, `"model":`, `"providers":`, "api_key", "base_url", `"fallbacks":`} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("saved WEB config contains removed API-era field %q: %s", forbidden, serialized)
		}
	}
}

func TestSaveWebConfigurationRejectsInvalidProfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := bootstrap.Config{Web: bootstrap.WebAIConfig{Enabled: true, Site: bootstrap.WebModelName}}
	cfg.FillDefaults()
	h := &Host{cfg: cfg, configPath: path}
	if err := h.SaveWebConfiguration("", "../escape"); err == nil {
		t.Fatal("invalid Chrome profile name must be rejected")
	}
}
