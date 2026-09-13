package desktopui

import (
	"context"
	"testing"

	"github.com/voocel/ainovel-cli/internal/appruntime"
)

func TestReviewTargetSwitchReplacesPresentationProjection(t *testing.T) {
	runtime := newReviewWorkspaceRuntime()
	controller := NewController(runtime, 1440)
	ctx := context.Background()
	first := appruntime.ReviewTargetDTO{Scope: appruntime.ReviewScopeChapter, Chapter: 3}
	second := appruntime.ReviewTargetDTO{Scope: appruntime.ReviewScopeChapter, Chapter: 9}
	if err := controller.OpenReview(ctx, first); err != nil {
		t.Fatal(err)
	}
	controller.Review().LastCommand = &appruntime.ReviewCommandResultDTO{Target: first, Action: string(appruntime.CommandReviewRun)}
	if err := controller.OpenReview(ctx, second); err != nil {
		t.Fatal(err)
	}
	if controller.Review().Target != second || controller.Review().Status.Target != second {
		t.Fatalf("target switch did not reconcile: %+v", controller.Review())
	}
	if controller.Review().LastCommand != nil || controller.Review().PromotionReceipt != nil {
		t.Fatal("transient Review action state leaked across target switch")
	}
}
