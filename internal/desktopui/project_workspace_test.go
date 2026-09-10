package desktopui

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/voocel/ainovel-cli/internal/appruntime"
)

type workspaceSubscription struct {
	events chan appruntime.DesktopEvent
}

func (s *workspaceSubscription) Events() <-chan appruntime.DesktopEvent { return s.events }
func (s *workspaceSubscription) Close() error                           { return nil }

type workspaceRuntime struct {
	snapshot      appruntime.DesktopSnapshot
	queryCalls    []appruntime.QueryRequest
	dispatchCalls int
	results       map[appruntime.QueryKind]appruntime.QueryResult
	errs          map[appruntime.QueryKind]error
	sub           *workspaceSubscription
}

func (f *workspaceRuntime) Snapshot(context.Context) (appruntime.DesktopSnapshot, error) {
	return f.snapshot, nil
}

func (f *workspaceRuntime) Query(_ context.Context, req appruntime.QueryRequest) (appruntime.QueryResult, error) {
	f.queryCalls = append(f.queryCalls, req)
	if err := f.errs[req.Kind]; err != nil {
		return appruntime.QueryResult{
			ContractVersion: appruntime.ContractVersion,
			Kind:            req.Kind,
			Error:           appErrorFromTestError(err),
		}, err
	}
	result, ok := f.results[req.Kind]
	if !ok {
		return appruntime.QueryResult{
			ContractVersion: appruntime.ContractVersion,
			Kind:            req.Kind,
			Data:            json.RawMessage(`{}`),
		}, nil
	}
	return result, nil
}

func (f *workspaceRuntime) Dispatch(context.Context, appruntime.CommandRequest) (appruntime.CommandResult, error) {
	f.dispatchCalls++
	return appruntime.CommandResult{}, nil
}

func (f *workspaceRuntime) Subscribe(context.Context, appruntime.EventCursor) (appruntime.EventSubscription, error) {
	if f.sub == nil {
		f.sub = &workspaceSubscription{events: make(chan appruntime.DesktopEvent, 1)}
	}
	return f.sub, nil
}

func (f *workspaceRuntime) Close(context.Context) error { return nil }

func appErrorFromTestError(err error) *appruntime.AppError {
	var appErr *appruntime.AppError
	if errors.As(err, &appErr) {
		return appErr
	}
	return nil
}

func projectQueryResult(t *testing.T, kind appruntime.QueryKind, value any) appruntime.QueryResult {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return appruntime.QueryResult{
		ContractVersion: appruntime.ContractVersion,
		Kind:            kind,
		Data:            raw,
	}
}

func projectSnapshot(revision uint64) appruntime.DesktopSnapshot {
	return appruntime.DesktopSnapshot{
		Contract: appruntime.ContractInfo{
			Version:       appruntime.ContractVersion,
			SchemaVersion: appruntime.SchemaVersion,
		},
		Revision: revision,
		Product:  appruntime.ProductViewSnapshot{Name: "AINOVEL"},
		Project:  appruntime.ProjectViewSnapshot{Title: "Novel", OutputDir: "project"},
	}
}

func fullWorkspaceRuntime(t *testing.T) *workspaceRuntime {
	t.Helper()
	return &workspaceRuntime{
		snapshot: projectSnapshot(7),
		results: map[appruntime.QueryKind]appruntime.QueryResult{
			appruntime.QueryProjectOverview: projectQueryResult(t, appruntime.QueryProjectOverview, appruntime.ProjectOverviewResultDTO{
				FormatVersion:  2,
				Title:          "Novel",
				Premise:        "Premise",
				CurrentChapter: 2,
				TotalWordCount: 3200,
			}),
			appruntime.QueryChaptersList: projectQueryResult(t, appruntime.QueryChaptersList, appruntime.ChaptersListResultDTO{
				Items: []appruntime.ChapterListItemDTO{
					{Chapter: 1, Title: "One", Status: "accepted", WordCount: 1600},
					{Chapter: 2, Title: "Two", Status: "planned"},
				},
				Offset: 0,
				Limit:  ProjectWorkspaceQueryLimit,
				Total:  2,
			}),
			appruntime.QueryChaptersGet: projectQueryResult(t, appruntime.QueryChaptersGet, appruntime.ChaptersGetResultDTO{
				Chapter: 2,
				Title:   "Two",
				Status:  "planned",
			}),
			appruntime.QueryOutlineGet: projectQueryResult(t, appruntime.QueryOutlineGet, appruntime.OutlineGetResultDTO{
				Chapters: []appruntime.OutlineChapterDTO{
					{Chapter: 1, Title: "One"},
					{Chapter: 2, Title: "Two"},
				},
			}),
		},
	}
}

