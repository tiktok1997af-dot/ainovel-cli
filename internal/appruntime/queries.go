package appruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strings"
)

type queryRoute struct {
	Kind    QueryKind
	Request any
}

var documentIDRE = regexp.MustCompile(`^[a-z0-9][a-z0-9._:-]{0,127}$`)

func (r *Runtime) routeQuery(ctx context.Context, req QueryRequest) (json.RawMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	route, err := decodeQueryRoute(req)
	if err != nil {
		return nil, err
	}

	switch route.Kind {
	case QueryProjectOverview:
		return r.queryProjectOverview()
	case QueryChaptersList:
		return r.queryChaptersList(route.Request.(ChaptersListQuery))
	case QueryChaptersGet:
		return r.queryChaptersGet(route.Request.(ChaptersGetQuery))
	case QueryOutlineGet:
		return r.queryOutlineGet(route.Request.(OutlineGetQuery))
	case QueryDocumentsList:
		return r.queryDocumentsList(route.Request.(DocumentsListQuery))
	case QueryDocumentsGet:
		return r.queryDocumentsGet(route.Request.(DocumentsGetQuery))
	case QueryKnowledgeContext:
		return r.queryKnowledgeContext(route.Request.(KnowledgeContextQuery))
	case QueryKnowledgeCanon:
		return r.queryKnowledgeCanon(route.Request.(KnowledgeCanonQuery))
	case QueryKnowledgeCharacters:
		return r.queryKnowledgeCharacters(route.Request.(KnowledgeCharactersQuery))
	case QueryKnowledgeWorld:
		return r.queryKnowledgeWorld(route.Request.(KnowledgeWorldQuery))
	case QueryKnowledgeTimeline:
		return r.queryKnowledgeTimeline(route.Request.(KnowledgeTimelineQuery))
	default:
		return nil, ErrNotImplemented
	}
}

func decodeQueryRoute(req QueryRequest) (queryRoute, error) {
	switch req.Kind {
	case QueryProjectOverview:
		var payload ProjectOverviewQuery
		if err := decodeQueryPayload(req.Payload, &payload); err != nil {
			return queryRoute{}, err
		}
		return queryRoute{Kind: req.Kind, Request: payload}, nil
	case QueryChaptersList:
		var payload ChaptersListQuery
		if err := decodeQueryPayload(req.Payload, &payload); err != nil {
			return queryRoute{}, err
		}
		if err := validatePage(payload.PageQuery); err != nil {
			return queryRoute{}, err
		}
		return queryRoute{Kind: req.Kind, Request: payload}, nil
	case QueryChaptersGet:
		var payload ChaptersGetQuery
		if err := decodeQueryPayload(req.Payload, &payload); err != nil {
			return queryRoute{}, err
		}
		if payload.Chapter <= 0 {
			return queryRoute{}, invalidQuery("chapter must be greater than zero")
		}
		return queryRoute{Kind: req.Kind, Request: payload}, nil
	case QueryOutlineGet:
		var payload OutlineGetQuery
		if err := decodeQueryPayload(req.Payload, &payload); err != nil {
			return queryRoute{}, err
		}
		if payload.FromChapter < 0 {
			return queryRoute{}, invalidQuery("from_chapter cannot be negative")
		}
		if err := validateLimit(payload.Limit); err != nil {
			return queryRoute{}, err
		}
		return queryRoute{Kind: req.Kind, Request: payload}, nil
	case QueryDocumentsList:
		var payload DocumentsListQuery
		if err := decodeQueryPayload(req.Payload, &payload); err != nil {
			return queryRoute{}, err
		}
		if err := validatePage(payload.PageQuery); err != nil {
			return queryRoute{}, err
		}
		payload.Kind = strings.TrimSpace(payload.Kind)
		payload.Prefix = strings.TrimSpace(payload.Prefix)
		if err := validateDocumentKind(payload.Kind); err != nil {
			return queryRoute{}, err
		}
		if err := validateDocumentPrefix(payload.Prefix); err != nil {
			return queryRoute{}, err
		}
		return queryRoute{Kind: req.Kind, Request: payload}, nil
	case QueryDocumentsGet:
		var payload DocumentsGetQuery
		if err := decodeQueryPayload(req.Payload, &payload); err != nil {
			return queryRoute{}, err
		}
		payload.ID = strings.TrimSpace(payload.ID)
		if !validDocumentID(payload.ID) {
			return queryRoute{}, invalidQuery("document id is invalid")
		}
		return queryRoute{Kind: req.Kind, Request: payload}, nil
	case QueryKnowledgeContext:
		var payload KnowledgeContextQuery
		if err := decodeQueryPayload(req.Payload, &payload); err != nil {
			return queryRoute{}, err
		}
		if payload.Chapter < 0 {
			return queryRoute{}, invalidQuery("chapter cannot be negative")
		}
		if err := validateLimit(payload.MaxItems); err != nil {
			return queryRoute{}, err
		}
		payload.Scope = strings.TrimSpace(payload.Scope)
		if err := validateContextScope(payload.Scope); err != nil {
			return queryRoute{}, err
		}
		return queryRoute{Kind: req.Kind, Request: payload}, nil
	case QueryKnowledgeCanon:
		var payload KnowledgeCanonQuery
		if err := decodeQueryPayload(req.Payload, &payload); err != nil {
			return queryRoute{}, err
		}
		if payload.Chapter < 0 {
			return queryRoute{}, invalidQuery("chapter cannot be negative")
		}
		if err := validatePage(payload.PageQuery); err != nil {
			return queryRoute{}, err
		}
		payload.Scope = strings.TrimSpace(payload.Scope)
		if err := validateCanonScope(payload.Scope); err != nil {
			return queryRoute{}, err
		}
		return queryRoute{Kind: req.Kind, Request: payload}, nil
	case QueryKnowledgeCharacters:
		var payload KnowledgeCharactersQuery
		if err := decodeQueryPayload(req.Payload, &payload); err != nil {
			return queryRoute{}, err
		}
		if err := validatePage(payload.PageQuery); err != nil {
			return queryRoute{}, err
		}
		payload.Scope = strings.TrimSpace(payload.Scope)
		if payload.Scope != "" && payload.Scope != "all" && payload.Scope != "core" && payload.Scope != "cast" {
			return queryRoute{}, invalidQuery("character scope is not supported")
		}
		return queryRoute{Kind: req.Kind, Request: payload}, nil
	case QueryKnowledgeWorld:
		var payload KnowledgeWorldQuery
		if err := decodeQueryPayload(req.Payload, &payload); err != nil {
			return queryRoute{}, err
		}
		if err := validateLimit(payload.Limit); err != nil {
			return queryRoute{}, err
		}
		for i := range payload.Sections {
			payload.Sections[i] = strings.TrimSpace(payload.Sections[i])
		}
		if err := validateWorldSections(payload.Sections); err != nil {
			return queryRoute{}, err
		}
		return queryRoute{Kind: req.Kind, Request: payload}, nil
	case QueryKnowledgeTimeline:
		var payload KnowledgeTimelineQuery
		if err := decodeQueryPayload(req.Payload, &payload); err != nil {
			return queryRoute{}, err
		}
		if payload.FromChapter < 0 || payload.ToChapter < 0 {
			return queryRoute{}, invalidQuery("timeline chapter bounds cannot be negative")
		}
		if payload.FromChapter > 0 && payload.ToChapter > 0 && payload.FromChapter > payload.ToChapter {
			return queryRoute{}, invalidQuery("from_chapter cannot exceed to_chapter")
		}
		if err := validateLimit(payload.Limit); err != nil {
			return queryRoute{}, err
		}
		return queryRoute{Kind: req.Kind, Request: payload}, nil
	default:
		return queryRoute{}, invalidQuery("query kind is not supported")
	}
}

