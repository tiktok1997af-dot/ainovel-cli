package sites

import (
	"strings"
	"testing"
)

func TestGeminiClickSeedsSanitizedFramingStateBeforeSend(t *testing.T) {
	seed := strings.Index(geminiClickSendExpression, "window.__ainovelWebCaptureState =")
	click := strings.Index(geminiClickSendExpression, "sendButton.click()")
	if seed < 0 {
		t.Fatal("send expression does not seed framing capture state")
	}
	if click < 0 {
		t.Fatal("send expression does not click the send control")
	}
	if seed > click {
		t.Fatal("framing capture state must be seeded before the side-effecting send click")
	}
	for _, want := range []string{"responseCount", "lastSignature", "currentSignature", "lastChangedAt"} {
		if !strings.Contains(geminiClickSendExpression, want) {
			t.Fatalf("send framing state missing sanitized field %q", want)
		}
	}
	if strings.Contains(geminiClickSendExpression, "responseText:") || strings.Contains(geminiClickSendExpression, "promptText:") {
		t.Fatal("framing state must not persist raw prompt/response text")
	}
}

func TestGeminiConversationHoldsIncompleteFramingThroughStreamingPause(t *testing.T) {
	for _, want := range []string{
		"window.__ainovelWebCaptureState",
		"framingGraceMs = 6000",
		"lastChangedAt = Date.now()",
		"trimmed.startsWith('<<<AINOVEL_WEB_RESPONSE>>>')",
		"trimmed.endsWith('<<<END_AINOVEL_WEB_RESPONSE>>>')",
		"if (elapsed < framingGraceMs) busy = true",
	} {
		if !strings.Contains(geminiConversationExpression, want) {
			t.Fatalf("conversation expression missing framing-grace invariant %q", want)
		}
	}
}

func TestGeminiConversationClearsFramingStateOnlyAfterCompleteOuterEnvelope(t *testing.T) {
	completeCheck := strings.Index(geminiConversationExpression, "const framingComplete =")
	clearState := strings.Index(geminiConversationExpression, "window.__ainovelWebCaptureState = null")
	if completeCheck < 0 || clearState < 0 || clearState < completeCheck {
		t.Fatal("framing capture state is not cleared after the complete-envelope check")
	}
}
