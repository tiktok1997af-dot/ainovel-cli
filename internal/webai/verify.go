package webai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const verificationEvidenceSchema = "ainovel.webai.w2e.v1"

// VerificationEvent records only coarse operational state. It deliberately
// excludes browser cookies, tokens, account identity, page contents and project data.
type VerificationEvent struct {
	Sequence int          `json:"sequence"`
	At       time.Time    `json:"at"`
	Phase    string       `json:"phase"`
	State    SessionState `json:"state"`
	Reason   string       `json:"reason,omitempty"`
}

// VerificationEvidence is the privacy-bounded local evidence emitted by W2E.
type VerificationEvidence struct {
	Schema             string              `json:"schema"`
	Site               string              `json:"site"`
	ProfileName        string              `json:"profile_name"`
	StartedAt          time.Time           `json:"started_at"`
	CompletedAt        *time.Time          `json:"completed_at,omitempty"`
	Result             string              `json:"result"`
	InitialAuth        bool                `json:"initial_auth_required"`
	LoginReady         bool                `json:"ready_after_manual_login"`
	RestartReady       bool                `json:"ready_after_restart"`
	UserActionObserved bool                `json:"user_action_state_observed"`
	Events             []VerificationEvent `json:"events"`
}

// VerificationConfig controls the W2E real-browser verification harness.
type VerificationConfig struct {
	Site                   string
	BrowserPath            string
	ProfileName            string
	ProfileDir             string
	EvidencePath           string
	PollInterval           time.Duration
	LoginTimeout           time.Duration
	RestartTimeout         time.Duration
	RestartDelay           time.Duration
	WatchUserActionTimeout time.Duration
	AllowExistingProfile   bool
	OnEvent                func(VerificationEvent)

	sessionFactory func(SessionConfig) verificationSession
	now            func() time.Time
}

// VerificationResult is the final W2E outcome.
type VerificationResult struct {
	EvidencePath string
	Evidence     VerificationEvidence
}

type verificationSession interface {
	Start(context.Context) (SessionSnapshot, error)
	Refresh(context.Context) (SessionSnapshot, error)
	Stop() error
	Snapshot() SessionSnapshot
}

