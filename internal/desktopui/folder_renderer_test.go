package desktopui

import (
	"testing"

	"github.com/voocel/ainovel-cli/internal/appruntime"
)

func folderRendererShell(width int, selected bool) *ShellState {
	shell := NewShell(width)
	shell.Route = RouteProject
	shell.Project.Tab = ProjectTabFolder
	shell.Project.Load = LoadReady
	shell.Project.Documents = appruntime.DocumentsListResultDTO{
		Items: []appruntime.DocumentSummaryDTO{
			{ID: "chapter-1", Kind: appruntime.DocumentKindChapter, Path: "chapters/chapter-001.md", Title: "Chapter One", ContentType: "text/markdown", ReadOnly: true},
			{ID: "canon", Kind: appruntime.DocumentKindKnowledge, Path: "knowledge/canon.json", Title: "Canon", ContentType: "application/json", ReadOnly: true},
		},
		Offset: 0,
		Limit:  ProjectWorkspaceQueryLimit,
		Total:  2,
	}
	if selected {
		shell.Project.SelectedDocumentID = "canon"
		shell.Project.SelectedDocument = &appruntime.DocumentsGetResultDTO{
			Document: appruntime.DocumentSummaryDTO{ID: "canon", Kind: appruntime.DocumentKindKnowledge, Path: "knowledge/canon.json", Title: "Canon", ContentType: "application/json", ReadOnly: true},
			Content:  "{\"locked\":true}",
		}
	}
	return shell
}

func TestFolderViewRendererUsesExistingDesktopLayoutForSplitView(t *testing.T) {
	shell := folderRendererShell(1440, true)
	render := shell.FolderViewRender()
	if !render.Active || render.Layout.Mode != FolderViewRenderSplit {
		t.Fatalf("desktop render = %+v", render)
	}
	wantWorkspace := 1440 - 256 - 360
	if render.Layout.Viewport != ViewportDesktop || render.Layout.WorkspaceWidth != wantWorkspace {
		t.Fatalf("desktop layout = %+v, want workspace %d", render.Layout, wantWorkspace)
	}
	if !render.Layout.TreeVisible || !render.Layout.DocumentVisible || render.Layout.TreeWidth+render.Layout.DocumentWidth != wantWorkspace {
		t.Fatalf("desktop split widths = %+v", render.Layout)
	}
	if render.Document == nil || render.Document.ID != "canon" || render.Document.Content != "{\"locked\":true}" || !render.Document.ReadOnly {
		t.Fatalf("selected document render = %+v", render.Document)
	}
	if len(render.Rows) != 4 || render.Rows[0].Kind != FolderNodeDirectory || render.Rows[1].Depth != 1 {
		t.Fatalf("folder rows = %+v", render.Rows)
	}
}

func TestFolderViewRendererUsesStackedFullWidthOnTabletAndMobile(t *testing.T) {
	for _, width := range []int{900, 600} {
		shell := folderRendererShell(width, true)
		render := shell.FolderViewRender()
		if render.Layout.Mode != FolderViewRenderStackedFullWidth {
			t.Fatalf("width %d mode = %q", width, render.Layout.Mode)
		}
		if render.Layout.TreeWidth != render.Layout.WorkspaceWidth || render.Layout.DocumentWidth != render.Layout.WorkspaceWidth {
			t.Fatalf("width %d full-width layout = %+v", width, render.Layout)
		}
		if width == 900 && render.Layout.Viewport != ViewportTablet {
			t.Fatalf("width %d viewport = %q", width, render.Layout.Viewport)
		}
		if width == 600 && render.Layout.Viewport != ViewportMobile {
			t.Fatalf("width %d viewport = %q", width, render.Layout.Viewport)
		}
	}
}

func TestFolderViewRendererUsesTreeFullWidthWithoutSelection(t *testing.T) {
	shell := folderRendererShell(1440, false)
	render := shell.FolderViewRender()
	if render.Layout.Mode != FolderViewRenderTreeFullWidth || !render.Layout.TreeVisible || render.Layout.DocumentVisible {
		t.Fatalf("tree-only render = %+v", render.Layout)
	}
	if render.Layout.TreeWidth != render.Layout.WorkspaceWidth || render.Document != nil {
		t.Fatalf("tree-only widths/document = %+v / %+v", render.Layout, render.Document)
	}
}

func TestFolderViewRendererFailsClosedOnSelectedDocumentMetadataDrift(t *testing.T) {
	shell := folderRendererShell(1440, true)
	shell.Project.SelectedDocument.Document.Path = "knowledge/other.json"
	render := shell.FolderViewRender()
	if render.Document != nil {
		t.Fatalf("unsafe selected document rendered = %+v", render.Document)
	}
	if render.Layout.Mode != FolderViewRenderTreeFullWidth || render.Layout.DocumentVisible {
		t.Fatalf("metadata drift layout = %+v", render.Layout)
	}
}

func TestFolderViewRendererIsInactiveOutsideProjectFolderTab(t *testing.T) {
	shell := folderRendererShell(1440, true)
	shell.Project.Tab = ProjectTabDocuments
	render := shell.FolderViewRender()
	if render.Active || len(render.Rows) != 0 || render.Document != nil {
		t.Fatalf("inactive render leaked Folder View state = %+v", render)
	}
}

func TestFolderViewRendererCapsRowsDeterministically(t *testing.T) {
	shell := NewShell(1440)
	shell.Route = RouteProject
	shell.Project.Tab = ProjectTabFolder
	items := make([]appruntime.DocumentSummaryDTO, 0, ProjectWorkspaceQueryLimit)
	for i := 0; i < ProjectWorkspaceQueryLimit; i++ {
		id := "doc-" + threeDigits(i)
		items = append(items, appruntime.DocumentSummaryDTO{
			ID: id, Kind: appruntime.DocumentKindKnowledge, Path: "knowledge/" + id + "/a/b/c/d/e/" + id + ".json", ReadOnly: true,
		})
	}
	shell.Project.Documents = appruntime.DocumentsListResultDTO{Items: items, Limit: ProjectWorkspaceQueryLimit, Total: len(items)}
	render := shell.FolderViewRender()
	if len(render.Rows) != MaxFolderRenderRows || !render.RowsTruncated {
		t.Fatalf("bounded render rows = %d truncated=%v, want %d/true", len(render.Rows), render.RowsTruncated, MaxFolderRenderRows)
	}
}

func threeDigits(value int) string {
	return string([]byte{'0' + byte((value/100)%10), '0' + byte((value/10)%10), '0' + byte(value%10)})
}
