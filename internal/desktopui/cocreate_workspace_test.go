package desktopui

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/voocel/ainovel-cli/internal/appruntime"
)

type coCreateRuntimeStub struct {
	snapshot appruntime.DesktopSnapshot
	calls    []appruntime.CommandRequest
	dispatch func(appruntime.CommandRequest) (appruntime.CommandResult, error)
}

func newCoCreateRuntimeStub() *coCreateRuntimeStub {
	return &coCreateRuntimeStub{
		snapshot: appruntime.DesktopSnapshot{
			Contract: appruntime.CurrentContract(),
			Revision: 1,
			Runtime:  appruntime.RuntimeViewSnapshot{State: string(appruntime.LifecycleReady)},
		},
	}
}

func (f *coCreateRuntimeStub) Snapshot(context.Context) (appruntime.DesktopSnapshot, error) {
	return f.snapshot, nil
}

func (f *coCreateRuntimeStub) Query(context.Context, appruntime.QueryRequest) (appruntime.QueryResult, error) {
	return appruntime.QueryResult{}, nil
}

func (f *coCreateRuntimeStub) Dispatch(_ context.Context, req appruntime.CommandRequest) (appruntime.CommandResult, error) {
	f.calls = append(f.calls, req)
	if f.dispatch != nil {
		return f.dispatch(req)
	}
	return appruntime.CommandResult{}, nil
}

func (f *coCreateRuntimeStub) Subscribe(context.Context, appruntime.EventCursor) (appruntime.EventSubscription, error) {
	return &fakeSubscription{events: make(chan appruntime.DesktopEvent, 1)}, nil
}

func (f *coCreateRuntimeStub) Close(context.Context) error { return nil }

func acceptedCoCreateResult(t *testing.T, req appruntime.CommandRequest, dto any) appruntime.CommandResult {
	t.Helper()
	data, err := json.Marshal(dto)
	if err != nil {
		t.Fatal(err)
	}
	return appruntime.CommandResult{
		ContractVersion: appruntime.ContractVersion,
		CommandID:       req.ID,
		Accepted:        true,
		Data:            data,
	}
}

