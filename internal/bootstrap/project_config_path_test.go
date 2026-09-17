package bootstrap

import (
	"path/filepath"
	"testing"
)

func TestProjectConfigPathIsExplicitAndAbsolute(t *testing.T) {
	root := t.TempDir()
	got, err := ProjectConfigPath(root)
	if err != nil {
		t.Fatalf("ProjectConfigPath: %v", err)
	}
	want := filepath.Join(root, ".ainovel", "config.json")
	if got != want {
		t.Fatalf("ProjectConfigPath = %q, want %q", got, want)
	}
}

func TestProjectConfigPathRejectsBlankRoot(t *testing.T) {
	if _, err := ProjectConfigPath(""); err == nil {
		t.Fatal("ProjectConfigPath blank root error = nil, want error")
	}
}
