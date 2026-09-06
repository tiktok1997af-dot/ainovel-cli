package host

import (
	"context"
	"fmt"
	"strings"

	"github.com/voocel/ainovel-cli/internal/bootstrap"
	"github.com/voocel/ainovel-cli/internal/webai"
)

// WebConfigurationSnapshot is the non-secret browser configuration/status
// surface used by W5B. It never contains cookies, Google credentials or tokens.
type WebConfigurationSnapshot struct {
	Enabled     bool
	Site        string
	Model       string
	BrowserPath string
	ProfileName string
	StartURL    string
	ConfigPath  string
	Session     webai.SessionSnapshot
	HasSession  bool
}

// WebConfiguration returns browser settings plus the owned session's safe
// lifecycle metadata. SessionSnapshot intentionally contains no credentials.
func (h *Host) WebConfiguration() WebConfigurationSnapshot {
	h.mu.Lock()
	cfg := h.cfg
	session := h.webSession
	configPath := h.configPath
	h.mu.Unlock()

	out := WebConfigurationSnapshot{
		Enabled:     cfg.Web.Enabled,
		Site:        cfg.Web.Site,
		Model:       cfg.ModelName,
		BrowserPath: cfg.Web.BrowserPath,
		ProfileName: cfg.Web.ProfileName,
		StartURL:    cfg.Web.StartURL,
		ConfigPath:  configPath,
	}
	if session != nil {
		out.Session = session.Snapshot()
		out.HasSession = true
	}
	return out
}

// SaveWebConfiguration persists browser-only settings for the next process
// start. W5B deliberately does not replace an in-use browser session while a
// Host may be writing; that avoids losing an authenticated/in-flight session.
func (h *Host) SaveWebConfiguration(browserPath, profileName string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.cfg.Web.Enabled {
		return fmt.Errorf("WEB-only runtime is not enabled")
	}
	candidate := bootstrap.CloneConfig(h.cfg)
	candidate.Web.Enabled = true
	candidate.Web.Site = "gemini-web"
	candidate.Web.BrowserPath = strings.TrimSpace(browserPath)
	candidate.Web.ProfileName = strings.TrimSpace(profileName)
	candidate.Provider = "web"
	candidate.ModelName = "gemini-web"
	candidate.FillDefaults()
	if err := candidate.ValidateBase(); err != nil {
		return fmt.Errorf("invalid WEB browser configuration: %w", err)
	}
	if err := bootstrap.SaveConfig(h.configPath, candidate); err != nil {
		return fmt.Errorf("save WEB browser configuration: %w", err)
	}
	h.cfg.Web = candidate.Web
	return nil
}

// RefreshWebConfiguration asks the existing visible session to re-check login
// readiness. It does not submit a model prompt and does not perform login.
func (h *Host) RefreshWebConfiguration(ctx context.Context) (WebConfigurationSnapshot, error) {
	h.mu.Lock()
	session := h.webSession
	h.mu.Unlock()
	if session == nil {
		return h.WebConfiguration(), fmt.Errorf("WEB browser session is not running")
	}
	_, err := session.Refresh(ctx)
	return h.WebConfiguration(), err
}
