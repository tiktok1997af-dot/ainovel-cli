package desktopui

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/voocel/ainovel-cli/internal/appruntime"
)

type reviewSubscription struct {
	events chan appruntime.DesktopEvent
}

func (s *reviewSubscription) Events() <-chan appruntime.DesktopEvent { return s.events }
func (s *reviewSubscription) Close() error                           { return nil }

type reviewWorkspaceRuntime struct {
	catalog      appruntime.ReviewContractCatalog
	status       appruntime.ReviewStatusResultDTO
	history      appruntime.ReviewHistoryResultDTO
	queries      []appruntime.QueryKind
	commands     []appruntime.CommandRequest
	subscription *reviewSubscription
}

func newReviewWorkspaceRuntime() *reviewWorkspaceRuntime {
	target := appruntime.ReviewTargetDTO{Scope: appruntime.ReviewScopeChapter, Chapter: 3}
	catalog := appruntime.CurrentReviewContractCatalog()
	gates := make([]appruntime.ReviewGateResultDTO, 0, len(catalog.Gates))
	for _, definition := range catalog.Gates {
		gates = append(gates, appruntime.ReviewGateResultDTO{
			GateID:     definition.ID,
			State:      appruntime.ReviewGatePass,
			Actionable: false,
		})
	}
	return &reviewWorkspaceRuntime{
		catalog: catalog,
		status: appruntime.ReviewStatusResultDTO{
			ReviewID: "review-3",
			Target:   target,
			State:    appruntime.ReviewRunCompleted,
			Overall:  appruntime.ReviewGatePass,
			Verdict:  "pass",
			Freshness: appruntime.ReviewFreshnessDTO{
				Fingerprint: "fingerprint-3",
				Revisions: []appruntime.ReviewRevisionRefDTO{{
					Chapter:       3,
					Revision:      7,
					ContentSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				}},
			},
			Gates: gates,
		},
		history: appruntime.ReviewHistoryResultDTO{
			Items: []appruntime.ReviewHistoryItemDTO{{
				ReviewID:    "review-3",
				Target:      target,
				State:       appruntime.ReviewRunCompleted,
				Overall:     appruntime.ReviewGatePass,
				Fingerprint: "fingerprint-3",
			}},
			Total: 1,
			Limit: 100,
		},
		subscription: &reviewSubscription{events: make(chan appruntime.DesktopEvent, 8)},
	}
}

func (f *reviewWorkspaceRuntime) Snapshot(context.Context) (appruntime.DesktopSnapshot, error) {
	return testSnapshot(1, "Novel"), nil
}

func (f *reviewWorkspaceRuntime) Query(_ context.Context, req appruntime.QueryRequest) (appruntime.QueryResult, error) {
	f.queries = append(f.queries, req.Kind)
	var data []byte
	switch req.Kind {
	case appruntime.QueryReviewCatalog:
		data, _ = json.Marshal(appruntime.ReviewCatalogResultDTO{Contract: f.catalog})
	case appruntime.QueryReviewStatus:
		var query appruntime.ReviewStatusQuery
		_ = json.Unmarshal(req.Payload, &query)
		status := f.status
		status.Target = query.Target
		data, _ = json.Marshal(status)
	case appruntime.QueryReviewHistory:
		var query appruntime.ReviewHistoryQuery
		_ = json.Unmarshal(req.Payload, &query)
		history := f.history
		history.Items = append([]appruntime.ReviewHistoryItemDTO(nil), history.Items...)
		for i := range history.Items {
			history.Items[i].Target = query.Target
		}
		data, _ = json.Marshal(history)
	}
	return appruntime.QueryResult{
		ContractVersion: appruntime.ContractVersion,
		Kind:            req.Kind,
		Data:            data,
	}, nil
}

func (f *reviewWorkspaceRuntime) Dispatch(_ context.Context, cmd appruntime.CommandRequest) (appruntime.CommandResult, error) {
	f.commands = append(f.commands, cmd)
	result := appruntime.CommandResult{
		ContractVersion: appruntime.ContractVersion,
		CommandID:       cmd.ID,
		Accepted:        true,
		Status:          "queued",
	}
	if cmd.Kind == appruntime.CommandReviewPromoteOfficial {
		var payload appruntime.ReviewPromoteOfficialCommandPayload
		_ = json.Unmarshal(cmd.Payload, &payload)
		result.Status = "completed"
		result.Data, _ = json.Marshal(appruntime.ReviewPromoteOfficialResultDTO{
			Target:            payload.Target,
			Revisions:         payload.ExpectedRevisions,
			ReviewFingerprint: payload.ExpectedFingerprint,
			PromotedAt:        time.Unix(1, 0).UTC(),
		})
		return result, nil
	}

	var target appruntime.ReviewTargetDTO
	switch cmd.Kind {
	case appruntime.CommandReviewRun:
		var payload appruntime.ReviewRunCommandPayload
		_ = json.Unmarshal(cmd.Payload, &payload)
		target = payload.Target
	case appruntime.CommandReviewRepair:
		var payload appruntime.ReviewRepairCommandPayload
		_ = json.Unmarshal(cmd.Payload, &payload)
		target = payload.Target
	case appruntime.CommandReviewRerun:
		var payload appruntime.ReviewRerunCommandPayload
		_ = json.Unmarshal(cmd.Payload, &payload)
		target = payload.Target
	}
	result.Data, _ = json.Marshal(appruntime.ReviewCommandResultDTO{
		Target: target,
		State:  appruntime.ReviewRunQueued,
		Action: string(cmd.Kind),
	})
	return result, nil
}

