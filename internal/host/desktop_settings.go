package host

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/voocel/ainovel-cli/internal/bootstrap"
)

var ErrDesktopSettingsStale = errors.New("host: desktop settings stale")

const DesktopSettingsApplyNextProcess = "next_process_start"

type DesktopSettingsValues struct {
	BrowserPath     string
	ProfileName     string
	StartURL        string
	Language        string
	ReasoningEffort string
	Style           string
	ContextWindow   int
	NotifyEnabled   bool
	NotifyEvents    []string
}

type DesktopSettingsRuntimeSnapshot struct {
	BrowserPath     string
	ProfileName     string
	StartURL        string
	Language        string
	ReasoningEffort string
	Style           string
	ContextWindow   int
	NotifyEnabled   bool
	NotifyEvents    []string
}

type DesktopSettingsSnapshot struct {
	WebEnabled      bool
	WebSite         string
	Values          DesktopSettingsValues
	Runtime         DesktopSettingsRuntimeSnapshot
	TargetScope     string
	TargetPathClass string
	Fingerprint     string
	RestartRequired bool
	ApplyMode       string
}

type DesktopSettingsUpdate struct {
	BrowserPath     string
	ProfileName     string
	StartURL        string
	Language        string
	ReasoningEffort string
	Style           string
	ContextWindow   int
	NotifyEnabled   bool
	NotifyEvents    []string
}

func (h *Host) DesktopSettingsRead() (DesktopSettingsSnapshot, error) {
	if h == nil {
		return DesktopSettingsSnapshot{}, fmt.Errorf("host is nil")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	snapshot, _, _, err := h.desktopSettingsStateLocked()
	return snapshot, err
}

func (h *Host) DesktopSettingsSave(expectedFingerprint string, update DesktopSettingsUpdate) (DesktopSettingsSnapshot, error) {
	if h == nil {
		return DesktopSettingsSnapshot{}, fmt.Errorf("host is nil")
	}
	h.mu.Lock()
	defer h.mu.Unlock()

	current, cfg, target, err := h.desktopSettingsStateLocked()
	if err != nil {
		return DesktopSettingsSnapshot{}, err
	}
	if strings.TrimSpace(expectedFingerprint) == "" || expectedFingerprint != current.Fingerprint {
		return DesktopSettingsSnapshot{}, ErrDesktopSettingsStale
	}

	candidate := bootstrap.CloneConfig(cfg)
	candidate.Web.Enabled = true
	candidate.Web.Site = bootstrap.WebModelName
	candidate.Web.BrowserPath = strings.TrimSpace(update.BrowserPath)
	candidate.Web.ProfileName = strings.TrimSpace(update.ProfileName)
	candidate.Web.StartURL = strings.TrimSpace(update.StartURL)
	candidate.Language = strings.TrimSpace(update.Language)
	candidate.ReasoningEffort = strings.TrimSpace(update.ReasoningEffort)
	candidate.Style = strings.TrimSpace(update.Style)
	candidate.ContextWindow = update.ContextWindow
	enabled := update.NotifyEnabled
	candidate.Notify.Enabled = &enabled
	candidate.Notify.Events = append([]string(nil), update.NotifyEvents...)
	candidate.FillDefaults()
	if err := candidate.ValidateBase(); err != nil {
		return DesktopSettingsSnapshot{}, err
	}
	if err := bootstrap.SaveConfig(target, candidate); err != nil {
		return DesktopSettingsSnapshot{}, fmt.Errorf("save desktop settings: %w", err)
	}

	fresh, _, _, err := h.desktopSettingsStateLocked()
	return fresh, err
}

func (h *Host) desktopSettingsStateLocked() (DesktopSettingsSnapshot, bootstrap.Config, string, error) {
	cfg, err := bootstrap.LoadConfig()
	if err != nil {
		return DesktopSettingsSnapshot{}, bootstrap.Config{}, "", err
	}
	cfg.FillDefaults()
	if err := cfg.ValidateBase(); err != nil {
		return DesktopSettingsSnapshot{}, bootstrap.Config{}, "", err
	}

	target := bootstrap.EffectiveConfigPath()
	scope, pathClass := desktopSettingsTargetMetadata(target)
	fingerprint, err := desktopSettingsFingerprint(cfg, scope)
	if err != nil {
		return DesktopSettingsSnapshot{}, bootstrap.Config{}, "", err
	}

	values := desktopSettingsValuesFromConfig(cfg)
	runtime := desktopSettingsRuntimeFromConfig(h.cfg)
	return DesktopSettingsSnapshot{
		WebEnabled:      cfg.Web.Enabled,
		WebSite:         cfg.Web.Site,
		Values:          values,
		Runtime:         runtime,
		TargetScope:     scope,
		TargetPathClass: pathClass,
		Fingerprint:     fingerprint,
		RestartRequired: !desktopSettingsEqualRuntime(values, runtime),
		ApplyMode:       DesktopSettingsApplyNextProcess,
	}, cfg, target, nil
}

func desktopSettingsTargetMetadata(target string) (string, string) {
	global := bootstrap.DefaultConfigPath()
	if global != "" && filepath.Clean(target) == filepath.Clean(global) {
		return "global", "~/.ainovel/config.json"
	}
	return "project", "./.ainovel/config.json"
}

func desktopSettingsFingerprint(cfg bootstrap.Config, scope string) (string, error) {
	canonical := struct {
		Scope  string           `json:"scope"`
		Config bootstrap.Config `json:"config"`
	}{Scope: scope, Config: cfg}
	data, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("marshal settings fingerprint: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func desktopSettingsValuesFromConfig(cfg bootstrap.Config) DesktopSettingsValues {
	return DesktopSettingsValues{
		BrowserPath:     cfg.Web.BrowserPath,
		ProfileName:     cfg.Web.ProfileName,
		StartURL:        cfg.Web.StartURL,
		Language:        cfg.Language,
		ReasoningEffort: cfg.ReasoningEffort,
		Style:           cfg.Style,
		ContextWindow:   cfg.ContextWindow,
		NotifyEnabled:   cfg.Notify.IsEnabled(),
		NotifyEvents:    append([]string(nil), cfg.Notify.Events...),
	}
}

func desktopSettingsRuntimeFromConfig(cfg bootstrap.Config) DesktopSettingsRuntimeSnapshot {
	cfg.FillDefaults()
	return DesktopSettingsRuntimeSnapshot{
		BrowserPath:     cfg.Web.BrowserPath,
		ProfileName:     cfg.Web.ProfileName,
		StartURL:        cfg.Web.StartURL,
		Language:        cfg.Language,
		ReasoningEffort: cfg.ReasoningEffort,
		Style:           cfg.Style,
		ContextWindow:   cfg.ContextWindow,
		NotifyEnabled:   cfg.Notify.IsEnabled(),
		NotifyEvents:    append([]string(nil), cfg.Notify.Events...),
	}
}

func desktopSettingsEqualRuntime(saved DesktopSettingsValues, runtime DesktopSettingsRuntimeSnapshot) bool {
	return saved.BrowserPath == runtime.BrowserPath &&
		saved.ProfileName == runtime.ProfileName &&
		saved.StartURL == runtime.StartURL &&
		saved.Language == runtime.Language &&
		saved.ReasoningEffort == runtime.ReasoningEffort &&
		saved.Style == runtime.Style &&
		saved.ContextWindow == runtime.ContextWindow &&
		saved.NotifyEnabled == runtime.NotifyEnabled &&
		slices.Equal(saved.NotifyEvents, runtime.NotifyEvents)
}
