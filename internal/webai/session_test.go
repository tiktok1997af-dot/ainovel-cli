package webai

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

type fakeBrowserProcess struct {
	pid      int
	done     chan error
	stopOnce sync.Once
}

func newFakeBrowserProcess(pid int) *fakeBrowserProcess {
	return &fakeBrowserProcess{pid: pid, done: make(chan error, 1)}
}

func (p *fakeBrowserProcess) PID() int           { return p.pid }
func (p *fakeBrowserProcess) Done() <-chan error { return p.done }
func (p *fakeBrowserProcess) Stop() error {
	p.stopOnce.Do(func() { close(p.done) })
	return nil
}

type fakeBrowserLauncher struct {
	mu      sync.Mutex
	configs []BrowserLaunchConfig
	nextPID int
}

func (f *fakeBrowserLauncher) Launch(_ context.Context, cfg BrowserLaunchConfig) (BrowserProcess, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.configs = append(f.configs, cfg)
	f.nextPID++
	return newFakeBrowserProcess(1000 + f.nextPID), nil
}

func (f *fakeBrowserLauncher) lastConfig() BrowserLaunchConfig {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.configs[len(f.configs)-1]
}

type profileMarkerProbe struct{}

func (profileMarkerProbe) Probe(_ context.Context, snap SessionSnapshot) (ReadinessResult, error) {
	_, err := os.Stat(filepath.Join(snap.ProfileDir, "logged-in.marker"))
	if err == nil {
		return ReadinessResult{State: SessionReady, Reason: "persisted login detected"}, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return ReadinessResult{State: SessionAuthRequired, Reason: "manual login required"}, nil
	}
	return ReadinessResult{}, err
}

type fixedReadinessProbe struct {
	result ReadinessResult
	err    error
}

func (p fixedReadinessProbe) Probe(context.Context, SessionSnapshot) (ReadinessResult, error) {
	return p.result, p.err
}

func fakeBrowserExecutable(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "chrome-test-binary")
	if err := os.WriteFile(path, []byte("fake"), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSessionManagerCleanProfileThenPersistedLogin(t *testing.T) {
	profileDir := filepath.Join(t.TempDir(), "persistent-profile")
	browserPath := fakeBrowserExecutable(t)
	launcher := &fakeBrowserLauncher{}

	first := NewSessionManager(SessionConfig{
		Site:        "gemini-web",
		BrowserPath: browserPath,
		ProfileDir:  profileDir,
		StartURL:    "https://gemini.google.com/app",
		Launcher:    launcher,
		Probe:       profileMarkerProbe{},
	})
	firstSnap, err := first.Start(context.Background())
	if err != nil {
		t.Fatalf("first Start: %v", err)
	}
	if firstSnap.State != SessionAuthRequired {
		t.Fatalf("clean profile state = %s, want AUTH_REQUIRED", firstSnap.State)
	}
	if firstSnap.ProfileDir != profileDir || firstSnap.PID == 0 {
		t.Fatalf("unexpected first snapshot: %+v", firstSnap)
	}
	launch := launcher.lastConfig()
	if launch.ProfileDir != profileDir || launch.StartURL != "https://gemini.google.com/app" {
		t.Fatalf("unexpected launch config: %+v", launch)
	}
	if err := first.Stop(); err != nil {
		t.Fatalf("first Stop: %v", err)
	}

	if err := os.MkdirAll(profileDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profileDir, "logged-in.marker"), []byte("session-owned-by-browser"), 0o600); err != nil {
		t.Fatal(err)
	}

	second := NewSessionManager(SessionConfig{
		Site:        "gemini-web",
		BrowserPath: browserPath,
		ProfileDir:  profileDir,
		StartURL:    "https://gemini.google.com/app",
		Launcher:    launcher,
		Probe:       profileMarkerProbe{},
	})
	secondSnap, err := second.Start(context.Background())
	if err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if secondSnap.State != SessionReady {
		t.Fatalf("persisted profile state = %s, want READY", secondSnap.State)
	}
	if _, err := os.Stat(filepath.Join(profileDir, "logged-in.marker")); err != nil {
		t.Fatalf("Stop/start removed persistent browser profile: %v", err)
	}
	if err := second.Stop(); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
	if got := second.Snapshot().State; got != SessionStopped {
		t.Fatalf("state after Stop = %s", got)
	}
}

func TestSessionManagerWithoutProbeStaysAuthRequired(t *testing.T) {
	manager := NewSessionManager(SessionConfig{
		BrowserPath: fakeBrowserExecutable(t),
		ProfileDir:  filepath.Join(t.TempDir(), "profile"),
		Launcher:    &fakeBrowserLauncher{},
	})
	snap, err := manager.Start(context.Background())
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer manager.Stop()
	if snap.State != SessionAuthRequired {
		t.Fatalf("state = %s, want AUTH_REQUIRED", snap.State)
	}
}

func TestSessionManagerInvalidProbeStateFailsClosed(t *testing.T) {
	manager := NewSessionManager(SessionConfig{
		BrowserPath: fakeBrowserExecutable(t),
		ProfileDir:  filepath.Join(t.TempDir(), "profile"),
		Launcher:    &fakeBrowserLauncher{},
		Probe:       fixedReadinessProbe{result: ReadinessResult{State: SessionBusy}},
	})
	snap, err := manager.Start(context.Background())
	if err == nil {
		t.Fatal("expected invalid readiness state error")
	}
	defer manager.Stop()
	if snap.State != SessionFailed {
		t.Fatalf("state = %s, want FAILED", snap.State)
	}
}

func TestSessionManagerProbeErrorBecomesDegraded(t *testing.T) {
	manager := NewSessionManager(SessionConfig{
		BrowserPath: fakeBrowserExecutable(t),
		ProfileDir:  filepath.Join(t.TempDir(), "profile"),
		Launcher:    &fakeBrowserLauncher{},
		Probe:       fixedReadinessProbe{err: errors.New("temporary DevTools inspection failure")},
	})
	snap, err := manager.Start(context.Background())
	if err == nil {
		t.Fatal("expected probe error")
	}
	defer manager.Stop()
	if snap.State != SessionDegraded {
		t.Fatalf("state = %s, want DEGRADED", snap.State)
	}
}

func TestDefaultBrowserProfileDirRejectsTraversal(t *testing.T) {
	for _, name := range []string{"..", "../other", `..\\other`, "a/b"} {
		if _, err := DefaultBrowserProfileDir(name); err == nil {
			t.Fatalf("profile name %q should be rejected", name)
		}
	}
}

func TestResolveChromeExecutableAcceptsExplicitFile(t *testing.T) {
	path := fakeBrowserExecutable(t)
	got, err := ResolveChromeExecutable(path)
	if err != nil {
		t.Fatalf("ResolveChromeExecutable: %v", err)
	}
	want, _ := filepath.Abs(path)
	if got != filepath.Clean(want) {
		t.Fatalf("resolved path = %q, want %q", got, filepath.Clean(want))
	}
}
