package appruntime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/voocel/ainovel-cli/internal/domain"
)

// isReviewCommandKind is deliberately narrower than the reserved G06.2
// vocabulary. review.promote_official remains CLOSED until G06.5.
func isReviewCommandKind(kind CommandKind) bool {
	switch kind {
	case CommandReviewRun, CommandReviewRepair, CommandReviewRerun:
		return true
	default:
		return false
	}
}

type reviewCommandIntent struct {
	Target              ReviewTargetDTO
	ExpectedFingerprint string
	GateIDs             []ReviewGateID
	Chapters            []int
}

func (r *Runtime) dispatchReviewCommand(ctx context.Context, cmd CommandRequest) (CommandResult, error) {
	r.commandMu.Lock()
	defer r.commandMu.Unlock()

	cmd.ID = r.ensureCommandID(cmd.ID)
	result := CommandResult{CommandID: cmd.ID, RunID: cmd.RunID, TaskID: cmd.TaskID}
	if r.run == nil {
		return result, ErrNotImplemented
	}
	if strings.TrimSpace(cmd.Resource) != "" {
		return result, fmt.Errorf("%w: review commands derive the canonical story resource", ErrInvalidCommand)
	}

	intent, err := decodeReviewCommandIntent(cmd)
	if err != nil {
		return result, err
	}
	runID, err := reviewCommandRunID(cmd)
	if err != nil {
		return result, err
	}
	taskID := domain.TaskID(strings.TrimSpace(cmd.TaskID))
	if taskID == "" {
		taskID = domain.TaskID(cmd.Kind)
	}
	if err := (domain.RunIdentity{RunID: runID, TaskID: taskID}).Validate(); err != nil || runID == domain.LegacySingleRunID {
		return result, fmt.Errorf("%w: invalid review run identity", ErrInvalidCommand)
	}
	result.RunID = string(runID)
	result.TaskID = string(taskID)

	r.run.mu.Lock()
	defer r.run.mu.Unlock()

	record, err := r.run.backend.DesktopRunLoad(runID)
	if err != nil {
		return result, err
	}
	if record != nil {
		if !reviewCommandMatchesExisting(record, cmd.Kind, taskID, intent) {
			return result, fmt.Errorf("%w: command id already belongs to different run work", ErrCommandRejected)
		}
		// Durable command identity wins on replay. Do not recompute a fresh CAS
		// against story state that the already-accepted repair/rerun may itself
		// have changed.
		result.Accepted = true
		result.Status = record.State
		result.Data, _ = json.Marshal(reviewCommandResult(intent.Target, cmd.Kind, record.State))
		return result, nil
	}

	work, err := r.prepareReviewWork(cmd.Kind, intent)
	if err != nil {
		return result, err
	}
	record = &domain.RunRegistryRecord{
		RunID:      runID,
		TaskID:     taskID,
		Source:     domain.RunRecordSourceRegistry,
		ReviewWork: &work,
	}
	if err := r.run.queueRecordLocked(record, RunStateQueued, "review action waiting for deterministic scheduler"); err != nil {
		return result, err
	}
	result.Accepted = true
	result.Status = string(RunStateQueued)
	if err := r.run.reconcileAndScheduleLocked(ctx); err != nil {
		return result, err
	}
	if latest, loadErr := r.run.backend.DesktopRunLoad(runID); loadErr == nil && latest != nil {
		result.Status = latest.State
	}
	result.Data, _ = json.Marshal(reviewCommandResult(intent.Target, cmd.Kind, result.Status))
	r.emitReviewAction(runID, taskID, intent.Target, cmd.Kind, result.Status, work.Chapters)
	r.run.wake()
	return result, nil
}

