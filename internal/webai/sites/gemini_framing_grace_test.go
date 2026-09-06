package sites

import (
	"strings"
	"testing"
)

func TestGeminiClickSeedsSanitizedCaptureStateBeforeSend(t *testing.T) {
	seed := strings.Index(geminiClickSendExpression, "window.__ainovelWebCaptureState =")
	click := strings.Index(geminiClickSendExpression, "sendButton.click()")
	if seed < 0 {
		t.Fatal("send expression does not seed capture state")
	}
	if click < 0 {
		t.Fatal("send expression does not click the send control")
	}
	if seed > click {
		t.Fatal("capture state must be seeded before the side-effecting send click")
	}
	for _, want := range []string{"responseCount", "lastSignature", "currentSignature", "lastChangedAt", "observedChange"} {
		if !strings.Contains(geminiClickSendExpression, want) {
			t.Fatalf("send capture state missing sanitized field %q", want)
		}
	}
	if strings.Contains(geminiClickSendExpression, "responseText:") || strings.Contains(geminiClickSendExpression, "promptText:") {
		t.Fatal("capture state must not persist raw prompt/response text")
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
