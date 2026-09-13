package desktopui

import (
	"context"
	"testing"

	"github.com/voocel/ainovel-cli/internal/appruntime"
)

func TestReviewActionEventRefreshesOnlyWhenReviewWorkspaceOpen(t *testing.T) {
	runtime := newReviewWorkspaceRuntime()
	controller := NewController(runtime, 1440)
	ctx := context.Background()
	if err := controller.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	if err := controller.OpenReview(ctx, runtime.status.Target); err != nil {
		t.Fatal(err)
	}
	before := len(runtime.queries)
	runtime.subscription.events <- appruntime.DesktopEvent{
		ContractVersion: appruntime.ContractVersion,
		Seq:             3,
		Category:        appruntime.ReviewEventCategory,
		Type:            appruntime.EventTypeReviewAction,
	}
	if err := controller.PumpEvent(ctx); err != nil {
		t.Fatal(err)
	}
	if len(runtime.queries) != before+3 {
		t.Fatalf("open Review action event did not reconcile: before=%d after=%d", before, len(runtime.queries))
	}

	if !controller.Shell().SelectRoute(RouteOverview) {
		t.Fatal("failed to leave Review route")
	}
	before = len(runtime.queries)
	runtime.subscription.events <- appruntime.DesktopEvent{
		ContractVersion: appruntime.ContractVersion,
		Seq:             4,
		Category:        appruntime.ReviewEventCategory,
		Type:            appruntime.EventTypeReviewState,
	}
	if err := controller.PumpEvent(ctx); err != nil {
		t.Fatal(err)
	}
	if len(runtime.queries) != before {
		t.Fatalf("closed Review workspace unexpectedly queried: before=%d after=%d", before, len(runtime.queries))
	}
}
