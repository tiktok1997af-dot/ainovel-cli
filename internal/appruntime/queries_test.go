package appruntime

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func TestSupportedQueryKindsStableCatalog(t *testing.T) {
	want := []QueryKind{
		QueryProjectOverview,
		QueryChaptersList,
		QueryChaptersGet,
		QueryOutlineGet,
		QueryDocumentsList,
		QueryDocumentsGet,
		QueryKnowledgeContext,
		QueryKnowledgeCanon,
		QueryKnowledgeCharacters,
		QueryKnowledgeWorld,
		QueryKnowledgeTimeline,
	}
	if got := SupportedQueryKinds(); !reflect.DeepEqual(got, want) {
		t.Fatalf("supported query catalog = %#v, want %#v", got, want)
	}
}

func TestDecodeQueryRouteAcceptsTypedCatalog(t *testing.T) {
	cases := []struct {
		kind    QueryKind
		payload string
		want    any
	}{
		{QueryProjectOverview, `{}`, ProjectOverviewQuery{}},
		{QueryChaptersList, `{"offset":1,"limit":20}`, ChaptersListQuery{PageQuery: PageQuery{Offset: 1, Limit: 20}}},
		{QueryChaptersGet, `{"chapter":7,"include_content":true}`, ChaptersGetQuery{Chapter: 7, IncludeContent: true}},
		{QueryOutlineGet, `{"from_chapter":10,"limit":30}`, OutlineGetQuery{FromChapter: 10, Limit: 30}},
		{QueryDocumentsList, `{"kind":"outline","prefix":"meta/","limit":25}`, DocumentsListQuery{PageQuery: PageQuery{Limit: 25}, Kind: "outline", Prefix: "meta/"}},
		{QueryDocumentsGet, `{"id":"outline.layered"}`, DocumentsGetQuery{ID: "outline.layered"}},
		{QueryKnowledgeContext, `{"chapter":22,"scope":"writer","max_items":40}`, KnowledgeContextQuery{Chapter: 22, Scope: "writer", MaxItems: 40}},
		{QueryKnowledgeCanon, `{"chapter":20,"scope":"continuity","offset":2,"limit":50}`, KnowledgeCanonQuery{PageQuery: PageQuery{Offset: 2, Limit: 50}, Chapter: 20, Scope: "continuity"}},
		{QueryKnowledgeCharacters, `{"scope":"core","limit":40}`, KnowledgeCharactersQuery{PageQuery: PageQuery{Limit: 40}, Scope: "core"}},
		{QueryKnowledgeWorld, `{"sections":["rules","foreshadow"],"limit":60}`, KnowledgeWorldQuery{Sections: []string{"rules", "foreshadow"}, Limit: 60}},
		{QueryKnowledgeTimeline, `{"from_chapter":3,"to_chapter":12,"limit":80}`, KnowledgeTimelineQuery{FromChapter: 3, ToChapter: 12, Limit: 80}},
	}

	for _, tc := range cases {
		t.Run(string(tc.kind), func(t *testing.T) {
			route, err := decodeQueryRoute(QueryRequest{Kind: tc.kind, Payload: json.RawMessage(tc.payload)})
			if err != nil {
				t.Fatalf("decode route: %v", err)
			}
			if route.Kind != tc.kind {
				t.Fatalf("route kind = %q, want %q", route.Kind, tc.kind)
			}
			if !reflect.DeepEqual(route.Request, tc.want) {
				t.Fatalf("request = %#v, want %#v", route.Request, tc.want)
			}
		})
	}
}

func TestQueryValidationRejectsInvalidAndUnknownPayloads(t *testing.T) {
	cases := []QueryRequest{
		{Kind: QueryKind("project.unknown")},
		{Kind: QueryChaptersGet, Payload: json.RawMessage(`{"chapter":0}`)},
		{Kind: QueryChaptersList, Payload: json.RawMessage(`{"limit":501}`)},
		{Kind: QueryDocumentsGet, Payload: json.RawMessage(`{"id":"   "}`)},
		{Kind: QueryDocumentsGet, Payload: json.RawMessage(`{"id":"../meta/book.json"}`)},
		{Kind: QueryDocumentsList, Payload: json.RawMessage(`{"prefix":"../meta/"}`)},
		{Kind: QueryDocumentsList, Payload: json.RawMessage(`{"kind":"filesystem"}`)},
		{Kind: QueryKnowledgeContext, Payload: json.RawMessage(`{"scope":"secret"}`)},
		{Kind: QueryKnowledgeCanon, Payload: json.RawMessage(`{"scope":"secret"}`)},
		{Kind: QueryKnowledgeCharacters, Payload: json.RawMessage(`{"scope":"secret"}`)},
		{Kind: QueryKnowledgeWorld, Payload: json.RawMessage(`{"sections":["filesystem"]}`)},
		{Kind: QueryKnowledgeTimeline, Payload: json.RawMessage(`{"from_chapter":9,"to_chapter":3}`)},
		{Kind: QueryProjectOverview, Payload: json.RawMessage(`{"unknown":true}`)},
		{Kind: QueryProjectOverview, Payload: json.RawMessage(`{`)},
	}
	for _, req := range cases {
		_, err := decodeQueryRoute(req)
		if err == nil {
			t.Fatalf("expected validation error for %+v", req)
		}
		if !errors.Is(err, ErrInvalidQuery) {
			t.Fatalf("error %v does not preserve ErrInvalidQuery", err)
		}
		app := normalizeAppError(err)
		if app.Code != ErrorCodeInvalidArgument || app.Category != ErrorCategoryValidation {
			t.Fatalf("query validation mapping = %+v", app)
		}
	}
}

