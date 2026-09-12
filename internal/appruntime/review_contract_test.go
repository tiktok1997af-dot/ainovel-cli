package appruntime

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestReviewGateCatalogIsFrozenAndUnique(t *testing.T) {
	want := []ReviewGateID{
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

	catalog := ReviewGateCatalog()
	if len(catalog) != 12 {
		t.Fatalf("gate count = %d, want 12", len(catalog))
	}

	got := make([]ReviewGateID, 0, len(catalog))
	seen := make(map[ReviewGateID]bool, len(catalog))
	for _, gate := range catalog {
		if gate.ID == "" || gate.Label == "" || gate.Class == "" || gate.EvidenceOwner == "" {
			t.Fatalf("incomplete gate definition: %+v", gate)
		}
		if seen[gate.ID] {
			t.Fatalf("duplicate gate id: %s", gate.ID)
		}
		seen[gate.ID] = true
		got = append(got, gate.ID)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("gate order = %v, want %v", got, want)
	}
}

func TestCurrentReviewContractCatalogIsStable(t *testing.T) {
	got := CurrentReviewContractCatalog()
	if !reflect.DeepEqual(got.QueryKinds, []QueryKind{QueryReviewCatalog, QueryReviewStatus, QueryReviewHistory}) {
		t.Fatalf("query kinds = %v", got.QueryKinds)
	}
	if !reflect.DeepEqual(got.CommandKinds, []CommandKind{CommandReviewRun, CommandReviewRepair, CommandReviewRerun, CommandReviewPromoteOfficial}) {
		t.Fatalf("command kinds = %v", got.CommandKinds)
	}
	if !reflect.DeepEqual(got.EventTypes, []string{EventTypeReviewState, EventTypeReviewGate, EventTypeReviewAction}) {
		t.Fatalf("event types = %v", got.EventTypes)
	}
	if !reflect.DeepEqual(got.GateStates, []ReviewGateState{
		ReviewGateNotRun,
		ReviewGateRunning,
		ReviewGatePass,
		ReviewGateWarn,
		ReviewGateFail,
		ReviewGateStale,
		ReviewGateUnavailable,
	}) {
		t.Fatalf("gate states = %v", got.GateStates)
	}
}

func TestReviewProjectionDTOJSONRoundTrip(t *testing.T) {
	score := 88
	in := ReviewStatusResultDTO{
		ReviewID:         "review-1",
		Target:           ReviewTargetDTO{Scope: ReviewScopeChapter, Chapter: 7, ThroughChapter: 7},
		State:            ReviewRunCompleted,
		Overall:          ReviewGateWarn,
		Verdict:          "polish",
		Summary:          "bounded summary",
		AffectedChapters: []int{7},
		Freshness: ReviewFreshnessDTO{
			Fingerprint: "sha256:abc",
			Revisions:   []ReviewRevisionRefDTO{{Chapter: 7, Revision: 3, ContentSHA256: "abc"}},
			EvaluatedAt: time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC),
		},
		Gates: []ReviewGateResultDTO{{
			GateID:     ReviewGateConsistency,
			State:      ReviewGateWarn,
			Score:      &score,
			Summary:    "one issue",
			Actionable: true,
			Freshness:  ReviewFreshnessDTO{Fingerprint: "sha256:abc"},
			Evidence: []ReviewEvidenceDTO{{
				Source:   "review_entry.dimension",
				Code:     "consistency",
				Severity: "warning",
				Summary:  "bounded evidence",
				Chapters: []int{7},
			}},
		}},
	}

	data, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out ReviewStatusResultDTO
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(out, in) {
		t.Fatalf("round trip mismatch:\n got: %#v\nwant: %#v", out, in)
	}
}

func TestReviewContractDoesNotAdvertiseOperationalRouting(t *testing.T) {
	// G06.2 freezes vocabulary only. Operational query/command routing belongs
	// to G06.3-G06.5 and therefore must not be added to the G03 supported-query
	// discovery list just because the constants exist.
	for _, kind := range SupportedQueryKinds() {
		if kind == QueryReviewCatalog || kind == QueryReviewStatus || kind == QueryReviewHistory {
			t.Fatalf("review query %s advertised before operationalization", kind)
		}
	}
}
