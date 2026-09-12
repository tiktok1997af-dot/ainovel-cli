package desktopui

import (
	"testing"

	"github.com/voocel/ainovel-cli/internal/appruntime"
)

func testSnapshot(revision uint64, title string) appruntime.DesktopSnapshot {
	return appruntime.DesktopSnapshot{
		Contract: appruntime.CurrentContract(),
		Revision: revision,
		Product:  appruntime.ProductViewSnapshot{Name: "AINOVEL"},
		Project: appruntime.ProjectViewSnapshot{
			Title: title,
			OutputDir: func() string {
				if title == "" {
					return ""
				}
				return "project"
			}(),
		},
		Runtime:        appruntime.RuntimeViewSnapshot{State: "ready", Status: "Ready"},
		CurrentChapter: appruntime.ChapterViewSnapshot{Current: 3, Total: 100},
		Browser:        appruntime.BrowserViewSnapshot{State: string(appruntime.BrowserReady), Site: "gemini"},
		Recovery:       appruntime.RecoveryViewSnapshot{Label: "checkpoint", CanResume: true},
	}
}

func TestPrimaryNavigationReservesG01Destinations(t *testing.T) {
	items := PrimaryNavigation()
	want := []RouteID{RouteOverview, RouteProject, RouteCreative, RouteKnowledge, RouteReview, RouteRunCenter, RouteSettings}
	if len(items) != len(want) {
		t.Fatalf("navigation count = %d, want %d", len(items), len(want))
	}
	for i, route := range want {
		if items[i].ID != route {
			t.Fatalf("navigation[%d] = %q, want %q", i, items[i].ID, route)
		}
		shouldEnable := route == RouteOverview || route == RouteProject || route == RouteCreative || route == RouteKnowledge || route == RouteRunCenter
		if items[i].Enabled != shouldEnable {
			t.Fatalf("%s enabled = %v, want %v through G05.7", route, items[i].Enabled, shouldEnable)
		}
	}
}

func TestLayoutForWidthUsesThreeTwoOneRegionProgression(t *testing.T) {
	desktop := LayoutForWidth(1440, true)
	if desktop.Class != ViewportDesktop || !desktop.NavigationVisible || !desktop.MainVisible || !desktop.InspectorVisible {
		t.Fatalf("unexpected desktop layout: %+v", desktop)
	}
	if desktop.NavigationWidth != 256 || desktop.InspectorWidth != 360 {
		t.Fatalf("desktop widths = %d/%d, want 256/360", desktop.NavigationWidth, desktop.InspectorWidth)
	}
	tablet := LayoutForWidth(900, true)
	if tablet.Class != ViewportTablet || !tablet.NavigationVisible || !tablet.MainVisible || tablet.InspectorVisible || !tablet.InspectorDrawer {
		t.Fatalf("unexpected tablet layout: %+v", tablet)
	}
	mobile := LayoutForWidth(600, true)
	if mobile.Class != ViewportMobile || mobile.NavigationVisible || !mobile.MainVisible || mobile.InspectorVisible || !mobile.InspectorDrawer {
		t.Fatalf("unexpected mobile layout: %+v", mobile)
	}
}

func TestSnapshotRequestIdentitySuppressesObsoleteResponse(t *testing.T) {
	shell := NewShell(1440)
	if !shell.BeginSnapshot("old") || !shell.BeginSnapshot("new") {
		t.Fatal("expected snapshot requests to start")
	}
	if shell.AcceptSnapshot("old", testSnapshot(1, "Old")) {
		t.Fatal("obsolete request must be discarded")
	}
	if !shell.AcceptSnapshot("new", testSnapshot(2, "New")) {
		t.Fatal("newest request should be accepted")
	}
	if shell.Header.ProjectTitle != "New" || shell.SnapshotRevision != 2 {
		t.Fatalf("unexpected accepted state: %+v", shell.Header)
	}
}

func TestSnapshotRevisionCannotMoveBackwards(t *testing.T) {
	shell := NewShell(1440)
	shell.BeginSnapshot("first")
	if !shell.AcceptSnapshot("first", testSnapshot(8, "Current")) {
		t.Fatal("first snapshot should be accepted")
	}
	shell.BeginSnapshot("stale")
	if shell.AcceptSnapshot("stale", testSnapshot(7, "Stale")) {
		t.Fatal("older revision must not overwrite current projection")
	}
	if shell.SnapshotRevision != 8 || shell.Header.ProjectTitle != "Current" || shell.Load != LoadReady {
		t.Fatalf("stale response changed canonical projection: %+v", shell)
	}
}

