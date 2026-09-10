package desktopui

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/voocel/ainovel-cli/internal/appruntime"
)

type knowledgeRuntime struct {
	queries       []appruntime.QueryRequest
	dispatchCalls int
	badContract   bool
}

func (r *knowledgeRuntime) Snapshot(context.Context) (appruntime.DesktopSnapshot, error) {
	return testSnapshot(7, "Novel"), nil
}

func (r *knowledgeRuntime) Query(_ context.Context, req appruntime.QueryRequest) (appruntime.QueryResult, error) {
	r.queries = append(r.queries, req)
	var value any
	switch req.Kind {
	case appruntime.QueryKnowledgeContext:
		value = appruntime.KnowledgeContextResultDTO{
			Chapter: 3,
			Scope:   "overview",
			Sections: []appruntime.KnowledgeSectionDTO{{
				Name:  "continuity",
				Items: []appruntime.KnowledgeItemDTO{{Kind: "fact", Title: "Current state"}},
			}},
		}
	case appruntime.QueryKnowledgeCanon:
		value = appruntime.KnowledgeCanonResultDTO{
			Facts:  []appruntime.CanonFactDTO{{Kind: "state", Subject: "hero", Value: "ready"}},
			Offset: 0,
			Limit:  KnowledgeWorkspaceQueryLimit,
			Total:  1,
		}
	case appruntime.QueryKnowledgeCharacters:
		value = appruntime.KnowledgeCharactersResultDTO{
			Items:  []appruntime.CharacterViewDTO{{Name: "Hero", Origin: "core"}},
			Offset: 0,
			Limit:  KnowledgeWorkspaceQueryLimit,
			Total:  1,
		}
	case appruntime.QueryKnowledgeWorld:
		value = appruntime.KnowledgeWorldResultDTO{
			Rules: []appruntime.WorldRuleViewDTO{{Category: "magic", Rule: "Rule A"}},
		}
	case appruntime.QueryKnowledgeTimeline:
		value = appruntime.KnowledgeTimelineResultDTO{
			Events: []appruntime.TimelineEventViewDTO{{Chapter: 2, Event: "Arrival"}},
			Total:  1,
		}
	default:
		type unsupported struct{}
		value = unsupported{}
	}
	data, err := json.Marshal(value)
	if err != nil {
		return appruntime.QueryResult{}, err
	}
	contract := appruntime.ContractVersion
	if r.badContract {
		contract = "unexpected.contract"
	}
	return appruntime.QueryResult{ContractVersion: contract, Kind: req.Kind, Data: data}, nil
}

func (r *knowledgeRuntime) Dispatch(context.Context, appruntime.CommandRequest) (appruntime.CommandResult, error) {
	r.dispatchCalls++
	return appruntime.CommandResult{}, nil
}

func (r *knowledgeRuntime) Subscribe(context.Context, appruntime.EventCursor) (appruntime.EventSubscription, error) {
	return nil, nil
}

func (r *knowledgeRuntime) Close(context.Context) error { return nil }

func readyKnowledgeController(runtime *knowledgeRuntime) *Controller {
	controller := NewController(runtime, 1440)
	controller.shell.BeginSnapshot("ready")
	controller.shell.AcceptSnapshot("ready", testSnapshot(7, "Novel"))
	return controller
}

