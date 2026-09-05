package webai

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// ReadinessResult is the site adapter verdict after inspecting the visible
// browser session. W2 permits AUTH_REQUIRED, READY, DEGRADED or FAILED.
type ReadinessResult struct {
	State  SessionState
	Reason string
}

// ReadinessProbe detects whether the logged-in website is ready without
// submitting prompts. Prompt submission/capture belongs to W3.
type ReadinessProbe interface {
	Probe(ctx context.Context, session SessionSnapshot) (ReadinessResult, error)
}

// SessionConfig configures one persistent visible browser session.
type SessionConfig struct {
	Site        string
	BrowserPath string
	ProfileDir  string
	ProfileName string
	StartURL    string
	ExtraArgs   []string
	Launcher    BrowserLauncher
	Probe       ReadinessProbe
}

// SessionManager owns the visible Chrome process and login-profile lifecycle.
// It never stores credentials; Chrome owns its persistent profile on disk.
type SessionManager struct {
	mu       sync.RWMutex
	cfg      SessionConfig
	launcher BrowserLauncher
	probe    ReadinessProbe
	process  BrowserProcess
	snapshot SessionSnapshot
}

func NewSessionManager(cfg SessionConfig) *SessionManager {
	launcher := cfg.Launcher
	if launcher == nil {
		launcher = ExecBrowserLauncher{}
	}
	site := strings.ToLower(strings.TrimSpace(cfg.Site))
	if strings.TrimSpace(cfg.StartURL) == "" && (site == "gemini" || site == "gemini-web") {
		cfg.StartURL = "https://gemini.google.com/app"
	}
	probe := cfg.Probe
	if probe == nil && (site == "gemini" || site == "gemini-web") {
		probe = NewGeminiDevToolsReadinessProbe()
	}
	now := time.Now()
	return &SessionManager{
		cfg:      cfg,
		launcher: launcher,
		probe:    probe,
		snapshot: SessionSnapshot{State: SessionStopped, Site: site, ChangedAt: now},
	}
}

// Start launches a visible Chrome session using a dedicated persistent profile.
// AUTH_REQUIRED is a successful lifecycle state: the user can log in manually
// in the browser window and Refresh can later promote the session to READY.
func (m *SessionManager) Start(ctx context.Context) (SessionSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return m.Snapshot(), err
	}

	m.mu.Lock()
	if m.process != nil && m.snapshot.State != SessionStopped && m.snapshot.State != SessionFailed {
		snap := m.snapshot
		m.mu.Unlock()
		return snap, fmt.Errorf("webai: browser session already active (%s)", snap.State)
	}
	m.transitionLocked(SessionStarting, "")
	m.mu.Unlock()

	profileDir := strings.TrimSpace(m.cfg.ProfileDir)
	if profileDir == "" {
		var err error
		profileDir, err = DefaultBrowserProfileDir(m.cfg.ProfileName)
		if err != nil {
			m.fail(err)
			return m.Snapshot(), err
		}
	}
	browserPath, err := ResolveChromeExecutable(m.cfg.BrowserPath)
	if err != nil {
		m.fail(err)
		return m.Snapshot(), err
	}

	process, err := m.launcher.Launch(ctx, BrowserLaunchConfig{
		Executable: browserPath,
		ProfileDir: profileDir,
		StartURL:   strings.TrimSpace(m.cfg.StartURL),
		ExtraArgs:  append([]string(nil), m.cfg.ExtraArgs...),
	})
	if err != nil {
		wrapped := &Error{Kind: ErrorTransport, Op: "start browser", Cause: err}
		m.fail(wrapped)
		return m.Snapshot(), wrapped
	}

	now := time.Now()
	m.mu.Lock()
	m.process = process
	m.snapshot.BrowserPath = browserPath
	m.snapshot.ProfileDir = profileDir
	m.snapshot.PID = process.PID()
	m.snapshot.StartedAt = now
	m.snapshot.ChangedAt = now
	m.snapshot.State = SessionAuthRequired
	m.snapshot.Reason = "browser launched; waiting for web login readiness"
	m.mu.Unlock()

	go m.watchProcess(process)

	if m.probe == nil {
		return m.Snapshot(), nil
	}
	return m.Refresh(ctx)
}

// Refresh re-checks website login/readiness without submitting a model prompt.
func (m *SessionManager) Refresh(ctx context.Context) (SessionSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return m.Snapshot(), err
	}
	m.mu.RLock()
	process := m.process
	probe := m.probe
	snap := m.snapshot
	m.mu.RUnlock()
	if process == nil || snap.State == SessionStopped {
		return snap, fmt.Errorf("webai: browser session is not running")
	}
	if probe == nil {
		return snap, nil
	}

	result, err := probe.Probe(ctx, snap)
	if err != nil {
		m.mu.Lock()
		if m.process == process && m.snapshot.State != SessionStopped {
			m.transitionLocked(SessionDegraded, err.Error())
		}
		out := m.snapshot
		m.mu.Unlock()
		return out, err
	}
	if !validReadinessState(result.State) {
		err := fmt.Errorf("webai: readiness probe returned invalid state %q", result.State)
		m.mu.Lock()
		if m.process == process && m.snapshot.State != SessionStopped {
			m.transitionLocked(SessionFailed, err.Error())
		}
		out := m.snapshot
		m.mu.Unlock()
		return out, err
	}

	m.mu.Lock()
	if m.process == process && m.snapshot.State != SessionStopped {
		m.transitionLocked(result.State, strings.TrimSpace(result.Reason))
	}
	out := m.snapshot
	m.mu.Unlock()
	return out, nil
}

// Stop terminates the browser process but deliberately leaves ProfileDir
// untouched so login state can survive the next tool restart.
func (m *SessionManager) Stop() error {
	m.mu.Lock()
	process := m.process
	m.process = nil
	m.snapshot.PID = 0
	m.transitionLocked(SessionStopped, "")
	m.mu.Unlock()
	if process == nil {
		return nil
	}
	return process.Stop()
}

func (m *SessionManager) Snapshot() SessionSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.snapshot
}

func (m *SessionManager) watchProcess(process BrowserProcess) {
	err, ok := <-process.Done()
	if !ok {
		err = nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.process != process || m.snapshot.State == SessionStopped {
		return
	}
	m.process = nil
	m.snapshot.PID = 0
	if err != nil {
		m.transitionLocked(SessionFailed, "browser exited: "+err.Error())
		return
	}
	m.transitionLocked(SessionStopped, "browser exited")
}

func (m *SessionManager) fail(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.process = nil
	m.snapshot.PID = 0
	m.transitionLocked(SessionFailed, err.Error())
}

func (m *SessionManager) transitionLocked(state SessionState, reason string) {
	m.snapshot.State = state
	m.snapshot.Reason = reason
	m.snapshot.ChangedAt = time.Now()
}

func validReadinessState(state SessionState) bool {
	switch state {
	case SessionAuthRequired, SessionReady, SessionDegraded, SessionFailed:
		return true
	default:
		return false
	}
}
