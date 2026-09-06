package bootstrap

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/voocel/ainovel-cli/internal/webai"
)

type w5aReadyProbe struct{}

func (w5aReadyProbe) Probe(context.Context, webai.SessionSnapshot) (webai.ReadinessResult, error) {
	return webai.ReadinessResult{State: webai.SessionReady, Reason: "test ready"}, nil
}

type w5aFakeLauncher struct {
	process *w5aFakeProcess
}

func (l *w5aFakeLauncher) Launch(context.Context, webai.BrowserLaunchConfig) (webai.BrowserProcess, error) {
	return l.process, nil
}

type w5aFakeProcess struct {
	done      chan error
	stopOnce  sync.Once
	stopCount atomic.Int32
}

func newW5AFakeProcess() *w5aFakeProcess {
	return &w5aFakeProcess{done: make(chan error, 1)}
}

func (p *w5aFakeProcess) PID() int             { return 4242 }
func (p *w5aFakeProcess) Done() <-chan error   { return p.done }
func (p *w5aFakeProcess) Stop() error {
	p.stopOnce.Do(func() {
		p.stopCount.Add(1)
		close(p.done)
	})
	return nil
}

func TestNewModelSetWebOnlyBootsWithoutAPIAndClosesSession(t *testing.T) {
	browser := t.TempDir() + string(os.PathSeparator) + "chrome-test"
	if err := os.WriteFile(browser, []byte("fake"), 0o700); err != nil {
		t.Fatalf("write fake browser: %v", err)
	}
	process := newW5AFakeProcess()
	launcher := &w5aFakeLauncher{process: process}

	originalFactory := webSessionFactory
	webSessionFactory = func(cfg webai.SessionConfig) *webai.SessionManager {
		cfg.BrowserPath = browser
		cfg.Launcher = launcher
		cfg.Probe = w5aReadyProbe{}
		return webai.NewSessionManager(cfg)
	}
	t.Cleanup(func() { webSessionFactory = originalFactory })

	cfg := Config{
		OutputDir: t.TempDir(),
		Web: WebAIConfig{
			Enabled:     true,
			Site:        DefaultWebSite,
			ProfileName: "w5a-test",
		},
		Roles: map[string]RoleConfig{
			"writer": {ReasoningEffort: "high"},
		},
	}
	cfg.FillDefaults()
	if err := cfg.ValidateBase(); err != nil {
		t.Fatalf("ValidateBase: %v", err)
	}
	models, err := NewModelSet(cfg)
	if err != nil {
		t.Fatalf("NewModelSet WEB-only: %v", err)
	}
	if got := models.Summary(); got != "default=web/gemini-web" {
		t.Fatalf("Summary = %q", got)
	}
	if models.ForRole("architect") != models.Default || models.ForRole("writer") != models.Default || models.ForRoleWithFailover("editor", nil) != models.Default {
		t.Fatal("all WEB-only roles must share the single browser ChatModel")
	}
	if snap, ok := WebRuntimeSnapshot(cfg.OutputDir); !ok || snap.State != webai.SessionReady || snap.PID != 4242 {
		t.Fatalf("runtime snapshot = %#v ok=%v", snap, ok)
	}
	if err := CloseWebRuntimeForOutputDir(cfg.OutputDir); err != nil {
		t.Fatalf("CloseWebRuntimeForOutputDir: %v", err)
	}
	if got := process.stopCount.Load(); got != 1 {
		t.Fatalf("browser Stop count = %d, want 1", got)
	}
	if _, ok := WebRuntimeSnapshot(cfg.OutputDir); ok {
		t.Fatal("closed WEB-only runtime remained registered")
	}
}

func TestNewWebModelSetRejectsLegacyProviderRouting(t *testing.T) {
	cfg := Config{
		Web: WebAIConfig{Enabled: true, Site: DefaultWebSite},
		Providers: map[string]ProviderConfig{
			"openai": {APIKey: "must-never-be-used"},
		},
	}
	cfg.FillDefaults()
	session := webai.NewSessionManager(webai.SessionConfig{Site: DefaultWebSite})
	if _, err := NewWebModelSet(cfg, session); err == nil {
		t.Fatal("expected WEB-only model set to reject legacy API providers")
	}
}
