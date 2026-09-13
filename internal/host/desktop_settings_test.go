package host

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/voocel/ainovel-cli/internal/bootstrap"
)

func desktopSettingsTestConfig() bootstrap.Config {
	enabled := true
	cfg := bootstrap.Config{
		Web: bootstrap.WebAIConfig{
			Enabled:     true,
			Site:        bootstrap.WebModelName,
			ProfileName: "default",
		},
		Language:        "vi",
		ReasoningEffort: "medium",
		Style:           "default",
		ContextWindow:   200000,
		Roles: map[string]bootstrap.RoleConfig{
			"writer": {ReasoningEffort: "high"},
		},
		Notify: bootstrap.NotifyConfig{Enabled: &enabled, Command: "echo ready"},
	}
	cfg.FillDefaults()
	return cfg
}

func TestDesktopSettingsProjectTargetCASAndRestartSemantics(t *testing.T) {
	t.Chdir(t.TempDir())
	cfg := desktopSettingsTestConfig()
	projectPath := filepath.Join(".ainovel", "config.json")
	if err := bootstrap.SaveConfig(projectPath, cfg); err != nil {
		t.Fatal(err)
	}
	h := &Host{cfg: bootstrap.CloneConfig(cfg)}

	initial, err := h.DesktopSettingsRead()
	if err != nil {
		t.Fatal(err)
	}
	if initial.TargetScope != "project" || initial.TargetPathClass != "./.ainovel/config.json" {
		t.Fatalf("target=%q/%q, want project path class", initial.TargetScope, initial.TargetPathClass)
	}
	if initial.Fingerprint == "" || initial.RestartRequired {
		t.Fatalf("initial projection=%+v", initial)
	}

	updated, err := h.DesktopSettingsSave(initial.Fingerprint, DesktopSettingsUpdate{
		BrowserPath:     " /opt/chrome ",
		ProfileName:     "g07-profile",
		StartURL:        " https://gemini.google.com/app ",
		Language:        "zh",
		ReasoningEffort: "high",
		Style:           "noir",
		ContextWindow:   123456,
		NotifyEnabled:   false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Fingerprint == initial.Fingerprint || !updated.RestartRequired || updated.ApplyMode != DesktopSettingsApplyNextProcess {
		t.Fatalf("updated projection=%+v", updated)
	}
	if updated.Values.BrowserPath != "/opt/chrome" || updated.Values.ProfileName != "g07-profile" || updated.Values.Language != "zh" {
		t.Fatalf("saved values=%+v", updated.Values)
	}
	if updated.Runtime.ProfileName != "default" {
		t.Fatalf("running runtime was hot-swapped: %+v", updated.Runtime)
	}

	persisted, err := bootstrap.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	persisted.FillDefaults()
	if persisted.Roles["writer"].ReasoningEffort != "high" || persisted.Notify.Command != "echo ready" {
		t.Fatalf("closed config fields were not preserved: roles=%+v notify=%+v", persisted.Roles, persisted.Notify)
	}
	if persisted.Web.Enabled != true || persisted.Web.Site != bootstrap.WebModelName {
		t.Fatalf("WEB-only identity changed: %+v", persisted.Web)
	}

	if _, err := h.DesktopSettingsSave(initial.Fingerprint, DesktopSettingsUpdate{ProfileName: "stale"}); !errors.Is(err, ErrDesktopSettingsStale) {
		t.Fatalf("stale save error=%v, want ErrDesktopSettingsStale", err)
	}
}

func TestDesktopSettingsCanonicalValidationDoesNotOverwrite(t *testing.T) {
	t.Chdir(t.TempDir())
	cfg := desktopSettingsTestConfig()
	if err := bootstrap.SaveConfig(filepath.Join(".ainovel", "config.json"), cfg); err != nil {
		t.Fatal(err)
	}
	h := &Host{cfg: bootstrap.CloneConfig(cfg)}
	before, err := h.DesktopSettingsRead()
	if err != nil {
		t.Fatal(err)
	}
	_, err = h.DesktopSettingsSave(before.Fingerprint, DesktopSettingsUpdate{
		ProfileName:   "../escape",
		Language:      before.Values.Language,
		Style:         before.Values.Style,
		ContextWindow: before.Values.ContextWindow,
		NotifyEnabled: before.Values.NotifyEnabled,
		NotifyEvents:  before.Values.NotifyEvents,
	})
	if err == nil {
		t.Fatal("invalid canonical profile unexpectedly saved")
	}
	after, readErr := h.DesktopSettingsRead()
	if readErr != nil {
		t.Fatal(readErr)
	}
	if after.Fingerprint != before.Fingerprint {
		t.Fatalf("invalid save changed canonical fingerprint: before=%s after=%s", before.Fingerprint, after.Fingerprint)
	}
}

func TestDesktopSettingsUsesGlobalTargetWithoutProjectConfig(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(root, "work")
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(work)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	cfg := desktopSettingsTestConfig()
	if err := bootstrap.SaveConfig(bootstrap.DefaultConfigPath(), cfg); err != nil {
		t.Fatal(err)
	}
	h := &Host{cfg: bootstrap.CloneConfig(cfg)}
	state, err := h.DesktopSettingsRead()
	if err != nil {
		t.Fatal(err)
	}
	if state.TargetScope != "global" || state.TargetPathClass != "~/.ainovel/config.json" {
		t.Fatalf("target=%q/%q, want global", state.TargetScope, state.TargetPathClass)
	}
	if _, err := h.DesktopSettingsSave(state.Fingerprint, DesktopSettingsUpdate{
		ProfileName:     "global-profile",
		Language:        state.Values.Language,
		ReasoningEffort: state.Values.ReasoningEffort,
		Style:           state.Values.Style,
		ContextWindow:   state.Values.ContextWindow,
		NotifyEnabled:   state.Values.NotifyEnabled,
		NotifyEvents:    state.Values.NotifyEvents,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(work, ".ainovel", "config.json")); !os.IsNotExist(err) {
		t.Fatalf("Settings implicitly created project config: err=%v", err)
	}
}
