package bootstrap

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigForProjectUsesExplicitRootWithoutChangingCWD(t *testing.T) {
	writeGlobal(t, validGlobal)
	cwd := t.TempDir()
	t.Chdir(cwd)

	project := t.TempDir()
	projectConfigDir := filepath.Join(project, ".ainovel")
	if err := os.MkdirAll(projectConfigDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectConfigDir, "config.json"), []byte(`{
  "web": {"profile_name": "project-explicit"},
  "language": "zh"
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	before, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfigForProject(project)
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
	if !cfg.Web.Enabled || cfg.Web.Site != WebModelName || cfg.Web.ProfileName != "project-explicit" {
		t.Fatalf("project web overlay not applied: %#v", cfg.Web)
	}
	if cfg.Language != "zh" {
		t.Fatalf("language = %q, want zh", cfg.Language)
	}
}

func TestLoadConfigForProjectRejectsCorruptExplicitProjectConfig(t *testing.T) {
	writeGlobal(t, validGlobal)
	project := t.TempDir()
	if err := os.MkdirAll(filepath.Join(project, ".ainovel"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, ".ainovel", "config.json"), []byte(`{"web": {`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfigForProject(project); err == nil {
		t.Fatal("corrupt explicit project config must fail loud")
	}
}
