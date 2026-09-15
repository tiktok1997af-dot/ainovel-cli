package desktopui

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/voocel/ainovel-cli/internal/appruntime"
)

func TestD09BGenericLifecycleStartCarriesSelectedProviderIntoD05Envelope(t *testing.T) {
	runtime := &lifecycleTestRuntime{snapshots: []appruntime.DesktopSnapshot{
		lifecycleSnapshot(1, appruntime.LifecycleReady),
		lifecycleSnapshot(2, appruntime.LifecycleRunning),
	}}
	controller := NewController(runtime, 1440)
	if err := controller.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := controller.SelectModelProvider(appruntime.ProviderChatGPTWeb); err != nil {
		t.Fatal(err)
	}

	result, err := controller.ExecuteLifecycle(context.Background(), LifecycleStart)
	if err != nil {
		t.Fatalf("ExecuteLifecycle(start) error = %v", err)
	}
	if !result.Accepted || len(runtime.dispatchCalls) != 1 {
		t.Fatalf("start result/calls = %+v / %d", result, len(runtime.dispatchCalls))
	}

	request := runtime.dispatchCalls[0]
	if request.Kind != appruntime.CommandStart {
		t.Fatalf("start kind = %q", request.Kind)
	}
	var payload appruntime.DesktopStartCommandPayload
	if err := json.Unmarshal(request.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Mode != appruntime.StartModeResume || payload.Requirement != "" {
		t.Fatalf("legacy G02 start semantics drifted: %+v", payload.StartCommandPayload)
	}
	if payload.Provider != appruntime.ProviderChatGPTWeb {
		t.Fatalf("D05 provider was not forward-ported: %q", payload.Provider)
	}
}

func TestD09BGenericLifecycleStartWithoutSelectionUsesFailClosedD05Envelope(t *testing.T) {
	controller := NewController(nil, 1440)
	payloadJSON, err := controller.lifecycleCommandPayload(LifecycleStart)
	if err != nil {
		t.Fatal(err)
	}
	var payload appruntime.DesktopStartCommandPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Provider != "" {
		t.Fatalf("unselected provider must remain empty for AppRuntime D05 fail-closed gate, got %q", payload.Provider)
	}
	if payload.Mode != appruntime.StartModeResume {
		t.Fatalf("start mode = %q, want resume", payload.Mode)
	}
}

func TestD09BNonStartLifecyclePayloadRemainsEmpty(t *testing.T) {
	controller := NewController(nil, 1440)
	for _, action := range []LifecycleAction{LifecyclePause, LifecycleResume, LifecycleStop, LifecycleCancel, LifecycleRetry} {
		payload, err := controller.lifecycleCommandPayload(action)
		if err != nil {
			t.Fatalf("%s payload error = %v", action, err)
		}
		if len(payload) != 0 {
			t.Fatalf("%s unexpectedly changed payload contract: %s", action, payload)
		}
	}
}