func TestProjectNavigationPreservesG04PointFourAndAllowsG04PointFiveRoute(t *testing.T) {
	nav := PrimaryNavigation()
	enabled := map[RouteID]bool{}
	for _, item := range nav {
		enabled[item.ID] = item.Enabled
	}
	if !enabled[RouteOverview] || !enabled[RouteProject] || !enabled[RouteKnowledge] {
		t.Fatalf("overview/project/knowledge must be enabled: %+v", enabled)
	}
	for _, route := range []RouteID{RouteCreative, RouteReview, RouteRunCenter, RouteSettings} {
		if enabled[route] {
			t.Fatalf("later route %q opened prematurely", route)
		}
	}
}

func TestOpenProjectWorkspaceBindsOnlyApprovedReadPlane(t *testing.T) {
	runtime := fullWorkspaceRuntime(t)
	controller := NewController(runtime, 1440)
	if err := controller.Bootstrap(context.Background()); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	if err := controller.OpenProjectWorkspace(context.Background()); err != nil {
		t.Fatalf("OpenProjectWorkspace() error = %v", err)
	}
	if err := controller.LoadChapter(context.Background(), 2); err != nil {
		t.Fatalf("LoadChapter() error = %v", err)
	}

	wantKinds := []appruntime.QueryKind{
		appruntime.QueryProjectOverview,
		appruntime.QueryChaptersList,
		appruntime.QueryOutlineGet,
		appruntime.QueryChaptersGet,
	}
	if len(runtime.queryCalls) != len(wantKinds) {
		t.Fatalf("query calls = %d, want %d", len(runtime.queryCalls), len(wantKinds))
	}
	for i, want := range wantKinds {
		got := runtime.queryCalls[i]
		if got.Kind != want {
			t.Fatalf("query[%d].Kind = %q, want %q", i, got.Kind, want)
		}
		if got.ContractVersion != appruntime.ContractVersion {
			t.Fatalf("query[%d] contract = %q", i, got.ContractVersion)
		}
	}
	if runtime.dispatchCalls != 0 {
		t.Fatalf("G04.4 must remain read-only, dispatch calls = %d", runtime.dispatchCalls)
	}

	var listReq appruntime.ChaptersListQuery
	if err := json.Unmarshal(runtime.queryCalls[1].Payload, &listReq); err != nil {
		t.Fatal(err)
	}
	if listReq.Offset != 0 || listReq.Limit != ProjectWorkspaceQueryLimit {
		t.Fatalf("chapters.list payload = %+v", listReq)
	}
	var outlineReq appruntime.OutlineGetQuery
	if err := json.Unmarshal(runtime.queryCalls[2].Payload, &outlineReq); err != nil {
		t.Fatal(err)
	}
	if outlineReq.FromChapter != 1 || outlineReq.Limit != ProjectWorkspaceQueryLimit {
		t.Fatalf("outline.get payload = %+v", outlineReq)
	}
	var chapterReq appruntime.ChaptersGetQuery
	if err := json.Unmarshal(runtime.queryCalls[3].Payload, &chapterReq); err != nil {
		t.Fatal(err)
	}
	if chapterReq.Chapter != 2 || chapterReq.IncludeContent {
		t.Fatalf("chapters.get must be metadata/plan only in G04.4: %+v", chapterReq)
	}

	project := controller.Shell().Project
	if controller.Shell().Route != RouteProject || project.Load != LoadReady {
		t.Fatalf("workspace state = route:%q project:%+v", controller.Shell().Route, project)
	}
	if project.Overview == nil || project.Overview.Title != "Novel" || project.Chapters.Total != 2 {
		t.Fatalf("project projection incomplete: %+v", project)
	}
	if project.Outline == nil || len(project.Outline.Chapters) != 2 {
		t.Fatalf("outline projection incomplete: %+v", project.Outline)
	}
	if project.SelectedChapter == nil || project.SelectedChapter.Chapter != 2 || project.Tab != ProjectTabChapters {
		t.Fatalf("chapter projection incomplete: %+v", project)
	}
}