func decodeReviewCommandIntent(cmd CommandRequest) (reviewCommandIntent, error) {
	var intent reviewCommandIntent
	switch cmd.Kind {
	case CommandReviewRun:
		var payload ReviewRunCommandPayload
		if err := decodeReviewPayload(cmd.Payload, &payload); err != nil {
			return intent, err
		}
		intent.Target = payload.Target
		intent.ExpectedFingerprint = strings.TrimSpace(payload.ExpectedFingerprint)
	case CommandReviewRepair:
		var payload ReviewRepairCommandPayload
		if err := decodeReviewPayload(cmd.Payload, &payload); err != nil {
			return intent, err
		}
		intent.Target = payload.Target
		intent.ExpectedFingerprint = strings.TrimSpace(payload.ExpectedFingerprint)
		intent.GateIDs = slices.Clone(payload.GateIDs)
		intent.Chapters = slices.Clone(payload.Chapters)
		if intent.ExpectedFingerprint == "" {
			return intent, fmt.Errorf("%w: review.repair requires expected_fingerprint", ErrInvalidCommand)
		}
	case CommandReviewRerun:
		var payload ReviewRerunCommandPayload
		if err := decodeReviewPayload(cmd.Payload, &payload); err != nil {
			return intent, err
		}
		intent.Target = payload.Target
		intent.ExpectedFingerprint = strings.TrimSpace(payload.ExpectedFingerprint)
		if intent.ExpectedFingerprint == "" {
			return intent, fmt.Errorf("%w: review.rerun requires expected_fingerprint", ErrInvalidCommand)
		}
	default:
		return intent, fmt.Errorf("%w: %q", ErrInvalidCommand, cmd.Kind)
	}
	if err := validateReviewTarget(intent.Target); err != nil {
		return intent, fmt.Errorf("%w: invalid review target: %v", ErrInvalidCommand, err)
	}
	return intent, nil
}

func reviewCommandMatchesExisting(record *domain.RunRegistryRecord, kind CommandKind, taskID domain.TaskID, intent reviewCommandIntent) bool {
	if record == nil || record.ReviewWork == nil || record.TaskID != taskID {
		return false
	}
	work := record.ReviewWork
	if work.Action != string(kind) || work.Target != reviewWorkTarget(intent.Target) || work.ExpectedFingerprint != intent.ExpectedFingerprint {
		return false
	}
	if kind == CommandReviewRepair {
		return slices.Equal(work.GateIDs, reviewGateStrings(intent.GateIDs)) && slices.Equal(work.Chapters, intent.Chapters)
	}
	return len(work.GateIDs) == 0 && len(work.Chapters) == 0 && work.RepairMode == ""
}

func (r *Runtime) prepareReviewWork(kind CommandKind, intent reviewCommandIntent) (domain.ReviewRunWork, error) {
	snapshot, err := r.core.DesktopReviewRead(hostReviewTarget(intent.Target))
	if err != nil {
		return domain.ReviewRunWork{}, err
	}
	status, err := aggregateReviewStatus(intent.Target, snapshot)
	if err != nil {
		return domain.ReviewRunWork{}, err
	}
	if intent.ExpectedFingerprint != "" && intent.ExpectedFingerprint != status.Freshness.Fingerprint {
		return domain.ReviewRunWork{}, fmt.Errorf("%w: review target fingerprint changed", ErrMutationPrecondition)
	}
	var baselineSeq int64
	if snapshot.ReviewCheckpoint != nil {
		baselineSeq = snapshot.ReviewCheckpoint.Seq
	}

	work := domain.ReviewRunWork{
		Action:              string(kind),
		Target:              reviewWorkTarget(intent.Target),
		ExpectedFingerprint: intent.ExpectedFingerprint,
		ExpectedRevisions:   reviewWorkRevisions(status.Freshness.Revisions),
		BaselineReviewSeq:   baselineSeq,
	}
	if kind == CommandReviewRepair {
		mode, err := validateRepairSelection(status, intent.GateIDs, intent.Chapters)
		if err != nil {
			return domain.ReviewRunWork{}, err
		}
		work.GateIDs = reviewGateStrings(intent.GateIDs)
		work.Chapters = slices.Clone(intent.Chapters)
		work.RepairMode = mode
	}
	if err := work.Validate(); err != nil {
		return domain.ReviewRunWork{}, fmt.Errorf("%w: %v", ErrInvalidCommand, err)
	}
	return work, nil
}

func decodeReviewPayload(raw json.RawMessage, dst any) error {
	if len(raw) == 0 || string(raw) == "null" {
		return fmt.Errorf("%w: review command payload is required", ErrInvalidCommand)
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return fmt.Errorf("%w: review payload: %v", ErrInvalidCommand, err)
	}
	return nil
}

