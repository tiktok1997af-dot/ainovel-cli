package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/voocel/ainovel-cli/internal/bootstrap"
	"github.com/voocel/ainovel-cli/internal/webai"
)

const evidenceSchema = "ainovel-w5e-readiness/1"

type evidence struct {
	Schema       string    `json:"schema"`
	Site         string    `json:"site"`
	ProfileName  string    `json:"profile_name"`
	FirstReady   bool      `json:"first_ready"`
	RestartReady bool      `json:"restart_ready"`
	States       []string  `json:"states"`
	VerifiedAt   time.Time `json:"verified_at"`
}

func main() {
	var timeout time.Duration
	var evidencePath string
	flag.DurationVar(&timeout, "timeout", 45*time.Second, "maximum time to wait for each READY transition")
	flag.StringVar(&evidencePath, "evidence", "w5e-readiness-evidence.json", "sanitized JSON evidence output path")
	flag.Parse()

	if timeout <= 0 {
		fatalf("timeout must be positive")
	}

	cfg, err := bootstrap.LoadConfig()
	if err != nil {
		fatalf("load WEB-only config: %v", err)
	}
	cfg.FillDefaults()
	if err := cfg.ValidateBase(); err != nil {
		fatalf("validate WEB-only config: %v", err)
	}

	profileName := strings.TrimSpace(cfg.Web.ProfileName)
	if profileName == "" {
		profileName = "default"
	}

	mgr := webai.NewSessionManager(webai.SessionConfig{
		Site:        cfg.Web.Site,
		BrowserPath: cfg.Web.BrowserPath,
		ProfileName: profileName,
		StartURL:    cfg.Web.StartURL,
	})
	defer func() { _ = mgr.Stop() }()

	ev := evidence{
		Schema:      evidenceSchema,
		Site:        strings.TrimSpace(cfg.Web.Site),
		ProfileName: profileName,
	}
	if ev.Site == "" {
		ev.Site = bootstrap.WebModelName
	}

	first, err := requireReady(mgr, timeout)
	if err != nil {
		fatalf("first READY verification failed: %v", err)
	}
	ev.FirstReady = true
	ev.States = append(ev.States, string(first.State))

	if err := mgr.Stop(); err != nil {
		fatalf("stop browser after first READY: %v", err)
	}
	ev.States = append(ev.States, string(webai.SessionStopped))
	time.Sleep(time.Second)

	restarted, err := requireReady(mgr, timeout)
	if err != nil {
		fatalf("restart READY verification failed: %v", err)
	}
	ev.RestartReady = true
	ev.States = append(ev.States, string(restarted.State))
	ev.VerifiedAt = time.Now().UTC()

	if err := writeEvidence(evidencePath, ev); err != nil {
		fatalf("write evidence: %v", err)
	}
	fmt.Printf("W5E readiness PASS: %s -> %s using persistent profile %q\n", first.State, restarted.State, profileName)
}

func requireReady(mgr *webai.SessionManager, timeout time.Duration) (webai.SessionSnapshot, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	snap, startErr := mgr.Start(ctx)
	if snap.State == webai.SessionReady {
		return snap, nil
	}
	if startErr != nil && snap.State != webai.SessionDegraded && snap.State != webai.SessionAuthRequired {
		return snap, startErr
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	authSince := time.Time{}
	for {
		if snap.State == webai.SessionAuthRequired {
			if authSince.IsZero() {
				authSince = time.Now()
			} else if time.Since(authSince) >= 3*time.Second {
				return snap, fmt.Errorf("existing Chrome profile is AUTH_REQUIRED; complete normal visible Gemini login outside this verifier, then rerun")
			}
		} else {
			authSince = time.Time{}
		}

		select {
		case <-ctx.Done():
			return snap, fmt.Errorf("READY timeout after %s (last state %s): %w", timeout, snap.State, ctx.Err())
		case <-ticker.C:
			var err error
			snap, err = mgr.Refresh(ctx)
			if snap.State == webai.SessionReady {
				return snap, nil
			}
			if snap.State == webai.SessionFailed {
				if err != nil {
					return snap, err
				}
				return snap, fmt.Errorf("browser session entered FAILED")
			}
		}
	}
}

func writeEvidence(path string, ev evidence) error {
	data, err := json.MarshalIndent(ev, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o600)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "W5E readiness FAIL: "+format+"\n", args...)
	os.Exit(1)
}
