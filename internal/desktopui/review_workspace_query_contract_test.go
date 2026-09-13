package desktopui

import (
	"context"
	"testing"

	"github.com/voocel/ainovel-cli/internal/appruntime"
)

func TestReviewWorkspaceQueriesUseDesktopContractVersion(t *testing.T) {
	runtime := newReviewWorkspaceRuntime()
	controller := NewController(runtime, 1440)
	if err := controller.OpenReview(context.Background(), runtime.status.Target); err != nil {
		t.Fatal(err)
	}
	if controller.Review().Load != LoadReady {
		t.Fatalf("load=%q, want ready", controller.Review().Load)
	}
	if controller.Review().Catalog.QueryKinds[0] != appruntime.QueryReviewCatalog {
		t.Fatalf("unexpected Review query catalog: %+v", controller.Review().Catalog.QueryKinds)
	}
}