// RunBrowserVerification performs the real-browser W2E sequence:
// clean profile -> AUTH_REQUIRED -> manual login -> READY -> restart -> READY.
// It never submits a prompt and never logs the user out automatically.
func RunBrowserVerification(ctx context.Context, cfg VerificationConfig) (VerificationResult, error) {
	now := cfg.now
	if now == nil {
		now = time.Now
	}
	started := now().UTC()

	site := strings.ToLower(strings.TrimSpace(cfg.Site))
	if site == "" {
		site = "gemini-web"
	}
	if site != "gemini" && site != "gemini-web" {
		return VerificationResult{}, fmt.Errorf("webai: W2E currently supports gemini-web only")
	}
	profileName := strings.TrimSpace(cfg.ProfileName)
	if profileName == "" {
		profileName = defaultVerificationProfileName(started)
	}
	profileDir := strings.TrimSpace(cfg.ProfileDir)
	if profileDir == "" {
		var err error
		profileDir, err = DefaultBrowserProfileDir(profileName)
		if err != nil {
			return VerificationResult{}, err
		}
	}
	if !cfg.AllowExistingProfile {
		if err := requireEmptyOrMissingDir(profileDir); err != nil {
			return VerificationResult{}, err
		}
	}
	evidencePath := strings.TrimSpace(cfg.EvidencePath)
	if evidencePath == "" {
		var err error
		evidencePath, err = defaultVerificationEvidencePath(profileName)
		if err != nil {
			return VerificationResult{}, err
		}
	}

	poll := cfg.PollInterval
	if poll <= 0 {
		poll = 2 * time.Second
	}
	loginTimeout := cfg.LoginTimeout
	if loginTimeout <= 0 {
		loginTimeout = 15 * time.Minute
	}
	restartTimeout := cfg.RestartTimeout
	if restartTimeout <= 0 {
		restartTimeout = 30 * time.Second
	}
	restartDelay := cfg.RestartDelay
	if restartDelay <= 0 {
		restartDelay = 2 * time.Second
	}

	evidence := VerificationEvidence{
		Schema:      verificationEvidenceSchema,
		Site:        "gemini-web",
		ProfileName: profileName,
		StartedAt:   started,
		Result:      "IN_PROGRESS",
	}
	emit := func(phase string, snap SessionSnapshot) error {
		event := VerificationEvent{
			Sequence: len(evidence.Events) + 1,
			At:       now().UTC(),
			Phase:    phase,
			State:    snap.State,
			Reason:   strings.TrimSpace(snap.Reason),
		}
		if len(evidence.Events) > 0 {
			last := evidence.Events[len(evidence.Events)-1]
			if last.Phase == event.Phase && last.State == event.State && last.Reason == event.Reason {
				return nil
			}
		}
		evidence.Events = append(evidence.Events, event)
		if cfg.OnEvent != nil {
			cfg.OnEvent(event)
		}
		return writeVerificationEvidence(evidencePath, evidence)
	}
	finish := func(result string) {
		completed := now().UTC()
		evidence.Result = result
		evidence.CompletedAt = &completed
		_ = writeVerificationEvidence(evidencePath, evidence)
	}

	factory := cfg.sessionFactory
	if factory == nil {
		factory = func(sc SessionConfig) verificationSession { return NewSessionManager(sc) }
	}
	manager := factory(SessionConfig{
		Site:        "gemini-web",
		BrowserPath: cfg.BrowserPath,
		ProfileName: profileName,
		ProfileDir:  profileDir,
	})
	defer manager.Stop()

	initial, err := manager.Start(ctx)
	if emit("initial", initial) != nil {
		finish("FAILED_EVIDENCE_WRITE")
		return VerificationResult{EvidencePath: evidencePath, Evidence: evidence}, fmt.Errorf("webai: write W2E evidence")
	}
	if err != nil && initial.State != SessionAuthRequired && initial.State != SessionDegraded {
		finish("FAILED_START")
		return VerificationResult{EvidencePath: evidencePath, Evidence: evidence}, err
	}
	if initial.State == SessionReady && !cfg.AllowExistingProfile {
		finish("FAILED_NOT_CLEAN")
		return VerificationResult{EvidencePath: evidencePath, Evidence: evidence}, fmt.Errorf("webai: fresh verification profile unexpectedly started READY")
	}
	if initial.State == SessionAuthRequired {
		evidence.InitialAuth = true
		_ = writeVerificationEvidence(evidencePath, evidence)
	} else {
		initial, err = waitForVerificationState(ctx, manager, poll, 30*time.Second, "initial_auth_wait", emit, func(s SessionState) bool {
			return s == SessionAuthRequired || s == SessionReady
		})
		if err != nil {
			finish("FAILED_INITIAL_AUTH")
			return VerificationResult{EvidencePath: evidencePath, Evidence: evidence}, err
		}
		if initial.State == SessionReady && !cfg.AllowExistingProfile {
			finish("FAILED_NOT_CLEAN")
			return VerificationResult{EvidencePath: evidencePath, Evidence: evidence}, fmt.Errorf("webai: fresh verification profile reached READY before AUTH_REQUIRED")
		}
		if initial.State == SessionAuthRequired {
			evidence.InitialAuth = true
			_ = writeVerificationEvidence(evidencePath, evidence)
		}
	}
	if !evidence.InitialAuth && !cfg.AllowExistingProfile {
		finish("FAILED_INITIAL_AUTH")
		return VerificationResult{EvidencePath: evidencePath, Evidence: evidence}, fmt.Errorf("webai: clean profile did not produce AUTH_REQUIRED")
	}

	ready, err := waitForVerificationState(ctx, manager, poll, loginTimeout, "login_wait", emit, func(s SessionState) bool {
		return s == SessionReady
	})
	if err != nil {
		finish("FAILED_LOGIN_READY")
		return VerificationResult{EvidencePath: evidencePath, Evidence: evidence}, err
	}
	if ready.State != SessionReady {
		finish("FAILED_LOGIN_READY")
		return VerificationResult{EvidencePath: evidencePath, Evidence: evidence}, fmt.Errorf("webai: login readiness ended in %s", ready.State)
	}
	evidence.LoginReady = true
	if err := emit("login_ready", ready); err != nil {
		finish("FAILED_EVIDENCE_WRITE")
		return VerificationResult{EvidencePath: evidencePath, Evidence: evidence}, err
	}

	if err := manager.Stop(); err != nil {
		finish("FAILED_STOP_BEFORE_RESTART")
		return VerificationResult{EvidencePath: evidencePath, Evidence: evidence}, fmt.Errorf("webai: stop browser before restart: %w", err)
	}
	stopped := manager.Snapshot()
	_ = emit("restart_stop", stopped)
	if err := waitVerificationDelay(ctx, restartDelay); err != nil {
		finish("FAILED_RESTART_DELAY")
		return VerificationResult{EvidencePath: evidencePath, Evidence: evidence}, err
	}

	restarted, err := manager.Start(ctx)
	_ = emit("restart_start", restarted)
	if err != nil && restarted.State != SessionAuthRequired && restarted.State != SessionDegraded {
		finish("FAILED_RESTART")
		return VerificationResult{EvidencePath: evidencePath, Evidence: evidence}, err
	}
	restarted, err = waitForVerificationState(ctx, manager, poll, restartTimeout, "restart_wait", emit, func(s SessionState) bool {
		return s == SessionReady
	})
	if err != nil {
		finish("FAILED_RESTART_READY")
		return VerificationResult{EvidencePath: evidencePath, Evidence: evidence}, err
	}
	evidence.RestartReady = restarted.State == SessionReady
	if !evidence.RestartReady {
		finish("FAILED_RESTART_READY")
		return VerificationResult{EvidencePath: evidencePath, Evidence: evidence}, fmt.Errorf("webai: persisted profile did not return READY after restart")
	}
	_ = emit("restart_ready", restarted)

	if cfg.WatchUserActionTimeout > 0 {
		observed, watchErr := waitForVerificationState(ctx, manager, poll, cfg.WatchUserActionTimeout, "user_action_watch", emit, func(s SessionState) bool {
			return s == SessionAuthRequired
		})
		if watchErr == nil && observed.State == SessionAuthRequired {
			evidence.UserActionObserved = true
			_ = emit("user_action_observed", observed)
		}
	}

	finish("PASS_RESTART_READY")
	return VerificationResult{EvidencePath: evidencePath, Evidence: evidence}, nil
}

