package desktopui

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/voocel/ainovel-cli/internal/appruntime"
)

type creativeWriteRuntime struct {
	snapshot      appruntime.DesktopSnapshot
	chapter       appruntime.ChaptersGetResultDTO
	queries       []appruntime.QueryRequest
	dispatches    []appruntime.CommandRequest
	dispatchError *appruntime.AppError
	badContract   bool
}

func (r *creativeWriteRuntime) Snapshot(context.Context) (appruntime.DesktopSnapshot, error) {
	return r.snapshot, nil
}

func (r *creativeWriteRuntime) Query(_ context.Context, req appruntime.QueryRequest) (appruntime.QueryResult, error) {
	r.queries = append(r.queries, req)
	data, _ := json.Marshal(r.chapter)
	return appruntime.QueryResult{ContractVersion: appruntime.ContractVersion, Kind: req.Kind, Data: data}, nil
}

func (r *creativeWriteRuntime) Dispatch(_ context.Context, req appruntime.CommandRequest) (appruntime.CommandResult, error) {
	r.dispatches = append(r.dispatches, req)
	if r.dispatchError != nil {
		return appruntime.CommandResult{
			ContractVersion: appruntime.ContractVersion,
			CommandID:       req.ID,
			Error:           r.dispatchError,
		}, r.dispatchError
	}
	contract := appruntime.ContractVersion
	if r.badContract {
		contract = "wrong.desktop.v1"
	}
	return appruntime.CommandResult{
		ContractVersion: contract,
		CommandID:       req.ID,
		Accepted:        true,
		Status:          "completed",
		Resource:        "canonical:resource",
		Data:            json.RawMessage(`{"content_sha256":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}`),
	}, nil
}

func (r *creativeWriteRuntime) Subscribe(context.Context, appruntime.EventCursor) (appruntime.EventSubscription, error) {
	return &workspaceSubscription{events: make(chan appruntime.DesktopEvent, 1)}, nil
}
func (r *creativeWriteRuntime) Close(context.Context) error { return nil }

func creativeSnapshot(state string) appruntime.DesktopSnapshot {
	s := testSnapshot(20, "Novel")
	s.Runtime.State = state
	s.CurrentChapter.Current = 4
	return s
}

