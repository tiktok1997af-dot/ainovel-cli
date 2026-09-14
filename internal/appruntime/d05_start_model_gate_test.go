package appruntime

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/voocel/ainovel-cli/internal/host"
	"github.com/voocel/ainovel-cli/internal/webai"
)

func d05StartPayload(t *testing.T, provider WebAIProvider) json.RawMessage {
	t.Helper()
	payload, err := json.Marshal(DesktopStartCommandPayload{
		StartCommandPayload: StartCommandPayload{Mode: StartModeResume},
		Provider:            provider,
	})
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func d05InertRuntime(state DesktopLifecycleState) *Runtime {
	// A zero-value Host is intentionally sufficient here: ready() only requires
	// a non-nil core, WebSessionSnapshot fails closed as STOPPED, and every
	// rejected D05 case must return before dispatchStart can touch Host engine
	// or durable project state.
	return &Runtime{
		core:           &host.Host{},
		lifecycleState: state,
	}
}

func assertD05GateRejectedBeforeLifecycle(t *testing.T, rt *Runtime, result CommandResult, err error) {
	t.Helper()
	if !errors.Is(err, ErrMutationPrecondition) {
		t.Fatalf("START error = %v, want ErrMutationPrecondition", err)
	}
	if result.Accepted {
		t.Fatalf("rejected START was accepted: %+v", result)
	}
	if result.Status != "" {
		t.Fatalf("rejected START reached lifecycle dispatch: status=%q", result.Status)
	}
	if got := rt.currentLifecycleState(); got != LifecycleReady {
		t.Fatalf("rejected START mutated lifecycle state: got %q want %q", got, LifecycleReady)
	}
}

func TestD05StartRejectsMissingProviderBeforeLifecycleEngine(t *testing.T) {
	rt := d05InertRuntime(LifecycleReady)
	result, err := rt.Dispatch(context.Background(), CommandRequest{
		ContractVersion: ContractVersion,
		ID:              "d05-start-missing-provider",
		Kind:            CommandStart,
		Payload:         d05StartPayload(t, ""),
	})
	assertD05GateRejectedBeforeLifecycle(t, rt, result, err)
}

func TestD05StartRejectsInvalidProviderBeforeLifecycleEngine(t *testing.T) {
	rt := d05InertRuntime(LifecycleReady)
	result, err := rt.Dispatch(context.Background(), CommandRequest{
		ContractVersion: ContractVersion,
		ID:              "d05-start-invalid-provider",
		Kind:            CommandStart,
		Payload:         d05StartPayload(t, WebAIProvider("other")),
	})
	assertD05GateRejectedBeforeLifecycle(t, rt, result, err)
}

func TestD05GeminiStartRequiresReadyWebSessionBeforeLifecycleEngine(t *testing.T) {
	rt := d05InertRuntime(LifecycleReady)
	// The inert Host reports SessionStopped. A valid Gemini selection therefore
	// exercises the authoritative Host readiness check and must fail closed.
	result, err := rt.Dispatch(context.Background(), CommandRequest{
		ContractVersion: ContractVersion,
		ID:              "d05-start-gemini-not-ready",
		Kind:            CommandStart,
		Payload:         d05StartPayload(t, ProviderGeminiWeb),
	})
	assertD05GateRejectedBeforeLifecycle(t, rt, result, err)
}

func TestD05ChatGPTStartLazyStartsLaneAndRejectsUnreadyBeforeLifecycleEngine(t *testing.T) {
	lane := &fakeAppChatGPTLane{
		snapshot: webai.ChatGPTLaneSnapshot{
			LaneID: webai.ChatGPTWebLaneID,
			State:  webai.SessionStopped,
		},
		startSnapshot: webai.ChatGPTLaneSnapshot{
			LaneID: webai.ChatGPTWebLaneID,
			State:  webai.SessionAuthRequired,
		},
	}
	rt := d05InertRuntime(LifecycleReady)
	rt.dualWeb = newDualWebProviderRuntime()
	rt.chatGPTLane = lane

	result, err := rt.Dispatch(context.Background(), CommandRequest{
		ContractVersion: ContractVersion,
		ID:              "d05-start-chatgpt-not-ready",
		Kind:            CommandStart,
		Payload:         d05StartPayload(t, ProviderChatGPTWeb),
	})
	assertD05GateRejectedBeforeLifecycle(t, rt, result, err)
	if lane.startCalls != 1 || lane.refreshCalls != 0 {
		t.Fatalf("ChatGPT readiness authority calls: start=%d refresh=%d, want start=1 refresh=0", lane.startCalls, lane.refreshCalls)
	}
}

func TestD05ChatGPTReadySelectionPassesModelPreflightBeforeLifecycleDispatch(t *testing.T) {
	lane := &fakeAppChatGPTLane{
		snapshot: webai.ChatGPTLaneSnapshot{
			LaneID: webai.ChatGPTWebLaneID,
			State:  webai.SessionStopped,
		},
		startSnapshot: d03ChatGPTReadySnapshot("chatgpt-observed", "rev-d05"),
	}
	// Running is deliberate: after the D05 gate verifies the model/preflight,
	// the existing lifecycle authority rejects a second START before Host engine
	// access. This lets the test distinguish "gate passed" from "engine touched".
	rt := d05InertRuntime(LifecycleRunning)
	rt.dualWeb = newDualWebProviderRuntime()
	rt.chatGPTLane = lane

	result, err := rt.Dispatch(context.Background(), CommandRequest{
		ContractVersion: ContractVersion,
		ID:              "d05-start-chatgpt-preflight",
		Kind:            CommandStart,
		Payload:         d05StartPayload(t, ProviderChatGPTWeb),
	})
	if !errors.Is(err, ErrCommandNotAllowed) {
		t.Fatalf("START error = %v, want lifecycle ErrCommandNotAllowed after D05 preflight", err)
	}
	if result.Accepted {
		t.Fatalf("running lifecycle accepted duplicate START: %+v", result)
	}
	if result.Status != string(LifecycleRunning) {
		t.Fatalf("START did not reach lifecycle authority after D05 preflight: status=%q", result.Status)
	}
	if lane.startCalls != 1 || lane.refreshCalls != 0 {
		t.Fatalf("ChatGPT lane calls: start=%d refresh=%d, want start=1 refresh=0", lane.startCalls, lane.refreshCalls)
	}
	preflight, preflightErr := rt.dualWeb.preflight([]WebAIProvider{ProviderChatGPTWeb})
	if preflightErr != nil {
		t.Fatal(preflightErr)
	}
	if !preflight.StartAllowed || len(preflight.Selections) != 1 || !preflight.Selections[0].Verified {
		t.Fatalf("ChatGPT model preflight was not verified: %+v", preflight)
	}
}