func (f *reviewWorkspaceRuntime) Subscribe(context.Context, appruntime.EventCursor) (appruntime.EventSubscription, error) {
	return f.subscription, nil
}

func (f *reviewWorkspaceRuntime) Close(context.Context) error { return nil }

func TestOpenReviewProjectsFrozenCatalogStatusAndHistory(t *testing.T) {
	runtime := newReviewWorkspaceRuntime()
	controller := NewController(runtime, 1440)
	target := appruntime.ReviewTargetDTO{Scope: appruntime.ReviewScopeChapter, Chapter: 3}
	if err := controller.OpenReview(context.Background(), target); err != nil {
		t.Fatalf("OpenReview: %v", err)
	}
	if controller.Shell().Route != RouteReview {
		t.Fatalf("route=%q, want review", controller.Shell().Route)
	}
	state := controller.Review()
	if state.Load != LoadReady || state.Status.ReviewID != "review-3" || state.HistoryTotal != 1 {
		t.Fatalf("unexpected Review state: %+v", state)
	}
	wantCatalog := appruntime.ReviewGateCatalog()
	if len(state.Catalog.Gates) != len(wantCatalog) || len(state.Status.Gates) != len(wantCatalog) {
		t.Fatalf("gate projection sizes catalog=%d status=%d want=%d", len(state.Catalog.Gates), len(state.Status.Gates), len(wantCatalog))
	}
	for i, definition := range wantCatalog {
		if state.Catalog.Gates[i].ID != definition.ID || state.Status.Gates[i].GateID != definition.ID {
			t.Fatalf("gate[%d]=%q/%q want=%q", i, state.Catalog.Gates[i].ID, state.Status.Gates[i].GateID, definition.ID)
		}
	}
	wantQueries := []appruntime.QueryKind{appruntime.QueryReviewCatalog, appruntime.QueryReviewStatus, appruntime.QueryReviewHistory}
	if len(runtime.queries) != len(wantQueries) {
		t.Fatalf("queries=%v, want=%v", runtime.queries, wantQueries)
	}
	for i := range wantQueries {
		if runtime.queries[i] != wantQueries[i] {
			t.Fatalf("query[%d]=%q want=%q", i, runtime.queries[i], wantQueries[i])
		}
	}
}

func TestReviewCommandsUseAuthoritativeCASAndNeverClaimRunOwnership(t *testing.T) {
	runtime := newReviewWorkspaceRuntime()
	controller := NewController(runtime, 1440)
	target := appruntime.ReviewTargetDTO{Scope: appruntime.ReviewScopeChapter, Chapter: 3}
	if err := controller.OpenReview(context.Background(), target); err != nil {
		t.Fatal(err)
	}

	if err := controller.RunReview(context.Background()); err != nil {
		t.Fatalf("RunReview: %v", err)
	}
	if err := controller.RepairReview(context.Background(), []appruntime.ReviewGateID{appruntime.ReviewGateContinuity}, []int{3}); err != nil {
		t.Fatalf("RepairReview: %v", err)
	}
	if err := controller.RerunReview(context.Background()); err != nil {
		t.Fatalf("RerunReview: %v", err)
	}
	if len(runtime.commands) != 3 {
		t.Fatalf("command count=%d, want 3", len(runtime.commands))
	}
	for _, cmd := range runtime.commands {
		if cmd.RunID != "" || cmd.TaskID != "" || cmd.Resource != "" {
			t.Fatalf("Review UI claimed core-owned run fields: %+v", cmd)
		}
	}

	var runPayload appruntime.ReviewRunCommandPayload
	_ = json.Unmarshal(runtime.commands[0].Payload, &runPayload)
	if runtime.commands[0].Kind != appruntime.CommandReviewRun || runPayload.ExpectedFingerprint != "fingerprint-3" {
		t.Fatalf("run command=%+v payload=%+v", runtime.commands[0], runPayload)
	}
	var repairPayload appruntime.ReviewRepairCommandPayload
	_ = json.Unmarshal(runtime.commands[1].Payload, &repairPayload)
	if runtime.commands[1].Kind != appruntime.CommandReviewRepair || repairPayload.ExpectedFingerprint != "fingerprint-3" || len(repairPayload.GateIDs) != 1 || len(repairPayload.Chapters) != 1 {
		t.Fatalf("repair command=%+v payload=%+v", runtime.commands[1], repairPayload)
	}
	var rerunPayload appruntime.ReviewRerunCommandPayload
	_ = json.Unmarshal(runtime.commands[2].Payload, &rerunPayload)
	if runtime.commands[2].Kind != appruntime.CommandReviewRerun || rerunPayload.ExpectedFingerprint != "fingerprint-3" {
		t.Fatalf("rerun command=%+v payload=%+v", runtime.commands[2], rerunPayload)
	}
}

