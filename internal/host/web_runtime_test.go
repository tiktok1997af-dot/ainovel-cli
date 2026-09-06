package host

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/voocel/ainovel-cli/internal/bootstrap"
	"github.com/voocel/ainovel-cli/internal/webai"
)

type w5FakeProcess struct {
	done    chan error
	stopped bool
}

func (p *w5FakeProcess) PID() int           { return 5050 }
func (p *w5FakeProcess) Done() <-chan error { return p.done }
func (p *w5FakeProcess) Stop() error {
	p.stopped = true
	return nil
}

type w5FakeLauncher struct{ process *w5FakeProcess }

func (l w5FakeLauncher) Launch(context.Context, webai.BrowserLaunchConfig) (webai.BrowserProcess, error) {
	return l.process, nil
}

type w5ReadyProbe struct{}

func (w5ReadyProbe) Probe(context.Context, webai.SessionSnapshot) (webai.ReadinessResult, error) {
	return webai.ReadinessResult{State: webai.SessionReady, Reason: "test READY"}, nil
}

func TestStartWebRuntimeBootsWithoutAPICredential(t *testing.T) {
	fakeChrome := filepath.Join(t.TempDir(), "chrome")
	if err := os.WriteFile(fakeChrome, []byte("test"), 0o700); err != nil {
		t.Fatal(err)
	}
	process := &w5FakeProcess{done: make(chan error)}
	session := webai.NewSessionManager(webai.SessionConfig{
		Site:        "gemini-web",
		BrowserPath: fakeChrome,
		ProfileDir:  t.TempDir(),
		Launcher:    w5FakeLauncher{process: process},
		Probe:       w5ReadyProbe{},
	})
	cfg := bootstrap.Config{Web: bootstrap.WebAIConfig{Enabled: true}}
	cfg.FillDefaults()
	gotSession, models, err := startWebRuntime(context.Background(), cfg, session)
	if err != nil {
		t.Fatalf("startWebRuntime: %v", err)
	}
	if gotSession != session || models == nil {
		t.Fatal("runtime not wired")
	}
	if snap := session.Snapshot(); snap.State != webai.SessionReady {
		t.Fatalf("state = %s", snap.State)
	}
	if err := session.Stop(); err != nil {
		t.Fatal(err)
	}
	if !process.stopped {
		t.Fatal("owned browser was not stopped")
	}
}
