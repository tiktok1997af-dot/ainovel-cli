package appruntime

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/voocel/ainovel-cli/internal/domain"
)

func TestRunCenterContractCatalogIsStableAndSeparate(t *testing.T) {
	got := CurrentRunCenterContractCatalog()
	wantQueries := []QueryKind{QueryRunsList, QueryRunsGet, QueryRunsActivity}
	wantCommands := []CommandKind{CommandRunStart, CommandRunPause, CommandRunResume, CommandRunStop, CommandRunCancel, CommandRunRetry}
	wantEvents := []string{EventTypeRunState, EventTypeRunProgress, EventTypeRunTask}
	if !reflect.DeepEqual(got.QueryKinds, wantQueries) {
		t.Fatalf("query catalog = %#v, want %#v", got.QueryKinds, wantQueries)
	}
	if !reflect.DeepEqual(got.CommandKinds, wantCommands) {
		t.Fatalf("command catalog = %#v, want %#v", got.CommandKinds, wantCommands)
	}
	if !reflect.DeepEqual(got.EventTypes, wantEvents) {
		t.Fatalf("event catalog = %#v, want %#v", got.EventTypes, wantEvents)
	}
}

func TestRunControlCommandUsesTypedRunIdentityAndLeavesLaterGateFieldsEmpty(t *testing.T) {
	cmd, err := NewRunControlCommand(" run-control-1 ", CommandRunPause, domain.RunID("run-001"))
	if err != nil {
		t.Fatal(err)
	}
	if cmd.ContractVersion != ContractVersion || cmd.ID != "run-control-1" || cmd.Kind != CommandRunPause || cmd.RunID != "run-001" {
		t.Fatalf("unexpected command: %#v", cmd)
	}
	if cmd.TaskID != "" || cmd.Resource != "" || len(cmd.Payload) != 0 {
		t.Fatalf("G05.2 command crossed later-gate ownership: %#v", cmd)
	}
}

func TestRunControlCommandRejectsUnknownKindAndPathLikeIdentity(t *testing.T) {
	if _, err := NewRunControlCommand("x", CommandKind("run.priority.set"), domain.RunID("run-001")); err == nil {
		t.Fatal("expected closed later-gate command to be rejected")
	}
	if _, err := NewRunControlCommand("x", CommandRunCancel, domain.RunID("../run-001")); err == nil {
		t.Fatal("expected path-like run identity to be rejected")
	}
}

func TestRunCenterDTOsSerializeOpaqueProjectionWithoutPolicyEnums(t *testing.T) {
	in := RunsGetResultDTO{Run: RunSummaryDTO{
		RunID:      domain.RunID("run-001"),
		State:      "queued",
		Priority:   "future-policy-value",
		WaitReason: "future-wait-reason",
		Resource:   domain.ResourceKey("chapter:12"),
		LaneID:     domain.BrowserLaneID("lane-1"),
	}}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var roundTrip RunsGetResultDTO
	if err := json.Unmarshal(data, &roundTrip); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(roundTrip, in) {
		t.Fatalf("round trip mismatch: got %#v want %#v", roundTrip, in)
	}
}

func TestRunEventPayloadsDoNotCarryBrowserSecretsOrPaths(t *testing.T) {
	typ := reflect.TypeOf(RunStateEventPayloadDTO{})
	for _, forbidden := range []string{"ProfilePath", "BrowserPath", "Cookie", "Token", "Credential", "Password"} {
		if _, ok := typ.FieldByName(forbidden); ok {
			t.Fatalf("forbidden field %s exposed in run event contract", forbidden)
		}
	}
}
