package desktopui

import (
	"testing"

	"github.com/voocel/ainovel-cli/internal/appruntime"
)

func TestReviewSupportsRejectsUnadvertisedCommand(t *testing.T) {
	state := NewReviewWorkspaceState()
	state.Catalog = appruntime.CurrentReviewContractCatalog()
	if state.Supports(appruntime.CommandKind("review.unknown")) {
		t.Fatal("unadvertised Review command must not be exposed by UI")
	}
}
