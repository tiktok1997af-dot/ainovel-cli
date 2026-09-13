package desktopui

import (
	"context"
	"testing"
)

func TestReviewWorkspaceKeepsHistoryAsReadOnlyProjection(t *testing.T) {
	runtime := newReviewWorkspaceRuntime()
	controller := NewController(runtime, 1440)
	if err := controller.OpenReview(context.Background(), runtime.status.Target); err != nil {
		t.Fatal(err)
	}
	state := controller.Review()
	if state.HistoryTotal != 1 || len(state.History) != 1 || state.History[0].ReviewID != "review-3" {
		t.Fatalf("unexpected history projection: %+v", state.History)
	}
}