func TestProjectWorkspaceRejectsInvalidSelectorBeforeQuery(t *testing.T) {
	runtime := fullWorkspaceRuntime(t)
	controller := NewController(runtime, 1440)
	controller.shell.Route = RouteProject
	controller.shell.SnapshotRevision = 7

	if err := controller.LoadChapter(context.Background(), 0); err == nil {
		t.Fatal("LoadChapter(0) error = nil")
	}
	if len(runtime.queryCalls) != 0 {
		t.Fatalf("invalid selector reached AppRuntime.Query: %d", len(runtime.queryCalls))
	}
	if controller.Shell().Project.Load != LoadValidationError {
		t.Fatalf("load = %q, want validation_error", controller.Shell().Project.Load)
	}
}

func TestProjectWorkspaceMapsStructuredQueryError(t *testing.T) {
	appErr := &appruntime.AppError{
		Code:      appruntime.ErrorCodeStoreRead,
		Category:  appruntime.ErrorCategoryStore,
		Message:   "Project data could not be read.",
		Retryable: true,
	}
	runtime := &workspaceRuntime{
		snapshot: projectSnapshot(4),
		results:  map[appruntime.QueryKind]appruntime.QueryResult{},
		errs:     map[appruntime.QueryKind]error{appruntime.QueryProjectOverview: appErr},
	}
	controller := NewController(runtime, 1440)
	if err := controller.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := controller.OpenProjectWorkspace(context.Background()); err == nil {
		t.Fatal("OpenProjectWorkspace() error = nil")
	}
	if controller.Shell().Project.Error == nil ||
		controller.Shell().Project.Error.Code != string(appruntime.ErrorCodeStoreRead) ||
		controller.Shell().Project.Load != LoadRuntimeError {
		t.Fatalf("structured query error not projected: %+v", controller.Shell().Project)
	}
	if controller.Shell().Load != LoadReady {
		t.Fatalf("workspace query failure must not destroy shell snapshot load: %q", controller.Shell().Load)
	}
}

func TestProjectWorkspaceSuppressesStaleQueryTicket(t *testing.T) {
	shell := NewShell(1440)
	shell.Route = RouteProject
	shell.SnapshotRevision = 10
	first := workspaceQueryTicket{
		ID:               "first",
		Kind:             appruntime.QueryChaptersList,
		SnapshotRevision: 10,
		Route:            RouteProject,
	}
	second := first
	second.ID = "second"

	shell.Project.begin(first)
	shell.Project.begin(second)
	if shell.Project.current(first, shell) {
		t.Fatal("superseded request remained current")
	}
	if !shell.Project.current(second, shell) {
		t.Fatal("latest request is not current")
	}

	shell.SnapshotRevision = 11
	if shell.Project.current(second, shell) {
		t.Fatal("request from prior snapshot revision remained current")
	}
	shell.Project.discard(second)
	if len(shell.Project.pending) != 0 {
		t.Fatalf("stale pending request not discarded: %+v", shell.Project.pending)
	}
}

func TestProjectWorkspaceRejectsMismatchedQueryContract(t *testing.T) {
	runtime := fullWorkspaceRuntime(t)
	result := runtime.results[appruntime.QueryProjectOverview]
	result.ContractVersion = "other.desktop.v1"
	runtime.results[appruntime.QueryProjectOverview] = result

	controller := NewController(runtime, 1440)
	if err := controller.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := controller.OpenProjectWorkspace(context.Background()); err == nil {
		t.Fatal("contract mismatch error = nil")
	}
	viewErr := controller.Shell().Project.Error
	if viewErr == nil || viewErr.Code != string(appruntime.ErrorCodeInternal) ||
		viewErr.Message != "An internal runtime error occurred." {
		t.Fatalf("unsafe/incorrect protocol error: %+v", viewErr)
	}
}

func TestProjectWorkspaceEmptyProjectionUsesEmptyState(t *testing.T) {
	runtime := &workspaceRuntime{
		snapshot: projectSnapshot(2),
		results: map[appruntime.QueryKind]appruntime.QueryResult{
			appruntime.QueryProjectOverview: projectQueryResult(t, appruntime.QueryProjectOverview, appruntime.ProjectOverviewResultDTO{}),
			appruntime.QueryChaptersList:    projectQueryResult(t, appruntime.QueryChaptersList, appruntime.ChaptersListResultDTO{}),
			appruntime.QueryOutlineGet:      projectQueryResult(t, appruntime.QueryOutlineGet, appruntime.OutlineGetResultDTO{}),
		},
	}
	controller := NewController(runtime, 1440)
	if err := controller.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := controller.OpenProjectWorkspace(context.Background()); err != nil {
		t.Fatal(err)
	}
	if controller.Shell().Project.Load != LoadEmpty {
		t.Fatalf("empty projection load = %q", controller.Shell().Project.Load)
	}
}
