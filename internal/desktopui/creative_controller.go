package desktopui

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/voocel/ainovel-cli/internal/appruntime"
)

var ErrCreativeWorkspaceUnavailable = errors.New("desktopui: creative workspace is unavailable")

func (c *Controller) OpenCreativeEditor(ctx context.Context, chapter int) error {
	if c.runtime == nil {
		return ErrNilRuntime
	}
	if !c.shell.SelectRoute(RouteCreative) {
		return ErrCreativeWorkspaceUnavailable
	}
	if chapter <= 0 && c.shell.HasSnapshot {
		chapter = c.shell.Snapshot.CurrentChapter.Current
	}
	if chapter <= 0 {
		return c.failCreativeValidation()
	}
	return c.loadCreativeChapter(ctx, chapter, false)
}

func (c *Controller) LoadCreativeChapter(ctx context.Context, chapter int) error {
	if chapter <= 0 {
		return c.failCreativeValidation()
	}
	return c.loadCreativeChapter(ctx, chapter, false)
}

func (c *Controller) loadCreativeChapter(ctx context.Context, chapter int, preserveDirty bool) error {
	ticket := creativeQueryTicket{
		ID:               c.nextRequestID(),
		SnapshotRevision: c.shell.SnapshotRevision,
		Route:            RouteCreative,
		Chapter:          chapter,
	}
	c.creative.begin(ticket)

	payload, err := json.Marshal(appruntime.ChaptersGetQuery{Chapter: chapter, IncludeContent: true})
	if err != nil {
		if c.creative.current(ticket, c.shell) {
			c.creative.finish(ticket)
			return c.failCreativeProtocol()
		}
		c.creative.discard(ticket)
		return nil
	}
	result, err := c.runtime.Query(ctx, appruntime.QueryRequest{
		ContractVersion: appruntime.ContractVersion,
		Kind:            appruntime.QueryChaptersGet,
		Payload:         payload,
	})
	if !c.creative.current(ticket, c.shell) {
		c.creative.discard(ticket)
		return nil
	}
	if err != nil {
		c.creative.finish(ticket)
		c.creative.Error = viewErrorFromError(err)
		c.creative.Load = loadStateForWriteError(c.creative.Error)
		return err
	}
	if result.Error != nil {
		c.creative.finish(ticket)
		c.creative.Error = viewErrorFromAppError(result.Error)
		c.creative.Load = loadStateForWriteError(c.creative.Error)
		return result.Error
	}
	if result.ContractVersion != appruntime.ContractVersion || result.Kind != appruntime.QueryChaptersGet ||
		len(result.Data) == 0 || !json.Valid(result.Data) {
		c.creative.finish(ticket)
		return c.failCreativeProtocol()
	}
	var dto appruntime.ChaptersGetResultDTO
	if err := json.Unmarshal(result.Data, &dto); err != nil || dto.Chapter != chapter {
		c.creative.finish(ticket)
		return c.failCreativeProtocol()
	}
	c.creative.applyCanonical(dto, preserveDirty)
	c.creative.finish(ticket)
	return nil
}

func (c *Controller) SaveCreativePlan(ctx context.Context) (appruntime.CommandResult, error) {
	state := &c.creative
	if !state.canSavePlan() {
		return appruntime.CommandResult{}, c.failCreativeValidation()
	}
	result, err := c.SaveChapterPlan(ctx, state.Plan.Value)
	if err == nil {
		state.Plan.Dirty = false
	}
	return result, err
}

func (c *Controller) SaveCreativeDraft(ctx context.Context) (appruntime.CommandResult, error) {
	state := &c.creative
	if state.Chapter <= 0 || !state.Draft.Dirty || state.Draft.Content == "" {
		return appruntime.CommandResult{}, c.failCreativeValidation()
	}
	payload := appruntime.ChapterTextSavePayload{
		Chapter:                state.Chapter,
		Content:                state.Draft.Content,
		ExpectedSHA256:         state.Draft.BaselineSHA256,
		ExpectedRecordRevision: state.expectedRecordRevision(),
	}
	result, err := c.SaveChapterDraft(ctx, payload)
	if err == nil {
		var dto appruntime.ChapterTextSaveResultDTO
		if json.Unmarshal(result.Data, &dto) == nil {
			state.Draft.BaselineSHA256 = dto.ContentSHA256
		}
		state.Draft.Dirty = false
	}
	return result, err
}

func (c *Controller) SaveCreativeWorkspace(ctx context.Context) (appruntime.CommandResult, error) {
	state := &c.creative
	if state.Chapter <= 0 || !state.Workspace.Dirty || state.Workspace.Content == "" {
		return appruntime.CommandResult{}, c.failCreativeValidation()
	}
	payload := appruntime.ChapterTextSavePayload{
		Chapter:                state.Chapter,
		Content:                state.Workspace.Content,
		ExpectedSHA256:         state.Workspace.BaselineSHA256,
		ExpectedRecordRevision: state.expectedRecordRevision(),
	}
	result, err := c.SaveChapterWorkspace(ctx, payload)
	if err == nil {
		var dto appruntime.ChapterTextSaveResultDTO
		if json.Unmarshal(result.Data, &dto) == nil {
			state.Workspace.BaselineSHA256 = dto.ContentSHA256
		}
		state.Workspace.Dirty = false
	}
	return result, err
}

func (c *Controller) reconcileCreativeChapter(ctx context.Context, chapter int, preserveDirty bool) error {
	refreshErr := c.Refresh(ctx)
	if c.shell.Route != RouteCreative || c.creative.Selected != chapter {
		return refreshErr
	}
	queryErr := c.loadCreativeChapter(ctx, chapter, preserveDirty)
	return errors.Join(refreshErr, queryErr)
}

func (c *Controller) failCreativeValidation() error {
	err := &appruntime.AppError{
		Code:     appruntime.ErrorCodeInvalidArgument,
		Category: appruntime.ErrorCategoryValidation,
		Message:  "The request is invalid.",
	}
	c.creative.Error = viewErrorFromAppError(err)
	c.creative.Load = LoadValidationError
	return err
}

func (c *Controller) failCreativeProtocol() error {
	err := &appruntime.AppError{
		Code:     appruntime.ErrorCodeInternal,
		Category: appruntime.ErrorCategoryInternal,
		Message:  "An internal runtime error occurred.",
	}
	c.creative.Error = viewErrorFromAppError(err)
	c.creative.Load = LoadRuntimeError
	return err
}
