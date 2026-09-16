package webai

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBrowserLaunchArgsInspectionEnablesLoopbackDevTools(t *testing.T) {
	args, err := browserLaunchArgs(BrowserLaunchConfig{
		ProfileDir: "profile",
		StartURL:   "https://gemini.google.com/app",
	})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"--remote-debugging-address=127.0.0.1",
		"--remote-debugging-port=0",
		"--hide-crash-restore-bubble",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("inspection args missing %q: %v", want, args)
		}
	}
}

func TestBrowserLaunchArgsNormalLoginContainsNoDevToolsOrAutomation(t *testing.T) {
	args, err := browserLaunchArgs(BrowserLaunchConfig{
		ProfileDir:      "profile",
		StartURL:        "https://gemini.google.com/app",
		DisableDevTools: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.ToLower(strings.Join(args, " "))
	for _, forbidden := range []string{"remote-debugging", "enable-automation", "headless"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("normal login args contain forbidden %q: %v", forbidden, args)
		}
	}
	for _, want := range []string{
		"--user-data-dir=profile",
		"--hide-crash-restore-bubble",
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-background-mode",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("normal login args missing %q: %v", want, args)
		}
	}
}

func TestBrowserLaunchArgsNormalLoginRejectsInjectedDebugFlags(t *testing.T) {
	for _, arg := range []string{"--remote-debugging-port=9222", "--remote-debugging-pipe", "--enable-automation", "--headless=new"} {
		_, err := browserLaunchArgs(BrowserLaunchConfig{
			ProfileDir:      "profile",
			DisableDevTools: true,
			ExtraArgs:       []string{arg},
		})
		if err == nil {
			t.Fatalf("normal login should reject %q", arg)
		}
	}
}

func TestD08ClearStaleDevToolsActivePortPreservesProfile(t *testing.T) {
	profile := t.TempDir()
	locator := filepath.Join(profile, devToolsActivePortFile)
	marker := filepath.Join(profile, "persisted-login.marker")
	if err := os.WriteFile(locator, []byte("56210\n/devtools/browser/stale\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(marker, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := clearStaleDevToolsActivePort(profile); err != nil {
		t.Fatalf("clear stale locator: %v", err)
	}
	if _, err := os.Stat(locator); !os.IsNotExist(err) {
		t.Fatalf("stale locator still exists or unexpected stat error: %v", err)
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "keep" {
		t.Fatalf("persistent profile content changed: data=%q err=%v", data, err)
	}
	if err := clearStaleDevToolsActivePort(profile); err != nil {
		t.Fatalf("clearing an already absent locator should be idempotent: %v", err)
	}
}
