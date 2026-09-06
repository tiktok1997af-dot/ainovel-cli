package bootstrap

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/voocel/ainovel-cli/internal/errs"
)

const validGlobal = `{
  "web": {"enabled": true, "site": "gemini-web", "profile_name": "default"},
  "language": "vi",
  "context_window": 200000
}`

func writeGlobal(t *testing.T, content string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	dir := filepath.Join(home, ".ainovel")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if content != "" {
		if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(content), 0o644); err != nil {
			t.Fatalf("write global: %v", err)
		}
	}
	return home
}

func writeProjectConfig(t *testing.T, content string) {
	t.Helper()
	if err := os.MkdirAll(".ainovel", 0o755); err != nil {
		t.Fatalf("mkdir .ainovel: %v", err)
	}
	if err := os.WriteFile(filepath.Join(".ainovel", "config.json"), []byte(content), 0o644); err != nil {
		t.Fatalf("write project: %v", err)
	}
}

func TestLoadConfig_CorruptProjectFailsLoud(t *testing.T) {
	writeGlobal(t, validGlobal)
	t.Chdir(t.TempDir())
	writeProjectConfig(t, `{ "web": {"enabled": true}, }`)
	if _, err := LoadConfig(); err == nil {
		t.Fatal("corrupt project config must fail loud")
	}
}

func TestLoadConfig_CorruptGlobalDoesNotBlockProjectOverride(t *testing.T) {
	writeGlobal(t, `{ not json`)
	t.Chdir(t.TempDir())
	writeProjectConfig(t, validGlobal)
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("corrupt global must not block project config: %v", err)
	}
	if !cfg.Web.Enabled || cfg.Web.Site != "gemini-web" {
		t.Fatalf("project WEB config not loaded: %#v", cfg.Web)
	}
}

func TestEffectiveConfigPathPrefersProject(t *testing.T) {
	writeGlobal(t, validGlobal)
	t.Chdir(t.TempDir())
	if got := EffectiveConfigPath(); got != DefaultConfigPath() {
		t.Fatalf("no project config: got %q", got)
	}
	proj := t.TempDir()
	t.Chdir(proj)
	writeProjectConfig(t, validGlobal)
	want, _ := filepath.Abs(filepath.Join(".ainovel", "config.json"))
	if got := EffectiveConfigPath(); got != want {
		t.Fatalf("project config path = %q want %q", got, want)
	}
}

func TestLoadConfig_MissingFilesNoError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Chdir(t.TempDir())
	if _, err := LoadConfig(); err != nil {
		t.Fatalf("missing configs should be allowed: %v", err)
	}
}

func TestLoadConfig_WebOverlayMergesBrowserAndCreativeFields(t *testing.T) {
	writeGlobal(t, validGlobal)
	t.Chdir(t.TempDir())
	writeProjectConfig(t, `{
  "web": {"profile_name": "project-profile", "browser_path": "/custom/chrome"},
  "language": "zh",
  "reasoning_effort": "high",
  "roles": {"writer": {"reasoning_effort": "low"}}
}`)
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !cfg.Web.Enabled || cfg.Web.Site != "gemini-web" || cfg.Web.ProfileName != "project-profile" || cfg.Web.BrowserPath != "/custom/chrome" {
		t.Fatalf("web merge failed: %#v", cfg.Web)
	}
	if cfg.Language != "zh" || cfg.ReasoningEffort != "high" || cfg.Roles["writer"].ReasoningEffort != "low" {
		t.Fatalf("creative merge failed: %#v", cfg)
	}
}

func TestLegacyAPIFileLoadsOnlyForMigrationAndFailsValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.json")
	legacy := `{"provider":"openai","model":"gpt-5","providers":{"openai":{"api_key":"secret","base_url":"https://api.example/v1"}}}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfigFile(path)
	if err != nil {
		t.Fatalf("parse legacy file: %v", err)
	}
	if err := cfg.ValidateBase(); !errors.Is(err, errs.ErrConfig) || !strings.Contains(err.Error(), LegacyAPIMigrationHint) {
		t.Fatalf("legacy file must produce migration error: %v", err)
	}
}

func TestSaveConfigRejectsAPICredentialsAndSanitizesWebAliases(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".ainovel", "config.json")
	legacy := Config{Provider: "openai", ModelName: "gpt-5", Providers: map[string]ProviderConfig{"openai": {APIKey: "secret"}}}
	if err := SaveConfig(path, legacy); err == nil || !strings.Contains(err.Error(), "legacy AI provider/API") {
		t.Fatalf("SaveConfig must reject API config: %v", err)
	}

	cfg := Config{Web: WebAIConfig{Enabled: true, Site: "gemini-web", ProfileName: "default"}, Provider: "web", ModelName: "gemini-web", Language: "vi"}
	if err := SaveConfig(path, cfg); err != nil {
		t.Fatalf("save WEB config: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, forbidden := range []string{`"provider"`, `"model"`, `"providers"`, `"api_key"`, `"base_url"`} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("WEB persistence leaked legacy key %s:\n%s", forbidden, text)
		}
	}
	var round Config
	if err := json.Unmarshal(data, &round); err != nil {
		t.Fatalf("saved JSON: %v", err)
	}
	if !round.Web.Enabled || round.Web.Site != "gemini-web" {
		t.Fatalf("saved WEB config invalid: %#v", round.Web)
	}
}

func TestExampleConfigIsValidAndSelfConsistent(t *testing.T) {
	root, err := os.ReadFile(filepath.Join("..", "..", "config.example.jsonc"))
	if err != nil {
		t.Fatal(err)
	}
	packaged, err := os.ReadFile(filepath.Join("..", "..", "config", "config.example.jsonc"))
	if err != nil {
		t.Fatal(err)
	}
	if string(root) != exampleConfig || string(packaged) != exampleConfig {
		t.Fatal("example configs must be synchronized")
	}
	var cfg Config
	if err := json.Unmarshal(stripJSONComments([]byte(exampleConfig)), &cfg); err != nil {
		t.Fatalf("example JSON: %v", err)
	}
	if !cfg.Web.Enabled || cfg.Web.Site != "gemini-web" || len(cfg.Providers) != 0 {
		t.Fatalf("example is not WEB-only: %#v", cfg)
	}
	cfg.FillDefaults()
	if err := cfg.ValidateBase(); err != nil {
		t.Fatalf("example ValidateBase: %v", err)
	}
}

func TestWriteStartupError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	path := WriteStartupError("boom: browser config invalid")
	if path == "" {
		t.Fatal("startup error path missing")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "boom: browser config invalid") {
		t.Fatalf("startup log missing message: %s", data)
	}
}
