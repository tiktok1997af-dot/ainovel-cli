package host

import (
	"context"
	"fmt"
	"slices"

	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/flow"
)

// DesktopExecuteReviewWork executes one durable G06.4 review job after the G05
// coordinator has already acquired the canonical story resource and browser lane.
// It never schedules work itself and never advances outside the bounded target.
func (h *Host) DesktopExecuteReviewWork(ctx context.Context, runID domain.RunID) error {
	if h == nil || h.store == nil || h.store.Runtime == nil || h.engine == nil {
		return fmt.Errorf("host: review execution is unavailable")
	}
	record, err := h.store.Runtime.LoadRun(runID)
	if err != nil {
		return err
	}
	if record == nil || record.ReviewWork == nil {
		return fmt.Errorf("host: run %q has no durable review work", runID)
	}
	work := *record.ReviewWork
	if err := work.Validate(); err != nil {
		return fmt.Errorf("host: invalid review work: %w", err)
	}
	if work.Executed {
		return nil
	}

	target := desktopReviewTargetFromWork(work.Target)
	if _, err := h.DesktopReviewRead(target); err != nil {
		return err
	}

	switch work.Action {
	case domain.ReviewWorkRun, domain.ReviewWorkRerun:
		return h.executeDesktopReviewEvaluation(ctx, runID, work, target)
	case domain.ReviewWorkRepair:
		return h.executeDesktopReviewRepair(ctx, runID, work, target)
	default:
		return fmt.Errorf("host: unsupported review action %q", work.Action)
	}
}

func (h *Host) executeDesktopReviewEvaluation(ctx context.Context, runID domain.RunID, work domain.ReviewRunWork, target DesktopReviewTarget) error {
	if work.Prepared {
		snapshot, err := h.DesktopReviewRead(target)
		if err != nil {
			return err
		}
		if latestReviewSeq(snapshot) > work.BaselineReviewSeq {
			return h.updateDesktopReviewWork(runID, func(current *domain.ReviewRunWork) error {
				current.Executed = true
				return nil
			})
		}
	} else {
		if err := h.updateDesktopReviewWork(runID, func(current *domain.ReviewRunWork) error {
			current.Prepared = true
			return nil
		}); err != nil {
			return err
		}
	}

	inst := desktopReviewInstruction(target)
	if err := h.engine.runDesktopReviewInstruction(ctx, inst); err != nil {
		return err
	}
	snapshot, err := h.DesktopReviewRead(target)
	if err != nil {
		return err
	}
	if latestReviewSeq(snapshot) <= work.BaselineReviewSeq {
		return fmt.Errorf("review worker returned without committing a new canonical review checkpoint")
	}
	return h.updateDesktopReviewWork(runID, func(current *domain.ReviewRunWork) error {
		current.Executed = true
		return nil
	})
}

