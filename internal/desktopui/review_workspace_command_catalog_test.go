package desktopui

import (
	"testing"

	"github.com/voocel/ainovel-cli/internal/appruntime"
)

func TestReviewWorkspaceConsumesExactlyFrozenFourCommands(t *testing.T) {
	catalog := appruntime.CurrentReviewContractCatalog()
	want := []appruntime.CommandKind{
		appruntime.CommandReviewRun,
		appruntime.CommandReviewRepair,
		appruntime.CommandReviewRerun,
		appruntime.CommandReviewPromoteOfficial,
	}
	if len(catalog.CommandKinds) != len(want) {
		t.Fatalf("command count=%d, want=%d", len(catalog.CommandKinds), len(want))
	}
	for i := range want {
		if catalog.CommandKinds[i] != want[i] {
			t.Fatalf("command[%d]=%q want=%q", i, catalog.CommandKinds[i], want[i])
		}
	}
}
