package desktopui

import (
	"strings"

	"github.com/voocel/ainovel-cli/internal/appruntime"
)

const MaxFolderRenderRows = 512

type FolderViewRenderMode string

const (
	FolderViewRenderTreeFullWidth    FolderViewRenderMode = "tree_full_width"
	FolderViewRenderSplit            FolderViewRenderMode = "split"
	FolderViewRenderStackedFullWidth FolderViewRenderMode = "stacked_full_width"
)

type FolderRenderRow struct {
	Key        string
	Kind       FolderNodeKind
	Label      string
	Depth      int
	DocumentID string
	Selected   bool
}

type FolderDocumentRenderState struct {
	ID          string
	Kind        string
	Path        string
	Title       string
	ContentType string
	Content     string
	ReadOnly    bool
}

type FolderViewRenderLayout struct {
	Mode            FolderViewRenderMode
	Viewport        ViewportClass
	WorkspaceWidth  int
	TreeVisible     bool
	TreeWidth       int
	DocumentVisible bool
	DocumentWidth   int
}

type FolderViewRenderState struct {
	Active             bool
	Load               LoadState
	Layout             FolderViewRenderLayout
	Rows               []FolderRenderRow
	Document           *FolderDocumentRenderState
	Offset             int
	Limit              int
	Total              int
	ProjectedDocuments int
	OmittedUnsafe      int
	RowsTruncated      bool
	Error              *ErrorView
}

func NewFolderViewRenderState() FolderViewRenderState {
	return FolderViewRenderState{Rows: []FolderRenderRow{}}
}

// FolderViewRender projects the already-loaded Folder View state into a
// deterministic renderer model. It performs no query, dispatch, filesystem,
// network, Host, Engine, Store or browser operation.
func (s *ShellState) FolderViewRender() FolderViewRenderState {
	if s == nil || s.Route != RouteProject || s.Project.Tab != ProjectTabFolder {
		return NewFolderViewRenderState()
	}
	return BuildFolderViewRender(s.Project, s.Layout)
}

func BuildFolderViewRender(project ProjectWorkspaceState, shellLayout LayoutState) FolderViewRenderState {
	folder := project.FolderView()
	render := NewFolderViewRenderState()
	render.Active = true
	render.Load = project.Load
	render.Offset = folder.Offset
	render.Limit = folder.Limit
	render.Total = folder.Total
	render.ProjectedDocuments = folder.ProjectedDocuments
	render.OmittedUnsafe = folder.OmittedUnsafe
	render.Error = project.Error

	flattenFolderRenderRows(folder.Roots, 0, &render.Rows, &render.RowsTruncated)
	render.Document = safeFolderDocumentRender(folder, project.SelectedDocument)
	render.Layout = folderViewRenderLayout(shellLayout, render.Document != nil)
	return render
}

func flattenFolderRenderRows(nodes []FolderViewNode, depth int, rows *[]FolderRenderRow, truncated *bool) {
	for _, node := range nodes {
		if len(*rows) >= MaxFolderRenderRows {
			*truncated = true
			return
		}
		label := strings.TrimSpace(node.Name)
		if node.Kind == FolderNodeDocument {
			if title := strings.TrimSpace(node.Title); title != "" {
				label = title
			}
		}
		*rows = append(*rows, FolderRenderRow{
			Key:        node.Key,
			Kind:       node.Kind,
			Label:      label,
			Depth:      depth,
			DocumentID: node.DocumentID,
			Selected:   node.Selected,
		})
		if len(node.Children) > 0 {
			flattenFolderRenderRows(node.Children, depth+1, rows, truncated)
			if *truncated {
				return
			}
		}
	}
}

func safeFolderDocumentRender(folder FolderViewState, selected *appruntime.DocumentsGetResultDTO) *FolderDocumentRenderState {
	if selected == nil {
		return nil
	}
	id := strings.TrimSpace(selected.Document.ID)
	if id == "" || id != folder.SelectedDocumentID || !selected.Document.ReadOnly {
		return nil
	}
	node, ok := uniqueFolderDocument(folder.Roots, id)
	if !ok || !node.Selected {
		return nil
	}
	segments, safe := safeLogicalDocumentSegments(selected.Document.Path)
	if !safe || strings.Join(segments, "/") != node.Path || selected.Document.Kind != node.DocumentKind {
		return nil
	}
	title := strings.TrimSpace(selected.Document.Title)
	if title == "" {
		title = strings.TrimSpace(node.Title)
	}
	if title == "" {
		title = node.Name
	}
	return &FolderDocumentRenderState{
		ID:          id,
		Kind:        selected.Document.Kind,
		Path:        node.Path,
		Title:       title,
		ContentType: selected.Document.ContentType,
		Content:     selected.Content,
		ReadOnly:    true,
	}
}

func folderViewRenderLayout(shell LayoutState, hasDocument bool) FolderViewRenderLayout {
	workspace := shell.Width
	if shell.NavigationVisible {
		workspace -= shell.NavigationWidth
	}
	if shell.InspectorVisible {
		workspace -= shell.InspectorWidth
	}
	if workspace < 0 {
		workspace = 0
	}

	layout := FolderViewRenderLayout{
		Viewport:       shell.Class,
		WorkspaceWidth: workspace,
		TreeVisible:    true,
		TreeWidth:      workspace,
	}
	if !hasDocument {
		layout.Mode = FolderViewRenderTreeFullWidth
		return layout
	}

	layout.DocumentVisible = true
	if shell.Class != ViewportDesktop {
		layout.Mode = FolderViewRenderStackedFullWidth
		layout.DocumentWidth = workspace
		return layout
	}

	layout.Mode = FolderViewRenderSplit
	treeWidth := workspace / 3
	if treeWidth < 220 && workspace >= 440 {
		treeWidth = 220
	}
	if treeWidth > 360 {
		treeWidth = 360
	}
	if treeWidth > workspace {
		treeWidth = workspace
	}
	layout.TreeWidth = treeWidth
	layout.DocumentWidth = workspace - treeWidth
	return layout
}