func (h *Host) executeDesktopReviewRepair(ctx context.Context, runID domain.RunID, work domain.ReviewRunWork, target DesktopReviewTarget) error {
	if !work.Prepared {
		if err := h.prepareDesktopRepairQueue(work); err != nil {
			return err
		}
		// Mark Prepared only after the canonical Progress queue is durable. If a
		// crash occurs between those writes, prepareDesktopRepairQueue recognizes
		// the exact existing bounded queue and is idempotent on restart.
		if err := h.updateDesktopReviewWork(runID, func(current *domain.ReviewRunWork) error {
			current.Prepared = true
			return nil
		}); err != nil {
			return err
		}
		work.Prepared = true
	}

	completed := make(map[int]struct{}, len(work.CompletedChapters))
	for _, chapter := range work.CompletedChapters {
		completed[chapter] = struct{}{}
	}

	for _, chapter := range work.Chapters {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, done := completed[chapter]; done {
			continue
		}
		expected, ok := expectedReviewRevision(work, chapter)
		if !ok {
			return fmt.Errorf("review repair chapter %d has no expected revision", chapter)
		}
		changed, err := h.desktopReviewChapterChanged(expected)
		if err != nil {
			return err
		}
		if changed {
			if err := h.markDesktopReviewChapterComplete(runID, chapter); err != nil {
				return err
			}
			completed[chapter] = struct{}{}
			continue
		}

		if err := h.ensureDesktopRepairQueue(work, completed); err != nil {
			return err
		}
		inst := &flow.Instruction{
			Agent:   "writer",
			Task:    fmt.Sprintf("%s第 %d 章；仅处理本次 Review 已授权的返工，不得推进新章节", repairVerb(work.RepairMode), chapter),
			Reason:  "G06.4 bounded review repair",
			Chapter: chapter,
		}
		if err := h.engine.runDesktopReviewInstruction(ctx, inst); err != nil {
			return err
		}
		changed, err = h.desktopReviewChapterChanged(expected)
		if err != nil {
			return err
		}
		if !changed {
			return fmt.Errorf("review repair chapter %d completed without creating a new accepted revision", chapter)
		}
		if err := h.markDesktopReviewChapterComplete(runID, chapter); err != nil {
			return err
		}
		completed[chapter] = struct{}{}
	}

	return h.updateDesktopReviewWork(runID, func(current *domain.ReviewRunWork) error {
		current.Executed = true
		return nil
	})
}

func (h *Host) prepareDesktopRepairQueue(work domain.ReviewRunWork) error {
	progress, err := h.store.Progress.Load()
	if err != nil {
		return err
	}
	if progress == nil {
		return fmt.Errorf("review repair requires project progress")
	}
	if len(progress.PendingRewrites) != 0 {
		if !slices.Equal(progress.PendingRewrites, work.Chapters) {
			return fmt.Errorf("review repair cannot replace existing pending rewrite queue %v", progress.PendingRewrites)
		}
		if progress.Flow == repairFlow(work.RepairMode) {
			return nil
		}
		_, err = h.store.Progress.ApplyReviewOutcome(repairFlow(work.RepairMode), work.Chapters, "desktop review repair")
		return err
	}

	if progress.Phase == domain.PhaseComplete {
		// Reopen is the sole Store-owned legal complete->writing transition. Do
		// not bypass it with a generic Flow update.
		if err := h.store.Progress.Reopen(work.Chapters, "desktop review repair"); err != nil {
			return err
		}
		if work.RepairMode == "polish" {
			_, err = h.store.Progress.ApplyReviewOutcome(domain.FlowPolishing, work.Chapters, "desktop review repair")
			return err
		}
		return nil
	}
	_, err = h.store.Progress.ApplyReviewOutcome(repairFlow(work.RepairMode), work.Chapters, "desktop review repair")
	return err
}

func (h *Host) ensureDesktopRepairQueue(work domain.ReviewRunWork, completed map[int]struct{}) error {
	progress, err := h.store.Progress.Load()
	if err != nil {
		return err
	}
	if progress == nil {
		return fmt.Errorf("review repair requires project progress")
	}
	remaining := make([]int, 0, len(work.Chapters))
	for _, chapter := range work.Chapters {
		if _, done := completed[chapter]; !done {
			remaining = append(remaining, chapter)
		}
	}
	for _, pending := range progress.PendingRewrites {
		if !slices.Contains(remaining, pending) {
			return fmt.Errorf("pending rewrite chapter %d escaped bounded review repair set %v", pending, remaining)
		}
	}
	if slices.Equal(progress.PendingRewrites, remaining) && progress.Flow == repairFlow(work.RepairMode) {
		return nil
	}
	_, err = h.store.Progress.ApplyReviewOutcome(repairFlow(work.RepairMode), remaining, "desktop review repair recovery")
	return err
}

