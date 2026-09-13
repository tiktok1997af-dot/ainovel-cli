package appruntime

import (
	"errors"
	"reflect"
	"testing"

	"github.com/voocel/ainovel-cli/internal/domain"
)

func TestG064OperationalReviewCommandSet(t *testing.T) {
	for _, kind := range []CommandKind{CommandReviewRun, CommandReviewRepair, CommandReviewRerun} {
		if !isReviewCommandKind(kind) {
			t.Fatalf("%s must be operational in G06.4", kind)
		}
	}
	if isReviewCommandKind(CommandReviewPromoteOfficial) {
		t.Fatal("review.promote_official must remain CLOSED for G06.5")
	}
}

func TestReviewCommandMatchesExistingIgnoresRecoveryProgress(t *testing.T) {
	record := &domain.RunRegistryRecord{
		RunID:  "review-run",
		TaskID: domain.TaskID(CommandReviewRepair),
		Source: domain.RunRecordSourceRegistry,
		ReviewWork: &domain.ReviewRunWork{
			Action:              domain.ReviewWorkRepair,
			Target:              domain.ReviewWorkTarget{Scope: "chapter", Chapter: 7},
			ExpectedFingerprint: "sha256:abc",
			ExpectedRevisions: []domain.ReviewWorkRevision{{
				Chapter:       7,
				Revision:      2,
				ContentSHA256: "abc",
			}},
			GateIDs:           []string{"consistency"},
			Chapters:          []int{7},
			RepairMode:        "rewrite",
			Prepared:          true,
			CompletedChapters: []int{7},
			Executed:          true,
		},
	}
	intent := reviewCommandIntent{
		Target:              ReviewTargetDTO{Scope: ReviewScopeChapter, Chapter: 7},
		ExpectedFingerprint: "sha256:abc",
		GateIDs:             []ReviewGateID{ReviewGateConsistency},
		Chapters:            []int{7},
	}
	if !reviewCommandMatchesExisting(record, CommandReviewRepair, domain.TaskID(CommandReviewRepair), intent) {
		t.Fatal("durable replay must match despite Prepared/Completed/Executed progress")
	}
	intent.ExpectedFingerprint = "sha256:def"
	if reviewCommandMatchesExisting(record, CommandReviewRepair, domain.TaskID(CommandReviewRepair), intent) {
		t.Fatal("different fingerprint must not match existing durable command")
	}
}

func TestValidateRepairSelectionAcceptsBoundedActionableRewrite(t *testing.T) {
	status := ReviewStatusResultDTO{
		ReviewID:         "review-1",
		Verdict:          "rewrite",
		AffectedChapters: []int{7, 8},
		Gates: []ReviewGateResultDTO{{
			GateID:     ReviewGateConsistency,
			State:      ReviewGateWarn,
			Actionable: true,
		}},
	}
	mode, err := validateRepairSelection(status, []ReviewGateID{ReviewGateConsistency}, []int{7})
	if err != nil {
		t.Fatalf("valid bounded repair rejected: %v", err)
	}
	if mode != "rewrite" {
		t.Fatalf("repair mode = %q, want rewrite", mode)
	}
}

func TestValidateRepairSelectionRejectsChapterOutsideCanonicalAffectedSet(t *testing.T) {
	status := ReviewStatusResultDTO{
		ReviewID:         "review-1",
		Verdict:          "polish",
		AffectedChapters: []int{7},
	}
	_, err := validateRepairSelection(status, nil, []int{8})
	if err == nil || !errors.Is(err, ErrCommandNotAllowed) {
		t.Fatalf("outside chapter error = %v, want ErrCommandNotAllowed", err)
	}
}

func TestValidateRepairSelectionRejectsNonActionableGate(t *testing.T) {
	status := ReviewStatusResultDTO{
		ReviewID:         "review-1",
		Verdict:          "rewrite",
		AffectedChapters: []int{7},
		Gates: []ReviewGateResultDTO{{
			GateID:     ReviewGateConsistency,
			State:      ReviewGatePass,
			Actionable: false,
		}},
	}
	_, err := validateRepairSelection(status, []ReviewGateID{ReviewGateConsistency}, []int{7})
	if err == nil || !errors.Is(err, ErrCommandNotAllowed) {
		t.Fatalf("non-actionable gate error = %v, want ErrCommandNotAllowed", err)
	}
}

func TestReviewCommandRunIDIsDeterministicAndOpaque(t *testing.T) {
	cmd := CommandRequest{ID: "command-123", Kind: CommandReviewRun}
	first, err := reviewCommandRunID(cmd)
	if err != nil {
		t.Fatal(err)
	}
	second, err := reviewCommandRunID(cmd)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || first == "" {
		t.Fatalf("deterministic run id mismatch: %q vs %q", first, second)
	}
	if err := (domain.RunIdentity{RunID: first}).Validate(); err != nil {
		t.Fatalf("generated run id is not valid opaque identity: %v", err)
	}

	_, err = reviewCommandRunID(CommandRequest{ID: "x", RunID: "../path", Kind: CommandReviewRun})
	if err == nil || !errors.Is(err, ErrInvalidCommand) {
		t.Fatalf("path-like supplied run id error = %v, want ErrInvalidCommand", err)
	}
}

func TestReviewWorkTargetMappingPreservesSemanticCoordinates(t *testing.T) {
	in := ReviewTargetDTO{Scope: ReviewScopeArc, Volume: 2, Arc: 3, ThroughChapter: 18}
	got := reviewWorkTarget(in)
	want := domain.ReviewWorkTarget{Scope: "arc", Volume: 2, Arc: 3, ThroughChapter: 18}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("target mapping = %#v, want %#v", got, want)
	}
}
