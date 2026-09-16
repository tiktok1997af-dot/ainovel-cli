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

func writeProjectConfigAt(t *testing.T, root, content string) {
	t.Helper()
	dir := filepath.Join(root, ".ainovel")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir project config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(content), 0o644); err != nil {
		t.Fatalf("write project config: %v", err)
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
	if !cfg.Web.Enabled || cfg.Web.Site != WebModelName {
		t.Fatalf("project WEB config not loaded: %#v", cfg.Web)
	}
}

func TestLoadConfig_LegacyGlobalDoesNotBlockValidProjectOverride(t *testing.T) {
	writeGlobal(t, `{"provider":"openai","providers":{"openai":{"api_key":"secret"}}}`)
	t.Chdir(t.TempDir())
	writeProjectConfig(t, validGlobal)
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("valid project WEB config must supersede legacy global config: %v", err)
	}
	if !cfg.Web.Enabled {
		t.Fatal("project WEB config was not selected")
	}
}

func TestLoadConfig_LegacyGlobalAloneReturnsMigrationError(t *testing.T) {
	writeGlobal(t, `{"provider":"openai","providers":{"openai":{"api_key":"secret"}}}`)
	t.Chdir(t.TempDir())
	_, err := LoadConfig()
	if !errors.Is(err, errs.ErrConfig) || !strings.Contains(err.Error(), LegacyAPIMigrationHint) {
		t.Fatalf("legacy global config must return migration error: %v", err)
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
	if !cfg.Web.Enabled || cfg.Web.Site != WebModelName || cfg.Web.ProfileName != "project-profile" || cfg.Web.BrowserPath != "/custom/chrome" {
		t.Fatalf("web merge failed: %#v", cfg.Web)
	}
	if cfg.Language != "zh" || cfg.ReasoningEffort != "high" || cfg.Roles["writer"].ReasoningEffort != "low" {
		t.Fatalf("creative merge failed: %#v", cfg)
	}
}

func TestLoadConfigFileRejectsLegacyAPIKeysBeforeDecode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.json")
	legacy := `{"provider":"openai","model":"gpt-5","providers":{"openai":{"api_key":"secret","base_url":"https://api.example/v1"}}}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadConfigFile(path)
	if !errors.Is(err, errs.ErrConfig) || !strings.Contains(err.Error(), LegacyAPIMigrationHint) {
		t.Fatalf("legacy file must fail before decode with migration error: %v", err)
	}
}

func TestSaveConfigPersistsNoRuntimeOrCredentialAliases(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".ainovel", "config.json")
	cfg := Config{
		Web:       WebAIConfig{Enabled: true, Site: WebModelName, ProfileName: "default"},
		Provider:  WebProviderName,
		ModelName: WebModelName,
		Language:  "vi",
	}
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
	if !round.Web.Enabled || round.Web.Site != WebModelName {
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
	if !cfg.Web.Enabled || cfg.Web.Site != WebModelName {
		t.Fatalf("example is not WEB-only: %#v", cfg.Web)
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

func TestLoadConfigForProjectUsesExplicitRootWithoutChangingCWD(t *testing.T) {
	writeGlobal(t, validGlobal)
	cwd := t.TempDir()
	t.Chdir(cwd)
	writeProjectConfig(t, `{"language":"en"}`)

	projectRoot := t.TempDir()
	writeProjectConfigAt(t, projectRoot, `{
  "web": {"profile_name": "project-profile"},
  "language": "zh",
  "reasoning_effort": "high"
}`)

	before, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfigForProject(projectRoot)
	if err != nil {
		t.Fatalf("LoadConfigForProject: %v", err)
	}
	after, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("cwd changed: before=%q after=%q", before, after)
	}
	if cfg.Language != "zh" || cfg.ReasoningEffort != "high" {
		t.Fatalf("explicit project overlay not loaded: %#v", cfg)
	}
	if !cfg.Web.Enabled || cfg.Web.Site != WebModelName || cfg.Web.ProfileName != "project-profile" {
		t.Fatalf("global+project merge failed: %#v", cfg.Web)
	}
}

func TestLoadConfigForProjectMissingProjectUsesGlobal(t *testing.T) {
	writeGlobal(t, validGlobal)
	cfg, err := LoadConfigForProject(t.TempDir())
	if err != nil {
		t.Fatalf("missing project config should be allowed: %v", err)
	}
	if !cfg.Web.Enabled || cfg.Language != "vi" {
		t.Fatalf("global config not preserved: %#v", cfg)
	}
}

func TestLoadConfigForProjectCorruptProjectFailsLoud(t *testing.T) {
	writeGlobal(t, validGlobal)
	projectRoot := t.TempDir()
	writeProjectConfigAt(t, projectRoot, `{ "web": {"enabled": true}, }`)
	if _, err := LoadConfigForProject(projectRoot); err == nil {
		t.Fatal("corrupt explicit project config must fail loud")
	}
}