func TestPromoteUsesExactFreshnessRefsAndReceiptStaysTransient(t *testing.T) {
	runtime := newReviewWorkspaceRuntime()
	controller := NewController(runtime, 1440)
	target := appruntime.ReviewTargetDTO{Scope: appruntime.ReviewScopeChapter, Chapter: 3}
	if err := controller.OpenReview(context.Background(), target); err != nil {
		t.Fatal(err)
	}
	if err := controller.PromoteReviewOfficial(context.Background()); err != nil {
		t.Fatalf("PromoteReviewOfficial: %v", err)
	}
	cmd := runtime.commands[len(runtime.commands)-1]
	if cmd.Kind != appruntime.CommandReviewPromoteOfficial || cmd.RunID != "" || cmd.TaskID != "" || cmd.Resource != "" {
		t.Fatalf("unexpected promotion command: %+v", cmd)
	}
	var payload appruntime.ReviewPromoteOfficialCommandPayload
	_ = json.Unmarshal(cmd.Payload, &payload)
	if payload.ExpectedFingerprint != "fingerprint-3" || len(payload.ExpectedRevisions) != 1 || payload.ExpectedRevisions[0].Revision != 7 {
		t.Fatalf("promotion CAS payload=%+v", payload)
	}
	if controller.Review().PromotionReceipt == nil || controller.Review().PromotionReceipt.ReviewFingerprint != "fingerprint-3" {
		t.Fatalf("missing transient promotion receipt: %+v", controller.Review().PromotionReceipt)
	}

	other := appruntime.ReviewTargetDTO{Scope: appruntime.ReviewScopeChapter, Chapter: 4}
	if err := controller.OpenReview(context.Background(), other); err != nil {
		t.Fatal(err)
	}
	if controller.Review().PromotionReceipt != nil {
		t.Fatal("promotion receipt leaked across target reload as durable Official truth")
	}
}

func TestReviewCommandAvailabilityComesOnlyFromRuntimeCatalog(t *testing.T) {
	runtime := newReviewWorkspaceRuntime()
	runtime.status.Overall = appruntime.ReviewGateFail
	runtime.status.Verdict = "rewrite"
	controller := NewController(runtime, 1440)
	if err := controller.OpenReview(context.Background(), runtime.status.Target); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []appruntime.CommandKind{
		appruntime.CommandReviewRun,
		appruntime.CommandReviewRepair,
		appruntime.CommandReviewRerun,
		appruntime.CommandReviewPromoteOfficial,
	} {
		if !controller.Review().Supports(kind) {
			t.Fatalf("catalog command %q was hidden by local UI policy", kind)
		}
	}
}

func TestReviewEventsTriggerFreshQueriesInsteadOfPayloadMutation(t *testing.T) {
	runtime := newReviewWorkspaceRuntime()
	controller := NewController(runtime, 1440)
	ctx := context.Background()
	if err := controller.Bootstrap(ctx); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if err := controller.OpenReview(ctx, runtime.status.Target); err != nil {
		t.Fatalf("OpenReview: %v", err)
	}
	before := len(runtime.queries)
	runtime.status.Summary = "fresh-from-query"
	runtime.subscription.events <- appruntime.DesktopEvent{
		ContractVersion: appruntime.ContractVersion,
		Seq:             1,
		Category:        appruntime.ReviewEventCategory,
		Type:            appruntime.EventTypeReviewGate,
		Summary:         "event payload must not become state",
	}
	if err := controller.PumpEvent(ctx); err != nil {
		t.Fatalf("PumpEvent: %v", err)
	}
	if len(runtime.queries) != before+3 {
		t.Fatalf("Review event queries=%d, want %d", len(runtime.queries), before+3)
	}
	if controller.Review().Status.Summary != "fresh-from-query" {
		t.Fatalf("Review state did not reconcile from query: %+v", controller.Review().Status)
	}
}

func TestReviewRunLifecycleEventAlsoRefreshesOpenReview(t *testing.T) {
	runtime := newReviewWorkspaceRuntime()
	controller := NewController(runtime, 1440)
	ctx := context.Background()
	if err := controller.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	if err := controller.OpenReview(ctx, runtime.status.Target); err != nil {
		t.Fatal(err)
	}
	before := len(runtime.queries)
	runtime.subscription.events <- appruntime.DesktopEvent{
		ContractVersion: appruntime.ContractVersion,
		Seq:             2,
		Category:        appruntime.RunEventCategory,
		Type:            "run_state",
		TaskID:          string(appruntime.CommandReviewRerun),
	}
	if err := controller.PumpEvent(ctx); err != nil {
		t.Fatal(err)
	}
	if len(runtime.queries) != before+3 {
		t.Fatalf("Review run event did not trigger authoritative refresh: before=%d after=%d", before, len(runtime.queries))
	}
}
