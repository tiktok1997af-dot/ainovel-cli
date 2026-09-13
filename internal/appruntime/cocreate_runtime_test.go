package appruntime

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
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

func g084RuntimeWithColdStart(t *testing.T, fn func(context.Context, []host.CoCreateMessage) (host.CoCreateReply, error)) *Runtime {
	t.Helper()
	core := &host.Host{}
	return &Runtime{
		core: core,
		coCreate: &coCreateRuntimeAdapter{
			core:          core,
			coldStartTurn: fn,
		},
	}
}

func TestG084RuntimeDispatchExecutesColdStartTurn(t *testing.T) {
	rt := g084RuntimeWithColdStart(t, func(_ context.Context, history []host.CoCreateMessage) (host.CoCreateReply, error) {
		if len(history) != 1 || history[0].Role != "user" || history[0].Content != "Build a survival horror novel." {
			t.Fatalf("Host history = %#v", history)
		}
		return host.CoCreateReply{
			Message:     "Choose the road threat.",
			Prompt:      "## Premise\n- Survival horror road novel.",
			Ready:       true,
			Suggestions: []string{"Use moving safe zones.", "Make supplies upgradeable."},
			Raw:         "must never cross AppRuntime",
		}, nil
	})

	result, err := rt.Dispatch(context.Background(), g083ColdStartCommand())
	if err != nil {
		t.Fatalf("cold-start dispatch: %v", err)
	}
	if !result.Accepted {
		t.Fatalf("cold-start result not accepted: %+v", result)
	}

	var got CoCreateTurnResultDTO
	if err := json.Unmarshal(result.Data, &got); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if got.Session.Mode != CoCreateModeColdStart || got.Session.HistoryCount != 2 || got.Session.StageActive {
		t.Fatalf("session = %+v", got.Session)
	}
	if got.Message != "Choose the road threat." || got.Draft != "## Premise\n- Survival horror road novel." || !got.Ready {
		t.Fatalf("turn result = %+v", got)
	}
	if len(got.Suggestions) != 2 {
		t.Fatalf("suggestions = %#v", got.Suggestions)
	}
	if strings.Contains(string(result.Data), "must never cross AppRuntime") {
		t.Fatalf("raw Host/provider payload leaked: %s", result.Data)
	}
}

func TestG084ColdStartReconstructsSanitizedAssistantContinuity(t *testing.T) {
	cmd := CommandRequest{
		ID:   "cmd-g084-history",
		Kind: CommandCoCreateTurn,
		Payload: json.RawMessage(`{
			"mode":"cold_start",
			"history":[
				{"role":"user","message":"I want road survival horror."},
				{"role":"assistant","message":"Pick the resource rule.","draft":"## Genre\n- Road survival horror.","ready":false,"suggestions":["Upgradeable supplies."]},
				{"role":"user","message":"Supplies can be upgraded."}
			]
		}`),
	}
	rt := g084RuntimeWithColdStart(t, func(_ context.Context, history []host.CoCreateMessage) (host.CoCreateReply, error) {
		if len(history) != 3 {
			t.Fatalf("history count = %d", len(history))
		}
		assistant := history[1].Content
		for _, want := range []string{
			"<reply>\nPick the resource rule.\n</reply>",
			"<draft>\n## Genre\n- Road survival horror.\n</draft>",
			"<ready>false</ready>",
			"- Upgradeable supplies.",
		} {
			if !strings.Contains(assistant, want) {
				t.Fatalf("assistant protocol missing %q:\n%s", want, assistant)
			}
		}
		return host.CoCreateReply{
			Message: "Good, that rule is clear.",
			Prompt:  "## Genre\n- Road survival horror.\n## Resources\n- Supplies can be upgraded.",
		}, nil
	})

	if _, err := rt.Dispatch(context.Background(), cmd); err != nil {
		t.Fatalf("continued cold-start turn: %v", err)
	}
}

func TestG084ColdStartDraftHandsOffToExistingStartNewPayload(t *testing.T) {
	draft := "## Premise\n- A bounded accepted creative brief."
	raw, err := json.Marshal(StartCommandPayload{Mode: StartModeNew, Requirement: draft})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := decodeStartPayload(raw)
	if err != nil {
		t.Fatalf("existing start(new) decoder rejected CoCreate draft: %v", err)
	}
	if payload.Mode != StartModeNew || payload.Requirement != draft {
		t.Fatalf("start(new) handoff = %+v", payload)
	}
}

