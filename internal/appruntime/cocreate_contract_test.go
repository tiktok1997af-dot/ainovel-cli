package appruntime

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestG082CoCreateCommandCatalogStable(t *testing.T) {
	want := []CommandKind{
		"cocreate.turn",
		"cocreate.stage.begin",
		"cocreate.stage.finish",
		"cocreate.stage.cancel",
	}
	if got := G08CoCreateCommandKinds(); !reflect.DeepEqual(got, want) {
		t.Fatalf("catalog changed:\n got: %#v\nwant: %#v", got, want)
	}
	for _, kind := range want {
		if !isCoCreateCommandKind(kind) {
			t.Fatalf("catalog kind not recognized: %s", kind)
		}
	}
	if isCoCreateCommandKind("cocreate.raw") {
		t.Fatal("unknown CoCreate command recognized")
	}
}

func TestG082CoCreateTurnContractAcceptsBoundedSanitizedHistory(t *testing.T) {
	payload := CoCreateTurnCommandPayload{
		Mode: CoCreateModeColdStart,
		History: []CoCreateHistoryItemDTO{
			{Role: CoCreateRoleUser, Message: "  write a haunted-road survival novel  "},
			{
				Role:        CoCreateRoleAssistant,
				Message:     "Which risk should dominate?",
				Draft:       "## Direction\n- Haunted road",
				Suggestions: []string{"Combat first", "Scarce supplies"},
			},
			{Role: CoCreateRoleUser, Message: "Combat first."},
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	route, err := decodeCoCreateContract(CommandRequest{Kind: CommandCoCreateTurn, Payload: raw})
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	got, ok := route.Request.(CoCreateTurnCommandPayload)
	if !ok {
		t.Fatalf("request type = %T", route.Request)
	}
	if got.Mode != CoCreateModeColdStart || len(got.History) != 3 {
		t.Fatalf("request = %+v", got)
	}
	if got.History[0].Message != "write a haunted-road survival novel" {
		t.Fatalf("user message was not normalized: %q", got.History[0].Message)
	}
}

func TestG082CoCreateStageSkeletonAcceptsOnlyTypedPayloads(t *testing.T) {
	for _, kind := range []CommandKind{CommandCoCreateStageBegin, CommandCoCreateStageCancel} {
		for _, raw := range []json.RawMessage{nil, json.RawMessage(`null`), json.RawMessage(`{}`)} {
			if _, err := decodeCoCreateContract(CommandRequest{Kind: kind, Payload: raw}); err != nil {
				t.Fatalf("%s rejected empty typed payload %q: %v", kind, raw, err)
			}
		}
		if _, err := decodeCoCreateContract(CommandRequest{Kind: kind, Payload: json.RawMessage(`{"force":true}`)}); err == nil {
			t.Fatalf("%s accepted unknown field", kind)
		}
	}

	route, err := decodeCoCreateContract(CommandRequest{
		Kind:    CommandCoCreateStageFinish,
		Payload: json.RawMessage(`{"draft":"  ## Next arc\n- Pay off the broken seal  "}`),
	})
	if err != nil {
		t.Fatalf("stage finish: %v", err)
	}
	finish, ok := route.Request.(CoCreateStageFinishCommandPayload)
	if !ok || !strings.HasPrefix(finish.Draft, "## Next arc") {
		t.Fatalf("finish = %#v", route.Request)
	}
}

func TestG082CoCreateContractRejectsTransportAuthoritySpoofing(t *testing.T) {
	payload := json.RawMessage(`{"mode":"cold_start","history":[{"role":"user","message":"idea"}]}`)
	cases := []CommandRequest{
		{Kind: CommandCoCreateTurn, RunID: "run-1", Payload: payload},
		{Kind: CommandCoCreateTurn, TaskID: "task-1", Payload: payload},
		{Kind: CommandCoCreateTurn, Resource: "story:project", Payload: payload},
	}
	for _, cmd := range cases {
		if _, err := decodeCoCreateContract(cmd); err == nil || !errors.Is(err, ErrInvalidCommand) {
			t.Fatalf("authority spoof accepted: %+v err=%v", cmd, err)
		}
	}
}

func TestG082CoCreateTurnRejectsMalformedHistory(t *testing.T) {
	cases := []CoCreateTurnCommandPayload{
		{Mode: "unknown", History: []CoCreateHistoryItemDTO{{Role: CoCreateRoleUser, Message: "idea"}}},
		{Mode: CoCreateModeColdStart, History: nil},
		{Mode: CoCreateModeColdStart, History: []CoCreateHistoryItemDTO{{Role: CoCreateRoleAssistant, Message: "reply"}}},
		{Mode: CoCreateModeColdStart, History: []CoCreateHistoryItemDTO{{Role: CoCreateRoleUser, Message: "idea"}, {Role: CoCreateRoleUser, Message: "again"}}},
		{Mode: CoCreateModeColdStart, History: []CoCreateHistoryItemDTO{{Role: CoCreateRoleUser, Message: "   "}}},
		{Mode: CoCreateModeColdStart, History: []CoCreateHistoryItemDTO{{Role: CoCreateRoleUser, Message: "idea", Draft: "not allowed"}}},
		{Mode: CoCreateModeStage, History: []CoCreateHistoryItemDTO{{Role: CoCreateRoleUser, Message: "plan"}, {Role: CoCreateRole("tool"), Message: "hidden"}, {Role: CoCreateRoleUser, Message: "continue"}}},
	}
	for i, payload := range cases {
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := decodeCoCreateContract(CommandRequest{Kind: CommandCoCreateTurn, Payload: raw}); err == nil || !errors.Is(err, ErrInvalidCommand) {
			t.Fatalf("case %d accepted: %v", i, err)
		}
	}

	if _, err := decodeCoCreateContract(CommandRequest{
		Kind:    CommandCoCreateTurn,
		Payload: json.RawMessage(`{"mode":"cold_start","history":[{"role":"user","message":"idea"}],"raw":"secret"}`),
	}); err == nil || !errors.Is(err, ErrInvalidCommand) {
		t.Fatalf("unknown/raw field accepted: %v", err)
	}
}

func TestG082CoCreateFiniteBoundsFailClosed(t *testing.T) {
	tooLongMessage := strings.Repeat("x", MaxCoCreateMessageBytes+1)
	raw, err := json.Marshal(CoCreateTurnCommandPayload{
		Mode:    CoCreateModeColdStart,
		History: []CoCreateHistoryItemDTO{{Role: CoCreateRoleUser, Message: tooLongMessage}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeCoCreateContract(CommandRequest{Kind: CommandCoCreateTurn, Payload: raw}); err == nil {
		t.Fatal("oversized message accepted")
	}

	history := make([]CoCreateHistoryItemDTO, 0, MaxCoCreateHistoryItems+1)
	for i := 0; i < MaxCoCreateHistoryItems+1; i++ {
		role := CoCreateRoleUser
		if i%2 == 1 {
			role = CoCreateRoleAssistant
		}
		history = append(history, CoCreateHistoryItemDTO{Role: role, Message: "x"})
	}
	raw, err = json.Marshal(CoCreateTurnCommandPayload{Mode: CoCreateModeColdStart, History: history})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeCoCreateContract(CommandRequest{Kind: CommandCoCreateTurn, Payload: raw}); err == nil {
		t.Fatal("unbounded history accepted")
	}

	tooLongDraft := strings.Repeat("d", MaxCoCreateDraftBytes+1)
	raw, err = json.Marshal(CoCreateStageFinishCommandPayload{Draft: tooLongDraft})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeCoCreateContract(CommandRequest{Kind: CommandCoCreateStageFinish, Payload: raw}); err == nil {
		t.Fatal("oversized stage draft accepted")
	}

	if err := validateCoCreateTurnResult(CoCreateTurnResultDTO{
		Session:     CoCreateSessionDTO{Mode: CoCreateModeStage, HistoryCount: 3, StageActive: true},
		Message:     "reply",
		Suggestions: []string{"a", "b", "c", "d"},
	}); err == nil {
		t.Fatal("oversized suggestion set accepted")
	}
}

func TestG082CoCreateResultDTOIsSanitizedAndBounded(t *testing.T) {
	result := CoCreateTurnResultDTO{
		Session:     CoCreateSessionDTO{Mode: CoCreateModeStage, HistoryCount: 3, StageActive: true},
		Message:     "Keep the next arc focused on the sealed bridge.",
		Draft:       "## Next arc\n- Sealed bridge",
		Ready:       true,
		Suggestions: []string{"Raise the cost", "Bring back the witness"},
	}
	if err := validateCoCreateTurnResult(result); err != nil {
		t.Fatalf("valid result rejected: %v", err)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ToLower(string(encoded))
	for _, forbidden := range []string{
		"thinking", "reasoning", "raw", "provider", "browser", "profile", "pid", "cookie", "credential", "token", "storage", "host", "store", "filesystem",
	} {
		if strings.Contains(text, `"`+forbidden+`"`) || strings.Contains(text, `"`+forbidden+`_`) {
			t.Fatalf("forbidden field leaked (%s): %s", forbidden, text)
		}
	}
}

func TestG082CoCreateUnknownCommandGetsExplicitUnsupportedCode(t *testing.T) {
	_, err := decodeCoCreateContract(CommandRequest{Kind: CommandKind("cocreate.raw"), Payload: json.RawMessage(`{}`)})
	if err == nil || !errors.Is(err, ErrInvalidCommand) {
		t.Fatalf("unexpected error: %v", err)
	}
	app := normalizeAppError(err)
	if app.Code != ErrorCodeUnsupportedOperation || app.Category != ErrorCategoryValidation || app.Retryable {
		t.Fatalf("mapping = %+v", app)
	}
}