func TestOpenKnowledgeStudioLoadsOnlyBoundedContextRead(t *testing.T) {
	runtime := &knowledgeRuntime{}
	controller := readyKnowledgeController(runtime)

	if err := controller.OpenKnowledgeStudio(context.Background()); err != nil {
		t.Fatalf("OpenKnowledgeStudio() error = %v", err)
	}
	if controller.shell.Route != RouteKnowledge || controller.shell.Knowledge.Tab != KnowledgeTabContext {
		t.Fatalf("unexpected knowledge route/tab: %q/%q", controller.shell.Route, controller.shell.Knowledge.Tab)
	}
	if controller.shell.Knowledge.Load != LoadReady || controller.shell.Knowledge.Context == nil {
		t.Fatalf("context was not projected: %+v", controller.shell.Knowledge)
	}
	if len(runtime.queries) != 1 || runtime.queries[0].Kind != appruntime.QueryKnowledgeContext {
		t.Fatalf("queries = %+v, want one context query", runtime.queries)
	}
	var payload appruntime.KnowledgeContextQuery
	if err := json.Unmarshal(runtime.queries[0].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Chapter != 3 || payload.Scope != "overview" || payload.MaxItems != KnowledgeWorkspaceQueryLimit {
		t.Fatalf("unexpected context payload: %+v", payload)
	}
	if runtime.dispatchCalls != 0 {
		t.Fatalf("G04.5 must not dispatch mutations/lifecycle commands: %d", runtime.dispatchCalls)
	}
}

func TestKnowledgeStudioUsesExactlyFiveLockedKnowledgeQueries(t *testing.T) {
	runtime := &knowledgeRuntime{}
	controller := readyKnowledgeController(runtime)
	if !controller.shell.SelectRoute(RouteKnowledge) {
		t.Fatal("knowledge route should be enabled")
	}

	calls := []func() error{
		func() error { return controller.LoadKnowledgeContext(context.Background(), 4, "writer") },
		func() error { return controller.LoadKnowledgeCanon(context.Background(), 0, 4, "all") },
		func() error { return controller.LoadKnowledgeCharacters(context.Background(), 0, "all") },
		func() error {
			return controller.LoadKnowledgeWorld(context.Background(), []string{"rules", "foreshadow"})
		},
		func() error { return controller.LoadKnowledgeTimeline(context.Background(), 1, 20) },
	}
	for i, call := range calls {
		if err := call(); err != nil {
			t.Fatalf("knowledge call %d error = %v", i, err)
		}
	}

	want := []appruntime.QueryKind{
		appruntime.QueryKnowledgeContext,
		appruntime.QueryKnowledgeCanon,
		appruntime.QueryKnowledgeCharacters,
		appruntime.QueryKnowledgeWorld,
		appruntime.QueryKnowledgeTimeline,
	}
	if len(runtime.queries) != len(want) {
		t.Fatalf("query count = %d, want %d", len(runtime.queries), len(want))
	}
	for i, kind := range want {
		if runtime.queries[i].Kind != kind || runtime.queries[i].ContractVersion != appruntime.ContractVersion {
			t.Fatalf("query[%d] = %+v, want %q via current contract", i, runtime.queries[i], kind)
		}
	}
	if runtime.dispatchCalls != 0 {
		t.Fatalf("read-only Knowledge Studio crossed write plane: %d dispatches", runtime.dispatchCalls)
	}

	var canon appruntime.KnowledgeCanonQuery
	if err := json.Unmarshal(runtime.queries[1].Payload, &canon); err != nil {
		t.Fatal(err)
	}
	if canon.Limit != KnowledgeWorkspaceQueryLimit {
		t.Fatalf("canon limit = %d, want %d", canon.Limit, KnowledgeWorkspaceQueryLimit)
	}
	var characters appruntime.KnowledgeCharactersQuery
	if err := json.Unmarshal(runtime.queries[2].Payload, &characters); err != nil {
		t.Fatal(err)
	}
	if characters.Limit != KnowledgeWorkspaceQueryLimit {
		t.Fatalf("characters limit = %d, want %d", characters.Limit, KnowledgeWorkspaceQueryLimit)
	}
	var world appruntime.KnowledgeWorldQuery
	if err := json.Unmarshal(runtime.queries[3].Payload, &world); err != nil {
		t.Fatal(err)
	}
	if world.Limit != KnowledgeWorkspaceQueryLimit {
		t.Fatalf("world limit = %d, want %d", world.Limit, KnowledgeWorkspaceQueryLimit)
	}
	var timeline appruntime.KnowledgeTimelineQuery
	if err := json.Unmarshal(runtime.queries[4].Payload, &timeline); err != nil {
		t.Fatal(err)
	}
	if timeline.Limit != KnowledgeWorkspaceQueryLimit {
		t.Fatalf("timeline limit = %d, want %d", timeline.Limit, KnowledgeWorkspaceQueryLimit)
	}
}

func TestKnowledgeTabSwitchSuppressesStaleResponse(t *testing.T) {
	shell := NewShell(1440)
	shell.BeginSnapshot("ready")
	shell.AcceptSnapshot("ready", testSnapshot(7, "Novel"))
	shell.SelectRoute(RouteKnowledge)

	ticket := knowledgeQueryTicket{
		ID:               "old-context",
		Kind:             appruntime.QueryKnowledgeContext,
		SnapshotRevision: shell.SnapshotRevision,
		Route:            RouteKnowledge,
		Tab:              KnowledgeTabContext,
	}
	shell.Knowledge.begin(ticket)
	if !shell.Knowledge.SelectTab(KnowledgeTabCanon) {
		t.Fatal("canon tab should be selectable")
	}
	if shell.Knowledge.current(ticket, shell) {
		t.Fatal("context response must become stale after tab switch")
	}
	shell.Knowledge.discard(ticket)
	if shell.Knowledge.Context != nil {
		t.Fatal("stale context response must not project data")
	}
}

func TestKnowledgeSnapshotRevisionSuppressesStaleResponse(t *testing.T) {
	shell := NewShell(1440)
	shell.BeginSnapshot("ready")
	shell.AcceptSnapshot("ready", testSnapshot(7, "Novel"))
	shell.SelectRoute(RouteKnowledge)
	ticket := knowledgeQueryTicket{
		ID:               "context-7",
		Kind:             appruntime.QueryKnowledgeContext,
		SnapshotRevision: 7,
		Route:            RouteKnowledge,
		Tab:              KnowledgeTabContext,
	}
	shell.Knowledge.begin(ticket)
	shell.BeginSnapshot("newer")
	shell.AcceptSnapshot("newer", testSnapshot(8, "Novel"))
	if shell.Knowledge.current(ticket, shell) {
		t.Fatal("older knowledge result must not overwrite a newer shell revision")
	}
}

func TestInvalidKnowledgeSelectorDoesNotReachAppRuntime(t *testing.T) {
	runtime := &knowledgeRuntime{}
	controller := readyKnowledgeController(runtime)
	controller.shell.SelectRoute(RouteKnowledge)

	if err := controller.LoadKnowledgeTimeline(context.Background(), 20, 10); err == nil {
		t.Fatal("invalid timeline range should fail")
	}
	if err := controller.LoadKnowledgeCharacters(context.Background(), 0, "secret"); err == nil {
		t.Fatal("unsupported character scope should fail")
	}
	if len(runtime.queries) != 0 {
		t.Fatalf("invalid selectors reached AppRuntime: %d queries", len(runtime.queries))
	}
	if controller.shell.Knowledge.Load != LoadValidationError {
		t.Fatalf("load = %q, want validation_error", controller.shell.Knowledge.Load)
	}
}

func TestKnowledgeProtocolMismatchStaysWorkspaceScoped(t *testing.T) {
	runtime := &knowledgeRuntime{badContract: true}
	controller := readyKnowledgeController(runtime)
	controller.shell.SelectRoute(RouteKnowledge)

	if err := controller.LoadKnowledgeCanon(context.Background(), 0, 0, "all"); err == nil {
		t.Fatal("contract mismatch should fail")
	}
	if controller.shell.Knowledge.Error == nil || controller.shell.Knowledge.Load != LoadRuntimeError {
		t.Fatalf("knowledge protocol failure not projected: %+v", controller.shell.Knowledge)
	}
	if controller.shell.Error != nil {
		t.Fatalf("workspace query error must not replace shell snapshot error state: %+v", controller.shell.Error)
	}
}
