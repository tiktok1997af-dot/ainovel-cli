package appruntime

import (
	"errors"
	"slices"
	"testing"
)

func TestG067FrozenReviewContractMatrix(t *testing.T) {
	catalog := CurrentReviewContractCatalog()

	wantGates := []ReviewGateID{
		ReviewGateContractFulfillment,
		ReviewGateConsistency,
		ReviewGateCharacter,
		ReviewGatePacing,
		ReviewGateContinuity,
		ReviewGateForeshadow,
		ReviewGateHook,
		ReviewGateAesthetic,
		ReviewGateStyleRegression,
		ReviewGateFlowIntegrity,
		ReviewGatePlanningIntegrity,
		ReviewGateContextIntegrity,
	}
	gotGates := make([]ReviewGateID, 0, len(catalog.Gates))
	for _, gate := range catalog.Gates {
		gotGates = append(gotGates, gate.ID)
	}
	if !slices.Equal(gotGates, wantGates) {
		t.Fatalf("gate catalog=%v, want=%v", gotGates, wantGates)
	}

	wantQueries := []QueryKind{QueryReviewCatalog, QueryReviewStatus, QueryReviewHistory}
	if !slices.Equal(catalog.QueryKinds, wantQueries) {
		t.Fatalf("review queries=%v, want=%v", catalog.QueryKinds, wantQueries)
	}
	wantCommands := []CommandKind{CommandReviewRun, CommandReviewRepair, CommandReviewRerun, CommandReviewPromoteOfficial}
	if !slices.Equal(catalog.CommandKinds, wantCommands) {
		t.Fatalf("review commands=%v, want=%v", catalog.CommandKinds, wantCommands)
	}
	wantEvents := []string{EventTypeReviewState, EventTypeReviewGate, EventTypeReviewAction}
	if !slices.Equal(catalog.EventTypes, wantEvents) {
		t.Fatalf("review events=%v, want=%v", catalog.EventTypes, wantEvents)
	}
	wantStates := []ReviewGateState{
		ReviewGateNotRun,
		ReviewGateRunning,
		ReviewGatePass,
		ReviewGateWarn,
		ReviewGateFail,
		ReviewGateStale,
		ReviewGateUnavailable,
	}
	if !slices.Equal(catalog.GateStates, wantStates) {
		t.Fatalf("review gate states=%v, want=%v", catalog.GateStates, wantStates)
	}
}

func TestG067RepairAndPromotionRemainFailClosed(t *testing.T) {
	payload, status, snapshot := readyPromotionFixture()

	status.Verdict = "rewrite"
	status.AffectedChapters = []int{7}
	for i := range status.Gates {
		if status.Gates[i].GateID == ReviewGateContinuity {
			status.Gates[i].Actionable = true
		}
	}
	mode, err := validateRepairSelection(status, []ReviewGateID{ReviewGateContinuity}, []int{7})
	if err != nil || mode != "rewrite" {
		t.Fatalf("bounded repair rejected: mode=%q err=%v", mode, err)
	}
	if _, err := validateRepairSelection(status, []ReviewGateID{ReviewGateContinuity}, []int{8}); err == nil || !errors.Is(err, ErrCommandNotAllowed) {
		t.Fatalf("out-of-bound repair error=%v, want ErrCommandNotAllowed", err)
	}

	if err := validateReviewPromotionEligibility(payload, status, snapshot); err != nil {
		t.Fatalf("fresh promotion-ready status rejected: %v", err)
	}
	stale := payload
	stale.ExpectedFingerprint = "sha256:stale"
	if err := validateReviewPromotionEligibility(stale, status, snapshot); err == nil || !errors.Is(err, ErrMutationStale) {
		t.Fatalf("stale promotion error=%v, want ErrMutationStale", err)
	}

	notReady := status
	notReady.Gates = append([]ReviewGateResultDTO(nil), status.Gates...)
	for i := range notReady.Gates {
		if notReady.Gates[i].GateID == ReviewGateContinuity {
			notReady.Gates[i].State = ReviewGateFail
			break
		}
	}
	if err := validateReviewPromotionEligibility(payload, notReady, snapshot); err == nil || !errors.Is(err, ErrMutationPrecondition) {
		t.Fatalf("failed required gate error=%v, want ErrMutationPrecondition", err)
	}
}
