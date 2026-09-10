package appruntime

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func TestG036MutationCatalogStable(t *testing.T) {
	want := []CommandKind{
		"project.metadata.update",
		"project.premise.update",
		"chapter.plan.save",
		"chapter.draft.save",
		"chapter.workspace.save",
		"outline.tail.revise",
		"outline.arc.expand",
		"outline.volume.append",
		"outline.compass.update",
		"knowledge.characters.core.replace",
		"knowledge.world.rules.replace",
		"knowledge.timeline.append",
		"knowledge.relationships.update",
		"knowledge.foreshadow.update",
	}
	if got := G03MutationCommandKinds(); !reflect.DeepEqual(got, want) {
		t.Fatalf("catalog changed:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestG036MutationContractsDecodeAndDeriveCanonicalResource(t *testing.T) {
	cases := []struct {
		kind     CommandKind
		payload  string
		resource string
	}{
		{CommandProjectMetadataUpdate, `{"title":"Book","synopsis":"Story"}`, "project:metadata"},
		{CommandProjectPremiseUpdate, `{"premise":"A promise with a cost."}`, "project:premise"},
		{CommandChapterPlanSave, `{"chapter":3,"title":"Door","goal":"Cross it"}`, "chapter:000003:plan"},
		{CommandChapterDraftSave, `{"chapter":3,"content":"draft"}`, "chapter:000003:draft"},
		{CommandChapterWorkspaceSave, `{"chapter":3,"content":"workspace","expected_record_revision":2}`, "chapter:000003:workspace"},
		{CommandOutlineTailRevise, `{"from_chapter":8,"replacement":[{"title":"Turn","core_event":"Choice"}]}`, "outline:structure"},
		{CommandOutlineArcExpand, `{"volume":1,"arc":2,"expansion":{"title":"Arc","goal":"Escalate","chapters":[{"title":"Beat","core_event":"Event"}]}}`, "outline:structure"},
		{CommandOutlineVolumeAppend, `{"volume":{"index":2,"title":"V2","theme":"Cost","arcs":[{"index":1,"title":"A1","goal":"Enter","chapters":[{"title":"Beat","core_event":"Event"}]}]}}`, "outline:structure"},
		{CommandOutlineCompassUpdate, `{"compass":{"ending_direction":"Return changed","last_updated":7}}`, "outline:compass"},
		{CommandKnowledgeCharactersReplace, `{"characters":[{"name":"A","role":"lead","description":"desc"}]}`, "knowledge:characters:core"},
		{CommandKnowledgeWorldRulesReplace, `{"rules":[{"category":"magic","rule":"Costs matter"}]}`, "knowledge:world:rules"},
		{CommandKnowledgeTimelineAppend, `{"events":[{"chapter":4,"time":"night","event":"Gate opens"}]}`, "knowledge:timeline"},
		{CommandKnowledgeRelationshipsUpdate, `{"changes":[{"character_a":"A","character_b":"B","relation":"allies","chapter":4}]}`, "knowledge:relationships"},
		{CommandKnowledgeForeshadowUpdate, `{"chapter":4,"updates":[{"id":"f1","action":"plant","description":"broken seal"}]}`, "knowledge:foreshadow"},
	}

	for _, tc := range cases {
		t.Run(string(tc.kind), func(t *testing.T) {
			route, err := decodeMutationContract(CommandRequest{Kind: tc.kind, Payload: json.RawMessage(tc.payload)})
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if route.Kind != tc.kind || route.Resource != tc.resource || route.Request == nil {
				t.Fatalf("route = %+v", route)
			}
			if _, err := decodeMutationContract(CommandRequest{Kind: tc.kind, Resource: tc.resource, Payload: json.RawMessage(tc.payload)}); err != nil {
				t.Fatalf("canonical resource rejected: %v", err)
			}
		})
	}
}

func TestG036MutationContractRejectsUnknownFieldsAndResourceSpoofing(t *testing.T) {
	_, err := decodeMutationContract(CommandRequest{
		Kind:    CommandProjectMetadataUpdate,
		Payload: json.RawMessage(`{"title":"Book","synopsis":"Story","path":"C:/secret"}`),
	})
	if err == nil || !errors.Is(err, ErrInvalidMutation) {
		t.Fatalf("unknown field was not rejected: %v", err)
	}

	_, err = decodeMutationContract(CommandRequest{
		Kind:     CommandChapterDraftSave,
		Resource: "project:metadata",
		Payload:  json.RawMessage(`{"chapter":2,"content":"draft"}`),
	})
	if err == nil || !errors.Is(err, ErrInvalidMutation) {
		t.Fatalf("resource spoofing was not rejected: %v", err)
	}
}

func TestG036MutationContractRejectsMalformedOrUnsafeValues(t *testing.T) {
	bad := []CommandRequest{
		{Kind: CommandProjectMetadataUpdate, Payload: json.RawMessage(`{"title":"","synopsis":"Story"}`)},
		{Kind: CommandProjectPremiseUpdate, Payload: json.RawMessage(`{"premise":"   "}`)},
		{Kind: CommandChapterPlanSave, Payload: json.RawMessage(`{"chapter":0,"title":"T","goal":"G"}`)},
		{Kind: CommandChapterDraftSave, Payload: json.RawMessage(`{"chapter":1,"content":"draft","expected_sha256":"ABC"}`)},
		{Kind: CommandChapterWorkspaceSave, Payload: json.RawMessage(`{"chapter":1,"content":"","expected_record_revision":-1}`)},
		{Kind: CommandOutlineTailRevise, Payload: json.RawMessage(`{"from_chapter":1,"replacement":[]}`)},
		{Kind: CommandOutlineArcExpand, Payload: json.RawMessage(`{"volume":1,"arc":1,"expansion":{"title":"A","goal":"G","chapters":[]}}`)},
		{Kind: CommandOutlineVolumeAppend, Payload: json.RawMessage(`{"volume":{"index":2,"title":"V","theme":"T","arcs":[{"index":1,"title":"A","goal":"G","estimated_chapters":2}]}}`)},
		{Kind: CommandOutlineCompassUpdate, Payload: json.RawMessage(`{"compass":{"ending_direction":""}}`)},
		{Kind: CommandKnowledgeCharactersReplace, Payload: json.RawMessage(`{"characters":[{"name":"A","role":"lead","description":"x"},{"name":"a","role":"other","description":"y"}]}`)},
		{Kind: CommandKnowledgeWorldRulesReplace, Payload: json.RawMessage(`{"rules":[{"rule":""}]}`)},
		{Kind: CommandKnowledgeTimelineAppend, Payload: json.RawMessage(`{"events":[{"chapter":0,"event":"x"}]}`)},
		{Kind: CommandKnowledgeRelationshipsUpdate, Payload: json.RawMessage(`{"changes":[{"character_a":"A","character_b":"A","relation":"self","chapter":1}]}`)},
		{Kind: CommandKnowledgeForeshadowUpdate, Payload: json.RawMessage(`{"chapter":1,"updates":[{"id":"f1","action":"plant"}]}`)},
	}
	for _, cmd := range bad {
		if _, err := decodeMutationContract(cmd); err == nil || !errors.Is(err, ErrInvalidMutation) {
			t.Fatalf("invalid %s accepted: %v", cmd.Kind, err)
		}
	}
}

func TestG036UnknownMutationGetsExplicitUnsupportedCode(t *testing.T) {
	_, err := decodeMutationContract(CommandRequest{Kind: CommandKind("documents.write"), Payload: json.RawMessage(`{}`)})
	if err == nil || !errors.Is(err, ErrUnsupportedMutation) {
		t.Fatalf("unexpected error: %v", err)
	}
	app := normalizeAppError(err)
	if app.Code != ErrorCodeUnsupportedOperation || app.Category != ErrorCategoryValidation || app.Retryable {
		t.Fatalf("mapping = %+v", app)
	}
}

func TestG036ConflictErrorCodesAreDistinct(t *testing.T) {
	cases := []struct {
		err  error
		code ErrorCode
	}{
		{ErrMutationTargetNotFound, ErrorCodeTargetNotFound},
		{ErrMutationPrecondition, ErrorCodePreconditionConflict},
		{ErrMutationStale, ErrorCodeStaleConflict},
	}
	for _, tc := range cases {
		app := normalizeAppError(tc.err)
		if app.Code != tc.code || app.Category != ErrorCategoryConflict {
			t.Fatalf("%v mapped to %+v", tc.err, app)
		}
	}
}

func TestG036CommandResultCarriesTypedDataAndCanonicalResource(t *testing.T) {
	data, err := json.Marshal(ProjectMetadataUpdateResultDTO{Title: "Book", Synopsis: "Story"})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(CommandResult{
		ContractVersion: ContractVersion,
		CommandID:       "cmd-1",
		Accepted:        true,
		Resource:        "project:metadata",
		Status:          "completed",
		Data:            data,
	})
	if err != nil {
		t.Fatal(err)
	}
	var roundTrip CommandResult
	if err := json.Unmarshal(encoded, &roundTrip); err != nil {
		t.Fatal(err)
	}
	if roundTrip.Resource != "project:metadata" || string(roundTrip.Data) != string(data) {
		t.Fatalf("round trip = %+v", roundTrip)
	}
}
