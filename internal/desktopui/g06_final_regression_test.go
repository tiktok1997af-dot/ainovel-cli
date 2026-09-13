package desktopui

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/voocel/ainovel-cli/internal/appruntime"
)

func TestG067CumulativeReviewWorkspaceAuthorityFlow(t *testing.T) {
	runtime := newReviewWorkspaceRuntime()
	runtime.status.Verdict = "rewrite"
	runtime.status.AffectedChapters = []int{3}
	for i := range runtime.status.Gates {
		if runtime.status.Gates[i].GateID == appruntime.ReviewGateContinuity {
			runtime.status.Gates[i].Actionable = true
		}
	}

	controller := NewController(runtime, 1440)
	ctx := context.Background()
	if err := controller.Bootstrap(ctx); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if err := controller.OpenReview(ctx, runtime.status.Target); err != nil {
		t.Fatalf("OpenReview: %v", err)
	}
	if controller.Shell().Route != RouteReview {
		t.Fatalf("route=%q, want review", controller.Shell().Route)
	}
	if len(controller.Review().Catalog.Gates) != 12 || len(controller.Review().Status.Gates) != 12 {
		t.Fatalf("review gate projection catalog=%d status=%d, want 12/12", len(controller.Review().Catalog.Gates), len(controller.Review().Status.Gates))
	}

	if err := controller.RunReview(ctx); err != nil {
		t.Fatalf("RunReview: %v", err)
	}
	if err := controller.RepairReview(ctx, []appruntime.ReviewGateID{appruntime.ReviewGateContinuity}, []int{3}); err != nil {
		t.Fatalf("RepairReview: %v", err)
	}
	if err := controller.RerunReview(ctx); err != nil {
		t.Fatalf("RerunReview: %v", err)
	}
	if err := controller.PromoteReviewOfficial(ctx); err != nil {
		t.Fatalf("PromoteReviewOfficial: %v", err)
	}

	wantCommands := []appruntime.CommandKind{
		appruntime.CommandReviewRun,
		appruntime.CommandReviewRepair,
		appruntime.CommandReviewRerun,
		appruntime.CommandReviewPromoteOfficial,
	}
	if len(runtime.commands) != len(wantCommands) {
		t.Fatalf("commands=%d, want=%d", len(runtime.commands), len(wantCommands))
	}
	for i, cmd := range runtime.commands {
		if cmd.Kind != wantCommands[i] {
			t.Fatalf("command[%d]=%q, want=%q", i, cmd.Kind, wantCommands[i])
		}
		if cmd.RunID != "" || cmd.TaskID != "" || cmd.Resource != "" {
			t.Fatalf("Review UI claimed core-owned execution fields: %+v", cmd)
		}
	}

	var promote appruntime.ReviewPromoteOfficialCommandPayload
	if err := json.Unmarshal(runtime.commands[3].Payload, &promote); err != nil {
		t.Fatalf("decode promotion payload: %v", err)
	}
	if promote.ExpectedFingerprint != runtime.status.Freshness.Fingerprint {
		t.Fatalf("promotion fingerprint=%q, want=%q", promote.ExpectedFingerprint, runtime.status.Freshness.Fingerprint)
	}
	if len(promote.ExpectedRevisions) != len(runtime.status.Freshness.Revisions) || promote.ExpectedRevisions[0] != runtime.status.Freshness.Revisions[0] {
		t.Fatalf("promotion revisions=%+v, want=%+v", promote.ExpectedRevisions, runtime.status.Freshness.Revisions)
	}
	if controller.Review().PromotionReceipt == nil {
		t.Fatal("missing transient promotion receipt")
	}

	before := len(runtime.queries)
	runtime.status.Summary = "fresh-after-review-event"
	runtime.subscription.events <- appruntime.DesktopEvent{
		ContractVersion: appruntime.ContractVersion,
		Seq:             1,
		Category:        appruntime.ReviewEventCategory,
		Type:            appruntime.EventTypeReviewAction,
		Summary:         "notification-only",
	}
	if err := controller.PumpEvent(ctx); err != nil {
		t.Fatalf("PumpEvent: %v", err)
	}
	if len(runtime.queries) != before+3 {
		t.Fatalf("event refresh queries=%d, want=%d", len(runtime.queries), before+3)
	}
	if controller.Review().Status.Summary != "fresh-after-review-event" {
		t.Fatalf("workspace trusted event payload instead of fresh query: %+v", controller.Review().Status)
	}

	other := appruntime.ReviewTargetDTO{Scope: appruntime.ReviewScopeChapter, Chapter: 4}
	if err := controller.OpenReview(ctx, other); err != nil {
		t.Fatalf("OpenReview other target: %v", err)
	}
	if controller.Review().PromotionReceipt != nil {
		t.Fatal("transient promotion receipt leaked into durable/local Official truth")
	}
}
