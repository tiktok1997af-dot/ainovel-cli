package desktopui

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/voocel/ainovel-cli/internal/appruntime"
)

type lifecycleTestSubscription struct {
	events chan appruntime.DesktopEvent
}

func (s *lifecycleTestSubscription) Events() <-chan appruntime.DesktopEvent { return s.events }
func (s *lifecycleTestSubscription) Close() error                           { return nil }

type lifecycleTestRuntime struct {
	snapshots     []appruntime.DesktopSnapshot
	snapshotCalls int
	queryCalls    int
	dispatchCalls []appruntime.CommandRequest
	dispatchFn    func(appruntime.CommandRequest) (appruntime.CommandResult, error)
	sub           *lifecycleTestSubscription
}

func (f *lifecycleTestRuntime) Snapshot(context.Context) (appruntime.DesktopSnapshot, error) {
	if len(f.snapshots) == 0 {
		return appruntime.DesktopSnapshot{}, errors.New("missing test snapshot")
	}
	index := f.snapshotCalls
	if index >= len(f.snapshots) {
		index = len(f.snapshots) - 1
	}
	f.snapshotCalls++
	return f.snapshots[index], nil
}

func (f *lifecycleTestRuntime) Query(context.Context, appruntime.QueryRequest) (appruntime.QueryResult, error) {
	f.queryCalls++
	return appruntime.QueryResult{}, nil
}

func (f *lifecycleTestRuntime) Dispatch(_ context.Context, req appruntime.CommandRequest) (appruntime.CommandResult, error) {
	f.dispatchCalls = append(f.dispatchCalls, req)
	if f.dispatchFn != nil {
		return f.dispatchFn(req)
	}
	return appruntime.CommandResult{
		ContractVersion: appruntime.ContractVersion,
		CommandID:       req.ID,
		Accepted:        true,
		Status:          string(appruntime.LifecycleRunning),
	}, nil
}

func (f *lifecycleTestRuntime) Subscribe(context.Context, appruntime.EventCursor) (appruntime.EventSubscription, error) {
	if f.sub == nil {
		f.sub = &lifecycleTestSubscription{events: make(chan appruntime.DesktopEvent, 8)}
	}
	return f.sub, nil
}

func (f *lifecycleTestRuntime) Close(context.Context) error { return nil }

func lifecycleSnapshot(revision uint64, state appruntime.DesktopLifecycleState) appruntime.DesktopSnapshot {
	snapshot := testSnapshot(revision, "Novel")
	snapshot.Runtime.State = string(state)
	return snapshot
}

func controlMap(controls []LifecycleControlView) map[string]LifecycleControlView {
	out := make(map[string]LifecycleControlView, len(controls))
	for _, control := range controls {
		out[control.ID] = control
	}
	return out
}

func TestBootstrapActivatesLifecycleControlsFromAuthoritativeSnapshot(t *testing.T) {
	runtime := &lifecycleTestRuntime{snapshots: []appruntime.DesktopSnapshot{
		lifecycleSnapshot(1, appruntime.LifecyclePaused),
	}}
	controller := NewController(runtime, 1440)
	if err := controller.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}

	controls := controlMap(controller.Shell().Controls)
	for _, id := range []string{"resume", "stop", "cancel"} {
		if !controls[id].Eligible || !controls[id].Enabled || controls[id].Reason != "" {
			t.Fatalf("%s control not activated from paused snapshot: %+v", id, controls[id])
		}
	}
	for _, id := range []string{"start", "pause", "retry"} {
		if controls[id].Enabled || controls[id].Reason == "" {
			t.Fatalf("%s must stay disabled from paused snapshot: %+v", id, controls[id])
		}
	}
	if len(runtime.dispatchCalls) != 0 || runtime.queryCalls != 0 {
		t.Fatalf("bootstrap crossed command/query boundary: dispatch=%d query=%d", len(runtime.dispatchCalls), runtime.queryCalls)
	}
}

