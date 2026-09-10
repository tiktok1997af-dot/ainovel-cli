package host

import (
	"fmt"
	"sort"

	"github.com/voocel/ainovel-cli/internal/domain"
	apperrs "github.com/voocel/ainovel-cli/internal/errs"
)

// DesktopCharacterKnowledgeSnapshot is the read-only composition used by the
// desktop Characters view. Designed characters and the supporting-cast ledger
// stay distinct so AppRuntime can preserve origin and write routing later.
type DesktopCharacterKnowledgeSnapshot struct {
	Characters []domain.Character
	Cast       []domain.CastEntry
}

// DesktopWorldKnowledgeSnapshot is a read-only view over the WorldStore
// subdomains exposed by G03. It contains domain values only, never Store
// pointers or mutation handles.
type DesktopWorldKnowledgeSnapshot struct {
	Rules         []domain.WorldRule
	Foreshadow    []domain.ForeshadowEntry
	Relationships []domain.RelationshipEntry
	StateChanges  []domain.StateChange
}

// DesktopKnowledgeReadSnapshot is the bounded-source composition used by the
// desktop Context and Canon read models. Project/knowledge persistence remains
// owned by the existing stores; this struct is ephemeral and is never saved.
type DesktopKnowledgeReadSnapshot struct {
	Project    DesktopProjectReadSnapshot
	Characters DesktopCharacterKnowledgeSnapshot
	World      DesktopWorldKnowledgeSnapshot
	Timeline   []domain.TimelineEvent
	Records    []domain.ChapterRecord
	Summaries  []domain.ChapterSummary
}

// DesktopCharacterKnowledgeRead loads the two canonical character authorities
// without merging them. Same-name precedence is projected by AppRuntime.
func (h *Host) DesktopCharacterKnowledgeRead() (DesktopCharacterKnowledgeSnapshot, error) {
	if h == nil || h.store == nil {
		return DesktopCharacterKnowledgeSnapshot{}, fmt.Errorf("desktop project store unavailable: %w", apperrs.ErrStoreRead)
	}
	characters, err := h.store.Characters.Load()
	if err != nil {
		return DesktopCharacterKnowledgeSnapshot{}, desktopProjectReadError("characters", err)
	}
	cast, err := h.store.Cast.Load()
	if err != nil {
		return DesktopCharacterKnowledgeSnapshot{}, desktopProjectReadError("cast ledger", err)
	}
	return DesktopCharacterKnowledgeSnapshot{Characters: characters, Cast: cast}, nil
}

// DesktopWorldKnowledgeRead loads the current canonical WorldStore facts used
// by the desktop World and semantic Canon projections.
func (h *Host) DesktopWorldKnowledgeRead() (DesktopWorldKnowledgeSnapshot, error) {
	if h == nil || h.store == nil {
		return DesktopWorldKnowledgeSnapshot{}, fmt.Errorf("desktop project store unavailable: %w", apperrs.ErrStoreRead)
	}
	rules, err := h.store.World.LoadWorldRules()
	if err != nil {
		return DesktopWorldKnowledgeSnapshot{}, desktopProjectReadError("world rules", err)
	}
	foreshadow, err := h.store.World.LoadForeshadowLedger()
	if err != nil {
		return DesktopWorldKnowledgeSnapshot{}, desktopProjectReadError("foreshadow ledger", err)
	}
	relationships, err := h.store.World.LoadRelationships()
	if err != nil {
		return DesktopWorldKnowledgeSnapshot{}, desktopProjectReadError("relationship state", err)
	}
	stateChanges, err := h.store.World.LoadStateChanges()
	if err != nil {
		return DesktopWorldKnowledgeSnapshot{}, desktopProjectReadError("state changes", err)
	}
	return DesktopWorldKnowledgeSnapshot{
		Rules:         rules,
		Foreshadow:    foreshadow,
		Relationships: relationships,
		StateChanges:  stateChanges,
	}, nil
}

// DesktopTimelineKnowledgeRead keeps Timeline a WorldStore subdomain while
// exposing only a detached slice for AppRuntime projection.
func (h *Host) DesktopTimelineKnowledgeRead() ([]domain.TimelineEvent, error) {
	if h == nil || h.store == nil {
		return nil, fmt.Errorf("desktop project store unavailable: %w", apperrs.ErrStoreRead)
	}
	timeline, err := h.store.World.LoadTimeline()
	if err != nil {
		return nil, desktopProjectReadError("timeline", err)
	}
	return timeline, nil
}

// DesktopKnowledgeRead composes the existing project, character and world
// authorities for Context/Canon. Accepted chapter records and summaries are
// discovered only through Progress.CompletedChapters; it does not scan the
// filesystem or create a parallel knowledge index. Missing optional records or
// summaries remain missing, while corrupt/unreadable artifacts are errors.
func (h *Host) DesktopKnowledgeRead() (DesktopKnowledgeReadSnapshot, error) {
	project, err := h.DesktopProjectRead()
	if err != nil {
		return DesktopKnowledgeReadSnapshot{}, err
	}
	characters, err := h.DesktopCharacterKnowledgeRead()
	if err != nil {
		return DesktopKnowledgeReadSnapshot{}, err
	}
	world, err := h.DesktopWorldKnowledgeRead()
	if err != nil {
		return DesktopKnowledgeReadSnapshot{}, err
	}
	timeline, err := h.DesktopTimelineKnowledgeRead()
	if err != nil {
		return DesktopKnowledgeReadSnapshot{}, err
	}

	var records []domain.ChapterRecord
	var summaries []domain.ChapterSummary
	for _, chapter := range uniqueSortedCompletedChapters(project.Progress) {
		record, readErr := h.store.ChapterRecords.Load(chapter)
		if readErr != nil {
			return DesktopKnowledgeReadSnapshot{}, desktopProjectReadError("chapter record", readErr)
		}
		if record != nil {
			records = append(records, *record)
		}
		summary, readErr := h.store.Summaries.LoadSummary(chapter)
		if readErr != nil {
			return DesktopKnowledgeReadSnapshot{}, desktopProjectReadError("chapter summary", readErr)
		}
		if summary != nil {
			summaries = append(summaries, *summary)
		}
	}

	return DesktopKnowledgeReadSnapshot{
		Project:    project,
		Characters: characters,
		World:      world,
		Timeline:   timeline,
		Records:    records,
		Summaries:  summaries,
	}, nil
}

func uniqueSortedCompletedChapters(progress *domain.Progress) []int {
	if progress == nil || len(progress.CompletedChapters) == 0 {
		return nil
	}
	seen := make(map[int]struct{}, len(progress.CompletedChapters))
	chapters := make([]int, 0, len(progress.CompletedChapters))
	for _, chapter := range progress.CompletedChapters {
		if chapter <= 0 {
			continue
		}
		if _, ok := seen[chapter]; ok {
			continue
		}
		seen[chapter] = struct{}{}
		chapters = append(chapters, chapter)
	}
	sort.Ints(chapters)
	return chapters
}
