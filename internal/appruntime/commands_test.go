package appruntime

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestLifecycleFromCore(t *testing.T) {
	cases := map[string]DesktopLifecycleState{
		"":          LifecycleReady,
		"idle":      LifecycleReady,
		"running":   LifecycleRunning,
		"pausing":   LifecyclePausing,
		"paused":    LifecyclePaused,
		"completed": LifecycleCompleted,
		"unknown":   LifecycleReady,
	}
	for input, want := range cases {
		if got := lifecycleFromCore(input); got != want {
			t.Fatalf("lifecycleFromCore(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestDecodeStartPayloadDefaultsToResume(t *testing.T) {
	got, err := decodeStartPayload(nil)
	if err != nil {
		t.Fatalf("decode empty start payload: %v", err)
	}
	if got.Mode != StartModeResume {
		t.Fatalf("empty payload mode = %q, want %q", got.Mode, StartModeResume)
	}

	got, err = decodeStartPayload(json.RawMessage(`{"requirement":"new story"}`))
	if err != nil {
		t.Fatalf("decode implicit resume payload: %v", err)
	}
	if got.Mode != StartModeResume {
		t.Fatalf("implicit mode = %q, want safe resume default", got.Mode)
	}
}

func TestDecodeStartPayloadRejectsMalformedJSON(t *testing.T) {
	_, err := decodeStartPayload(json.RawMessage(`{"mode":`))
	if !errors.Is(err, ErrInvalidCommand) {
		t.Fatalf("malformed payload error = %v, want ErrInvalidCommand", err)
	}
}

func TestAllowsStartOnlyFromRestartableStates(t *testing.T) {
	allowed := []DesktopLifecycleState{LifecycleReady, LifecycleStopped, LifecycleCancelled, LifecycleFailed}
	for _, state := range allowed {
		if !allowsStart(state) {
			t.Fatalf("start should be allowed from %q", state)
		}
	}
	blocked := []DesktopLifecycleState{
		LifecycleStarting, LifecycleRunning, LifecyclePausing, LifecyclePaused,
		LifecycleResuming, LifecycleStopping, LifecycleCancelling,
		LifecycleRecovering, LifecycleCompleted,
	}
	for _, state := range blocked {
		if allowsStart(state) {
			t.Fatalf("start should be blocked from %q", state)
		}
	}
}

func TestCommandStateErrorIsStableSentinel(t *testing.T) {
	err := commandStateError(CommandResume, LifecycleRunning)
	if !errors.Is(err, ErrCommandNotAllowed) {
		t.Fatalf("commandStateError = %v, want ErrCommandNotAllowed", err)
	}
}

func TestLifecycleEventKeepsStopAndCancelDistinct(t *testing.T) {
	rt := &Runtime{subscribers: make(map[uint64]*desktopSubscription)}
	sub := newDesktopSubscription(4, nil)
	rt.subscribers[1] = sub

	stop := CommandRequest{ID: "cmd-stop", Kind: CommandStop, RunID: "run-1"}
	rt.emitAcceptedTransition(stop, LifecycleRunning, LifecycleStopping, "stop")
	stopEvent := <-sub.Events()
	if stopEvent.Summary != string(LifecycleStopping) {
		t.Fatalf("stop event state = %q, want %q", stopEvent.Summary, LifecycleStopping)
	}
	var stopPayload lifecycleEventPayload
	if err := json.Unmarshal(stopEvent.Payload, &stopPayload); err != nil {
		t.Fatalf("decode stop payload: %v", err)
	}
	if stopPayload.Command != CommandStop || stopPayload.State != LifecycleStopping {
		t.Fatalf("stop payload = %+v", stopPayload)
	}

	cancel := CommandRequest{ID: "cmd-cancel", Kind: CommandCancel, RunID: "run-1"}
	rt.emitAcceptedTransition(cancel, LifecycleRunning, LifecycleCancelling, "cancel")
	cancelEvent := <-sub.Events()
	var cancelPayload lifecycleEventPayload
	if err := json.Unmarshal(cancelEvent.Payload, &cancelPayload); err != nil {
		t.Fatalf("decode cancel payload: %v", err)
	}
	if cancelPayload.Command != CommandCancel || cancelPayload.State != LifecycleCancelling {
		t.Fatalf("cancel payload = %+v", cancelPayload)
	}
	if stopPayload.State == cancelPayload.State {
		t.Fatal("stop and cancel must not collapse to the same desktop state")
	}
}

func TestGeneratedCommandIDsAreMonotonicAndPreserveCallerID(t *testing.T) {
	rt := &Runtime{}
	if got := rt.ensureCommandID(" caller-id "); got != "caller-id" {
		t.Fatalf("caller id = %q, want caller-id", got)
	}
	first := rt.ensureCommandID("")
	second := rt.ensureCommandID("")
	if first != "cmd-1" || second != "cmd-2" {
		t.Fatalf("generated ids = %q, %q; want cmd-1, cmd-2", first, second)
	}
}