func TestExecuteLifecycleMapsExactlySixLockedCommands(t *testing.T) {
	cases := []struct {
		name    string
		action  LifecycleAction
		initial appruntime.DesktopLifecycleState
		status  appruntime.DesktopLifecycleState
		kind    appruntime.CommandKind
	}{
		{name: "start", action: LifecycleStart, initial: appruntime.LifecycleReady, status: appruntime.LifecycleRunning, kind: appruntime.CommandStart},
		{name: "pause", action: LifecyclePause, initial: appruntime.LifecycleRunning, status: appruntime.LifecyclePausing, kind: appruntime.CommandPause},
		{name: "resume", action: LifecycleResume, initial: appruntime.LifecyclePaused, status: appruntime.LifecycleRunning, kind: appruntime.CommandResume},
		{name: "stop", action: LifecycleStop, initial: appruntime.LifecycleRunning, status: appruntime.LifecycleStopping, kind: appruntime.CommandStop},
		{name: "cancel", action: LifecycleCancel, initial: appruntime.LifecycleRunning, status: appruntime.LifecycleCancelling, kind: appruntime.CommandCancel},
		{name: "retry", action: LifecycleRetry, initial: appruntime.LifecycleFailed, status: appruntime.LifecycleRunning, kind: appruntime.CommandRetry},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runtime := &lifecycleTestRuntime{snapshots: []appruntime.DesktopSnapshot{
				lifecycleSnapshot(1, tc.initial),
				lifecycleSnapshot(2, tc.status),
			}}
			runtime.dispatchFn = func(req appruntime.CommandRequest) (appruntime.CommandResult, error) {
				return appruntime.CommandResult{
					ContractVersion: appruntime.ContractVersion,
					CommandID:       req.ID,
					Accepted:        true,
					Status:          string(tc.status),
				}, nil
			}
			controller := NewController(runtime, 1440)
			if err := controller.Bootstrap(context.Background()); err != nil {
				t.Fatal(err)
			}

			result, err := controller.ExecuteLifecycle(context.Background(), tc.action)
			if err != nil {
				t.Fatalf("ExecuteLifecycle() error = %v", err)
			}
			if !result.Accepted || len(runtime.dispatchCalls) != 1 {
				t.Fatalf("dispatch result/calls = %+v / %d", result, len(runtime.dispatchCalls))
			}
			request := runtime.dispatchCalls[0]
			if request.ContractVersion != appruntime.ContractVersion || request.Kind != tc.kind {
				t.Fatalf("request = %+v, want kind %q contract %q", request, tc.kind, appruntime.ContractVersion)
			}
			if request.ID != "desktop-lifecycle-1" || request.RunID != "" || request.TaskID != "" || request.Resource != "" {
				t.Fatalf("unexpected command identity/scheduling fields: %+v", request)
			}
			if tc.action == LifecycleStart {
				var payload appruntime.StartCommandPayload
				if err := json.Unmarshal(request.Payload, &payload); err != nil {
					t.Fatal(err)
				}
				if payload.Mode != appruntime.StartModeResume || payload.Requirement != "" {
					t.Fatalf("G04.6 Start must use safe resume mode: %+v", payload)
				}
			} else if len(request.Payload) != 0 {
				t.Fatalf("%s unexpectedly carried payload %s", tc.action, request.Payload)
			}
			if runtime.snapshotCalls != 2 {
				t.Fatalf("snapshot calls = %d, want bootstrap + post-command refresh", runtime.snapshotCalls)
			}
			state := controller.Lifecycle()
			if state.Pending || !state.Accepted || state.CommandID != request.ID || state.Action != tc.action ||
				state.ResultStatus != string(tc.status) || state.ObservedState != string(tc.status) || state.Error != nil {
				t.Fatalf("lifecycle state = %+v", state)
			}
		})
	}
}