func decodeQueryPayload(payload json.RawMessage, dst any) error {
	if len(payload) == 0 || string(payload) == "null" {
		return nil
	}
	if !json.Valid(payload) {
		return invalidQuery("query payload is not valid JSON")
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return invalidQuery("query payload does not match the typed contract")
	}
	return nil
}

func validatePage(page PageQuery) error {
	if page.Offset < 0 {
		return invalidQuery("offset cannot be negative")
	}
	return validateLimit(page.Limit)
}

func validateLimit(limit int) error {
	if limit < 0 {
		return invalidQuery("limit cannot be negative")
	}
	if limit > MaxQueryPageSize {
		return invalidQuery(fmt.Sprintf("limit exceeds maximum %d", MaxQueryPageSize))
	}
	return nil
}

func validateContextScope(scope string) error {
	switch scope {
	case "", knowledgeScopeWriter, knowledgeScopeEditor, knowledgeScopeOverview:
		return nil
	default:
		return invalidQuery("context scope is not supported")
	}
}

func validateCanonScope(scope string) error {
	switch scope {
	case "", knowledgeScopeAll, "continuity", "state", "relationship", "foreshadow":
		return nil
	default:
		return invalidQuery("canon scope is not supported")
	}
}

func validateWorldSections(sections []string) error {
	for _, section := range sections {
		switch section {
		case knowledgeSectionRules, knowledgeSectionForeshadow, knowledgeSectionRelationships, knowledgeSectionStateChanges:
		default:
			return invalidQuery("world section is not supported")
		}
	}
	return nil
}

func validateDocumentKind(kind string) error {
	switch kind {
	case "", DocumentKindProject, DocumentKindOutline, DocumentKindKnowledge, DocumentKindChapter, DocumentKindSummary:
		return nil
	default:
		return invalidQuery("document kind is not supported")
	}
}

func validateDocumentPrefix(prefix string) error {
	if prefix == "" {
		return nil
	}
	if strings.ContainsRune(prefix, '\x00') || strings.Contains(prefix, "\\") || strings.Contains(prefix, ":") || path.IsAbs(prefix) {
		return invalidQuery("document prefix is invalid")
	}
	trimmed := strings.TrimSuffix(prefix, "/")
	if trimmed == "" {
		return invalidQuery("document prefix is invalid")
	}
	clean := path.Clean(trimmed)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || clean != trimmed {
		return invalidQuery("document prefix is invalid")
	}
	for _, segment := range strings.Split(clean, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return invalidQuery("document prefix is invalid")
		}
	}
	return nil
}

func validDocumentID(id string) bool {
	return id != "" && !strings.Contains(id, "..") && documentIDRE.MatchString(id)
}

func invalidQuery(_ string) error {
	// Never echo raw client payloads, paths, or selectors into the desktop error.
	return &AppError{
		Code:     ErrorCodeInvalidArgument,
		Category: ErrorCategoryValidation,
		Message:  safeErrorMessage(ErrorCodeInvalidArgument),
		Cause:    ErrInvalidQuery,
	}
}
