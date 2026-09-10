package desktopui

import (
	"context"
	"errors"
	"testing"

	"github.com/voocel/ainovel-cli/internal/appruntime"
)

type fakeSubscription struct {
	events chan appruntime.DesktopEvent
	closed bool
}

func (s *fakeSubscription) Events() <-chan appruntime.DesktopEvent { return s.events }
func (s *fakeSubscription) Close() error {
	s.closed = true
	return nil
}

type fakeRuntime struct {
	snapshot      appruntime.DesktopSnapshot
	snapshotErr   error
	subscribeErr  error
	sub           *fakeSubscription
	queryCalls    int
	dispatchCalls int
	closeCalls    int
}

func (f *fakeRuntime) Snapshot(context.Context) (appruntime.DesktopSnapshot, error) {
	return f.snapshot, f.snapshotErr
}

func (f *fakeRuntime) Query(context.Context, appruntime.QueryRequest) (appruntime.QueryResult, error) {
	f.queryCalls++
	return appruntime.QueryResult{}, nil
}

func (f *fakeRuntime) Dispatch(context.Context, appruntime.CommandRequest) (appruntime.CommandResult, error) {
	f.dispatchCalls++
	return appruntime.CommandResult{}, nil
}

func (f *fakeRuntime) Subscribe(context.Context, appruntime.EventCursor) (appruntime.EventSubscription, error) {
	if f.subscribeErr != nil {
		return nil, f.subscribeErr
	}
	if f.sub == nil {
		f.sub = &fakeSubscription{events: make(chan appruntime.DesktopEvent, 4)}
	}
	return f.sub, nil
}

func (f *fakeRuntime) Close(context.Context) error {
	f.closeCalls++
	return nil
}

func TestBootstrapUsesSnapshotAndSubscriptionWithoutLaterGateCalls(t *testing.T) {
	runtime := &fakeRuntime{snapshot: testSnapshot(3, "Novel")}
	controller := NewController(runtime, 1440)

	if err := controller.Bootstrap(context.Background()); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	if controller.Shell().Load != LoadReady || controller.Shell().Header.ProjectTitle != "Novel" {
		t.Fatalf("unexpected shell state: %+v", controller.Shell())
	}
	if runtime.queryCalls != 0 || runtime.dispatchCalls != 0 {
		t.Fatalf("G04.3 crossed successor boundary: query=%d dispatch=%d", runtime.queryCalls, runtime.dispatchCalls)
	}
}

func TestBootstrapMapsStructuredAppErrorWithoutRawFallback(t *testing.T) {
	runtime := &fakeRuntime{
		snapshotErr: &appruntime.AppError{
			Code:      appruntime.ErrorCodeStoreRead,
			Category:  appruntime.ErrorCategoryStore,
			Message:   "Project data could not be read.",
			Retryable: true,
		},
	}
	controller := NewController(runtime, 1440)

	if err := controller.Bootstrap(context.Background()); err == nil {
		t.Fatal("Bootstrap() error = nil, want structured failure")
	}
	viewErr := controller.Shell().Error
	if viewErr == nil || viewErr.Code != string(appruntime.ErrorCodeStoreRead) || viewErr.Message != "Project data could not be read." {
		t.Fatalf("unexpected projected error: %+v", viewErr)
	}
}

func TestBootstrapMapsUnknownErrorToSafeRuntimeProjection(t *testing.T) {
	runtime := &fakeRuntime{snapshotErr: errors.New(`open C:\secret\project: access denied`)}
	controller := NewController(runtime, 1440)

	if err := controller.Bootstrap(context.Background()); err == nil {
		t.Fatal("Bootstrap() error = nil, want failure")
	}
	viewErr := controller.Shell().Error
	if viewErr == nil || viewErr.Message != "The application runtime is unavailable." {
		t.Fatalf("unexpected safe fallback error: %+v", viewErr)
	}
}

func TestPumpEventConsumesProjectedEventOnly(t *testing.T) {
	runtime := &fakeRuntime{snapshot: testSnapshot(2, "Novel")}
	controller := NewController(runtime, 1440)
	if err := controller.Bootstrap(context.Background()); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}

	runtime.sub.events <- appruntime.DesktopEvent{
		ContractVersion: appruntime.ContractVersion,
		Seq:             9,
		Category:        "runtime",
		Type:            "status",
		Summary:         "checkpoint saved",
	}
	if err := controller.PumpEvent(context.Background()); err != nil {
		t.Fatalf("PumpEvent() error = %v", err)
	}
	if controller.Shell().LastEventSeq != 9 || len(controller.Shell().Activity) != 1 {
		t.Fatalf("event was not projected: %+v", controller.Shell())
	}
	if runtime.queryCalls != 0 || runtime.dispatchCalls != 0 {
		t.Fatal("event projection must not issue hidden query/dispatch calls")
	}
}

func TestCloseClosesSubscriptionBeforeRuntimeFacade(t *testing.T) {
	runtime := &fakeRuntime{snapshot: testSnapshot(1, "Novel")}
	controller := NewController(runtime, 1440)
	if err := controller.Bootstrap(context.Background()); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	if err := controller.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if runtime.sub == nil || !runtime.sub.closed || runtime.closeCalls != 1 {
		t.Fatalf("close state invalid: sub=%+v runtimeClose=%d", runtime.sub, runtime.closeCalls)
	}
}
