package desktopui

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/voocel/ainovel-cli/internal/appruntime"
)

func TestG04FinalDocumentsSurfaceIsBoundedCatalogReadOnly(t *testing.T) {
	runtime := &workspaceRuntime{
		snapshot: projectSnapshot(31),
		results: map[appruntime.QueryKind]appruntime.QueryResult{
			appruntime.QueryDocumentsList: projectQueryResult(t, appruntime.QueryDocumentsList, appruntime.DocumentsListResultDTO{
				Items: []appruntime.DocumentSummaryDTO{{
					ID: "project-book", Kind: appruntime.DocumentKindProject, Path: "project/book.json",
					Title: "Project", ContentType: "application/json", ReadOnly: true, SizeBytes: 128,
				}},
				Offset: 0, Limit: ProjectWorkspaceQueryLimit, Total: 1,
			}),
			appruntime.QueryDocumentsGet: projectQueryResult(t, appruntime.QueryDocumentsGet, appruntime.DocumentsGetResultDTO{
				Document: appruntime.DocumentSummaryDTO{
					ID: "project-book", Kind: appruntime.DocumentKindProject, Path: "project/book.json",
					Title: "Project", ContentType: "application/json", ReadOnly: true, SizeBytes: 128,
				},
				Content: `{"title":"Novel"}`,
			}),
		},
	}
	controller := NewController(runtime, 1440)
	if err := controller.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !controller.Shell().SelectRoute(RouteProject) {
		t.Fatal("project route should be enabled")
	}
	if err := controller.LoadDocumentsPage(context.Background(), 0, appruntime.DocumentKindProject, "project"); err != nil {
		t.Fatal(err)
	}
	if err := controller.LoadDocument(context.Background(), "project-book"); err != nil {
		t.Fatal(err)
	}

	if len(runtime.queryCalls) != 2 {
		t.Fatalf("document query calls = %d, want 2", len(runtime.queryCalls))
	}
	if runtime.queryCalls[0].Kind != appruntime.QueryDocumentsList || runtime.queryCalls[1].Kind != appruntime.QueryDocumentsGet {
		t.Fatalf("unexpected document query catalog: %+v", runtime.queryCalls)
	}
	var listReq appruntime.DocumentsListQuery
	if err := json.Unmarshal(runtime.queryCalls[0].Payload, &listReq); err != nil {
		t.Fatal(err)
	}
	if listReq.Offset != 0 || listReq.Limit != ProjectWorkspaceQueryLimit || listReq.Kind != appruntime.DocumentKindProject || listReq.Prefix != "project" {
		t.Fatalf("documents.list payload = %+v", listReq)
	}
	var getReq appruntime.DocumentsGetQuery
	if err := json.Unmarshal(runtime.queryCalls[1].Payload, &getReq); err != nil {
		t.Fatal(err)
	}
	if getReq.ID != "project-book" {
		t.Fatalf("documents.get payload = %+v", getReq)
	}
	project := controller.Shell().Project
	if project.Tab != ProjectTabDocuments || project.Documents.Total != 1 || project.SelectedDocument == nil {
		t.Fatalf("document workspace projection incomplete: %+v", project)
	}
	if !project.SelectedDocument.Document.ReadOnly || project.SelectedDocument.Content == "" {
		t.Fatalf("document must remain a populated read-only projection: %+v", project.SelectedDocument)
	}
	if runtime.dispatchCalls != 0 {
		t.Fatalf("document surface must never dispatch, got %d writes", runtime.dispatchCalls)
	}

	before := len(runtime.queryCalls)
	if err := controller.LoadDocument(context.Background(), "   "); err == nil {
		t.Fatal("blank document id should fail local validation")
	}
	if len(runtime.queryCalls) != before {
		t.Fatal("invalid document selector reached AppRuntime.Query")
	}
}

func TestG04FinalDocumentProjectionFailsClosedIfRuntimeClaimsWritable(t *testing.T) {
	runtime := &workspaceRuntime{
		snapshot: projectSnapshot(32),
		results: map[appruntime.QueryKind]appruntime.QueryResult{
			appruntime.QueryDocumentsGet: projectQueryResult(t, appruntime.QueryDocumentsGet, appruntime.DocumentsGetResultDTO{
				Document: appruntime.DocumentSummaryDTO{ID: "unsafe-doc", Kind: appruntime.DocumentKindProject, Path: "project/book.json", ReadOnly: false},
				Content:  "unsafe",
			}),
		},
	}
	controller := NewController(runtime, 1440)
	if err := controller.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	controller.Shell().SelectRoute(RouteProject)
	if err := controller.LoadDocument(context.Background(), "unsafe-doc"); err == nil {
		t.Fatal("writable document projection must fail closed")
	}
	if controller.Shell().Project.SelectedDocument != nil || controller.Shell().Project.Load != LoadRuntimeError {
		t.Fatalf("unsafe document projection leaked into UI: %+v", controller.Shell().Project)
	}
}