func waitForVerificationState(
	ctx context.Context,
	session verificationSession,
	poll time.Duration,
	timeout time.Duration,
	phase string,
	emit func(string, SessionSnapshot) error,
	accept func(SessionState) bool,
) (SessionSnapshot, error) {
	if timeout <= 0 {
		return session.Snapshot(), fmt.Errorf("webai: verification timeout must be positive")
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(poll)
	defer ticker.Stop()

	for {
		snap := session.Snapshot()
		if accept(snap.State) {
			return snap, nil
		}
		if snap.State == SessionFailed || snap.State == SessionStopped {
			return snap, fmt.Errorf("webai: browser verification entered %s: %s", snap.State, snap.Reason)
		}
		select {
		case <-ctx.Done():
			return session.Snapshot(), ctx.Err()
		case <-deadline.C:
			return session.Snapshot(), fmt.Errorf("webai: timed out waiting during %s", phase)
		case <-ticker.C:
			next, err := session.Refresh(ctx)
			_ = emit(phase, next)
			if err != nil && next.State != SessionDegraded {
				return next, err
			}
		}
	}
}

func waitVerificationDelay(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func defaultVerificationProfileName(now time.Time) string {
	return "w2-verify-" + now.UTC().Format("20060102-150405")
}

func defaultVerificationEvidencePath(profileName string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("webai: resolve home directory: %w", err)
	}
	return filepath.Join(home, ".ainovel", "browser", "evidence", profileName+".json"), nil
}

func requireEmptyOrMissingDir(path string) error {
	entries, err := os.ReadDir(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("webai: inspect verification profile: %w", err)
	}
	if len(entries) != 0 {
		return fmt.Errorf("webai: verification profile is not clean: %s", path)
	}
	return nil
}

func writeVerificationEvidence(path string, evidence VerificationEvidence) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("webai: evidence path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("webai: create evidence directory: %w", err)
	}
	data, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		return fmt.Errorf("webai: encode verification evidence: %w", err)
	}
	data = append(data, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("webai: write verification evidence: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("webai: commit verification evidence: %w", err)
	}
	return nil
}