func TestEmptyAndErrorStatesAreDeterministic(t *testing.T) {
	shell := NewShell(1200)
	shell.BeginSnapshot("empty")
	if !shell.AcceptSnapshot("empty", testSnapshot(1, "")) || shell.Load != LoadEmpty {
		t.Fatalf("empty project state = %q, want %q", shell.Load, LoadEmpty)
	}
	shell.BeginSnapshot("failure")
	appErr := &appruntime.AppError{Code: appruntime.ErrorCodeStoreRead, Category: appruntime.ErrorCategoryStore, Message: "Project data could not be read.", Retryable: true}
	if !shell.FailSnapshot("failure", viewErrorFromAppError(appErr)) {
		t.Fatal("matching failed request should be accepted")
	}
	if shell.Load != LoadRuntimeError || shell.Error == nil || shell.Error.Code != string(appruntime.ErrorCodeStoreRead) {
		t.Fatalf("unexpected error state: load=%q error=%+v", shell.Load, shell.Error)
	}
}

func TestResizeDoesNotDiscardAuthoritativeProjection(t *testing.T) {
	shell := NewShell(1440)
	shell.BeginSnapshot("ready")
	shell.AcceptSnapshot("ready", testSnapshot(5, "Project A"))
	shell.Resize(600)
	if shell.Layout.Class != ViewportMobile {
		t.Fatalf("layout class = %q, want mobile", shell.Layout.Class)
	}
	if !shell.HasSnapshot || shell.SnapshotRevision != 5 || shell.Header.ProjectTitle != "Project A" {
		t.Fatalf("resize discarded authoritative data: %+v", shell)
	}
}

func TestG04RoutesRemainEnabledAndG057RunCenterOpensWithoutLaterRoutes(t *testing.T) {
	shell := NewShell(1440)
	for _, route := range []RouteID{RouteProject, RouteKnowledge, RouteCreative, RouteRunCenter} {
		if !shell.SelectRoute(route) {
			t.Fatalf("route %q should be enabled through G05.7", route)
		}
	}
	if shell.Route != RouteRunCenter {
		t.Fatalf("route = %q, want run_center", shell.Route)
	}
	for _, route := range []RouteID{RouteReview, RouteSettings} {
		if shell.SelectRoute(route) {
			t.Fatalf("later route %q opened prematurely", route)
		}
	}
	if shell.Route != RouteRunCenter {
		t.Fatalf("disabled later route changed selection to %q", shell.Route)
	}
}

func TestLifecycleControlsDeriveEligibilityButRemainUnwiredAtShellLayer(t *testing.T) {
	controls := ReservedLifecycleControls(string(appruntime.LifecycleRunning))
	if len(controls) != 6 {
		t.Fatalf("control count = %d, want 6", len(controls))
	}
	eligible := map[string]bool{"pause": true, "stop": true, "cancel": true}
	for _, control := range controls {
		if control.Enabled {
			t.Fatalf("%s shell reservation must be activated only by lifecycle controller", control.ID)
		}
		if control.Eligible != eligible[control.ID] {
			t.Fatalf("%s eligibility = %v, want %v", control.ID, control.Eligible, eligible[control.ID])
		}
		if control.Reason == "" {
			t.Fatalf("%s must explain why it is disabled", control.ID)
		}
	}
}

func TestAcceptedSnapshotRefreshesLifecyclePresentationFromAuthoritativeState(t *testing.T) {
	shell := NewShell(1440)
	snapshot := testSnapshot(4, "Novel")
	snapshot.Runtime.State = string(appruntime.LifecyclePaused)
	shell.BeginSnapshot("paused")
	if !shell.AcceptSnapshot("paused", snapshot) {
		t.Fatal("snapshot should be accepted")
	}
	for _, control := range shell.Controls {
		want := control.ID == "resume" || control.ID == "stop" || control.ID == "cancel"
		if control.Eligible != want {
			t.Fatalf("%s eligibility = %v, want %v", control.ID, control.Eligible, want)
		}
		if control.Enabled {
			t.Fatalf("%s base shell projection does not optimistically wire controls", control.ID)
		}
	}
}

func TestActivityIsOrderedDeduplicatedAndBounded(t *testing.T) {
	shell := NewShell(1440)
	for i := int64(1); i <= MaxActivityItems+5; i++ {
		if !shell.ApplyEvent(appruntime.DesktopEvent{ContractVersion: appruntime.ContractVersion, Seq: i, Category: "runtime", Type: "status", Summary: "event"}) {
			t.Fatalf("event %d should be accepted", i)
		}
	}
	if len(shell.Activity) != MaxActivityItems {
		t.Fatalf("activity length = %d, want %d", len(shell.Activity), MaxActivityItems)
	}
	if shell.Activity[0].Seq != 6 || shell.LastEventSeq != MaxActivityItems+5 {
		t.Fatalf("unexpected bounded activity window: first=%d last=%d", shell.Activity[0].Seq, shell.LastEventSeq)
	}
	if shell.ApplyEvent(appruntime.DesktopEvent{ContractVersion: appruntime.ContractVersion, Seq: 5}) {
		t.Fatal("out-of-order durable event must be discarded")
	}
}
