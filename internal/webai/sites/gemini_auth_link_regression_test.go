package sites

import (
	"strings"
	"testing"
)

// Regression: signed-in Gemini exposes accounts.google.com links such as
// SignOutOptions / ManageAccount. Those are authenticated account controls and
// must never be treated as a generic sign-in prompt merely because their host
// is accounts.google.com.
func TestGeminiReadinessExpressionDistinguishesAccountControlsFromSignIn(t *testing.T) {
	if strings.Contains(geminiReadinessExpression, "href.includes('accounts.google.com')") {
		t.Fatal("Gemini readiness still treats every accounts.google.com link as sign-in evidence")
	}

	expr := strings.ToLower(geminiReadinessExpression)
	for _, marker := range []string{
		"accounthrefrequiressignin",
		"/signoutoptions",
		"/manageaccount",
		"/servicelogin",
		"/signin",
		"/v3/signin",
		"/challenge",
		"/accountchooser",
	} {
		if !strings.Contains(expr, marker) {
			t.Fatalf("Gemini readiness expression missing guarded auth marker %q", marker)
		}
	}
}

func TestGeminiProbeAuthenticatedComposerWinsWithoutRealSignIn(t *testing.T) {
	result := probeGeminiPayload(t, `{
		"host":"gemini.google.com",
		"path":"/app",
		"has_account_control":true,
		"has_composer":true,
		"has_sign_in":false,
		"candidate_account_link":true
	}`)
	if result.State != ReadinessReady {
		t.Fatalf("state = %q, want %q (reason %q)", result.State, ReadinessReady, result.Reason)
	}
}
