package desktopui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/voocel/ainovel-cli/internal/appruntime"
)

var ErrKnowledgeWorkspaceUnavailable = errors.New("desktopui: knowledge workspace is unavailable")

func (c *Controller) OpenKnowledgeStudio(ctx context.Context) error {
	if c.runtime == nil {
		return ErrNilRuntime
	}
	if !c.shell.SelectRoute(RouteKnowledge) {
		return ErrKnowledgeWorkspaceUnavailable
	}
	c.shell.Knowledge.Error = nil
	chapter := c.shell.Header.ChapterCurrent
	if chapter < 0 {
		chapter = 0
	}
	return c.LoadKnowledgeContext(ctx, chapter, "overview")
}

func (c *Controller) LoadKnowledgeContext(ctx context.Context, chapter int, scope string) error {
	if chapter < 0 || !validContextScope(scope) {
		return c.failLocalKnowledgeValidation()
	}
	ticket := c.beginKnowledgeQuery(appruntime.QueryKnowledgeContext, KnowledgeTabContext)
	var dto appruntime.KnowledgeContextResultDTO
	payload := appruntime.KnowledgeContextQuery{Chapter: chapter, Scope: scope, MaxItems: KnowledgeWorkspaceQueryLimit}
	accepted, err := c.executeKnowledgeQuery(ctx, ticket, payload, &dto)
	if !accepted || err != nil {
		return err
	}
	c.shell.Knowledge.Context = &dto
	c.shell.Knowledge.finish(ticket)
	return nil
}

func (c *Controller) LoadKnowledgeCanon(ctx context.Context, offset, chapter int, scope string) error {
	if offset < 0 || chapter < 0 || !validCanonScope(scope) {
		return c.failLocalKnowledgeValidation()
	}
	ticket := c.beginKnowledgeQuery(appruntime.QueryKnowledgeCanon, KnowledgeTabCanon)
	var dto appruntime.KnowledgeCanonResultDTO
	payload := appruntime.KnowledgeCanonQuery{
		PageQuery: appruntime.PageQuery{Offset: offset, Limit: KnowledgeWorkspaceQueryLimit},
		Chapter:   chapter,
		Scope:     scope,
	}
	accepted, err := c.executeKnowledgeQuery(ctx, ticket, payload, &dto)
	if !accepted || err != nil {
		return err
	}
	c.shell.Knowledge.Canon = &dto
	c.shell.Knowledge.finish(ticket)
	return nil
}

func (c *Controller) LoadKnowledgeCharacters(ctx context.Context, offset int, scope string) error {
	if offset < 0 || !validCharacterScope(scope) {
		return c.failLocalKnowledgeValidation()
	}
	ticket := c.beginKnowledgeQuery(appruntime.QueryKnowledgeCharacters, KnowledgeTabCharacters)
	var dto appruntime.KnowledgeCharactersResultDTO
	payload := appruntime.KnowledgeCharactersQuery{
		PageQuery: appruntime.PageQuery{Offset: offset, Limit: KnowledgeWorkspaceQueryLimit},
		Scope:     scope,
	}
	accepted, err := c.executeKnowledgeQuery(ctx, ticket, payload, &dto)
	if !accepted || err != nil {
		return err
	}
	c.shell.Knowledge.Characters = &dto
	c.shell.Knowledge.finish(ticket)
	return nil
}

func (c *Controller) LoadKnowledgeWorld(ctx context.Context, sections []string) error {
	if !validWorldSections(sections) {
		return c.failLocalKnowledgeValidation()
	}
	ticket := c.beginKnowledgeQuery(appruntime.QueryKnowledgeWorld, KnowledgeTabWorld)
	var dto appruntime.KnowledgeWorldResultDTO
	payload := appruntime.KnowledgeWorldQuery{Sections: append([]string(nil), sections...), Limit: KnowledgeWorkspaceQueryLimit}
	accepted, err := c.executeKnowledgeQuery(ctx, ticket, payload, &dto)
	if !accepted || err != nil {
		return err
	}
	c.shell.Knowledge.World = &dto
	c.shell.Knowledge.finish(ticket)
	return nil
}

