package appruntime

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/host"
)

func g083ColdStartCommand() CommandRequest {
	return CommandRequest{
		ID:   "cmd-g083",
		Kind: CommandCoCreateTurn,
		Payload: json.RawMessage(`{
			"mode":"cold_start",
			"history":[{"role":"user","message":"Build a survival horror novel."}]
		}`),
	}
}

func TestG083RuntimeDispatchRoutesCoCreateToDedicatedAdapter(t *testing.T) {
	core := &host.Host{}
	rt := &Runtime{
		core:     core,
		coCreate: newCoCreateRuntimeAdapter(core),
	}

	result, err := rt.Dispatch(context.Background(), g083ColdStartCommand())
	if err == nil || !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("CoCreate was not routed to staged adapter: result=%+v err=%v", result, err)
	}
	if result.Accepted {
		t.Fatalf("G08.3 must not execute successor semantics: %+v", result)
	}
	if result.ContractVersion != ContractVersion || result.CommandID != "cmd-g083" {
		t.Fatalf("transport metadata = %+v", result)
	}
}

func TestG083CoCreateFailsClosedForQueuedRunCenterWork(t *testing.T) {
	backend := newFakeRunBackend()
	backend.tickets = []domain.RunScheduleTicket{{RunID: domain.RunID("run-g083")}}
	core := &host.Host{}
	rt := &Runtime{
		core:     core,
		run:      newRunCoordinator(backend, nil),
		coCreate: newCoCreateRuntimeAdapter(core),
	}

	result, err := rt.Dispatch(context.Background(), g083ColdStartCommand())
	if err == nil || !errors.Is(err, ErrCommandNotAllowed) {
		t.Fatalf("managed Run Center work did not block CoCreate: result=%+v err=%v", result, err)
	}
	if result.Accepted {
		t.Fatalf("blocked CoCreate was accepted: %+v", result)
	}
}

func TestG083CoCreateFailsClosedForActiveRunCenterWork(t *testing.T) {
	backend := newFakeRunBackend()
	coord := newRunCoordinator(backend, nil)
	coord.activeFlag.Store(true)
	core := &host.Host{}
	rt := &Runtime{
		core:     core,
		run:      coord,
		coCreate: newCoCreateRuntimeAdapter(core),
	}

	_, err := rt.Dispatch(context.Background(), g083ColdStartCommand())
	if err == nil || !errors.Is(err, ErrCommandNotAllowed) {
		t.Fatalf("active Run Center work did not block CoCreate: %v", err)
	}
}

func TestG083CoCreateStillUsesG082TypedValidation(t *testing.T) {
	core := &host.Host{}
	rt := &Runtime{
		core:     core,
		coCreate: newCoCreateRuntimeAdapter(core),
	}
	cmd := g083ColdStartCommand()
	cmd.Resource = "lane-001"

	_, err := rt.Dispatch(context.Background(), cmd)
	if err == nil || !errors.Is(err, ErrInvalidCommand) {
		t.Fatalf("CoCreate caller resource claim escaped G08.2 validation: %v", err)
	}
}

func TestG083SuccessorExecutionSlotsRemainClosed(t *testing.T) {
	core := &host.Host{}
	rt := &Runtime{
		core:     core,
		coCreate: newCoCreateRuntimeAdapter(core),
	}

	cases := []CommandRequest{
		g083ColdStartCommand(),
		{
			ID:   "cmd-stage-turn",
			Kind: CommandCoCreateTurn,
			Payload: json.RawMessage(`{
				"mode":"stage",
				"history":[{"role":"user","message":"Plan the next arc."}]
			}`),
		},
		{ID: "cmd-stage-begin", Kind: CommandCoCreateStageBegin, Payload: json.RawMessage(`{}`)},
		{ID: "cmd-stage-finish", Kind: CommandCoCreateStageFinish, Payload: json.RawMessage(`{"draft":"## Next arc\n- Raise the cost."}`)},
		{ID: "cmd-stage-cancel", Kind: CommandCoCreateStageCancel, Payload: json.RawMessage(`{}`)},
	}

	for _, cmd := range cases {
		t.Run(string(cmd.Kind), func(t *testing.T) {
			result, err := rt.Dispatch(context.Background(), cmd)
			if err == nil || !errors.Is(err, ErrNotImplemented) {
				t.Fatalf("successor execution opened early: result=%+v err=%v", result, err)
			}
			if result.Accepted {
				t.Fatalf("successor execution accepted early: %+v", result)
			}
		})
	}
}