func TestCoCreateColdStartTurnAndExistingStartNewHandoff(t *testing.T) {
	runtime := newCoCreateRuntimeStub()
	runtime.dispatch = func(req appruntime.CommandRequest) (appruntime.CommandResult, error) {
		switch req.Kind {
		case appruntime.CommandCoCreateTurn:
			var payload appruntime.CoCreateTurnCommandPayload
			if err := json.Unmarshal(req.Payload, &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Mode != appruntime.CoCreateModeColdStart || len(payload.History) != 1 || payload.History[0].Message != "một truyện sinh tồn" {
				t.Fatalf("unexpected cold-start payload: %+v", payload)
			}
			return acceptedCoCreateResult(t, req, appruntime.CoCreateTurnResultDTO{
				Session: appruntime.CoCreateSessionDTO{Mode: appruntime.CoCreateModeColdStart, HistoryCount: 2},
				Message: "Tôi đã phác thảo hướng truyện.",
				Draft:   "Sinh tồn trên đường cái, vật tư có thể thăng cấp.",
				Ready:   true,
				Suggestions: []string{
					"Thêm cơ chế thời tiết",
				},
			}), nil
		case appruntime.CommandStart:
			var payload appruntime.StartCommandPayload
			if err := json.Unmarshal(req.Payload, &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Mode != appruntime.StartModeNew || payload.Requirement != "Sinh tồn trên đường cái, vật tư có thể thăng cấp." {
				t.Fatalf("start-new handoff drifted: %+v", payload)
			}
			return appruntime.CommandResult{
				ContractVersion: appruntime.ContractVersion,
				CommandID:       req.ID,
				Accepted:        true,
				Status:          string(appruntime.LifecycleRunning),
			}, nil
		default:
			t.Fatalf("unexpected command: %s", req.Kind)
			return appruntime.CommandResult{}, nil
		}
	}

	controller := NewController(runtime, 1440)
	if _, err := controller.SendCoCreate(context.Background(), "  một truyện sinh tồn  "); err != nil {
		t.Fatalf("SendCoCreate() error = %v", err)
	}
	state := controller.CoCreate()
	if state.Mode != appruntime.CoCreateModeColdStart || len(state.History) != 2 || !state.Ready || state.Draft == "" {
		t.Fatalf("unexpected cold-start state: %+v", state)
	}
	if !state.Controls().StartNew.Enabled {
		t.Fatalf("accepted draft should enable explicit start-new handoff: %+v", state.Controls())
	}

	if _, err := controller.StartNewFromCoCreate(context.Background()); err != nil {
		t.Fatalf("StartNewFromCoCreate() error = %v", err)
	}
	if !state.HandoffAccepted || state.Pending {
		t.Fatalf("handoff state not finalized: %+v", state)
	}
	if len(runtime.calls) != 2 || runtime.calls[1].Kind != appruntime.CommandStart {
		t.Fatalf("expected exact CoCreate -> existing start path, calls=%+v", runtime.calls)
	}
}

func TestCoCreateStageBeginTurnFinishUsesOnlyFrozenCommands(t *testing.T) {
	runtime := newCoCreateRuntimeStub()
	runtime.dispatch = func(req appruntime.CommandRequest) (appruntime.CommandResult, error) {
		switch req.Kind {
		case appruntime.CommandCoCreateStageBegin:
			return acceptedCoCreateResult(t, req, appruntime.CoCreateStageStateResultDTO{
				Session: appruntime.CoCreateSessionDTO{Mode: appruntime.CoCreateModeStage, StageActive: true},
			}), nil
		case appruntime.CommandCoCreateTurn:
			var payload appruntime.CoCreateTurnCommandPayload
			if err := json.Unmarshal(req.Payload, &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Mode != appruntime.CoCreateModeStage || len(payload.History) != 1 {
				t.Fatalf("unexpected stage payload: %+v", payload)
			}
			return acceptedCoCreateResult(t, req, appruntime.CoCreateTurnResultDTO{
				Session: appruntime.CoCreateSessionDTO{Mode: appruntime.CoCreateModeStage, HistoryCount: 2, StageActive: true},
				Message: "Có thể đổi hướng arc tiếp theo.",
				Draft:   "Đưa nhân vật sang khu vực mới sau chương hiện tại.",
				Ready:   true,
			}), nil
		case appruntime.CommandCoCreateStageFinish:
			var payload appruntime.CoCreateStageFinishCommandPayload
			if err := json.Unmarshal(req.Payload, &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Draft != "Đưa nhân vật sang khu vực mới sau chương hiện tại." {
				t.Fatalf("finish draft drifted: %q", payload.Draft)
			}
			return acceptedCoCreateResult(t, req, appruntime.CoCreateStageStateResultDTO{
				Session: appruntime.CoCreateSessionDTO{Mode: appruntime.CoCreateModeStage, StageActive: false},
			}), nil
		default:
			t.Fatalf("unexpected command: %s", req.Kind)
			return appruntime.CommandResult{}, nil
		}
	}

	controller := NewController(runtime, 1280)
	if _, err := controller.BeginCoCreateStage(context.Background()); err != nil {
		t.Fatalf("BeginCoCreateStage() error = %v", err)
	}
	if !controller.CoCreate().StageActive || controller.CoCreate().Mode != appruntime.CoCreateModeStage {
		t.Fatalf("stage was not activated: %+v", controller.CoCreate())
	}
	if _, err := controller.SendCoCreate(context.Background(), "đổi hướng arc kế"); err != nil {
		t.Fatalf("SendCoCreate(stage) error = %v", err)
	}
	if _, err := controller.FinishCoCreateStage(context.Background()); err != nil {
		t.Fatalf("FinishCoCreateStage() error = %v", err)
	}
	if controller.CoCreate().StageActive {
		t.Fatal("stage should be inactive after finish")
	}
	wantKinds := []appruntime.CommandKind{
		appruntime.CommandCoCreateStageBegin,
		appruntime.CommandCoCreateTurn,
		appruntime.CommandCoCreateStageFinish,
	}
	if len(runtime.calls) != len(wantKinds) {
		t.Fatalf("calls=%d want=%d", len(runtime.calls), len(wantKinds))
	}
	for i, want := range wantKinds {
		if runtime.calls[i].Kind != want {
			t.Fatalf("call %d kind=%s want=%s", i, runtime.calls[i].Kind, want)
		}
		if runtime.calls[i].RunID != "" || runtime.calls[i].TaskID != "" || runtime.calls[i].Resource != "" {
			t.Fatalf("CoCreate UI must not claim G05 authority: %+v", runtime.calls[i])
		}
	}
}

func TestCoCreateStageCancelAndPendingControlsFailClosed(t *testing.T) {
	runtime := newCoCreateRuntimeStub()
	runtime.dispatch = func(req appruntime.CommandRequest) (appruntime.CommandResult, error) {
		switch req.Kind {
		case appruntime.CommandCoCreateStageBegin:
			return acceptedCoCreateResult(t, req, appruntime.CoCreateStageStateResultDTO{
				Session: appruntime.CoCreateSessionDTO{Mode: appruntime.CoCreateModeStage, StageActive: true},
			}), nil
		case appruntime.CommandCoCreateStageCancel:
			return acceptedCoCreateResult(t, req, appruntime.CoCreateStageStateResultDTO{
				Session: appruntime.CoCreateSessionDTO{Mode: appruntime.CoCreateModeStage},
			}), nil
		default:
			t.Fatalf("unexpected command: %s", req.Kind)
			return appruntime.CommandResult{}, nil
		}
	}

	controller := NewController(runtime, 1024)
	if _, err := controller.BeginCoCreateStage(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.CancelCoCreateStage(context.Background()); err != nil {
		t.Fatal(err)
	}
	if controller.CoCreate().StageActive {
		t.Fatal("cancel should close stage presentation state")
	}

	before := len(runtime.calls)
	controller.CoCreate().Pending = true
	controls := controller.CoCreate().Controls()
	if controls.Send.Enabled || controls.BeginStage.Enabled || controls.Finish.Enabled || controls.Cancel.Enabled || controls.StartNew.Enabled {
		t.Fatalf("all controls must disable while pending: %+v", controls)
	}
	if _, err := controller.SendCoCreate(context.Background(), "must not dispatch"); err == nil {
		t.Fatal("pending SendCoCreate should fail closed")
	}
	if len(runtime.calls) != before {
		t.Fatal("pending UI action must not reach AppRuntime")
	}
}

func TestCoCreateRejectsMalformedOrUnsanitizedRuntimeResult(t *testing.T) {
	runtime := newCoCreateRuntimeStub()
	runtime.dispatch = func(req appruntime.CommandRequest) (appruntime.CommandResult, error) {
		return appruntime.CommandResult{
			ContractVersion: appruntime.ContractVersion,
			CommandID:       req.ID,
			Accepted:        true,
			Data:            json.RawMessage(`{"session":{"mode":"cold_start","history_count":2,"stage_active":false},"message":"ok","draft":"brief","ready":true,"provider_payload":"secret"}`),
		}, nil
	}
	controller := NewController(runtime, 1440)
	if _, err := controller.SendCoCreate(context.Background(), "idea"); err == nil {
		t.Fatal("unknown runtime result fields must fail closed")
	}
	state := controller.CoCreate()
	if len(state.History) != 0 || state.Pending {
		t.Fatalf("failed turn must not mutate continuity: %+v", state)
	}
	if state.Error == nil || state.Error.Message != "An internal runtime error occurred." {
		t.Fatalf("raw runtime shape must map to safe UI error: %+v", state.Error)
	}
}

func TestCoCreateDispatchFailureDoesNotLeakRawErrorIntoWorkspace(t *testing.T) {
	runtime := newCoCreateRuntimeStub()
	runtime.dispatch = func(req appruntime.CommandRequest) (appruntime.CommandResult, error) {
		return appruntime.CommandResult{CommandID: req.ID}, errors.New(`open C:\secret\profile: access denied`)
	}
	controller := NewController(runtime, 1440)
	if _, err := controller.SendCoCreate(context.Background(), "idea"); err == nil {
		t.Fatal("dispatch failure expected")
	}
	state := controller.CoCreate()
	if state.Error == nil || state.Error.Message != "The application runtime is unavailable." || strings.Contains(state.Error.Message, "secret") {
		t.Fatalf("workspace leaked raw runtime error: %+v", state.Error)
	}
}

func TestCoCreateHistoryBoundReservesUserAndAssistantSlots(t *testing.T) {
	controller := NewController(newCoCreateRuntimeStub(), 1440)
	state := controller.CoCreate()
	for i := 0; i < appruntime.MaxCoCreateHistoryItems; i++ {
		role := appruntime.CoCreateRoleUser
		if i%2 == 1 {
			role = appruntime.CoCreateRoleAssistant
		}
		state.History = append(state.History, appruntime.CoCreateHistoryItemDTO{Role: role, Message: "x"})
	}
	if state.Controls().Send.Enabled {
		t.Fatal("bounded history must disable Send before another user+assistant pair")
	}
}
