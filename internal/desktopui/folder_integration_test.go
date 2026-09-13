package desktopui

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/voocel/ainovel-cli/internal/appruntime"
)

func folderIntegrationRuntime(t *testing.T) *workspaceRuntime {
	t.Helper()
	runtime := fullWorkspaceRuntime(t)
	runtime.results[appruntime.QueryDocumentsList] = projectQueryResult(t, appruntime.QueryDocumentsList, appruntime.DocumentsListResultDTO{
		Items: []appruntime.DocumentSummaryDTO{
			{ID: "chapter-1", Kind: appruntime.DocumentKindChapter, Path: "chapters/chapter-001.md", Title: "One", ContentType: "text/markdown", ReadOnly: true},
			{ID: "canon", Kind: appruntime.DocumentKindKnowledge, Path: "knowledge/canon.json", Title: "Canon", ContentType: "application/json", ReadOnly: true},
		},
		Offset: 0,
		Limit:  ProjectWorkspaceQueryLimit,
		Total:  2,
	})
	runtime.results[appruntime.QueryDocumentsGet] = projectQueryResult(t, appruntime.QueryDocumentsGet, appruntime.DocumentsGetResultDTO{
		Document: appruntime.DocumentSummaryDTO{ID: "canon", Kind: appruntime.DocumentKindKnowledge, Path: "knowledge/canon.json", Title: "Canon", ContentType: "application/json", ReadOnly: true},
		Content:  "{}",
	})
	return runtime
}

func TestOpenFolderViewIntegratesDocumentsListIntoProjectTree(t *testing.T) {
	runtime := folderIntegrationRuntime(t)
	controller := NewController(runtime, 1440)
	if err := controller.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := controller.OpenFolderView(context.Background(), 0, " knowledge ", " knowledge/ "); err != nil {
		t.Fatal(err)
	}

	if len(runtime.queryCalls) != 1 || runtime.queryCalls[0].Kind != appruntime.QueryDocumentsList {
		t.Fatalf("folder list query calls = %+v", runtime.queryCalls)
	}
	var req appruntime.DocumentsListQuery
	if err := json.Unmarshal(runtime.queryCalls[0].Payload, &req); err != nil {
		t.Fatal(err)
	}
	if req.Offset != 0 || req.Limit != ProjectWorkspaceQueryLimit || req.Kind != "knowledge" || req.Prefix != "knowledge/" {
		t.Fatalf("folder list payload = %+v", req)
	}

	project := controller.Shell().Project
	if controller.Shell().Route != RouteProject || project.Tab != ProjectTabFolder {
		t.Fatalf("folder view route/tab = %q/%q", controller.Shell().Route, project.Tab)
	}
	folder := project.FolderView()
	if folder.ProjectedDocuments != 2 || len(folder.Roots) != 2 {
		t.Fatalf("folder projection = %+v", folder)
	}
	if project.DocumentKind != "knowledge" || project.DocumentPrefix != "knowledge/" {
		t.Fatalf("folder filter state = kind:%q prefix:%q", project.DocumentKind, project.DocumentPrefix)
	}
	if runtime.dispatchCalls != 0 {
		t.Fatalf("Folder View must remain read-only, dispatch calls = %d", runtime.dispatchCalls)
	}
}

func TestSelectFolderDocumentUsesOpaqueIDWithDocumentsGet(t *testing.T) {
	runtime := folderIntegrationRuntime(t)
	controller := NewController(runtime, 1440)
	if err := controller.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := controller.OpenFolderView(context.Background(), 0, "", ""); err != nil {
		t.Fatal(err)
	}
	if err := controller.SelectFolderDocument(context.Background(), "canon"); err != nil {
		t.Fatal(err)
	}

	if len(runtime.queryCalls) != 2 || runtime.queryCalls[1].Kind != appruntime.QueryDocumentsGet {
		t.Fatalf("folder selection queries = %+v", runtime.queryCalls)
	}
	var req appruntime.DocumentsGetQuery
	if err := json.Unmarshal(runtime.queryCalls[1].Payload, &req); err != nil {
		t.Fatal(err)
	}
	if req.ID != "canon" {
		t.Fatalf("documents.get selector = %+v", req)
	}
	project := controller.Shell().Project
	if project.Tab != ProjectTabFolder || project.SelectedDocumentID != "canon" || project.SelectedDocument == nil || project.SelectedDocument.Content != "{}" {
		t.Fatalf("selected folder document = %+v", project)
	}
	if runtime.dispatchCalls != 0 {
		t.Fatalf("Folder View selection must not dispatch, dispatch calls = %d", runtime.dispatchCalls)
	}
}

