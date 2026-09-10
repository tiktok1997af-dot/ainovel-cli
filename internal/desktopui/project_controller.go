package desktopui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/voocel/ainovel-cli/internal/appruntime"
)

var ErrProjectWorkspaceUnavailable = errors.New("desktopui: project workspace is unavailable")

func (c *Controller) OpenProjectWorkspace(ctx context.Context) error {
	if c.runtime == nil {
		return ErrNilRuntime
	}
	if !c.shell.SelectRoute(RouteProject) {
		return ErrProjectWorkspaceUnavailable
	}
	c.shell.Project.Error = nil

	if err := c.LoadProjectOverview(ctx); err != nil {
		return err
	}
	if err := c.LoadChapterPage(ctx, 0); err != nil {
		return err
	}
	if err := c.LoadOutlineWindow(ctx, 1); err != nil {
		return err
	}
	return nil
}

func (c *Controller) LoadProjectOverview(ctx context.Context) error {
	ticket := c.beginProjectQuery(appruntime.QueryProjectOverview, 0)
	var dto appruntime.ProjectOverviewResultDTO
	accepted, err := c.executeProjectQuery(ctx, ticket, appruntime.ProjectOverviewQuery{}, &dto)
	if !accepted || err != nil {
		return err
	}
	c.shell.Project.Overview = &dto
	c.shell.Project.finish(ticket)
	return nil
}

func (c *Controller) LoadChapterPage(ctx context.Context, offset int) error {
	if offset < 0 {
		return c.failLocalProjectValidation("chapter offset cannot be negative")
	}
	ticket := c.beginProjectQuery(appruntime.QueryChaptersList, 0)
	var dto appruntime.ChaptersListResultDTO
	payload := appruntime.ChaptersListQuery{
		PageQuery: appruntime.PageQuery{Offset: offset, Limit: ProjectWorkspaceQueryLimit},
	}
	accepted, err := c.executeProjectQuery(ctx, ticket, payload, &dto)
	if !accepted || err != nil {
		return err
	}
	c.shell.Project.Chapters = dto
	c.shell.Project.finish(ticket)
	return nil
}

func (c *Controller) LoadChapter(ctx context.Context, chapter int) error {
	if chapter <= 0 {
		return c.failLocalProjectValidation("chapter must be greater than zero")
	}
	c.shell.Project.Selected = chapter
	c.shell.Project.SelectTab(ProjectTabChapters)
	ticket := c.beginProjectQuery(appruntime.QueryChaptersGet, chapter)
	var dto appruntime.ChaptersGetResultDTO
	payload := appruntime.ChaptersGetQuery{Chapter: chapter, IncludeContent: false}
	accepted, err := c.executeProjectQuery(ctx, ticket, payload, &dto)
	if !accepted || err != nil {
		return err
	}
	if dto.Chapter != chapter {
		c.shell.Project.fail(ticket, c.shell, queryProtocolViewError())
		return errors.New("desktopui: chapter query returned unexpected chapter")
	}
	if c.shell.Project.Selected != chapter {
		c.shell.Project.discard(ticket)
		return nil
	}
	c.shell.Project.SelectedChapter = &dto
	c.shell.Project.finish(ticket)
	return nil
}

func (c *Controller) LoadOutlineWindow(ctx context.Context, fromChapter int) error {
	if fromChapter < 0 {
		return c.failLocalProjectValidation("outline start cannot be negative")
	}
	if fromChapter == 0 {
		fromChapter = 1
	}
	ticket := c.beginProjectQuery(appruntime.QueryOutlineGet, 0)
	var dto appruntime.OutlineGetResultDTO
	payload := appruntime.OutlineGetQuery{FromChapter: fromChapter, Limit: ProjectWorkspaceQueryLimit}
	accepted, err := c.executeProjectQuery(ctx, ticket, payload, &dto)
	if !accepted || err != nil {
		return err
	}
	c.shell.Project.Outline = &dto
	c.shell.Project.finish(ticket)
	return nil
}

