package webai

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeVerificationSession struct {
	mu        sync.Mutex
	starts    []SessionSnapshot
	refreshes []SessionSnapshot
	startN    int
	refreshN  int
	stops     int
	current   SessionSnapshot
}

func (f *fakeVerificationSession) Start(context.Context) (SessionSnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.startN >= len(f.starts) {
		return f.current, errors.New("unexpected Start")
	}
	f.current = f.starts[f.startN]
	f.startN++
	return f.current, nil
}

func (f *fakeVerificationSession) Refresh(context.Context) (SessionSnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.refreshN < len(f.refreshes) {
		f.current = f.refreshes[f.refreshN]
		f.refreshN++
	}
	return f.current, nil
}

func (f *fakeVerificationSession) Stop() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stops++
	f.current = SessionSnapshot{State: SessionStopped}
	return nil
}

func (f *fakeVerificationSession) Snapshot() SessionSnapshot {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.current
}

func TestRunBrowserVerificationAuthReadyRestartReady(t *testing.T) {
	profileDir := filepath.Join(t.TempDir(), "profile")
	evidencePath := filepath.Join(t.TempDir(), "evidence.json")
	fake := &fakeVerificationSession{
		starts: []SessionSnapshot{
			{State: SessionAuthRequired, Reason: "manual login required"},
			{State: SessionReady, Reason: "persisted login ready"},
		},
		refreshes: []SessionSnapshot{{State: SessionReady, Reason: "authenticated"}},
	}
	var events []VerificationEvent
	result, err := RunBrowserVerification(context.Background(), VerificationConfig{
		Site:           "gemini-web",
		ProfileName:    "w2-test",
		ProfileDir:     profileDir,
		EvidencePath:   evidencePath,
		PollInterval:   time.Millisecond,
		LoginTimeout:   100 * time.Millisecond,
		RestartTimeout: 100 * time.Millisecond,
		OnEvent: func(event VerificationEvent) {
			events = append(events, event)
		},
		sessionFactory: func(SessionConfig) verificationSession { return fake },
		now:            func() time.Time { return time.Unix(100, 0).UTC() },
	})
	if err != nil {
		t.Fatalf("RunBrowserVerification: %v", err)
	}
	if !result.Evidence.InitialAuth || !result.Evidence.LoginReady || !result.Evidence.RestartReady {
		t.Fatalf("unexpected evidence: %+v", result.Evidence)
	}
	if result.Evidence.Result != "PASS_RESTART_READY" {
		t.Fatalf("result = %q", result.Evidence.Result)
	}
	if len(events) < 4 {
		t.Fatalf("events = %d, want at least 4", len(events))
	}
	data, err := os.ReadFile(evidencePath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, forbidden := range []string{"cookie", "token", "localStorage", profileDir} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("evidence leaked forbidden value %q: %s", forbidden, text)
		}
	}
}

func TestRunBrowserVerificationRejectsExistingProfile(t *testing.T) {
	profileDir := filepath.Join(t.TempDir(), "profile")
	if err := os.MkdirAll(profileDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profileDir, "Cookies"), []byte("browser-owned"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := RunBrowserVerification(context.Background(), VerificationConfig{
		ProfileDir: profileDir,
	})
	if err == nil || !strings.Contains(err.Error(), "not clean") {
		t.Fatalf("err = %v, want clean-profile rejection", err)
	}
}

func TestRunBrowserVerificationFreshProfileCannotStartReady(t *testing.T) {
	fake := &fakeVerificationSession{
		starts: []SessionSnapshot{{State: SessionReady, Reason: "unexpected prior login"}},
	}
	_, err := RunBrowserVerification(context.Background(), VerificationConfig{
		ProfileDir:   filepath.Join(t.TempDir(), "profile"),
		EvidencePath: filepath.Join(t.TempDir(), "evidence.json"),
		sessionFactory: func(SessionConfig) verificationSession {
			return fake
		},
	})
	if err == nil || !strings.Contains(err.Error(), "unexpectedly started READY") {
		t.Fatalf("err = %v", err)
	}
}

func TestVerificationEvidenceDoesNotContainSessionPathsOrSecrets(t *testing.T) {
	evidence := VerificationEvidence{
		Schema:      verificationEvidenceSchema,
		Site:        "gemini-web",
		ProfileName: "safe-name",
		Result:      "PASS_RESTART_READY",
		Events: []VerificationEvent{{
			Sequence: 1,
			State:    SessionReady,
			Reason:   "authenticated Gemini composer is ready",
		}},
	}
	path := filepath.Join(t.TempDir(), "evidence.json")
	if err := writeVerificationEvidence(path, evidence); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"browser_path", "profile_dir", "cookie", "auth_token", "local_storage"} {
		if strings.Contains(strings.ToLower(string(data)), forbidden) {
			t.Fatalf("evidence contains forbidden field %q: %s", forbidden, data)
		}
	}
}
