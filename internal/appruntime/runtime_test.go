package appruntime

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestNewRejectsNilHost(t *testing.T) {
	rt, err := New(nil)
	if !errors.Is(err, ErrNilHost) {
		t.Fatalf("New(nil) error = %v, want ErrNilHost", err)
	}
	if rt != nil {
		t.Fatal("New(nil) returned non-nil runtime")
	}
}

func TestZeroValueRuntimeIsUnavailable(t *testing.T) {
	var rt Runtime
	_, err := rt.Snapshot(context.Background())
	if !errors.Is(err, ErrRuntimeUnavailable) {
		t.Fatalf("Snapshot zero-value error = %v, want ErrRuntimeUnavailable", err)
	}
}

func TestContextCancellationWinsBeforeSkeletonDispatch(t *testing.T) {
	rt := &Runtime{core: nil}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := rt.Dispatch(ctx, CommandRequest{ID: "cmd-1"})
	if !errors.Is(err, ErrRuntimeUnavailable) {
		// A zero-value Runtime is invalid before any caller-context semantics.
		t.Fatalf("Dispatch zero-value error = %v, want ErrRuntimeUnavailable", err)
	}
}

func TestContractDTOsAreJSONSafe(t *testing.T) {
	input := CommandRequest{
		ID:       "cmd-42",
		Kind:     CommandKind("start"),
		RunID:    "run-7",
		TaskID:   "task-3",
		Resource: "chapter:12",
		Payload:  json.RawMessage(`{"mode":"write"}`),
	}
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal CommandRequest: %v", err)
	}
	var output CommandRequest
	if err := json.Unmarshal(data, &output); err != nil {
		t.Fatalf("unmarshal CommandRequest: %v", err)
	}
	if output.ID != input.ID || output.Kind != input.Kind || output.Resource != input.Resource {
		t.Fatalf("round trip mismatch: got %+v want %+v", output, input)
	}
	if string(output.Payload) != string(input.Payload) {
		t.Fatalf("payload mismatch: got %s want %s", output.Payload, input.Payload)
	}
}