func TestKnowledgeQueryScopesAreTrimmedAndRemainTyped(t *testing.T) {
	cases := []struct {
		kind    QueryKind
		payload string
		want    string
	}{
		{QueryKnowledgeContext, `{"scope":" writer "}`, "writer"},
		{QueryKnowledgeCanon, `{"scope":" continuity "}`, "continuity"},
		{QueryKnowledgeCharacters, `{"scope":" cast "}`, "cast"},
	}
	for _, tc := range cases {
		route, err := decodeQueryRoute(QueryRequest{Kind: tc.kind, Payload: json.RawMessage(tc.payload)})
		if err != nil {
			t.Fatal(err)
		}
		var got string
		switch value := route.Request.(type) {
		case KnowledgeContextQuery:
			got = value.Scope
		case KnowledgeCanonQuery:
			got = value.Scope
		case KnowledgeCharactersQuery:
			got = value.Scope
		default:
			t.Fatalf("unexpected typed request %T", route.Request)
		}
		if got != tc.want {
			t.Fatalf("scope = %q, want %q", got, tc.want)
		}
	}
}

func TestProjectKnowledgeDTOsAreJSONSafe(t *testing.T) {
	values := []any{
		ProjectOverviewResultDTO{FormatVersion: 2, Title: "Book", Layered: true},
		ChaptersListResultDTO{Items: []ChapterListItemDTO{{Chapter: 1, Status: "completed", HasFinal: true}}, Total: 1},
		ChaptersGetResultDTO{Chapter: 1, Status: "completed", Final: &ChapterTextViewDTO{Present: true, Content: "text", WordCount: 4}},
		OutlineGetResultDTO{Layered: true, Volumes: []VolumeOutlineViewDTO{{Index: 1, Title: "V1"}}},
		DocumentsListResultDTO{Items: []DocumentSummaryDTO{{ID: "project.book", Kind: DocumentKindProject, Path: "meta/book.json", ReadOnly: true}}, Total: 1},
		DocumentsGetResultDTO{Document: DocumentSummaryDTO{ID: "project.book", Kind: DocumentKindProject, Path: "meta/book.json", ReadOnly: true}, Content: "{}"},
		KnowledgeContextResultDTO{Sections: []KnowledgeSectionDTO{{Name: "canon", Items: []KnowledgeItemDTO{{Kind: "fact", Summary: "stable", Provenance: []ProvenanceDTO{{ArtifactID: "chapter.record:1", Kind: "chapter_record"}}}}}}},
		KnowledgeCanonResultDTO{Facts: []CanonFactDTO{{Kind: "state", Subject: "hero", Field: "status", Value: "alive", Provenance: []ProvenanceDTO{{ArtifactID: "chapter.record:1", Kind: "chapter_record"}}}}, Total: 1},
		KnowledgeCharactersResultDTO{Items: []CharacterViewDTO{{Name: "Hero", Origin: "core"}}, Total: 1},
		KnowledgeWorldResultDTO{Rules: []WorldRuleViewDTO{{Rule: "rule"}}, Foreshadow: []ForeshadowViewDTO{}, Relationships: []RelationshipViewDTO{}, StateChanges: []StateChangeViewDTO{}},
		KnowledgeTimelineResultDTO{Events: []TimelineEventViewDTO{{Chapter: 1, Event: "event"}}, Total: 1},
	}
	for _, value := range values {
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("marshal %T: %v", value, err)
		}
		if !json.Valid(data) {
			t.Fatalf("invalid JSON for %T: %s", value, data)
		}
	}
}

func TestQueryEnvelopeRoundTripWithTypedPayload(t *testing.T) {
	payload, err := json.Marshal(ChaptersGetQuery{Chapter: 9, IncludeContent: true})
	if err != nil {
		t.Fatal(err)
	}
	original := QueryRequest{ContractVersion: ContractVersion, Kind: QueryChaptersGet, Payload: payload}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var roundTrip QueryRequest
	if err := json.Unmarshal(data, &roundTrip); err != nil {
		t.Fatal(err)
	}
	if roundTrip.ContractVersion != ContractVersion || roundTrip.Kind != QueryChaptersGet {
		t.Fatalf("round trip envelope = %+v", roundTrip)
	}
	route, err := decodeQueryRoute(roundTrip)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := route.Request.(ChaptersGetQuery)
	if !ok || got.Chapter != 9 || !got.IncludeContent {
		t.Fatalf("typed payload round trip = %#v", route.Request)
	}
}
