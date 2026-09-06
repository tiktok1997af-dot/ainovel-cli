package webai

import (
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
	for _, want := range []string{"--user-data-dir=profile", "--hide-crash-restore-bubble"} {
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
