package appruntime

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/voocel/ainovel-cli/internal/host"
)

func TestDocumentsListDTOFiltersAndPaginates(t *testing.T) {
	artifacts := []host.DesktopDocumentArtifact{
		{ID: "chapter.final:1", Kind: DocumentKindChapter, Path: "chapters/01.md", Title: "Chapter 1 final", ContentType: "text/markdown; charset=utf-8", SizeBytes: 10},
		{ID: "chapter.final:2", Kind: DocumentKindChapter, Path: "chapters/02.md", Title: "Chapter 2 final", ContentType: "text/markdown; charset=utf-8", SizeBytes: 20},
		{ID: "outline.flat", Kind: DocumentKindOutline, Path: "outline.json", Title: "Flat outline", ContentType: "application/json", SizeBytes: 30},
		{ID: "project.book", Kind: DocumentKindProject, Path: "meta/book.json", Title: "Book metadata", ContentType: "application/json", SizeBytes: 40},
	}

	got := documentsListDTO(artifacts, DocumentsListQuery{
		PageQuery: PageQuery{Offset: 1, Limit: 1},
		Kind:      DocumentKindChapter,
		Prefix:    "chapters/",
	})
	if got.Total != 2 || got.Offset != 1 || got.Limit != 1 || len(got.Items) != 1 {
		t.Fatalf("paged result = %+v", got)
	}
	if got.Items[0].ID != "chapter.final:2" || !got.Items[0].ReadOnly {
		t.Fatalf("paged item = %+v", got.Items[0])
	}
	if got.Items[0].ContentType != "text/markdown; charset=utf-8" || got.Items[0].SizeBytes != 20 {
		t.Fatalf("metadata projection = %+v", got.Items[0])
	}
}

func TestDocumentsListDTODefaultLimitAndEmptyPage(t *testing.T) {
	artifacts := []host.DesktopDocumentArtifact{{ID: "project.book", Kind: DocumentKindProject, Path: "meta/book.json"}}
	got := documentsListDTO(artifacts, DocumentsListQuery{PageQuery: PageQuery{Offset: 10}})
	if got.Total != 1 || got.Offset != 10 || got.Limit != defaultProjectQueryPageSize || len(got.Items) != 0 {
		t.Fatalf("default/empty page = %+v", got)
	}
}

func TestDocumentSelectorsRejectTraversalAndUnsupportedKinds(t *testing.T) {
	cases := []QueryRequest{
		{Kind: QueryDocumentsGet, Payload: json.RawMessage(`{"id":"../meta/book.json"}`)},
		{Kind: QueryDocumentsGet, Payload: json.RawMessage(`{"id":"meta/book.json"}`)},
		{Kind: QueryDocumentsGet, Payload: json.RawMessage(`{"id":"a..b"}`)},
		{Kind: QueryDocumentsList, Payload: json.RawMessage(`{"prefix":"../"}`)},
		{Kind: QueryDocumentsList, Payload: json.RawMessage(`{"prefix":"/meta/"}`)},
		{Kind: QueryDocumentsList, Payload: json.RawMessage(`{"prefix":"meta/../chapters/"}`)},
		{Kind: QueryDocumentsList, Payload: json.RawMessage(`{"prefix":"meta\\chapter_records\\"}`)},
		{Kind: QueryDocumentsList, Payload: json.RawMessage(`{"kind":"filesystem"}`)},
	}
	for _, req := range cases {
		if _, err := decodeQueryRoute(req); err == nil || !errors.Is(err, ErrInvalidQuery) {
			t.Fatalf("unsafe selector %+v error = %v", req, err)
		}
	}
}

func TestDocumentSelectorsAcceptStableIDsAndSafePrefixes(t *testing.T) {
	cases := []struct {
		req  QueryRequest
		want any
	}{
		{
			req:  QueryRequest{Kind: QueryDocumentsGet, Payload: json.RawMessage(`{"id":"chapter.record:12"}`)},
			want: DocumentsGetQuery{ID: "chapter.record:12"},
		},
		{
			req:  QueryRequest{Kind: QueryDocumentsList, Payload: json.RawMessage(`{"kind":"summary","prefix":"summaries/","offset":2,"limit":5}`)},
			want: DocumentsListQuery{PageQuery: PageQuery{Offset: 2, Limit: 5}, Kind: DocumentKindSummary, Prefix: "summaries/"},
		},
	}
	for _, tc := range cases {
		route, err := decodeQueryRoute(tc.req)
		if err != nil {
			t.Fatalf("decode %+v: %v", tc.req, err)
		}
		if !reflect.DeepEqual(route.Request, tc.want) {
			t.Fatalf("request = %#v, want %#v", route.Request, tc.want)
		}
	}
}
