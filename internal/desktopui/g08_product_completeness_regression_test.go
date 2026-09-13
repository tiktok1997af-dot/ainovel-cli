package desktopui

import (
	"context"
	"strings"
	"testing"

	"github.com/voocel/ainovel-cli/internal/appruntime"
)

func TestG0811CrossWorkspaceNavigationPreservesFolderCreativeAndCoCreateState(t *testing.T) {
	runtime := folderIntegrationRuntime(t)
	controller := NewController(runtime, 1440)
	if err := controller.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}

	controller.Creative().Selected = 9
	controller.Creative().Chapter = 9
	controller.cocreate.Draft = "accepted draft"
	controller.cocreate.Ready = true

	if err := controller.OpenFolderView(context.Background(), 0, "", ""); err != nil {
		t.Fatal(err)
	}
	if err := controller.SelectFolderDocument(context.Background(), "canon"); err != nil {
		t.Fatal(err)
	}

	queryBefore := len(runtime.queryCalls)
	dispatchBefore := runtime.dispatchCalls
	for _, route := range []RouteID{RouteKnowledge, RouteCreative, RouteReview, RouteRunCenter, RouteSettings, RouteProject} {
		if !controller.Shell().SelectRoute(route) {
			t.Fatalf("route %q should remain selectable", route)
		}
	}

	project := controller.Shell().Project
	if project.Tab != ProjectTabFolder || project.SelectedDocumentID != "canon" || project.SelectedDocument == nil {
		t.Fatalf("Folder View state was overwritten by sibling navigation: %+v", project)
	}
	if controller.Creative().Selected != 9 || controller.Creative().Chapter != 9 {
		t.Fatalf("Creative state was overwritten by sibling navigation: %+v", controller.Creative())
	}
	if controller.cocreate.Draft != "accepted draft" || !controller.cocreate.Ready {
		t.Fatalf("CoCreate presentation state was overwritten by sibling navigation: %+v", controller.cocreate)
	}
	if len(runtime.queryCalls) != queryBefore || runtime.dispatchCalls != dispatchBefore {
		t.Fatalf("route changes issued hidden runtime work: queries=%d/%d dispatch=%d/%d", len(runtime.queryCalls), queryBefore, runtime.dispatchCalls, dispatchBefore)
	}

	render := controller.Shell().FolderViewRender()
	if !render.Active || render.Document == nil || render.Document.ID != "canon" || !render.Document.ReadOnly {
		t.Fatalf("returning to Project did not restore read-only Folder View render state: %+v", render)
	}
}

func TestG0811ResponsiveFolderRenderingIsPresentationOnly(t *testing.T) {
	runtime := folderIntegrationRuntime(t)
	controller := NewController(runtime, 1440)
	if err := controller.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := controller.OpenFolderView(context.Background(), 0, "", ""); err != nil {
		t.Fatal(err)
	}
	if err := controller.SelectFolderDocument(context.Background(), "canon"); err != nil {
		t.Fatal(err)
	}

	queryBefore := len(runtime.queryCalls)
	dispatchBefore := runtime.dispatchCalls
	revision := controller.Shell().SnapshotRevision

	cases := []struct {
		width int
		mode  FolderViewRenderMode
		class ViewportClass
	}{
		{1440, FolderViewRenderSplit, ViewportDesktop},
		{900, FolderViewRenderStackedFullWidth, ViewportTablet},
		{640, FolderViewRenderStackedFullWidth, ViewportMobile},
		{1200, FolderViewRenderSplit, ViewportDesktop},
	}
	for _, tc := range cases {
		controller.Shell().Resize(tc.width)
		render := controller.Shell().FolderViewRender()
		if !render.Active || render.Layout.Mode != tc.mode || render.Layout.Viewport != tc.class {
			t.Fatalf("width %d render = %+v", tc.width, render)
		}
		if render.Document == nil || render.Document.ID != "canon" || !render.Document.ReadOnly {
			t.Fatalf("width %d lost selected read-only document: %+v", tc.width, render.Document)
		}
	}

	if controller.Shell().SnapshotRevision != revision || controller.Shell().Project.SelectedDocumentID != "canon" {
		t.Fatalf("resize/render mutated authoritative workspace state: %+v", controller.Shell())
	}
	if len(runtime.queryCalls) != queryBefore || runtime.dispatchCalls != dispatchBefore {
		t.Fatalf("resize/render issued hidden runtime work: queries=%d/%d dispatch=%d/%d", len(runtime.queryCalls), queryBefore, runtime.dispatchCalls, dispatchBefore)
	}

	if !controller.Shell().SelectRoute(RouteSettings) {
		t.Fatal("Settings route should remain selectable")
	}
	if render := controller.Shell().FolderViewRender(); render.Active {
		t.Fatalf("Folder View renderer must be inactive outside Project route: %+v", render)
	}
	if !controller.Shell().SelectRoute(RouteProject) {
		t.Fatal("Project route should remain selectable")
	}
	if render := controller.Shell().FolderViewRender(); !render.Active || render.Document == nil || render.Document.ID != "canon" {
		t.Fatalf("Folder View state was not preserved after returning to Project: %+v", render)
	}
}

func TestG0811PrimaryNavigationAndLifecycleControlsRemainComplete(t *testing.T) {
	wantRoutes := map[RouteID]bool{
		RouteOverview:  true,
		RouteProject:   true,
		RouteCreative:  true,
		RouteKnowledge: true,
		RouteReview:    true,
		RouteRunCenter: true,
		RouteSettings:  true,
	}
	navigation := PrimaryNavigation()
	if len(navigation) != len(wantRoutes) {
		t.Fatalf("primary navigation count = %d, want %d; CoCreate/Folder View must not become second-shell routes", len(navigation), len(wantRoutes))
	}
	for _, item := range navigation {
		if !wantRoutes[item.ID] || !item.Enabled || strings.TrimSpace(item.Label) == "" {
			t.Fatalf("primary navigation drift: %+v", item)
		}
	}

	controls := activatedLifecycleControls(string(appruntime.LifecycleRunning), false)
	seen := map[string]bool{}
	for _, control := range controls {
		seen[control.ID] = true
	}
	for _, required := range []string{"start", "pause", "resume", "stop"} {
		if !seen[required] {
			t.Fatalf("required lifecycle control %q disappeared: %+v", required, controls)
		}
	}
}
