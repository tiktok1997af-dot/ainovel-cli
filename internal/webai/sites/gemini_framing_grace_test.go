package sites

import (
	"strings"
	"testing"
)

func TestGeminiResolverSeedsSanitizedCaptureStateBeforeTrustedSend(t *testing.T) {
	seed := strings.Index(geminiResolveSendExpression, "window.__ainovelWebCaptureState =")
	result := strings.Index(geminiResolveSendExpression, "return {ok: true")
	if seed < 0 {
		t.Fatal("send resolver does not seed capture state")
	}
	if result < 0 {
		t.Fatal("send resolver does not return the trusted-click coordinates")
	}
	if seed > result {
		t.Fatal("capture state must be seeded before trusted-click coordinates are returned")
	}
	for _, want := range []string{"responseCount", "lastSignature", "currentSignature", "lastChangedAt", "observedChange", "composerLengthBeforeClick", "submitAction"} {
		if !strings.Contains(geminiResolveSendExpression, want) {
			t.Fatalf("send capture state missing sanitized field %q", want)
		}
	}
	if strings.Contains(geminiResolveSendExpression, ".click()") || strings.Contains(geminiResolveSendExpression, "responseText:") || strings.Contains(geminiResolveSendExpression, "promptText:") {
		t.Fatal("send resolver must remain side-effect free and must not persist raw prompt/response text")
	}
}

func TestGeminiConversationHoldsRecentDOMActivityAsBusy(t *testing.T) {
	for _, want := range []string{
		"window.__ainovelWebCaptureState",
		"activityGraceMs = 2500",
		"captureState.observedChange",
		"captureState.lastChangedAt = Date.now()",
		"if (elapsed < activityGraceMs)",
		"busy = true",
	} {
		if !strings.Contains(geminiConversationExpression, want) {
			t.Fatalf("conversation expression missing DOM-activity invariant %q", want)
		}
	}
}

func TestGeminiConversationActivityStabilizationDoesNotDependOnProtocolMarkers(t *testing.T) {
	for _, forbidden := range []string{
		"framingGraceMs",
		"framingComplete",
		"<<<AINOVEL_WEB_RESPONSE>>>",
		"<<<END_AINOVEL_WEB_RESPONSE>>>",
	} {
		if strings.Contains(geminiConversationExpression, forbidden) {
			t.Fatalf("DOM activity stabilization still depends on protocol marker %q", forbidden)
		}
	}
	if !strings.Contains(geminiConversationExpression, "window.__ainovelWebCaptureState = null") {
		t.Fatal("stale capture state is not cleared after the activity grace")
	}
}