func (h *Host) desktopReviewChapterChanged(expected domain.ReviewWorkRevision) (bool, error) {
	records, err := h.store.ChapterRecords.LoadCompleted([]int{expected.Chapter})
	if err != nil {
		return false, err
	}
	if len(records) != 1 {
		return false, fmt.Errorf("chapter %d has no canonical accepted revision", expected.Chapter)
	}
	current := records[0]
	return current.Revision != expected.Revision || current.ContentSHA256 != expected.ContentSHA256, nil
}

func (h *Host) markDesktopReviewChapterComplete(runID domain.RunID, chapter int) error {
	return h.updateDesktopReviewWork(runID, func(current *domain.ReviewRunWork) error {
		if !slices.Contains(current.CompletedChapters, chapter) {
			current.CompletedChapters = append(current.CompletedChapters, chapter)
		}
		return nil
	})
}

func (h *Host) updateDesktopReviewWork(runID domain.RunID, mutate func(*domain.ReviewRunWork) error) error {
	record, err := h.store.Runtime.LoadRun(runID)
	if err != nil {
		return err
	}
	if record == nil || record.ReviewWork == nil {
		return fmt.Errorf("run %q has no review work", runID)
	}
	work := *record.ReviewWork
	work.GateIDs = slices.Clone(work.GateIDs)
	work.Chapters = slices.Clone(work.Chapters)
	work.CompletedChapters = slices.Clone(work.CompletedChapters)
	work.ExpectedRevisions = slices.Clone(work.ExpectedRevisions)
	if err := mutate(&work); err != nil {
		return err
	}
	if err := work.Validate(); err != nil {
		return err
	}
	record.ReviewWork = &work
	_, err = h.store.Runtime.SaveRun(*record)
	return err
}

func desktopReviewTargetFromWork(target domain.ReviewWorkTarget) DesktopReviewTarget {
	return DesktopReviewTarget{
		Scope:          target.Scope,
		Chapter:        target.Chapter,
		Volume:         target.Volume,
		Arc:            target.Arc,
		ThroughChapter: target.ThroughChapter,
	}
}

func desktopReviewInstruction(target DesktopReviewTarget) *flow.Instruction {
	switch target.Scope {
	case "chapter":
		return &flow.Instruction{Agent: "editor", Task: fmt.Sprintf("审阅第 %d 章：调用 novel_context(chapter=%d)，save_review 使用 scope=chapter、chapter=%d", target.Chapter, target.Chapter, target.Chapter), Reason: "G06.4 bounded chapter review"}
	case "arc":
		return &flow.Instruction{Agent: "editor", Task: fmt.Sprintf("审阅第 %d 卷第 %d 弧至第 %d 章：调用 novel_context(chapter=%d)，save_review 使用 scope=arc、chapter=%d", target.Volume, target.Arc, target.ThroughChapter, target.ThroughChapter, target.ThroughChapter), Reason: "G06.4 bounded arc review"}
	default:
		return &flow.Instruction{Agent: "editor", Task: fmt.Sprintf("审阅前 %d 章：调用 novel_context(chapter=%d)，save_review 使用 scope=global、chapter=%d", target.ThroughChapter, target.ThroughChapter, target.ThroughChapter), Reason: "G06.4 bounded global review"}
	}
}

func latestReviewSeq(snapshot DesktopReviewReadSnapshot) int64 {
	if snapshot.ReviewCheckpoint == nil {
		return 0
	}
	return snapshot.ReviewCheckpoint.Seq
}

func expectedReviewRevision(work domain.ReviewRunWork, chapter int) (domain.ReviewWorkRevision, bool) {
	for _, revision := range work.ExpectedRevisions {
		if revision.Chapter == chapter {
			return revision, true
		}
	}
	return domain.ReviewWorkRevision{}, false
}

func repairFlow(mode string) domain.FlowState {
	if mode == "polish" {
		return domain.FlowPolishing
	}
	return domain.FlowRewriting
}

func repairVerb(mode string) string {
	if mode == "polish" {
		return "打磨"
	}
	return "重写"
}