func TestSelectFolderDocumentRejectsPathOrUnknownIDBeforeGet(t *testing.T) {
	runtime := folderIntegrationRuntime(t)
	controller := NewController(runtime, 1440)
	if err := controller.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := controller.OpenFolderView(context.Background(), 0, "", ""); err != nil {
		t.Fatal(err)
	}
	before := len(runtime.queryCalls)
	if err := controller.SelectFolderDocument(context.Background(), "knowledge/canon.json"); err == nil {
		t.Fatal("path-shaped selector error = nil")
	}
	if len(runtime.queryCalls) != before {
		t.Fatalf("path selector reached documents.get: before=%d after=%d", before, len(runtime.queryCalls))
	}
	if controller.Shell().Project.Load != LoadValidationError {
		t.Fatalf("load = %q, want validation_error", controller.Shell().Project.Load)
	}
}

func TestSelectFolderDocumentFailsClosedOnGetMetadataMismatch(t *testing.T) {
	runtime := folderIntegrationRuntime(t)
	runtime.results[appruntime.QueryDocumentsGet] = projectQueryResult(t, appruntime.QueryDocumentsGet, appruntime.DocumentsGetResultDTO{
		Document: appruntime.DocumentSummaryDTO{ID: "canon", Kind: appruntime.DocumentKindKnowledge, Path: "knowledge/other.json", ReadOnly: true},
		Content:  "{}",
	})
	controller := NewController(runtime, 1440)
	if err := controller.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := controller.OpenFolderView(context.Background(), 0, "", ""); err != nil {
		t.Fatal(err)
	}
	if err := controller.SelectFolderDocument(context.Background(), "canon"); err == nil {
		t.Fatal("catalog metadata mismatch error = nil")
	}
	project := controller.Shell().Project
	if project.Error == nil || project.Error.Code != string(appruntime.ErrorCodeInternal) || project.SelectedDocument != nil {
		t.Fatalf("mismatched documents.get did not fail closed: %+v", project)
	}
}

func TestSelectFolderDocumentRejectsAmbiguousCatalogID(t *testing.T) {
	runtime := folderIntegrationRuntime(t)
	runtime.results[appruntime.QueryDocumentsList] = projectQueryResult(t, appruntime.QueryDocumentsList, appruntime.DocumentsListResultDTO{
		Items: []appruntime.DocumentSummaryDTO{
			{ID: "dup", Kind: appruntime.DocumentKindChapter, Path: "chapters/a.md", ReadOnly: true},
			{ID: "dup", Kind: appruntime.DocumentKindKnowledge, Path: "knowledge/b.json", ReadOnly: true},
		},
		Offset: 0, Limit: ProjectWorkspaceQueryLimit, Total: 2,
	})
	controller := NewController(runtime, 1440)
	if err := controller.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := controller.OpenFolderView(context.Background(), 0, "", ""); err != nil {
		t.Fatal(err)
	}
	before := len(runtime.queryCalls)
	if err := controller.SelectFolderDocument(context.Background(), "dup"); err == nil {
		t.Fatal("ambiguous document id error = nil")
	}
	if len(runtime.queryCalls) != before {
		t.Fatalf("ambiguous id reached documents.get: before=%d after=%d", before, len(runtime.queryCalls))
	}
}