func creativeChapter() appruntime.ChaptersGetResultDTO {
	return appruntime.ChaptersGetResultDTO{
		Chapter: 4,
		Title:   "Four",
		Status:  "accepted",
		Plan: &appruntime.ChapterPlanViewDTO{
			Chapter: 4, Title: "Four", Goal: "Goal", Required: []string{"beat"},
		},
		Draft: &appruntime.ChapterTextViewDTO{Present: true, Content: "draft v1", WordCount: 2},
		Final: &appruntime.ChapterTextViewDTO{Present: true, Content: "workspace v1", WordCount: 2},
		Record: &appruntime.ChapterRecordViewDTO{
			Revision: 3, Origin: "writer", ContentSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
	}
}

func readyCreativeController(r *creativeWriteRuntime) *Controller {
	c := NewController(r, 1440)
	c.shell.BeginSnapshot("ready")
	c.shell.AcceptSnapshot("ready", r.snapshot)
	c.reconcileLifecycleFromSnapshot()
	return c
}

func TestCreativeRouteLoadsContentAndKeepsArtifactsDistinct(t *testing.T) {
	r := &creativeWriteRuntime{snapshot: creativeSnapshot("ready"), chapter: creativeChapter()}
	c := readyCreativeController(r)
	if err := c.OpenCreativeEditor(context.Background(), 4); err != nil {
		t.Fatal(err)
	}
	state := c.Creative()
	if c.shell.Route != RouteCreative || state.Chapter != 4 || state.Canonical == nil {
		t.Fatalf("creative projection missing: route=%q state=%+v", c.shell.Route, state)
	}
	if state.Draft.Content != "draft v1" || state.Workspace.Content != "workspace v1" {
		t.Fatalf("draft/workspace collapsed: %+v", state)
	}
	if !state.Accepted.Present || state.Accepted.Record == nil || state.Accepted.Record.Revision != 3 {
		t.Fatalf("accepted record metadata missing: %+v", state.Accepted)
	}
	if state.Plan.CompleteReplacement {
		t.Fatal("existing plan read is intentionally incomplete and must not be silently replace-saveable")
	}
	if len(r.queries) != 1 || r.queries[0].Kind != appruntime.QueryChaptersGet {
		t.Fatalf("creative read calls = %+v", r.queries)
	}
	var req appruntime.ChaptersGetQuery
	if err := json.Unmarshal(r.queries[0].Payload, &req); err != nil {
		t.Fatal(err)
	}
	if req.Chapter != 4 || !req.IncludeContent {
		t.Fatalf("creative chapter query = %+v", req)
	}
}

func TestExistingPlanRequiresExplicitCompleteReplacement(t *testing.T) {
	r := &creativeWriteRuntime{snapshot: creativeSnapshot("ready"), chapter: creativeChapter()}
	c := readyCreativeController(r)
	if err := c.OpenCreativeEditor(context.Background(), 4); err != nil {
		t.Fatal(err)
	}
	value := c.Creative().Plan.Value
	value.Notes = "local edit"
	if !c.Creative().EditPlan(value, false) {
		t.Fatal("edit should stage")
	}
	if _, err := c.SaveCreativePlan(context.Background()); err == nil {
		t.Fatal("incomplete replacement should fail closed")
	}
	if len(r.dispatches) != 0 {
		t.Fatal("unsafe plan replacement reached Dispatch")
	}
	value.Contract.EvaluationFocus = []string{"pacing"}
	value.Contract.EmotionTarget = "tense"
	value.Contract.PayoffPoints = []string{"payoff"}
	value.Contract.HookGoal = "turn"
	c.Creative().EditPlan(value, true)
	if _, err := c.SaveCreativePlan(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(r.dispatches) != 1 || r.dispatches[0].Kind != appruntime.CommandChapterPlanSave {
		t.Fatalf("plan dispatch = %+v", r.dispatches)
	}
}

func TestDraftSaveUsesAcceptedRevisionAndNoG05Fields(t *testing.T) {
	r := &creativeWriteRuntime{snapshot: creativeSnapshot("ready"), chapter: creativeChapter()}
	c := readyCreativeController(r)
	if err := c.OpenCreativeEditor(context.Background(), 4); err != nil {
		t.Fatal(err)
	}
	c.Creative().EditDraft("draft v2")
	if _, err := c.SaveCreativeDraft(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(r.dispatches) != 1 {
		t.Fatalf("dispatches=%d", len(r.dispatches))
	}
	cmd := r.dispatches[0]
	if cmd.Kind != appruntime.CommandChapterDraftSave || cmd.RunID != "" || cmd.TaskID != "" || cmd.Resource != "" {
		t.Fatalf("unsafe command envelope: %+v", cmd)
	}
	var payload appruntime.ChapterTextSavePayload
	if err := json.Unmarshal(cmd.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Chapter != 4 || payload.Content != "draft v2" || payload.ExpectedRecordRevision != 3 {
		t.Fatalf("draft payload = %+v", payload)
	}
	if c.Creative().Draft.Dirty {
		t.Fatal("successful save should clear dirty only after reconciliation")
	}
}

func TestStaleConflictRefreshesCanonicalAndPreservesDirtyText(t *testing.T) {
	r := &creativeWriteRuntime{snapshot: creativeSnapshot("ready"), chapter: creativeChapter()}
	c := readyCreativeController(r)
	if err := c.OpenCreativeEditor(context.Background(), 4); err != nil {
		t.Fatal(err)
	}
	c.Creative().EditWorkspace("my local workspace")
	r.chapter.Final.Content = "external workspace"
	r.dispatchError = &appruntime.AppError{
		Code: appruntime.ErrorCodeStaleConflict, Category: appruntime.ErrorCategoryConflict,
		Message: "The project data changed after the action was prepared. Refresh and try again.",
	}
	if _, err := c.SaveCreativeWorkspace(context.Background()); err == nil {
		t.Fatal("stale conflict should be returned")
	}
	if c.Creative().Workspace.Content != "my local workspace" || !c.Creative().Workspace.Dirty {
		t.Fatalf("dirty editor was lost: %+v", c.Creative().Workspace)
	}
	if c.Creative().Canonical == nil || c.Creative().Canonical.Final.Content != "external workspace" {
		t.Fatalf("fresh canonical read missing: %+v", c.Creative().Canonical)
	}
	if c.Creative().Load != LoadConflict || c.Creative().Error == nil {
		t.Fatalf("conflict not projected: %+v", c.Creative())
	}
}

func TestAllFrozenG03MutationsHaveTypedSafeBindings(t *testing.T) {
	r := &creativeWriteRuntime{snapshot: creativeSnapshot("ready"), chapter: creativeChapter()}
	c := readyCreativeController(r)
	ctx := context.Background()
	calls := []func() error{
		func() error {
			_, e := c.SaveProjectMetadata(ctx, appruntime.ProjectMetadataUpdatePayload{Title: "N", Synopsis: "S"})
			return e
		},
		func() error {
			_, e := c.SaveProjectPremise(ctx, appruntime.ProjectPremiseUpdatePayload{Premise: "P"})
			return e
		},
		func() error {
			_, e := c.SaveChapterPlan(ctx, appruntime.ChapterPlanSavePayload{Chapter: 4, Title: "T", Goal: "G"})
			return e
		},
		func() error {
			_, e := c.SaveChapterDraft(ctx, appruntime.ChapterTextSavePayload{Chapter: 4, Content: "D"})
			return e
		},
		func() error {
			_, e := c.SaveChapterWorkspace(ctx, appruntime.ChapterTextSavePayload{Chapter: 4, Content: "W"})
			return e
		},
		func() error {
			_, e := c.ReviseOutlineTail(ctx, appruntime.OutlineTailRevisePayload{FromChapter: 5, Replacement: []appruntime.OutlineEntryMutationDTO{{Title: "T", CoreEvent: "E"}}})
			return e
		},
		func() error {
			_, e := c.ExpandOutlineArc(ctx, appruntime.OutlineArcExpandPayload{Volume: 1, Arc: 1, Expansion: appruntime.ArcExpansionMutationDTO{Title: "A", Goal: "G", Chapters: []appruntime.OutlineEntryMutationDTO{{Title: "T", CoreEvent: "E"}}}})
			return e
		},
		func() error {
			_, e := c.AppendOutlineVolume(ctx, appruntime.OutlineVolumeAppendPayload{Volume: appruntime.VolumeOutlineMutationDTO{Index: 2, Title: "V", Theme: "T", Arcs: []appruntime.ArcOutlineMutationDTO{{Index: 1, Title: "A", Goal: "G"}}}})
			return e
		},
		func() error {
			_, e := c.UpdateOutlineCompass(ctx, appruntime.OutlineCompassUpdatePayload{Compass: appruntime.StoryCompassMutationDTO{EndingDirection: "E"}})
			return e
		},
		func() error {
			_, e := c.ReplaceCoreCharacters(ctx, appruntime.KnowledgeCharactersReplacePayload{Characters: []appruntime.CoreCharacterMutationDTO{{Name: "A", Role: "hero", Description: "D"}}})
			return e
		},
		func() error {
			_, e := c.ReplaceWorldRules(ctx, appruntime.KnowledgeWorldRulesReplacePayload{Rules: []appruntime.WorldRuleMutationDTO{{Rule: "R"}}})
			return e
		},
		func() error {
			_, e := c.AppendTimeline(ctx, appruntime.KnowledgeTimelineAppendPayload{Events: []appruntime.TimelineEventMutationDTO{{Chapter: 4, Event: "E"}}})
			return e
		},
		func() error {
			_, e := c.UpdateRelationships(ctx, appruntime.KnowledgeRelationshipsUpdatePayload{Changes: []appruntime.RelationshipMutationDTO{{CharacterA: "A", CharacterB: "B", Relation: "ally", Chapter: 4}}})
			return e
		},
		func() error {
			_, e := c.UpdateForeshadow(ctx, appruntime.KnowledgeForeshadowUpdatePayload{Chapter: 4, Updates: []appruntime.ForeshadowMutationDTO{{ID: "f1", Action: "plant", Description: "D"}}})
			return e
		},
	}
	for i, call := range calls {
		if err := call(); err != nil {
			t.Fatalf("typed write %d: %v", i, err)
		}
	}
	want := appruntime.G03MutationCommandKinds()
	if len(r.dispatches) != len(want) {
		t.Fatalf("dispatch count=%d want=%d", len(r.dispatches), len(want))
	}
	for i, kind := range want {
		cmd := r.dispatches[i]
		if cmd.Kind != kind || cmd.ContractVersion != appruntime.ContractVersion {
			t.Fatalf("dispatch[%d]=%+v want kind=%q", i, cmd, kind)
		}
		if cmd.RunID != "" || cmd.TaskID != "" || cmd.Resource != "" {
			t.Fatalf("dispatch[%d] leaked G05/client resource authority: %+v", i, cmd)
		}
	}
}

func TestActiveRuntimeBlocksWriteBeforeDispatch(t *testing.T) {
	r := &creativeWriteRuntime{snapshot: creativeSnapshot("running"), chapter: creativeChapter()}
	c := readyCreativeController(r)
	_, err := c.SaveProjectPremise(context.Background(), appruntime.ProjectPremiseUpdatePayload{Premise: "P"})
	if err == nil {
		t.Fatal("active runtime must block write presentation")
	}
	if len(r.dispatches) != 0 {
		t.Fatal("blocked write reached Dispatch")
	}
}

func TestMutationContractMismatchFailsClosed(t *testing.T) {
	r := &creativeWriteRuntime{snapshot: creativeSnapshot("ready"), chapter: creativeChapter(), badContract: true}
	c := readyCreativeController(r)
	_, err := c.SaveProjectPremise(context.Background(), appruntime.ProjectPremiseUpdatePayload{Premise: "P"})
	if err == nil {
		t.Fatal("contract mismatch must fail")
	}
	state := c.WriteState()
	if state.Accepted || state.Error == nil || state.Error.Code != string(appruntime.ErrorCodeContractMismatch) {
		t.Fatalf("unsafe mismatch state: %+v", state)
	}
}
