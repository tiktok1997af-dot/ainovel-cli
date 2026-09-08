package guard

import (
	"context"
	"strings"
	"testing"

	"github.com/voocel/agentcore"
)

func TestEditorArcSummaryStopGuardBlockedTurnCarriesLocalToolRequiredMarker(t *testing.T) {
	s := newTestStore(t)
	g := NewEditorStopGuard(s, "请生成弧摘要并调用 save_arc_summary", nil)
	d := g(context.Background(), agentcore.StopInfo{
		TurnIndex: 1,
		Message:   agentcore.Message{StopReason: agentcore.StopReasonStop},
	})
	if d.Allow || d.Escalate {
		t.Fatalf("first empty editor arc-summary stop must be blocked, got %#v", d)
	}
	if !strings.Contains(d.InjectMessage, localToolRequiredMarker) {
		t.Fatalf("editor arc-summary block message is missing %q: %q", localToolRequiredMarker, d.InjectMessage)
	}
	if !strings.Contains(d.InjectMessage, "save_arc_summary") {
		t.Fatalf("editor arc-summary block message lost tool guidance: %q", d.InjectMessage)
	}
}

func TestEditorVolumeSummaryStopGuardBlockedTurnCarriesLocalToolRequiredMarker(t *testing.T) {
	s := newTestStore(t)
	g := NewEditorStopGuard(s, "请生成卷摘要并调用 save_volume_summary", nil)
	d := g(context.Background(), agentcore.StopInfo{
		TurnIndex: 1,
		Message:   agentcore.Message{StopReason: agentcore.StopReasonStop},
	})
	if d.Allow || d.Escalate {
		t.Fatalf("first empty editor volume-summary stop must be blocked, got %#v", d)
	}
	if !strings.Contains(d.InjectMessage, localToolRequiredMarker) {
		t.Fatalf("editor volume-summary block message is missing %q: %q", localToolRequiredMarker, d.InjectMessage)
	}
	if !strings.Contains(d.InjectMessage, "save_volume_summary") {
		t.Fatalf("editor volume-summary block message lost tool guidance: %q", d.InjectMessage)
	}
}
