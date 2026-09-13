package desktopui

import (
	"context"
	"errors"
	"strings"

	"github.com/voocel/ainovel-cli/internal/appruntime"
)

var ErrFolderViewUnavailable = errors.New("desktopui: folder view is unavailable")

// OpenFolderView loads one bounded AppRuntime document-catalog page and keeps
// Folder View inside the existing Project workspace. It never enumerates the
// local filesystem directly.
func (c *Controller) OpenFolderView(ctx context.Context, offset int, kind, prefix string) error {
	if c.runtime == nil {
		return ErrNilRuntime
	}
	if offset < 0 {
		return c.failLocalProjectValidation("document offset cannot be negative")
	}
	if !c.shell.SelectRoute(RouteProject) {
		return ErrFolderViewUnavailable
	}
	kind = strings.TrimSpace(kind)
	prefix = strings.TrimSpace(prefix)
	c.shell.Project.SelectTab(ProjectTabFolder)

	ticket := c.beginProjectQuery(appruntime.QueryDocumentsList, 0)
	var dto appruntime.DocumentsListResultDTO
	payload := appruntime.DocumentsListQuery{
		PageQuery: appruntime.PageQuery{Offset: offset, Limit: ProjectWorkspaceQueryLimit},
		Kind:      kind,
		Prefix:    prefix,
	}
	accepted, err := c.executeProjectQuery(ctx, ticket, payload, &dto)
	if !accepted || err != nil {
		return err
	}
	c.shell.Project.Documents = dto
	c.shell.Project.DocumentKind = kind
	c.shell.Project.DocumentPrefix = prefix
	c.shell.Project.finish(ticket)
	return nil
}

// SelectFolderDocument accepts only an opaque ID that is represented exactly
// once in the current safe Folder View projection. The logical path is checked
// for catalog continuity only; it is never used as a read selector.
func (c *Controller) SelectFolderDocument(ctx context.Context, id string) error {
	if c.runtime == nil {
		return ErrNilRuntime
	}
	if c.shell.Route != RouteProject || c.shell.Project.Tab != ProjectTabFolder {
		return ErrFolderViewUnavailable
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return c.failLocalProjectValidation("document id is required")
	}

	node, ok := uniqueFolderDocument(c.shell.Project.FolderView().Roots, id)
	if !ok {
		return c.failLocalProjectValidation("document id is not present in the current Folder View")
	}

	c.shell.Project.SelectedDocumentID = id
	ticket := c.beginProjectQuery(appruntime.QueryDocumentsGet, 0)
	var dto appruntime.DocumentsGetResultDTO
	accepted, err := c.executeProjectQuery(ctx, ticket, appruntime.DocumentsGetQuery{ID: id}, &dto)
	if !accepted || err != nil {
		return err
	}

	segments, safe := safeLogicalDocumentSegments(dto.Document.Path)
	logicalPath := ""
	if safe {
		logicalPath = strings.Join(segments, "/")
	}
	if dto.Document.ID != id || !dto.Document.ReadOnly || !safe || logicalPath != node.Path || dto.Document.Kind != node.DocumentKind {
		c.shell.Project.fail(ticket, c.shell, queryProtocolViewError())
		return errors.New("desktopui: folder document query returned unsafe or mismatched catalog metadata")
	}
	if c.shell.Project.SelectedDocumentID != id {
		c.shell.Project.discard(ticket)
		return nil
	}
	c.shell.Project.SelectedDocument = &dto
	c.shell.Project.finish(ticket)
	return nil
}

func uniqueFolderDocument(nodes []FolderViewNode, id string) (FolderViewNode, bool) {
	var match FolderViewNode
	count := 0
	var visit func([]FolderViewNode)
	visit = func(items []FolderViewNode) {
		for _, node := range items {
			if node.Kind == FolderNodeDocument && node.DocumentID == id {
				match = node
				count++
			}
			if len(node.Children) > 0 {
				visit(node.Children)
			}
		}
	}
	visit(nodes)
	return match, count == 1
}
