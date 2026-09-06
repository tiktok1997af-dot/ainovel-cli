package host

import (
	"path/filepath"
	"testing"

	"github.com/voocel/ainovel-cli/internal/bootstrap"
)

func TestSaveWebConfigurationPersistsBrowserOnlySettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := bootstrap.Config{
		Web: bootstrap.WebAIConfig{
			Enabled:     true,
			Site:        "gemini-web",
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
	if !loaded.Web.Enabled || loaded.Web.Site != "gemini-web" {
		t.Fatalf("saved web config = %#v", loaded.Web)
	}
	if loaded.Web.BrowserPath != browser || loaded.Web.ProfileName != "novel-login" {
		t.Fatalf("saved browser settings = %#v", loaded.Web)
	}
	if len(loaded.Providers) != 0 {
		t.Fatalf("browser config unexpectedly persisted API providers: %#v", loaded.Providers)
	}
	if loaded.Provider != "web" || loaded.ModelName != "gemini-web" {
		t.Fatalf("saved runtime identity = %s/%s", loaded.Provider, loaded.ModelName)
	}
}

func TestSaveWebConfigurationRejectsInvalidProfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := bootstrap.Config{Web: bootstrap.WebAIConfig{Enabled: true, Site: "gemini-web"}}
	cfg.FillDefaults()
	h := &Host{cfg: cfg, configPath: path}
	if err := h.SaveWebConfiguration("", "../escape"); err == nil {
		t.Fatal("invalid Chrome profile name must be rejected")
	}
}