func (c *Controller) LoadKnowledgeTimeline(ctx context.Context, fromChapter, toChapter int) error {
	if fromChapter < 0 || toChapter < 0 || (fromChapter > 0 && toChapter > 0 && fromChapter > toChapter) {
		return c.failLocalKnowledgeValidation()
	}
	ticket := c.beginKnowledgeQuery(appruntime.QueryKnowledgeTimeline, KnowledgeTabTimeline)
	var dto appruntime.KnowledgeTimelineResultDTO
	payload := appruntime.KnowledgeTimelineQuery{
		FromChapter: fromChapter,
		ToChapter:   toChapter,
		Limit:       KnowledgeWorkspaceQueryLimit,
	}
	accepted, err := c.executeKnowledgeQuery(ctx, ticket, payload, &dto)
	if !accepted || err != nil {
		return err
	}
	c.shell.Knowledge.Timeline = &dto
	c.shell.Knowledge.finish(ticket)
	return nil
}

func (c *Controller) beginKnowledgeQuery(kind appruntime.QueryKind, tab KnowledgeTab) knowledgeQueryTicket {
	ticket := knowledgeQueryTicket{
		ID:               c.nextRequestID(),
		Kind:             kind,
		SnapshotRevision: c.shell.SnapshotRevision,
		Route:            RouteKnowledge,
		Tab:              tab,
	}
	c.shell.Knowledge.begin(ticket)
	return ticket
}

func (c *Controller) executeKnowledgeQuery(ctx context.Context, ticket knowledgeQueryTicket, payload, dst any) (bool, error) {
	if c.runtime == nil {
		return false, ErrNilRuntime
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		c.shell.Knowledge.fail(ticket, c.shell, queryProtocolViewError())
		return false, err
	}
	result, err := c.runtime.Query(ctx, appruntime.QueryRequest{
		ContractVersion: appruntime.ContractVersion,
		Kind:            ticket.Kind,
		Payload:         raw,
	})
	if !c.shell.Knowledge.current(ticket, c.shell) {
		c.shell.Knowledge.discard(ticket)
		return false, nil
	}
	if err != nil {
		c.shell.Knowledge.fail(ticket, c.shell, viewErrorFromError(err))
		return false, err
	}
	if result.Error != nil {
		c.shell.Knowledge.fail(ticket, c.shell, viewErrorFromAppError(result.Error))
		return false, result.Error
	}
	if result.ContractVersion != appruntime.ContractVersion || result.Kind != ticket.Kind {
		c.shell.Knowledge.fail(ticket, c.shell, queryProtocolViewError())
		return false, errors.New("desktopui: knowledge query result contract mismatch")
	}
	if len(result.Data) == 0 || string(result.Data) == "null" || !json.Valid(result.Data) {
		c.shell.Knowledge.fail(ticket, c.shell, queryProtocolViewError())
		return false, errors.New("desktopui: knowledge query result data is invalid")
	}
	if err := json.Unmarshal(result.Data, dst); err != nil {
		c.shell.Knowledge.fail(ticket, c.shell, queryProtocolViewError())
		return false, fmt.Errorf("desktopui: decode knowledge query result: %w", err)
	}
	return true, nil
}

func (c *Controller) failLocalKnowledgeValidation() error {
	viewErr := &ErrorView{
		Code:     string(appruntime.ErrorCodeInvalidArgument),
		Category: string(appruntime.ErrorCategoryValidation),
		Message:  "The request is invalid.",
	}
	c.shell.Knowledge.Error = viewErr
	c.shell.Knowledge.Load = LoadValidationError
	return &appruntime.AppError{
		Code:     appruntime.ErrorCodeInvalidArgument,
		Category: appruntime.ErrorCategoryValidation,
		Message:  "The request is invalid.",
	}
}

func validContextScope(scope string) bool {
	switch scope {
	case "", "writer", "editor", "overview":
		return true
	default:
		return false
	}
}

func validCanonScope(scope string) bool {
	switch scope {
	case "", "all", "continuity", "state", "relationship", "foreshadow":
		return true
	default:
		return false
	}
}

func validCharacterScope(scope string) bool {
	switch scope {
	case "", "all", "core", "cast":
		return true
	default:
		return false
	}
}

func validWorldSections(sections []string) bool {
	for _, section := range sections {
		switch section {
		case "rules", "foreshadow", "relationships", "state_changes":
		default:
			return false
		}
	}
	return true
}
