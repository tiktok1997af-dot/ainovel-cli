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
	DefaultWebModel   = "gemini-web"
	DefaultWebProfile = "default"
)

var (
	webSessionFactory = webai.NewSessionManager
	webRuntimeMu      sync.Mutex
	webRuntimeByDir   = map[string]*webai.SessionManager{}
)

func isWebOnlyConfig(cfg Config) bool {
	return cfg.Web.Enabled
}

// NewWebModelSet wires the already-owned browser session into the same
// agentcore.ChatModel contract used by Architect/Writer/Editor/Arbiter. It does
// not start Chrome and cannot fall back to an API provider.
func NewWebModelSet(cfg Config, session *webai.SessionManager) (*ModelSet, error) {
	if !cfg.Web.Enabled {
		return nil, fmt.Errorf("WEB-only model set requires web.enabled=true")
	}
	if session == nil {
		return nil, fmt.Errorf("WEB-only model set requires a browser session")
	}
	if len(cfg.Providers) != 0 {
		return nil, fmt.Errorf("WEB-only model set rejects legacy API providers")
	}
	for role, rc := range cfg.Roles {
		if rc.Provider != "" || rc.Model != "" || len(rc.Fallbacks) != 0 {
			return nil, fmt.Errorf("WEB-only model set rejects provider/model routing for role %q", role)
		}
	}

	transport, err := webai.NewGeminiWebTransport(webai.GeminiWebTransportConfig{Session: session})
	if err != nil {
		return nil, fmt.Errorf("create Gemini web transport: %w", err)
	}
	model, err := webai.NewModel(webai.ModelConfig{
		Site:      DefaultWebSite,
		Model:     DefaultWebModel,
		Transport: transport,
	})
	if err != nil {
		return nil, fmt.Errorf("create web ChatModel: %w", err)
	}
	return &ModelSet{
		Default:   NewSwappableModel(WebOnlyProvider, DefaultWebModel, model, nil),
		models:    make(map[string]*SwappableModel),
		fallbacks: make(map[string][]modelTarget),
		config:    cfg,
	}, nil
}

// newWebOnlyModelSet is the production bootstrap path. It owns one visible,
// persistent Chrome session, but AUTH_REQUIRED and transient DEGRADED are valid
// startup states so the TUI can stay alive while the user logs in or DevTools
// becomes ready. Model calls themselves still require READY.
func newWebOnlyModelSet(cfg Config) (*ModelSet, error) {
	session := webSessionFactory(webai.SessionConfig{
		Site:        cfg.Web.Site,
		BrowserPath: strings.TrimSpace(cfg.Web.BrowserPath),
		ProfileDir:  strings.TrimSpace(cfg.Web.ProfileDir),
		ProfileName: strings.TrimSpace(cfg.Web.ProfileName),
		StartURL:    strings.TrimSpace(cfg.Web.StartURL),
	})
	startupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	snap, startErr := session.Start(startupCtx)
	if startErr != nil && snap.State != webai.SessionDegraded && snap.State != webai.SessionAuthRequired {
		_ = session.Stop()
		return nil, fmt.Errorf("start WEB-only browser session: %w", startErr)
	}
	if snap.State == webai.SessionFailed || snap.State == webai.SessionStopped {
		_ = session.Stop()
		if startErr == nil {
			startErr = fmt.Errorf("browser session entered %s: %s", snap.State, snap.Reason)
		}
		return nil, fmt.Errorf("start WEB-only browser session: %w", startErr)
	}

	models, err := NewWebModelSet(cfg, session)
	if err != nil {
		_ = session.Stop()
		return nil, err
	}
	if err := registerWebRuntime(cfg.OutputDir, session); err != nil {
		_ = session.Stop()
		return nil, err
	}
	return models, nil
}

func webRuntimeKey(outputDir string) (string, error) {
	value := strings.TrimSpace(outputDir)
	if value == "" {
		return "", fmt.Errorf("WEB-only runtime requires a non-empty output directory")
	}
	abs, err := filepath.Abs(value)
	if err != nil {
		return "", fmt.Errorf("resolve WEB-only runtime output directory: %w", err)
	}
	return filepath.Clean(abs), nil
}

func registerWebRuntime(outputDir string, session *webai.SessionManager) error {
	if session == nil {
		return fmt.Errorf("cannot register nil WEB-only browser session")
	}
	key, err := webRuntimeKey(outputDir)
	if err != nil {
		return err
	}
	webRuntimeMu.Lock()
	defer webRuntimeMu.Unlock()
	if existing := webRuntimeByDir[key]; existing != nil {
		return fmt.Errorf("WEB-only browser session already registered for %s", key)
	}
	webRuntimeByDir[key] = session
	return nil
}

// CloseWebRuntimeForOutputDir is invoked by the Host-owned book lease in
// Host.Close, making the browser process part of Host lifetime deterministically.
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
		return fmt.Errorf("stop WEB-only browser session: %w", err)
	}
	return nil
}

// WebRuntimeSnapshot exposes non-secret browser lifecycle metadata for W5B UI.
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