func TestG084ColdStartHostFailureFailsClosed(t *testing.T) {
	hostErr := errors.New("web turn failed")
	rt := g084RuntimeWithColdStart(t, func(context.Context, []host.CoCreateMessage) (host.CoCreateReply, error) {
		return host.CoCreateReply{}, hostErr
	})

	result, err := rt.Dispatch(context.Background(), g083ColdStartCommand())
	if err == nil {
		t.Fatalf("Host failure was accepted: result=%+v", result)
	}
	if result.Accepted {
		t.Fatalf("failed Host turn was accepted: %+v", result)
	}
	if strings.Contains(err.Error(), hostErr.Error()) || strings.Contains(string(result.Data), hostErr.Error()) {
		t.Fatalf("raw Host error leaked across AppRuntime boundary: result=%+v err=%v", result, err)
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

func g085StageRuntime(t *testing.T) (*Runtime, *int, *int, *string) {
	t.Helper()
	core := &host.Host{}
	beginCalls := 0
	cancelCalls := 0
	finishedDraft := ""
	rt := &Runtime{
		core: core,
		coCreate: &coCreateRuntimeAdapter{
			core: core,
			stageBegin: func() bool {
				beginCalls++
				return true
			},
			stageTurn: func(_ context.Context, history []host.CoCreateMessage) (host.CoCreateReply, error) {
				if len(history) != 1 || history[0].Role != "user" || history[0].Content != "Plan the next arc." {
					t.Fatalf("stage Host history = %#v", history)
				}
				return host.CoCreateReply{
					Message:     "Raise the cost before the next safe zone.",
					Prompt:      "## Next arc\n- Raise the cost.",
					Ready:       true,
					Suggestions: []string{"Add a false safe zone."},
					Raw:         "stage raw must stay internal",
				}, nil
			},
			stageFinish: func(draft string) error {
				finishedDraft = draft
				return nil
			},
			stageCancel: func() {
				cancelCalls++
			},
		},
	}
	return rt, &beginCalls, &cancelCalls, &finishedDraft
}

func TestG085StageBeginTurnFinishWrapExistingHostSeams(t *testing.T) {
	rt, beginCalls, _, finishedDraft := g085StageRuntime(t)

	begin, err := rt.Dispatch(context.Background(), CommandRequest{
		ID:      "cmd-stage-begin",
		Kind:    CommandCoCreateStageBegin,
		Payload: json.RawMessage(`{}`),
	})
	if err != nil || !begin.Accepted {
		t.Fatalf("stage begin: result=%+v err=%v", begin, err)
	}
	if *beginCalls != 1 || !rt.coCreate.stageActive {
		t.Fatalf("stage begin state: calls=%d active=%v", *beginCalls, rt.coCreate.stageActive)
	}
	var beginDTO CoCreateStageStateResultDTO
	if err := json.Unmarshal(begin.Data, &beginDTO); err != nil {
		t.Fatal(err)
	}
	if beginDTO.Session.Mode != CoCreateModeStage || !beginDTO.Session.StageActive || beginDTO.Session.HistoryCount != 0 {
		t.Fatalf("begin session = %+v", beginDTO.Session)
	}

	turn, err := rt.Dispatch(context.Background(), CommandRequest{
		ID:   "cmd-stage-turn",
		Kind: CommandCoCreateTurn,
		Payload: json.RawMessage(`{
			"mode":"stage",
			"history":[{"role":"user","message":"Plan the next arc."}]
		}`),
	})
	if err != nil || !turn.Accepted {
		t.Fatalf("stage turn: result=%+v err=%v", turn, err)
	}
	var turnDTO CoCreateTurnResultDTO
	if err := json.Unmarshal(turn.Data, &turnDTO); err != nil {
		t.Fatal(err)
	}
	if turnDTO.Session.Mode != CoCreateModeStage || !turnDTO.Session.StageActive || turnDTO.Session.HistoryCount != 2 {
		t.Fatalf("stage turn session = %+v", turnDTO.Session)
	}
	if turnDTO.Draft != "## Next arc\n- Raise the cost." || !turnDTO.Ready {
		t.Fatalf("stage turn dto = %+v", turnDTO)
	}
	if strings.Contains(string(turn.Data), "stage raw must stay internal") {
		t.Fatalf("raw stage Host/provider payload leaked: %s", turn.Data)
	}

	finish, err := rt.Dispatch(context.Background(), CommandRequest{
		ID:      "cmd-stage-finish",
		Kind:    CommandCoCreateStageFinish,
		Payload: json.RawMessage(`{"draft":"## Next arc\n- Raise the cost."}`),
	})
	if err != nil || !finish.Accepted {
		t.Fatalf("stage finish: result=%+v err=%v", finish, err)
	}
	if *finishedDraft != "## Next arc\n- Raise the cost." || rt.coCreate.stageActive {
		t.Fatalf("stage finish state: draft=%q active=%v", *finishedDraft, rt.coCreate.stageActive)
	}
	var finishDTO CoCreateStageStateResultDTO
	if err := json.Unmarshal(finish.Data, &finishDTO); err != nil {
		t.Fatal(err)
	}
	if finishDTO.Session.StageActive || finishDTO.Session.Mode != CoCreateModeStage {
		t.Fatalf("finish session = %+v", finishDTO.Session)
	}
}

func TestG085StageCancelAndOrderingFailClosed(t *testing.T) {
	rt, beginCalls, cancelCalls, _ := g085StageRuntime(t)

	stageTurn := CommandRequest{
		ID:   "cmd-stage-turn-before-begin",
		Kind: CommandCoCreateTurn,
		Payload: json.RawMessage(`{
			"mode":"stage",
			"history":[{"role":"user","message":"Plan the next arc."}]
		}`),
	}
	if result, err := rt.Dispatch(context.Background(), stageTurn); err == nil || result.Accepted || !errors.Is(err, ErrCommandNotAllowed) {
		t.Fatalf("stage turn before begin did not fail closed: result=%+v err=%v", result, err)
	}

	beginCmd := CommandRequest{ID: "cmd-stage-begin", Kind: CommandCoCreateStageBegin, Payload: json.RawMessage(`{}`)}
	if _, err := rt.Dispatch(context.Background(), beginCmd); err != nil {
		t.Fatalf("stage begin: %v", err)
	}
	if _, err := rt.Dispatch(context.Background(), beginCmd); err == nil || !errors.Is(err, ErrCommandNotAllowed) {
		t.Fatalf("duplicate stage begin did not fail closed: %v", err)
	}
	if *beginCalls != 1 {
		t.Fatalf("duplicate begin reached Host: %d", *beginCalls)
	}

	if result, err := rt.Dispatch(context.Background(), g083ColdStartCommand()); err == nil || result.Accepted || !errors.Is(err, ErrCommandNotAllowed) {
		t.Fatalf("cold-start turn escaped active stage occupancy: result=%+v err=%v", result, err)
	}

	cancel, err := rt.Dispatch(context.Background(), CommandRequest{
		ID:      "cmd-stage-cancel",
		Kind:    CommandCoCreateStageCancel,
		Payload: json.RawMessage(`{}`),
	})
	if err != nil || !cancel.Accepted {
		t.Fatalf("stage cancel: result=%+v err=%v", cancel, err)
	}
	if *cancelCalls != 1 || rt.coCreate.stageActive {
		t.Fatalf("stage cancel state: calls=%d active=%v", *cancelCalls, rt.coCreate.stageActive)
	}
	if _, err := rt.Dispatch(context.Background(), CommandRequest{
		ID:      "cmd-stage-cancel-again",
		Kind:    CommandCoCreateStageCancel,
		Payload: json.RawMessage(`{}`),
	}); err == nil || !errors.Is(err, ErrCommandNotAllowed) {
		t.Fatalf("cancel without active stage did not fail closed: %v", err)
	}
}

func TestG085StageHostFailuresFailClosedWithoutRawLeak(t *testing.T) {
	hostErr := errors.New("stage web turn failed")
	core := &host.Host{}
	rt := &Runtime{
		core: core,
		coCreate: &coCreateRuntimeAdapter{
			core:        core,
			stageActive: true,
			stageTurn: func(context.Context, []host.CoCreateMessage) (host.CoCreateReply, error) {
				return host.CoCreateReply{}, hostErr
			},
		},
	}
	result, err := rt.Dispatch(context.Background(), CommandRequest{
		ID:   "cmd-stage-turn-fail",
		Kind: CommandCoCreateTurn,
		Payload: json.RawMessage(`{
			"mode":"stage",
			"history":[{"role":"user","message":"Plan the next arc."}]
		}`),
	})
	if err == nil || result.Accepted {
		t.Fatalf("failed stage Host turn was accepted: result=%+v err=%v", result, err)
	}
	if strings.Contains(err.Error(), hostErr.Error()) || strings.Contains(string(result.Data), hostErr.Error()) {
		t.Fatalf("raw stage Host error leaked: result=%+v err=%v", result, err)
	}
	if !rt.coCreate.stageActive {
		t.Fatal("stage turn failure must keep stage active for retry/cancel")
	}
}

func TestG085StageFinishFailureReconcilesTerminalOccupancy(t *testing.T) {
	hostErr := errors.New("continue failed after stage occupancy cleared")
	core := &host.Host{}
	rt := &Runtime{
		core: core,
		coCreate: &coCreateRuntimeAdapter{
			core:        core,
			stageActive: true,
			stageFinish: func(string) error {
				return hostErr
			},
		},
	}
	result, err := rt.Dispatch(context.Background(), CommandRequest{
		ID:      "cmd-stage-finish-fail",
		Kind:    CommandCoCreateStageFinish,
		Payload: json.RawMessage(`{"draft":"## Next arc\n- Keep pressure high."}`),
	})
	if err == nil || result.Accepted {
		t.Fatalf("failed stage finish was accepted: result=%+v err=%v", result, err)
	}
	if rt.coCreate.stageActive {
		t.Fatal("stage finish error must not leave stale AppRuntime stage occupancy")
	}
	if strings.Contains(err.Error(), hostErr.Error()) {
		t.Fatalf("raw Host finish error leaked: %v", err)
	}
}
