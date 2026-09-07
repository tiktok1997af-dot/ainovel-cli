package guard

import (
	"context"
	"strings"
	"testing"

	"github.com/voocel/agentcore"
)

func TestWriterStopGuardBlockedTurnCarriesLocalToolRequiredMarker(t *testing.T) {
	s := newTestStore(t)
	g := NewWriterStopGuard(s, nil)
	d := g(context.Background(), agentcore.StopInfo{
		TurnIndex: 1,
		Message:   agentcore.Message{StopReason: agentcore.StopReasonStop},
	})
	if d.Allow || d.Escalate {
		t.Fatalf("first empty writer stop must be blocked, got %#v", d)
	}
	if !strings.Contains(d.InjectMessage, localToolRequiredMarker) {
		t.Fatalf("writer block message is missing %q: %q", localToolRequiredMarker, d.InjectMessage)
	}
	if !strings.Contains(d.InjectMessage, "draft_chapter") {
		t.Fatalf("writer block message lost its stage guidance: %q", d.InjectMessage)
	}
}