// LoadDocumentsPage exposes only the supported AppRuntime document catalog.
// It is a bounded read surface, not Folder View and not a filesystem browser.
func (c *Controller) LoadDocumentsPage(ctx context.Context, offset int, kind, prefix string) error {
	if offset < 0 {
		return c.failLocalProjectValidation("document offset cannot be negative")
	}
	kind = strings.TrimSpace(kind)
	prefix = strings.TrimSpace(prefix)
	c.shell.Project.SelectTab(ProjectTabDocuments)
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

// LoadDocument reads one catalog-backed read-only document by opaque ID. The
// UI never resolves or opens a raw filesystem path from document metadata.
func (c *Controller) LoadDocument(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return c.failLocalProjectValidation("document id is required")
	}
	c.shell.Project.SelectedDocumentID = id
	c.shell.Project.SelectTab(ProjectTabDocuments)
	ticket := c.beginProjectQuery(appruntime.QueryDocumentsGet, 0)
	var dto appruntime.DocumentsGetResultDTO
	accepted, err := c.executeProjectQuery(ctx, ticket, appruntime.DocumentsGetQuery{ID: id}, &dto)
	if !accepted || err != nil {
		return err
	}
	if dto.Document.ID != id || !dto.Document.ReadOnly {
		c.shell.Project.fail(ticket, c.shell, queryProtocolViewError())
		return errors.New("desktopui: document query returned unsafe or unexpected document")
	}
	if c.shell.Project.SelectedDocumentID != id {
		c.shell.Project.discard(ticket)
		return nil
	}
	c.shell.Project.SelectedDocument = &dto
	c.shell.Project.finish(ticket)
	return nil
}

func (c *Controller) beginProjectQuery(kind appruntime.QueryKind, chapter int) workspaceQueryTicket {
	ticket := workspaceQueryTicket{
		ID:               c.nextRequestID(),
		Kind:             kind,
		SnapshotRevision: c.shell.SnapshotRevision,
		Route:            RouteProject,
		Chapter:          chapter,
	}
	c.shell.Project.begin(ticket)
	return ticket
}

func (c *Controller) executeProjectQuery(
	ctx context.Context,
	ticket workspaceQueryTicket,
	payload any,
	dst any,
) (bool, error) {
	if c.runtime == nil {
		return false, ErrNilRuntime
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		c.shell.Project.fail(ticket, c.shell, queryProtocolViewError())
		return false, err
	}
	result, err := c.runtime.Query(ctx, appruntime.QueryRequest{
		ContractVersion: appruntime.ContractVersion,
		Kind:            ticket.Kind,
		Payload:         raw,
	})
	if !c.shell.Project.current(ticket, c.shell) {
		c.shell.Project.discard(ticket)
		return false, nil
	}
	if err != nil {
		c.shell.Project.fail(ticket, c.shell, viewErrorFromError(err))
		return false, err
	}
	if result.Error != nil {
		c.shell.Project.fail(ticket, c.shell, viewErrorFromAppError(result.Error))
		return false, result.Error
	}
	if result.ContractVersion != appruntime.ContractVersion || result.Kind != ticket.Kind {
		c.shell.Project.fail(ticket, c.shell, queryProtocolViewError())
		return false, errors.New("desktopui: query result contract mismatch")
	}
	if len(result.Data) == 0 || string(result.Data) == "null" || !json.Valid(result.Data) {
		c.shell.Project.fail(ticket, c.shell, queryProtocolViewError())
		return false, errors.New("desktopui: query result data is invalid")
	}
	if err := json.Unmarshal(result.Data, dst); err != nil {
		c.shell.Project.fail(ticket, c.shell, queryProtocolViewError())
		return false, fmt.Errorf("desktopui: decode query result: %w", err)
	}
	return true, nil
}

func (c *Controller) failLocalProjectValidation(_ string) error {
	viewErr := &ErrorView{
		Code:     string(appruntime.ErrorCodeInvalidArgument),
		Category: string(appruntime.ErrorCategoryValidation),
		Message:  "The request is invalid.",
	}
	c.shell.Project.Error = viewErr
	c.shell.Project.Load = LoadValidationError
	return &appruntime.AppError{
		Code:     appruntime.ErrorCodeInvalidArgument,
		Category: appruntime.ErrorCategoryValidation,
		Message:  "The request is invalid.",
	}
}
