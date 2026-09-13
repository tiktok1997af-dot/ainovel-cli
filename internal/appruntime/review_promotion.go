package appruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/voocel/ainovel-cli/internal/domain"
	apperrs "github.com/voocel/ainovel-cli/internal/errs"
	"github.com/voocel/ainovel-cli/internal/host"
)

const promotionStatusCompleted = "completed"

func (r *Runtime) dispatchReviewPromotion(ctx context.Context, cmd CommandRequest) (CommandResult, error) {
	r.commandMu.Lock()
	defer r.commandMu.Unlock()

	cmd.ID = r.ensureCommandID(cmd.ID)
	result := CommandResult{CommandID: cmd.ID, RunID: cmd.RunID, TaskID: cmd.TaskID}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if strings.TrimSpace(cmd.Resource) != "" {
		return result, fmt.Errorf("%w: review promotion derives its canonical target resource", ErrInvalidCommand)
	}
	if err := r.ensureMutationState(); err != nil {
		return result, err
	}

	payload, err := decodeReviewPromotionPayload(cmd.Payload)
	if err != nil {
		return result, err
	}
	snapshot, err := r.core.DesktopReviewRead(hostReviewTarget(payload.Target))
	if err != nil {
		return result, err
	}
	status, err := aggregateReviewStatus(payload.Target, snapshot)
	if err != nil {
		return result, err
	}
	if err := validateReviewPromotionEligibility(payload, status, snapshot); err != nil {
		return result, err
	}

	refs := make([]domain.OfficialRevisionRef, 0, len(payload.ExpectedRevisions))
	for _, ref := range payload.ExpectedRevisions {
		refs = append(refs, domain.OfficialRevisionRef{
			Chapter:       ref.Chapter,
			Revision:      ref.Revision,
			ContentSHA256: ref.ContentSHA256,
		})
	}
	selection, changed, err := r.core.DesktopPromoteOfficial(hostReviewTarget(payload.Target), refs, payload.ExpectedFingerprint)
	if err != nil {
		return result, mapPromotionCoreError(err)
	}

	data, err := json.Marshal(ReviewPromoteOfficialResultDTO{
		Target:            payload.Target,
		Revisions:         officialSelectionRefs(selection.Revisions),
		ReviewFingerprint: selection.ReviewFingerprint,
		PromotedAt:        selection.PromotedAt,
		AlreadyOfficial:   !changed,
	})
	if err != nil {
		return result, fmt.Errorf("marshal review promotion result: %w", err)
	}
	result.Accepted = true
	result.Status = promotionStatusCompleted
	result.Data = data
	r.emitReviewAction("", "", payload.Target, CommandReviewPromoteOfficial, promotionStatusCompleted, revisionRefChapters(payload.ExpectedRevisions))
	return result, nil
}

func decodeReviewPromotionPayload(raw json.RawMessage) (ReviewPromoteOfficialCommandPayload, error) {
	var payload ReviewPromoteOfficialCommandPayload
	if len(raw) == 0 || string(raw) == "null" {
		return payload, fmt.Errorf("%w: review promotion payload is required", ErrInvalidCommand)
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return payload, fmt.Errorf("%w: review promotion payload: %v", ErrInvalidCommand, err)
	}
	if err := validateReviewTarget(payload.Target); err != nil {
		return payload, fmt.Errorf("%w: invalid review target: %v", ErrInvalidCommand, err)
	}
	payload.ExpectedFingerprint = strings.TrimSpace(payload.ExpectedFingerprint)
	if payload.ExpectedFingerprint == "" || len(payload.ExpectedRevisions) == 0 {
		return payload, fmt.Errorf("%w: expected_fingerprint and expected_revisions are required", ErrInvalidCommand)
	}
	seen := make(map[int]struct{}, len(payload.ExpectedRevisions))
	for _, ref := range payload.ExpectedRevisions {
		if ref.Chapter <= 0 || ref.Revision <= 0 || !validSHA256(ref.ContentSHA256) {
			return payload, fmt.Errorf("%w: invalid expected revision for chapter %d", ErrInvalidCommand, ref.Chapter)
		}
		if _, exists := seen[ref.Chapter]; exists {
			return payload, fmt.Errorf("%w: duplicate expected revision for chapter %d", ErrInvalidCommand, ref.Chapter)
		}
		seen[ref.Chapter] = struct{}{}
	}
	return payload, nil
}

