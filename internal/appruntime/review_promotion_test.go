package appruntime

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/host"
)

func TestDecodeReviewPromotionPayloadRequiresExactCAS(t *testing.T) {
	_, err := decodeReviewPromotionPayload([]byte(`{"target":{"scope":"chapter","chapter":7}}`))
	if err == nil || !errors.Is(err, ErrInvalidCommand) {
		t.Fatalf("missing CAS error = %v, want ErrInvalidCommand", err)
	}

	hash := strings.Repeat("a", 64)
	payload, err := decodeReviewPromotionPayload([]byte(`{"target":{"scope":"chapter","chapter":7},"expected_fingerprint":"sha256:fp","expected_revisions":[{"chapter":7,"revision":3,"content_sha256":"` + hash + `"}]}`))
	if err != nil {
		t.Fatalf("valid payload rejected: %v", err)
	}
	if payload.ExpectedRevisions[0].Revision != 3 {
		t.Fatalf("decoded revision = %+v", payload.ExpectedRevisions[0])
	}
}

func TestReviewPromotionEligibilityAllowsBoundedStyleUnavailable(t *testing.T) {
	payload, status, snapshot := readyPromotionFixture()
	if err := validateReviewPromotionEligibility(payload, status, snapshot); err != nil {
		t.Fatalf("eligible chapter promotion rejected: %v", err)
	}
}

func TestReviewPromotionEligibilityRejectsCriticalAndMajorIssues(t *testing.T) {
	for _, severity := range []string{"critical", "error"} {
		t.Run(severity, func(t *testing.T) {
			payload, status, snapshot := readyPromotionFixture()
			snapshot.Review.Issues = []domain.ConsistencyIssue{{Severity: severity, Description: "block"}}
			err := validateReviewPromotionEligibility(payload, status, snapshot)
			if err == nil || !errors.Is(err, ErrMutationPrecondition) {
				t.Fatalf("severity %s error = %v, want ErrMutationPrecondition", severity, err)
			}
		})
	}
}

func TestReviewPromotionEligibilityRejectsStaleRevisionAndRequiredUnavailable(t *testing.T) {
	payload, status, snapshot := readyPromotionFixture()
	payload.ExpectedRevisions[0].Revision++
	if err := validateReviewPromotionEligibility(payload, status, snapshot); err == nil || !errors.Is(err, ErrMutationStale) {
		t.Fatalf("stale revision error = %v, want ErrMutationStale", err)
	}

	payload, status, snapshot = readyPromotionFixture()
	for i := range status.Gates {
		if status.Gates[i].GateID == ReviewGateContinuity {
			status.Gates[i].State = ReviewGateUnavailable
			break
		}
	}
	if err := validateReviewPromotionEligibility(payload, status, snapshot); err == nil || !errors.Is(err, ErrMutationPrecondition) {
		t.Fatalf("required unavailable error = %v, want ErrMutationPrecondition", err)
	}
}

func TestReviewPromotionEligibilityRejectsPostRepairStaleReview(t *testing.T) {
	payload, status, snapshot := readyPromotionFixture()
	snapshot.Revisions[0].AcceptedAt = snapshot.ReviewCheckpoint.OccurredAt.Add(time.Second)
	if err := validateReviewPromotionEligibility(payload, status, snapshot); err == nil || !errors.Is(err, ErrMutationPrecondition) {
		t.Fatalf("post-repair stale review error = %v, want ErrMutationPrecondition", err)
	}
}

func readyPromotionFixture() (ReviewPromoteOfficialCommandPayload, ReviewStatusResultDTO, host.DesktopReviewReadSnapshot) {
	now := time.Now().UTC()
	hash := strings.Repeat("b", 64)
	ref := ReviewRevisionRefDTO{Chapter: 7, Revision: 3, ContentSHA256: hash}
	gates := make([]ReviewGateResultDTO, 0, 12)
	for _, definition := range ReviewGateCatalog() {
		state := ReviewGatePass
		if definition.ID == ReviewGateStyleRegression {
			state = ReviewGateUnavailable
		}
		gates = append(gates, ReviewGateResultDTO{GateID: definition.ID, State: state})
	}
	payload := ReviewPromoteOfficialCommandPayload{
		Target:              ReviewTargetDTO{Scope: ReviewScopeChapter, Chapter: 7},
		ExpectedFingerprint: "sha256:fp",
		ExpectedRevisions:   []ReviewRevisionRefDTO{ref},
	}
	status := ReviewStatusResultDTO{
		ReviewID: "review-7",
		Target:   payload.Target,
		Overall:  ReviewGateUnavailable,
		Freshness: ReviewFreshnessDTO{
			Fingerprint: payload.ExpectedFingerprint,
			Revisions:   []ReviewRevisionRefDTO{ref},
		},
		Gates: gates,
	}
	snapshot := host.DesktopReviewReadSnapshot{
		Review:               &domain.ReviewEntry{Chapter: 7, Scope: "chapter", Verdict: "accept"},
		ReviewArtifactDigest: "digest",
		ReviewCheckpoint: &domain.Checkpoint{
			Seq: 1, Digest: "digest", OccurredAt: now,
		},
		Revisions: []domain.ChapterRecord{{
			Chapter: 7, Revision: 3, ContentSHA256: hash, AcceptedAt: now.Add(-time.Second),
		}},
		StyleStatus: "insufficient_sample",
	}
	return payload, status, snapshot
}
