package webai

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestW6CSmokeWaitsForGeminiProfileReleaseBeforeProduction(t *testing.T) {
	path := filepath.Join("..", "..", "scripts", "w6c-release-artifact-smoke.ps1")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read W6C smoke script: %v", err)
	}
	script := string(raw)

	helper := "function Wait-SmokeChromeProfileReleased"
	profile := "$profileDir = Join-Path"
	wait := "Wait-SmokeChromeProfileReleased $profileDir $ReadinessTimeoutSeconds"
	launch := "$first = Start-Process -FilePath $productionExe"
	stop := "Stop-SmokeChrome $profileDir"

	for _, marker := range []string{helper, profile, wait, launch, stop} {
		if !strings.Contains(script, marker) {
			t.Fatalf("W6C smoke script missing handoff marker %q", marker)
		}
	}

	profileAt := strings.Index(script, profile)
	firstWaitAt := strings.Index(script[profileAt:], wait)
	if firstWaitAt < 0 {
		t.Fatal("W6C smoke script does not wait for the Gemini profile after readiness")
	}
	firstWaitAt += profileAt
	launchAt := strings.Index(script, launch)
	if launchAt < 0 || firstWaitAt >= launchAt {
		t.Fatal("W6C Gemini profile release wait must happen before first packaged production launch")
	}

	stopAt := strings.Index(script[launchAt:], stop)
	if stopAt < 0 {
		t.Fatal("W6C checkpoint cleanup does not stop Gemini Chrome")
	}
	stopAt += launchAt
	resumeWaitAt := strings.Index(script[stopAt:], wait)
	if resumeWaitAt < 0 {
		t.Fatal("W6C checkpoint cleanup does not wait for Gemini profile release before resume")
	}

	if strings.Contains(script, "Stop-SmokeChrome $profileDir\n    Start-Sleep -Seconds 2") {
		t.Fatal("W6C still relies on a fixed sleep instead of bounded profile-release observation")
	}
}