func validateReviewPromotionEligibility(
	payload ReviewPromoteOfficialCommandPayload,
	status ReviewStatusResultDTO,
	snapshot host.DesktopReviewReadSnapshot,
) error {
	if payload.ExpectedFingerprint != status.Freshness.Fingerprint {
		return fmt.Errorf("%w: review fingerprint changed", ErrMutationStale)
	}
	if !sameReviewRevisionSet(payload.ExpectedRevisions, status.Freshness.Revisions) {
		return fmt.Errorf("%w: review target revisions changed", ErrMutationStale)
	}
	if !reviewSemanticFresh(snapshot) || snapshot.Review == nil {
		return fmt.Errorf("%w: canonical review is stale or incomplete; rerun review before promotion", ErrMutationPrecondition)
	}
	if snapshot.Review.CriticalCount() > 0 || snapshot.Review.ErrorCount() > 0 {
		return fmt.Errorf("%w: unresolved critical/major semantic review issues block promotion", ErrMutationPrecondition)
	}

	required := map[ReviewGateID]bool{
		ReviewGateContractFulfillment: true,
		ReviewGateConsistency:         true,
		ReviewGateCharacter:           true,
		ReviewGatePacing:              true,
		ReviewGateContinuity:          true,
		ReviewGateForeshadow:          true,
		ReviewGateHook:                true,
		ReviewGateAesthetic:           true,
		ReviewGateFlowIntegrity:       true,
		ReviewGatePlanningIntegrity:   true,
		ReviewGateContextIntegrity:    true,
	}
	seen := make(map[ReviewGateID]bool, len(status.Gates))
	for _, gate := range status.Gates {
		seen[gate.GateID] = true
		if gate.GateID == ReviewGateStyleRegression {
			if gate.State == ReviewGatePass || gate.State == ReviewGateWarn {
				continue
			}
			if gate.State == ReviewGateUnavailable && snapshot.StyleStatus == "insufficient_sample" && len(snapshot.Revisions) < 5 {
				continue
			}
			return fmt.Errorf("%w: style regression gate is not promotion-ready (%s)", ErrMutationPrecondition, gate.State)
		}
		if required[gate.GateID] && gate.State != ReviewGatePass && gate.State != ReviewGateWarn {
			return fmt.Errorf("%w: required review gate %s is not promotion-ready (%s)", ErrMutationPrecondition, gate.GateID, gate.State)
		}
	}
	for gateID := range required {
		if !seen[gateID] {
			return fmt.Errorf("%w: required review gate %s is missing", ErrMutationPrecondition, gateID)
		}
	}
	if !seen[ReviewGateStyleRegression] {
		return fmt.Errorf("%w: style regression gate is missing", ErrMutationPrecondition)
	}
	return nil
}

func sameReviewRevisionSet(a, b []ReviewRevisionRefDTO) bool {
	if len(a) != len(b) {
		return false
	}
	left := slices.Clone(a)
	right := slices.Clone(b)
	sort.Slice(left, func(i, j int) bool { return left[i].Chapter < left[j].Chapter })
	sort.Slice(right, func(i, j int) bool { return right[i].Chapter < right[j].Chapter })
	return slices.Equal(left, right)
}

func officialSelectionRefs(refs []domain.OfficialRevisionRef) []ReviewRevisionRefDTO {
	out := make([]ReviewRevisionRefDTO, 0, len(refs))
	for _, ref := range refs {
		out = append(out, ReviewRevisionRefDTO{Chapter: ref.Chapter, Revision: ref.Revision, ContentSHA256: ref.ContentSHA256})
	}
	return out
}

func revisionRefChapters(refs []ReviewRevisionRefDTO) []int {
	chapters := make([]int, 0, len(refs))
	for _, ref := range refs {
		chapters = append(chapters, ref.Chapter)
	}
	return chapters
}

func mapPromotionCoreError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, apperrs.ErrToolConflict):
		return fmt.Errorf("%w: %v", ErrMutationStale, err)
	case errors.Is(err, apperrs.ErrToolArgs):
		return fmt.Errorf("%w: %v", ErrInvalidCommand, err)
	default:
		return mapMutationCoreError(err)
	}
}