func TestLifecycleEventIsFollowedByFreshSnapshotReconciliation(t *testing.T) {
	runtime := &lifecycleTestRuntime{snapshots: []appruntime.DesktopSnapshot{
		lifecycleSnapshot(1, appruntime.LifecycleRunning),
		lifecycleSnapshot(2, appruntime.LifecyclePausing),
		lifecycleSnapshot(3, appruntime.LifecyclePaused),
	}}
	runtime.dispatchFn = func(req appruntime.CommandRequest) (appruntime.CommandResult, error) {
		return appruntime.CommandResult{
			ContractVersion: appruntime.ContractVersion,
			CommandID:       req.ID,
			Accepted:        true,
			Status:          string(appruntime.LifecyclePausing),
		}, nil
	}
	controller := NewController(runtime, 1440)
	if err := controller.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.ExecuteLifecycle(context.Background(), LifecyclePause); err != nil {
		t.Fatal(err)
	}

	payload, err := json.Marshal(map[string]any{
		"command_id":     "desktop-lifecycle-1",
		"command":        string(appruntime.CommandPause),
		"previous_state": string(appruntime.LifecyclePausing),
		"state":          string(appruntime.LifecyclePaused),
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime.sub.events <- appruntime.DesktopEvent{
		ContractVersion: appruntime.ContractVersion,
		Seq:             11,
		Category:        "LIFECYCLE",
		Type:            lifecycleStateEventType,
		Summary:         string(appruntime.LifecyclePaused),
		Payload:         payload,
	}
	if err := controller.PumpEvent(context.Background()); err != nil {
		t.Fatalf("PumpEvent() error = %v", err)
	}

	if runtime.snapshotCalls != 3 || controller.Shell().SnapshotRevision != 3 ||
		controller.Shell().Runtime.State != string(appruntime.LifecyclePaused) || controller.Shell().LastEventSeq != 11 {
		t.Fatalf("event/snapshot reconciliation failed: calls=%d shell=%+v", runtime.snapshotCalls, controller.Shell())
	}
	state := controller.Lifecycle()
	if state.ObservedState != string(appruntime.LifecyclePaused) || state.ResultStatus != string(appruntime.LifecyclePausing) {
		t.Fatalf("command acceptance/completion collapsed incorrectly: %+v", state)
	}
	controls := controlMap(controller.Shell().Controls)
	if !controls["resume"].Enabled || !controls["stop"].Enabled || !controls["cancel"].Enabled {
		t.Fatalf("paused controls not derived from fresh snapshot: %+v", controls)
	}
}

func TestLifecycleDispatchErrorStaysStructuredAndRefreshesAuthoritativeState(t *testing.T) {
	appErr := &appruntime.AppError{
		Code:      appruntime.ErrorCodeCommandRejected,
		Category:  appruntime.ErrorCategoryRuntime,
		Message:   "The runtime rejected this action.",
		Retryable: false,
	}
	runtime := &lifecycleTestRuntime{snapshots: []appruntime.DesktopSnapshot{
		lifecycleSnapshot(1, appruntime.LifecycleRunning),
		lifecycleSnapshot(2, appruntime.LifecycleRunning),
	}}
	runtime.dispatchFn = func(req appruntime.CommandRequest) (appruntime.CommandResult, error) {
		return appruntime.CommandResult{
			ContractVersion: appruntime.ContractVersion,
			CommandID:       req.ID,
			Accepted:        false,
			Status:          string(appruntime.LifecycleRunning),
			Error:           appErr,
		}, appErr
	}
	controller := NewController(runtime, 1440)
	if err := controller.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.ExecuteLifecycle(context.Background(), LifecyclePause); err == nil {
		t.Fatal("dispatch error = nil")
	}
	state := controller.Lifecycle()
	if state.Pending || state.Accepted || state.Error == nil || state.Error.Code != string(appruntime.ErrorCodeCommandRejected) ||
		state.ObservedState != string(appruntime.LifecycleRunning) {
		t.Fatalf("structured lifecycle failure lost: %+v", state)
	}
	if runtime.snapshotCalls != 2 {
		t.Fatalf("failed command must still refresh authoritative snapshot, calls=%d", runtime.snapshotCalls)
	}
	controls := controlMap(controller.Shell().Controls)
	if !controls["pause"].Enabled || !controls["stop"].Enabled || !controls["cancel"].Enabled {
		t.Fatalf("controls not restored from authoritative running state: %+v", controls)
	}
}

func TestLifecycleRejectsUnknownActionBeforeDispatch(t *testing.T) {
	runtime := &lifecycleTestRuntime{snapshots: []appruntime.DesktopSnapshot{
		lifecycleSnapshot(1, appruntime.LifecycleReady),
	}}
	controller := NewController(runtime, 1440)
	if err := controller.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, err := controller.ExecuteLifecycle(context.Background(), LifecycleAction("delete"))
	var appErr *appruntime.AppError
	if !errors.As(err, &appErr) || appErr.Code != appruntime.ErrorCodeInvalidArgument {
		t.Fatalf("unknown action error = %v", err)
	}
	if len(runtime.dispatchCalls) != 0 {
		t.Fatalf("unknown action reached Dispatch: %+v", runtime.dispatchCalls)
	}
}

func TestLifecyclePendingGuardDisablesAllControlsWithoutDispatchingSecondCommand(t *testing.T) {
	runtime := &lifecycleTestRuntime{snapshots: []appruntime.DesktopSnapshot{
		lifecycleSnapshot(1, appruntime.LifecycleRunning),
	}}
	controller := NewController(runtime, 1440)
	if err := controller.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	commandID, err := controller.beginLifecycleCommand(LifecyclePause, string(appruntime.LifecycleRunning))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.beginLifecycleCommand(LifecycleStop, string(appruntime.LifecycleRunning)); err == nil {
		t.Fatal("second pending command was accepted")
	}
	for _, control := range controller.Shell().Controls {
		if control.Enabled || control.Reason != "A lifecycle command is pending." {
			t.Fatalf("pending control state = %+v", control)
		}
	}
	controller.finishLifecycleFailure(commandID, string(appruntime.LifecycleRunning), nil)
}

func TestLifecycleRejectsMismatchedCommandResultContract(t *testing.T) {
	runtime := &lifecycleTestRuntime{snapshots: []appruntime.DesktopSnapshot{
		lifecycleSnapshot(1, appruntime.LifecycleReady),
		lifecycleSnapshot(2, appruntime.LifecycleReady),
	}}
	runtime.dispatchFn = func(req appruntime.CommandRequest) (appruntime.CommandResult, error) {
		return appruntime.CommandResult{
			ContractVersion: "other.desktop.v1",
			CommandID:       req.ID,
			Accepted:        true,
			Status:          string(appruntime.LifecycleRunning),
		}, nil
	}
	controller := NewController(runtime, 1440)
	if err := controller.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, err := controller.ExecuteLifecycle(context.Background(), LifecycleStart)
	var appErr *appruntime.AppError
	if !errors.As(err, &appErr) || appErr.Code != appruntime.ErrorCodeContractMismatch {
		t.Fatalf("contract mismatch error = %v", err)
	}
	state := controller.Lifecycle()
	if state.Error == nil || state.Error.Code != string(appruntime.ErrorCodeContractMismatch) || state.Pending {
		t.Fatalf("contract mismatch presentation = %+v", state)
	}
	if controller.Shell().Runtime.State != string(appruntime.LifecycleReady) || runtime.snapshotCalls != 2 {
		t.Fatalf("fresh snapshot did not recover projection after protocol error: state=%q calls=%d", controller.Shell().Runtime.State, runtime.snapshotCalls)
	}
}
