package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/voocel/ainovel-cli/internal/webai"
)

const (
	WebOnlyProvider   = "web"
	DefaultWebSite    = "gemini-web"
	DefaultWebModel   = "gemini-web-session"
	DefaultWebProfile = "default"
)

// W5A temporarily reads browser-only settings from providers.web.extra so the
// production runtime can migrate before W5B replaces the legacy provider-shaped
// config schema and UI. No value here is an API credential or remote endpoint.
type webOnlyRuntimeConfig struct {
	Site        string
	BrowserPath string
	ProfileName string
	ProfileDir  string
	StartURL    string
}

var (
	webSessionFactory = webai.NewSessionManager
	webRuntimeMu      sync.Mutex
	webRuntimeByDir   = map[string]*webai.SessionManager{}
)

func isWebOnlyConfig(cfg Config) bool {
	return strings.EqualFold(strings.TrimSpace(cfg.Provider), WebOnlyProvider)
}

func parseWebOnlyRuntimeConfig(cfg Config) (webOnlyRuntimeConfig, error) {
	if !isWebOnlyConfig(cfg) {
		return webOnlyRuntimeConfig{}, fmt.Errorf("web-only runtime requires provider %q", WebOnlyProvider)
	}
	if strings.TrimSpace(cfg.ModelName) == "" {
		return webOnlyRuntimeConfig{}, fmt.Errorf("web-only runtime requires a model label")
	}
	if len(cfg.Roles) != 0 {
		return webOnlyRuntimeConfig{}, fmt.Errorf("web-only runtime does not allow role provider/model overrides or fallbacks")
	}
	if len(cfg.Providers) != 1 {
		return webOnlyRuntimeConfig{}, fmt.Errorf("web-only runtime requires exactly one local transport entry: providers.web")
	}
	pc, ok := cfg.Providers[WebOnlyProvider]
	if !ok {
		return webOnlyRuntimeConfig{}, fmt.Errorf("web-only runtime requires providers.web")
	}
	if pc.Type != "" && !strings.EqualFold(strings.TrimSpace(pc.Type), WebOnlyProvider) {
		return webOnlyRuntimeConfig{}, fmt.Errorf("providers.web.type must be %q during W5 migration", WebOnlyProvider)
	}
	if strings.TrimSpace(pc.APIKey) != "" || strings.TrimSpace(pc.BaseURL) != "" || strings.TrimSpace(pc.API) != "" || len(pc.ExtraBody) != 0 {
		return webOnlyRuntimeConfig{}, fmt.Errorf("web-only runtime rejects API key, base URL, API mode and request-body provider settings")
	}

	out := webOnlyRuntimeConfig{
		Site:        DefaultWebSite,
		ProfileName: DefaultWebProfile,
	}
	for key, raw := range pc.Extra {
		value, ok := raw.(string)
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "site":
			if value != "" {
				out.Site = strings.ToLower(value)
			}
		case "browser_path":
			out.BrowserPath = value
		case "profile_name":
			if value != "" {
				out.ProfileName = value
			}
		case "profile_dir":
			out.ProfileDir = value
		case "start_url":
			out.StartURL = value
		}
	}
	if out.Site != DefaultWebSite && out.Site != "gemini" {
		return webOnlyRuntimeConfig{}, fmt.Errorf("unsupported web AI site %q; W5 currently supports Gemini Web only", out.Site)
	}
	out.Site = DefaultWebSite
	return out, nil
}

func newWebOnlyModelSet(cfg Config) (*ModelSet, error) {
	webCfg, err := parseWebOnlyRuntimeConfig(cfg)
	if err != nil {
		return nil, err
	}

	session := webSessionFactory(webai.SessionConfig{
		Site:        webCfg.Site,
		BrowserPath: webCfg.BrowserPath,
		ProfileDir:  webCfg.ProfileDir,
		ProfileName: webCfg.ProfileName,
		StartURL:    webCfg.StartURL,
	})
	startupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	snap, startErr := session.Start(startupCtx)
	if startErr != nil && snap.State != webai.SessionDegraded && snap.State != webai.SessionAuthRequired {
		_ = session.Stop()
		return nil, fmt.Errorf("start web AI browser session: %w", startErr)
	}
	if snap.State == webai.SessionFailed || snap.State == webai.SessionStopped {
		_ = session.Stop()
		if startErr == nil {
			startErr = fmt.Errorf("browser session entered %s: %s", snap.State, snap.Reason)
		}
		return nil, fmt.Errorf("start web AI browser session: %w", startErr)
	}

	transport, err := webai.NewGeminiWebTransport(webai.GeminiWebTransportConfig{Session: session})
	if err != nil {
		_ = session.Stop()
		return nil, fmt.Errorf("create Gemini web transport: %w", err)
	}
	model, err := webai.NewModel(webai.ModelConfig{
		Site:      DefaultWebSite,
		Model:     strings.TrimSpace(cfg.ModelName),
		Transport: transport,
	})
	if err != nil {
		_ = session.Stop()
		return nil, fmt.Errorf("create web ChatModel: %w", err)
	}

	ms := &ModelSet{
		Default:   NewSwappableModel(WebOnlyProvider, strings.TrimSpace(cfg.ModelName), model, nil),
		models:    make(map[string]*SwappableModel),
		fallbacks: make(map[string][]modelTarget),
		config:    cfg,
	}
	if err := registerWebRuntime(cfg.OutputDir, session); err != nil {
		_ = session.Stop()
		return nil, err
	}
	return ms, nil
}

func webRuntimeKey(outputDir string) (string, error) {
	value := strings.TrimSpace(outputDir)
	if value == "" {
		return "", fmt.Errorf("web-only runtime requires a non-empty output directory")
	}
	abs, err := filepath.Abs(value)
	if err != nil {
		return "", fmt.Errorf("resolve web runtime output directory: %w", err)
	}
	return filepath.Clean(abs), nil
}

func registerWebRuntime(outputDir string, session *webai.SessionManager) error {
	if session == nil {
		return fmt.Errorf("cannot register nil web browser session")
	}
	key, err := webRuntimeKey(outputDir)
	if err != nil {
		return err
	}
	webRuntimeMu.Lock()
	defer webRuntimeMu.Unlock()
	if existing := webRuntimeByDir[key]; existing != nil {
		return fmt.Errorf("web AI browser session already registered for %s", key)
	}
	webRuntimeByDir[key] = session
	return nil
}

// CloseWebRuntimeForOutputDir is called by the Host-owned book lease during
// Host.Close. This gives the browser process exactly the same application
// lifetime as the Host without exposing browser credentials or storage.
func CloseWebRuntimeForOutputDir(outputDir string) error {
	key, err := webRuntimeKey(outputDir)
	if err != nil {
		return err
	}
	webRuntimeMu.Lock()
	session := webRuntimeByDir[key]
	delete(webRuntimeByDir, key)
	webRuntimeMu.Unlock()
	if session == nil {
		return nil
	}
	if err := session.Stop(); err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("stop web AI browser session: %w", err)
	}
	return nil
}

// WebRuntimeSnapshot exposes only non-secret lifecycle metadata for W5B UI.
func WebRuntimeSnapshot(outputDir string) (webai.SessionSnapshot, bool) {
	key, err := webRuntimeKey(outputDir)
	if err != nil {
		return webai.SessionSnapshot{}, false
	}
	webRuntimeMu.Lock()
	session := webRuntimeByDir[key]
	webRuntimeMu.Unlock()
	if session == nil {
		return webai.SessionSnapshot{}, false
	}
	return session.Snapshot(), true
}
