package desktopui

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/voocel/ainovel-cli/internal/appruntime"
)

type runCenterRuntime struct {
	listResult     appruntime.RunsListResultDTO
	getResult      appruntime.RunsGetResultDTO
	activityResult appruntime.RunsActivityResultDTO
	lastCommand    appruntime.CommandRequest
}

func (f *runCenterRuntime) Snapshot(context.Context) (appruntime.DesktopSnapshot, error) {
	return testSnapshot(1, "Novel"), nil
}

func (f *runCenterRuntime) Query(_ context.Context, req appruntime.QueryRequest) (appruntime.QueryResult, error) {
	var data []byte
	switch req.Kind {
	case appruntime.QueryRunsList:
		data, _ = json.Marshal(f.listResult)
	case appruntime.QueryRunsGet:
		data, _ = json.Marshal(f.getResult)
	case appruntime.QueryRunsActivity:
		data, _ = json.Marshal(f.activityResult)
	}
	return appruntime.QueryResult{
		ContractVersion: appruntime.ContractVersion,
		Kind:            req.Kind,
		Data:            data,
	}, nil
}

func (f *runCenterRuntime) Dispatch(_ context.Context, cmd appruntime.CommandRequest) (appruntime.CommandResult, error) {
	f.lastCommand = cmd
	return appruntime.CommandResult{
		ContractVersion: appruntime.ContractVersion,
		CommandID:       cmd.ID,
		RunID:           cmd.RunID,
		Accepted:        true,
		Status:          string(appruntime.RunStateQueued),
	}, nil
}

func (f *runCenterRuntime) Subscribe(context.Context, appruntime.EventCursor) (appruntime.EventSubscription, error) {
	return &fakeSubscription{events: make(chan appruntime.DesktopEvent, 4)}, nil
}

func (f *runCenterRuntime) Close(context.Context) error { return nil }

func TestOpenRunCenterProjectsRunsAndSanitizedLanes(t *testing.T) {
	runtime := &runCenterRuntime{
		listResult: appruntime.RunsListResultDTO{
			Items: []appruntime.RunSummaryDTO{{
				RunID:  "run-001",
				State:  string(appruntime.RunStateRunning),
				LaneID: "lane-001",
			}},
			Lanes: []appruntime.BrowserLaneDTO{{
				LaneID: "lane-001",
				State:  "BUSY",
				RunID:  "run-001",
			}},
			Total: 1,
			Limit: 100,
		},
	}
	controller := NewController(runtime, 1440)
	if err := controller.OpenRunCenter(context.Background()); err != nil {
		t.Fatalf("OpenRunCenter: %v", err)
	}
	if controller.Shell().Route != RouteRunCenter {
		t.Fatalf("route=%q, want run_center", controller.Shell().Route)
	}
	state := controller.RunCenter()
	if state.Load != LoadReady || len(state.Runs) != 1 || len(state.Lanes) != 1 {
		t.Fatalf("unexpected Run Center state: %+v", state)
	}
}

func TestDispatchRunControlUsesTypedAppRuntimeCommand(t *testing.T) {
	runtime := &runCenterRuntime{
		listResult: appruntime.RunsListResultDTO{},
	}
	controller := NewController(runtime, 1440)
	if err := controller.DispatchRunControl(context.Background(), appruntime.CommandRunStart, "run-002"); err != nil {
		t.Fatalf("DispatchRunControl: %v", err)
	}
	if runtime.lastCommand.Kind != appruntime.CommandRunStart || runtime.lastCommand.RunID != "run-002" {
		t.Fatalf("unexpected command: %+v", runtime.lastCommand)
	}
	if runtime.lastCommand.Resource != "" || runtime.lastCommand.TaskID != "" {
		t.Fatalf("desktop claimed core-owned fields: %+v", runtime.lastCommand)
	}
}

func TestRunControlsFollowRunLifecycle(t *testing.T) {
	controls := RunControlsForState(string(appruntime.RunStateRunning))
	eligible := map[string]bool{}
	for _, control := range controls {
		eligible[control.ID] = control.Eligible && control.Enabled
	}
	if !eligible["run.pause"] || !eligible["run.stop"] || !eligible["run.cancel"] {
		t.Fatalf("running controls=%+v", controls)
	}
	if eligible["run.resume"] || eligible["run.retry"] {
		t.Fatalf("running run exposed recovery controls: %+v", controls)
	}
}

func TestRunCenterNavigationIsEnabledByG057(t *testing.T) {
	for _, item := range PrimaryNavigation() {
		if item.ID == RouteRunCenter {
			if !item.Enabled || item.Availability != "G05.7" {
				t.Fatalf("Run Center navigation = %+v, want enabled G05.7", item)
			}
			return
		}
	}
	t.Fatal("Run Center navigation item missing")
}

func TestTerminalRunUsesRetryNotStart(t *testing.T) {
	controls := RunControlsForState(string(appruntime.RunStateFailed))
	eligible := map[string]bool{}
	for _, control := range controls {
		eligible[control.ID] = control.Eligible && control.Enabled
	}
	if eligible["run.start"] || !eligible["run.retry"] {
		t.Fatalf("failed run controls=%+v, want retry only", controls)
	}
}
