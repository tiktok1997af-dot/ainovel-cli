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

const authRequiredGrace = 15 * time.Second

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
	var siteOverride string
	var profileOverride string
	var startURLOverride string
	flag.DurationVar(&timeout, "timeout", 45*time.Second, "maximum time to wait for each READY transition")
	flag.StringVar(&evidencePath, "evidence", "w5e-readiness-evidence.json", "sanitized JSON evidence output path")
	flag.StringVar(&siteOverride, "site", "", "optional WEB site override: gemini-web or chatgpt-web")
	flag.StringVar(&profileOverride, "profile-name", "", "optional persistent browser profile override")
	flag.StringVar(&startURLOverride, "start-url", "", "optional WEB start URL override")
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

	site := normalizeSite(siteOverride)
	if site == "" {
		site = normalizeSite(cfg.Web.Site)
	}
	if site != bootstrap.WebModelName && site != webai.ChatGPTWebSite {
		fatalf("unsupported WEB site %q", site)
	}

	profileName := strings.TrimSpace(profileOverride)
	if profileName == "" {
		if site == webai.ChatGPTWebSite {
			profileName = webai.DefaultChatGPTProfileName
		} else {
			profileName = strings.TrimSpace(cfg.Web.ProfileName)
			if profileName == "" {
				profileName = "default"
			}
		}
	}

	startURL := strings.TrimSpace(startURLOverride)
	if startURL == "" && site == normalizeSite(cfg.Web.Site) {
		startURL = strings.TrimSpace(cfg.Web.StartURL)
	}

	mgr := webai.NewSessionManager(webai.SessionConfig{
		Site:        site,
		BrowserPath: cfg.Web.BrowserPath,
		ProfileName: profileName,
		StartURL:    startURL,
	})
	defer func() { _ = mgr.Stop() }()

	ev := evidence{
		Schema:      evidenceSchema,
		Site:        site,
		ProfileName: profileName,
	}

	first, err := requireReady(mgr, site, timeout)
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

	restarted, err := requireReady(mgr, site, timeout)
	if err != nil {
		fatalf("restart READY verification failed: %v", err)
	}
	ev.RestartReady = true
	ev.States = append(ev.States, string(restarted.State))
	ev.VerifiedAt = time.Now().UTC()

	if err := writeEvidence(evidencePath, ev); err != nil {
		fatalf("write evidence: %v", err)
	}
	fmt.Printf("W5E readiness PASS: %s %s -> %s using persistent profile %q\n", site, first.State, restarted.State, profileName)
}

func normalizeSite(site string) string {
	switch strings.ToLower(strings.TrimSpace(site)) {
	case "gemini", "gemini-web":
		return bootstrap.WebModelName
	case "chatgpt", "chatgpt-web":
		return webai.ChatGPTWebSite
	default:
		return strings.ToLower(strings.TrimSpace(site))
	}
}

func requireReady(mgr *webai.SessionManager, site string, timeout time.Duration) (webai.SessionSnapshot, error) {
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
			} else if time.Since(authSince) >= authRequiredGrace {
				reason := strings.TrimSpace(snap.Reason)
				if reason == "" {
					reason = "no sanitized readiness reason"
				}
				return snap, fmt.Errorf("existing Chrome profile is AUTH_REQUIRED after %s for %s (%s); complete normal visible login outside this verifier, then rerun", authRequiredGrace, site, reason)
			}
		} else {
			authSince = time.Time{}
		}

		select {
		case <-ctx.Done():
			return snap, fmt.Errorf("READY timeout after %s for %s (last state %s, reason %q): %w", timeout, site, snap.State, strings.TrimSpace(snap.Reason), ctx.Err())
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
