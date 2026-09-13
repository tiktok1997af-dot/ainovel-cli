package desktopui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"

	"github.com/voocel/ainovel-cli/internal/appruntime"
)

func TestReviewWorkspaceProductionSurfaceUsesFrozenAppRuntimeVocabulary(t *testing.T) {
	data, err := os.ReadFile("review_workspace.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, token := range []string{
		"QueryReviewCatalog",
		"QueryReviewStatus",
		"QueryReviewHistory",
		"CommandReviewRun",
		"CommandReviewRepair",
		"CommandReviewRerun",
		"CommandReviewPromoteOfficial",
		"ReviewEventCategory",
		"EventTypeReviewState",
		"EventTypeReviewGate",
		"EventTypeReviewAction",
	} {
		if !strings.Contains(text, token) {
			t.Fatalf("Review workspace missing frozen AppRuntime seam %q", token)
		}
	}
	if got := len(appruntime.ReviewGateCatalog()); got != 12 {
		t.Fatalf("Review gate catalog count=%d, want frozen 12", got)
	}
}

func TestReviewWorkspaceDoesNotEncodeLocalQualityThresholds(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "review_workspace.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	forbiddenCalls := map[string]bool{
		"ReviewGatePass": true,
		"ReviewGateWarn": true,
		"ReviewGateFail": true,
	}
	ast.Inspect(file, func(node ast.Node) bool {
		sel, ok := node.(*ast.SelectorExpr)
		if !ok || sel.Sel == nil {
			return true
		}
		if forbiddenCalls[sel.Sel.Name] {
			t.Errorf("Review UI must not encode local quality threshold/state policy via %s", sel.Sel.Name)
		}
		return true
	})
}
