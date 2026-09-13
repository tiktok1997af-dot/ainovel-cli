package desktopui

import (
	"context"
	"testing"

	"github.com/voocel/ainovel-cli/internal/appruntime"
)

func TestBuildFolderViewProjectsDeterministicReadOnlyHierarchy(t *testing.T) {
	dto := appruntime.DocumentsListResultDTO{
		Items: []appruntime.DocumentSummaryDTO{
			{ID: "chapter-2", Kind: appruntime.DocumentKindChapter, Path: "chapters/chapter-002.md", Title: "Two", ContentType: "text/markdown", ReadOnly: true, SizeBytes: 22},
			{ID: "project", Kind: appruntime.DocumentKindProject, Path: "project.json", Title: "Project", ContentType: "application/json", ReadOnly: true, SizeBytes: 11},
			{ID: "chapter-1", Kind: appruntime.DocumentKindChapter, Path: "chapters/chapter-001.md", Title: "One", ContentType: "text/markdown", ReadOnly: true, SizeBytes: 21},
			{ID: "canon", Kind: appruntime.DocumentKindKnowledge, Path: "knowledge/canon.json", Title: "Canon", ContentType: "application/json", ReadOnly: true, SizeBytes: 31},
		},
		Offset: 0,
		Limit:  ProjectWorkspaceQueryLimit,
		Total:  4,
	}

	state := BuildFolderView(dto, "chapter-2")
	if state.ProjectedDocuments != 4 || state.OmittedUnsafe != 0 || state.Total != 4 {
		t.Fatalf("folder projection counters = %+v", state)
	}
	if len(state.Roots) != 3 {
		t.Fatalf("root count = %d, want 3: %+v", len(state.Roots), state.Roots)
	}
	if state.Roots[0].Kind != FolderNodeDirectory || state.Roots[0].Name != "chapters" {
		t.Fatalf("first root = %+v, want chapters folder", state.Roots[0])
	}
	if len(state.Roots[0].Children) != 2 || state.Roots[0].Children[0].DocumentID != "chapter-1" || state.Roots[0].Children[1].DocumentID != "chapter-2" {
		t.Fatalf("chapter children not deterministic: %+v", state.Roots[0].Children)
	}
	if !state.Roots[0].Children[1].Selected {
		t.Fatalf("selected document was not projected: %+v", state.Roots[0].Children[1])
	}
	if state.Roots[1].Kind != FolderNodeDirectory || state.Roots[1].Name != "knowledge" {
		t.Fatalf("second root = %+v, want knowledge folder", state.Roots[1])
	}
	if state.Roots[2].Kind != FolderNodeDocument || state.Roots[2].DocumentID != "project" {
		t.Fatalf("root document projection = %+v", state.Roots[2])
	}
}

func TestBuildFolderViewFailsClosedOnUnsafeCatalogEntries(t *testing.T) {
	dto := appruntime.DocumentsListResultDTO{
		Items: []appruntime.DocumentSummaryDTO{
			{ID: "safe", Path: "docs/safe.md", ReadOnly: true},
			{ID: "absolute", Path: "/etc/passwd", ReadOnly: true},
			{ID: "windows", Path: `C:\\secret\\file.md`, ReadOnly: true},
			{ID: "traversal", Path: "docs/../secret.md", ReadOnly: true},
			{ID: "mutable", Path: "docs/write.md", ReadOnly: false},
			{ID: "", Path: "docs/no-id.md", ReadOnly: true},
		},
		Offset: 100,
		Limit:  ProjectWorkspaceQueryLimit,
		Total:  106,
	}

	state := BuildFolderView(dto, "")
	if state.ProjectedDocuments != 1 || state.OmittedUnsafe != 5 {
		t.Fatalf("unsafe entries were not fail-closed: %+v", state)
	}
	if state.Offset != 100 || state.Limit != ProjectWorkspaceQueryLimit || state.Total != 106 {
		t.Fatalf("paging metadata changed: %+v", state)
	}
	if len(state.Roots) != 1 || state.Roots[0].Name != "docs" || len(state.Roots[0].Children) != 1 || state.Roots[0].Children[0].DocumentID != "safe" {
		t.Fatalf("safe projection = %+v", state.Roots)
	}
}

func TestFolderViewSelectDocumentUsesOpaqueIDOnly(t *testing.T) {
	state := BuildFolderView(appruntime.DocumentsListResultDTO{Items: []appruntime.DocumentSummaryDTO{
		{ID: "one", Path: "chapters/one.md", ReadOnly: true},
		{ID: "two", Path: "chapters/two.md", ReadOnly: true},
	}}, "one")
	state.SelectDocument("two")
	if state.SelectedDocumentID != "two" {
		t.Fatalf("selected id = %q", state.SelectedDocumentID)
	}
	children := state.Roots[0].Children
	if children[0].Selected || !children[1].Selected {
		t.Fatalf("selection projection = %+v", children)
	}
}

func TestLoadDocumentsPageBuildsFolderViewFromAppRuntimeCatalog(t *testing.T) {
	runtime := fullWorkspaceRuntime(t)
	runtime.results[appruntime.QueryDocumentsList] = projectQueryResult(t, appruntime.QueryDocumentsList, appruntime.DocumentsListResultDTO{
		Items: []appruntime.DocumentSummaryDTO{
			{ID: "chapter-1", Kind: appruntime.DocumentKindChapter, Path: "chapters/chapter-001.md", Title: "One", ReadOnly: true},
			{ID: "canon", Kind: appruntime.DocumentKindKnowledge, Path: "knowledge/canon.json", Title: "Canon", ReadOnly: true},
		},
		Offset: 0, Limit: ProjectWorkspaceQueryLimit, Total: 2,
	})
	runtime.results[appruntime.QueryDocumentsGet] = projectQueryResult(t, appruntime.QueryDocumentsGet, appruntime.DocumentsGetResultDTO{
		Document: appruntime.DocumentSummaryDTO{ID: "canon", Kind: appruntime.DocumentKindKnowledge, Path: "knowledge/canon.json", Title: "Canon", ReadOnly: true},
		Content:  "{}",
	})

	controller := NewController(runtime, 1440)
	if err := controller.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := controller.OpenProjectWorkspace(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := controller.LoadDocumentsPage(context.Background(), 0, "", ""); err != nil {
		t.Fatal(err)
	}

	project := controller.Shell().Project
	if project.Tab != ProjectTabDocuments || project.FolderView().ProjectedDocuments != 2 || len(project.FolderView().Roots) != 2 {
		t.Fatalf("folder workspace was not built from documents.list: %+v", project.FolderView())
	}
	if err := controller.LoadDocument(context.Background(), "canon"); err != nil {
		t.Fatal(err)
	}
	if controller.Shell().Project.FolderView().SelectedDocumentID != "canon" {
		t.Fatalf("folder selection did not follow opaque document id: %+v", controller.Shell().Project.FolderView())
	}
	if runtime.dispatchCalls != 0 {
		t.Fatalf("Folder View projection must remain read-only, dispatch calls = %d", runtime.dispatchCalls)
	}
}