func TestG04FinalCreativeTicketSuppressesObsoleteSelectionAndRevision(t *testing.T) {
	shell := NewShell(1440)
	shell.Route = RouteCreative
	shell.SnapshotRevision = 40
	state := NewCreativeWorkspaceState()
	first := creativeQueryTicket{ID: "first", SnapshotRevision: 40, Route: RouteCreative, Chapter: 4}
	second := creativeQueryTicket{ID: "second", SnapshotRevision: 40, Route: RouteCreative, Chapter: 5}
	state.begin(first)
	state.begin(second)
	if state.current(first, shell) {
		t.Fatal("superseded creative request remained current")
	}
	if !state.current(second, shell) || state.Selected != 5 {
		t.Fatalf("latest creative request is not current: %+v", state)
	}
	state.applyCanonical(appruntime.ChaptersGetResultDTO{Chapter: 5, Status: "planned"}, false)
	state.finish(second)
	state.discard(first)
	if state.Chapter != 5 || state.Canonical == nil || state.Canonical.Chapter != 5 {
		t.Fatalf("obsolete ticket changed creative projection: %+v", state)
	}

	third := creativeQueryTicket{ID: "third", SnapshotRevision: 40, Route: RouteCreative, Chapter: 6}
	state.begin(third)
	shell.SnapshotRevision = 41
	if state.current(third, shell) {
		t.Fatal("creative response from prior snapshot revision remained current")
	}
	state.discard(third)
}

func TestG04FinalCreativeReconcileDoesNotPullUserBackToOldChapter(t *testing.T) {
	runtime := &creativeWriteRuntime{snapshot: creativeSnapshot("ready"), chapter: creativeChapter()}
	controller := readyCreativeController(runtime)
	controller.Shell().Route = RouteCreative
	controller.Creative().Selected = 5
	controller.Creative().Chapter = 5
	before := len(runtime.queries)
	if err := controller.reconcileCreativeChapter(context.Background(), 4, true); err != nil {
		t.Fatal(err)
	}
	if len(runtime.queries) != before {
		t.Fatal("old chapter reconciliation issued a stale creative query")
	}
	if controller.Creative().Selected != 5 || controller.Creative().Chapter != 5 {
		t.Fatalf("reconcile pulled selection back to old chapter: %+v", controller.Creative())
	}
}

func TestG04FinalResponsiveNavigationAndAccessibilitySemantics(t *testing.T) {
	cases := []struct {
		width int
		want  ViewportClass
	}{
		{759, ViewportMobile},
		{760, ViewportTablet},
		{1199, ViewportTablet},
		{1200, ViewportDesktop},
	}
	for _, tc := range cases {
		layout := LayoutForWidth(tc.width, true)
		if layout.Class != tc.want || !layout.MainVisible {
			t.Fatalf("width %d layout = %+v, want class %q with main visible", tc.width, layout, tc.want)
		}
	}

	enabled := map[RouteID]bool{
		RouteOverview:  true,
		RouteProject:   true,
		RouteCreative:  true,
		RouteKnowledge: true,
	}
	seen := map[RouteID]bool{}
	for _, item := range PrimaryNavigation() {
		if strings.TrimSpace(item.Label) == "" || strings.TrimSpace(item.Availability) == "" {
			t.Fatalf("navigation item lacks accessible label/availability semantics: %+v", item)
		}
		if seen[item.ID] {
			t.Fatalf("duplicate navigation id %q", item.ID)
		}
		seen[item.ID] = true
		if item.Enabled != enabled[item.ID] {
			t.Fatalf("route %q enabled=%v, want %v", item.ID, item.Enabled, enabled[item.ID])
		}
	}

	controls := activatedLifecycleControls(string(appruntime.LifecycleRunning), false)
	controlIDs := map[string]bool{}
	for _, control := range controls {
		if strings.TrimSpace(control.ID) == "" || strings.TrimSpace(control.Label) == "" || controlIDs[control.ID] {
			t.Fatalf("lifecycle control lacks unique accessible identity: %+v", control)
		}
		controlIDs[control.ID] = true
		if !control.Enabled && strings.TrimSpace(control.Reason) == "" {
			t.Fatalf("disabled lifecycle control lacks reason: %+v", control)
		}
	}

	shell := NewShell(1440)
	shell.BeginSnapshot("integration")
	if !shell.AcceptSnapshot("integration", testSnapshot(50, "Integrated")) {
		t.Fatal("integration snapshot should be accepted")
	}
	for _, route := range []RouteID{RouteProject, RouteKnowledge, RouteCreative} {
		if !shell.SelectRoute(route) {
			t.Fatalf("locked G04 route %q should be selectable", route)
		}
		shell.Resize(759)
		shell.Resize(1200)
		if shell.SnapshotRevision != 50 || shell.Header.ProjectTitle != "Integrated" {
			t.Fatalf("route/resize discarded authoritative projection: %+v", shell)
		}
	}
	for _, route := range []RouteID{RouteReview, RouteRunCenter, RouteSettings} {
		if shell.SelectRoute(route) {
			t.Fatalf("successor route %q opened during G04 final gate", route)
		}
	}
}

func TestG04FinalConflictStateRemainsStructuredAndAnnounceable(t *testing.T) {
	appErr := &appruntime.AppError{
		Code:      appruntime.ErrorCodeStaleConflict,
		Category:  appruntime.ErrorCategoryConflict,
		Message:   "The project data changed after the action was prepared. Refresh and try again.",
		Retryable: true,
	}
	view := viewErrorFromAppError(appErr)
	if view == nil || view.Code == "" || view.Category != string(appruntime.ErrorCategoryConflict) || strings.TrimSpace(view.Message) == "" || !view.Retryable {
		t.Fatalf("structured conflict cannot be announced safely: %+v", view)
	}
	if loadStateForWriteError(view) != LoadConflict || loadStateForWorkspaceError(view) != LoadConflict {
		t.Fatalf("conflict category lost deterministic load-state mapping: %+v", view)
	}
}