func validateRepairSelection(status ReviewStatusResultDTO, gateIDs []ReviewGateID, chapters []int) (string, error) {
	if status.ReviewID == "" || status.Verdict == "" {
		return "", fmt.Errorf("%w: review.repair requires canonical review evidence", ErrMutationPrecondition)
	}
	mode := strings.ToLower(strings.TrimSpace(status.Verdict))
	if mode != "rewrite" && mode != "polish" {
		return "", fmt.Errorf("%w: review verdict %q has no repair action", ErrCommandNotAllowed, status.Verdict)
	}
	if len(chapters) == 0 {
		return "", fmt.Errorf("%w: review.repair requires chapters", ErrInvalidCommand)
	}
	seen := make(map[int]struct{}, len(chapters))
	for _, chapter := range chapters {
		if chapter <= 0 || !slices.Contains(status.AffectedChapters, chapter) {
			return "", fmt.Errorf("%w: chapter %d is outside canonical review affected chapters %v", ErrCommandNotAllowed, chapter, status.AffectedChapters)
		}
		if _, exists := seen[chapter]; exists {
			return "", fmt.Errorf("%w: duplicate repair chapter %d", ErrInvalidCommand, chapter)
		}
		seen[chapter] = struct{}{}
	}
	if len(gateIDs) != 0 {
		seenGates := make(map[ReviewGateID]struct{}, len(gateIDs))
		for _, gateID := range gateIDs {
			if _, exists := seenGates[gateID]; exists {
				return "", fmt.Errorf("%w: duplicate repair gate %q", ErrInvalidCommand, gateID)
			}
			seenGates[gateID] = struct{}{}
			found := false
			for _, gate := range status.Gates {
				if gate.GateID == gateID {
					found = true
					if !gate.Actionable {
						return "", fmt.Errorf("%w: review gate %q is not actionable", ErrCommandNotAllowed, gateID)
					}
					break
				}
			}
			if !found {
				return "", fmt.Errorf("%w: unknown review gate %q", ErrInvalidCommand, gateID)
			}
		}
	}
	return mode, nil
}

func reviewCommandRunID(cmd CommandRequest) (domain.RunID, error) {
	if supplied := strings.TrimSpace(cmd.RunID); supplied != "" {
		runID := domain.RunID(supplied)
		if err := (domain.RunIdentity{RunID: runID}).Validate(); err != nil || runID == domain.LegacySingleRunID {
			return "", fmt.Errorf("%w: invalid review run_id", ErrInvalidCommand)
		}
		return runID, nil
	}
	sum := sha256.Sum256([]byte(cmd.ID))
	return domain.RunID("review-" + hex.EncodeToString(sum[:12])), nil
}

func reviewWorkTarget(target ReviewTargetDTO) domain.ReviewWorkTarget {
	return domain.ReviewWorkTarget{Scope: string(target.Scope), Chapter: target.Chapter, Volume: target.Volume, Arc: target.Arc, ThroughChapter: target.ThroughChapter}
}

func reviewWorkRevisions(refs []ReviewRevisionRefDTO) []domain.ReviewWorkRevision {
	out := make([]domain.ReviewWorkRevision, 0, len(refs))
	for _, ref := range refs {
		out = append(out, domain.ReviewWorkRevision{Chapter: ref.Chapter, Revision: ref.Revision, ContentSHA256: ref.ContentSHA256})
	}
	return out
}

func reviewGateStrings(ids []ReviewGateID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, string(id))
	}
	return out
}

func reviewCommandResult(target ReviewTargetDTO, kind CommandKind, runState string) ReviewCommandResultDTO {
	return ReviewCommandResultDTO{Target: target, State: reviewRunState(runState), Action: string(kind)}
}

func reviewRunState(state string) ReviewRunState {
	switch RunLifecycleState(state) {
	case RunStateRunning:
		return ReviewRunRunning
	case RunStateCompleted:
		return ReviewRunCompleted
	case RunStateFailed:
		return ReviewRunFailed
	case RunStateCancelled, RunStateStopped:
		return ReviewRunCancelled
	default:
		return ReviewRunQueued
	}
}

func (r *Runtime) emitReviewAction(runID domain.RunID, taskID domain.TaskID, target ReviewTargetDTO, kind CommandKind, state string, chapters []int) {
	payload, _ := json.Marshal(ReviewActionEventPayloadDTO{Target: target, Action: string(kind), State: state, Chapters: slices.Clone(chapters)})
	r.broadcast(DesktopEvent{Time: time.Now().UTC(), Category: ReviewEventCategory, Type: EventTypeReviewAction, Level: "info", RunID: string(runID), TaskID: string(taskID), Summary: string(kind), Payload: payload})
}
