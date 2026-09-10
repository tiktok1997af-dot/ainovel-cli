package host

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/voocel/ainovel-cli/internal/domain"
	apperrs "github.com/voocel/ainovel-cli/internal/errs"
	"github.com/voocel/ainovel-cli/internal/store"
)

func newDesktopMutationTestHost(t *testing.T) (*Host, *store.Store) {
	t.Helper()
	st := store.NewStore(t.TempDir())
	if err := st.Init(); err != nil {
		t.Fatalf("init store: %v", err)
	}
	return &Host{store: st}, st
}

func TestDesktopSimpleMutationSeamsWriteCanonicalOwners(t *testing.T) {
	h, st := newDesktopMutationTestHost(t)

	book := domain.BookMetadata{Title: "Book", Synopsis: "Synopsis"}
	if err := h.DesktopSaveBookMetadata(book); err != nil {
		t.Fatalf("save book: %v", err)
	}
	if got, err := st.Book.Load(); err != nil || got == nil || *got != book {
		t.Fatalf("book = %+v err=%v", got, err)
	}

	if err := h.DesktopSavePremise("A promise with a cost."); err != nil {
		t.Fatalf("save premise: %v", err)
	}
	if got, err := st.Outline.LoadPremise(); err != nil || got != "A promise with a cost." {
		t.Fatalf("premise = %q err=%v", got, err)
	}

	plan := domain.ChapterPlan{Chapter: 2, Title: "Door", Goal: "Cross it"}
	if err := h.DesktopSaveChapterPlan(plan); err != nil {
		t.Fatalf("save plan: %v", err)
	}
	if got, err := st.Drafts.LoadChapterPlan(2); err != nil || got == nil || !reflect.DeepEqual(*got, plan) {
		t.Fatalf("plan = %+v err=%v", got, err)
	}

	if err := h.DesktopSaveChapterDraft(2, "working draft"); err != nil {
		t.Fatalf("save draft: %v", err)
	}
	if got, err := st.Drafts.LoadDraft(2); err != nil || got != "working draft" {
		t.Fatalf("draft = %q err=%v", got, err)
	}

	compass := domain.StoryCompass{EndingDirection: "Return changed", OpenThreads: []string{"seal"}, LastUpdated: 2}
	if err := h.DesktopSaveCompass(compass); err != nil {
		t.Fatalf("save compass: %v", err)
	}
	if got, err := st.Outline.LoadCompass(); err != nil || got == nil || !reflect.DeepEqual(*got, compass) {
		t.Fatalf("compass = %+v err=%v", got, err)
	}

	rules := []domain.WorldRule{{Category: "magic", Rule: "Power has a cost", Boundary: "No free resurrection"}}
	if err := h.DesktopReplaceWorldRules(rules); err != nil {
		t.Fatalf("replace world rules: %v", err)
	}
	if got, err := st.World.LoadWorldRules(); err != nil || !reflect.DeepEqual(got, rules) {
		t.Fatalf("world rules = %+v err=%v", got, err)
	}

	relationships := []domain.RelationshipEntry{{CharacterA: "Lead", CharacterB: "Guide", Relation: "allies", Chapter: 2}}
	if err := h.DesktopUpdateRelationships(relationships); err != nil {
		t.Fatalf("update relationships: %v", err)
	}
	if got, err := st.World.LoadRelationships(); err != nil || !reflect.DeepEqual(got, relationships) {
		t.Fatalf("relationships = %+v err=%v", got, err)
	}

	if err := h.DesktopUpdateForeshadow(2, []domain.ForeshadowUpdate{{ID: "seal", Action: "plant", Description: "broken seal"}}); err != nil {
		t.Fatalf("plant foreshadow: %v", err)
	}
	if err := h.DesktopUpdateForeshadow(3, []domain.ForeshadowUpdate{{ID: "seal", Action: "advance"}}); err != nil {
		t.Fatalf("advance foreshadow: %v", err)
	}
	ledger, err := st.World.LoadForeshadowLedger()
	if err != nil || len(ledger) != 1 || ledger[0].ID != "seal" || ledger[0].Status != "advanced" {
		t.Fatalf("foreshadow = %+v err=%v", ledger, err)
	}
}

func TestDesktopChapterWorkspaceMutationDoesNotAcceptRecord(t *testing.T) {
	h, st := newDesktopMutationTestHost(t)
	oldContent := "accepted baseline"
	record := domain.ChapterRecord{
		Version:       domain.ChapterRecordVersion,
		Chapter:       3,
		Revision:      4,
		Origin:        domain.ChapterOriginUser,
		Content:       oldContent,
		ContentSHA256: domain.ChapterContentSHA256(oldContent),
		AcceptedAt:    time.Date(2026, 9, 10, 8, 0, 0, 0, time.UTC),
	}
	if err := st.ChapterRecords.Save(record); err != nil {
		t.Fatalf("save record: %v", err)
	}
	if err := st.Drafts.SaveFinalChapter(3, oldContent); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}

	if err := h.DesktopSaveChapterWorkspace(3, "user edited workspace"); err != nil {
		t.Fatalf("save workspace: %v", err)
	}
	workspace, err := st.Drafts.LoadChapterText(3)
	if err != nil {
		t.Fatalf("load workspace: %v", err)
	}
	if workspace != "user edited workspace" {
		t.Fatalf("workspace = %q", workspace)
	}
	got, err := st.ChapterRecords.Load(3)
	if err != nil {
		t.Fatalf("load record: %v", err)
	}
	if got == nil || got.Revision != record.Revision || got.Content != record.Content || got.ContentSHA256 != record.ContentSHA256 || !got.AcceptedAt.Equal(record.AcceptedAt) {
		t.Fatalf("accepted record changed during workspace save: %+v", got)
	}
}

func TestDesktopCoreCharacterMutationPreservesCastAuthority(t *testing.T) {
	h, st := newDesktopMutationTestHost(t)
	cast := []domain.CastEntry{{
		Name: "Messenger", BriefRole: "courier", FirstSeenChapter: 1, LastSeenChapter: 2,
		AppearanceCount: 2, AppearanceChapters: []int{1, 2},
	}}
	if err := st.Cast.Save(cast); err != nil {
		t.Fatalf("seed cast: %v", err)
	}
	characters := []domain.Character{{Name: "Lead", Role: "protagonist", Description: "keeps moving", Tier: "core"}}
	if err := h.DesktopReplaceCoreCharacters(characters); err != nil {
		t.Fatalf("replace core characters: %v", err)
	}
	gotCharacters, err := st.Characters.Load()
	if err != nil {
		t.Fatalf("load characters: %v", err)
	}
	if !reflect.DeepEqual(gotCharacters, characters) {
		t.Fatalf("characters = %+v", gotCharacters)
	}
	gotCast, err := st.Cast.Load()
	if err != nil {
		t.Fatalf("load cast: %v", err)
	}
	if !reflect.DeepEqual(gotCast, cast) {
		t.Fatalf("core-character mutation rewrote cast: %+v", gotCast)
	}
}

func TestDesktopTimelineMutationUsesReplaySafeWorldStoreAppend(t *testing.T) {
	h, st := newDesktopMutationTestHost(t)
	events := []domain.TimelineEvent{{Chapter: 2, Time: "night", Event: "the gate opens", Characters: []string{"Lead"}}}
	if err := h.DesktopAppendTimelineEvents(events); err != nil {
		t.Fatalf("append timeline: %v", err)
	}
	if err := h.DesktopAppendTimelineEvents(events); err != nil {
		t.Fatalf("replay timeline append: %v", err)
	}
	got, err := st.World.LoadTimeline()
	if err != nil {
		t.Fatalf("load timeline: %v", err)
	}
	if len(got) != 1 || !reflect.DeepEqual(got[0], events[0]) {
		t.Fatalf("timeline replay duplicated facts: %+v", got)
	}
}

func TestDesktopTimelineMutationFailsClosedOnCorruptCanonicalLog(t *testing.T) {
	h, st := newDesktopMutationTestHost(t)
	path := filepath.Join(st.Dir(), "timeline.jsonl")
	corrupt := []byte("{broken json}\n")
	if err := os.WriteFile(path, corrupt, 0o644); err != nil {
		t.Fatalf("seed corrupt timeline: %v", err)
	}

	err := h.DesktopAppendTimelineEvents([]domain.TimelineEvent{{Chapter: 2, Event: "must not append"}})
	if !errors.Is(err, apperrs.ErrStoreRead) {
		t.Fatalf("corrupt timeline = %v, want store-read failure", err)
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read corrupt timeline: %v", readErr)
	}
	if !reflect.DeepEqual(got, corrupt) {
		t.Fatalf("corrupt canonical log was modified: %q", string(got))
	}
}

func TestDesktopOutlineMutationsKeepStoreCoordinationAndProtectedHistory(t *testing.T) {
	h, st := newDesktopMutationTestHost(t)
	volumes := []domain.VolumeOutline{{
		Index: 1, Title: "V1", Theme: "Entry",
		Arcs: []domain.ArcOutline{
			{Index: 1, Title: "A1", Goal: "Open", Chapters: []domain.OutlineEntry{{Title: "One", CoreEvent: "Door"}}},
			{Index: 2, Title: "A2", Goal: "Escalate", EstimatedChapters: 2},
		},
	}}
	if err := st.Outline.SaveLayeredOutline(volumes); err != nil {
		t.Fatalf("seed layered outline: %v", err)
	}
	progress := &domain.Progress{
		Phase: domain.PhaseWriting, Layered: true, CurrentVolume: 1, CurrentArc: 1,
		TotalChapters: domain.EstimatedChapterCapacity(volumes),
	}
	if err := st.Progress.Save(progress); err != nil {
		t.Fatalf("seed progress: %v", err)
	}

	expansion := domain.ArcExpansion{
		Title: "A2", Goal: "Escalate",
		Chapters: []domain.OutlineEntry{{Title: "Two", CoreEvent: "Choice"}, {Title: "Three", CoreEvent: "Cost"}},
	}
	if err := h.DesktopExpandArc(1, 2, expansion); err != nil {
		t.Fatalf("expand arc: %v", err)
	}
	if err := h.DesktopExpandArc(1, 2, expansion); err != nil {
		t.Fatalf("replay expand arc: %v", err)
	}
	gotProgress, err := st.Progress.Load()
	if err != nil {
		t.Fatalf("load progress: %v", err)
	}
	flat, err := st.Outline.LoadOutline()
	if err != nil {
		t.Fatalf("load flat outline: %v", err)
	}
	if gotProgress == nil || gotProgress.TotalChapters != 3 || len(flat) != 3 {
		t.Fatalf("expand did not coordinate outline/progress: progress=%+v flat=%+v", gotProgress, flat)
	}

	volume2 := domain.VolumeOutline{
		Index: 2, Title: "V2", Theme: "Cost",
		Arcs: []domain.ArcOutline{{Index: 1, Title: "A1", Goal: "Enter", Chapters: []domain.OutlineEntry{{Title: "Four", CoreEvent: "Arrival"}}}},
	}
	if err := h.DesktopAppendVolume(volume2); err != nil {
		t.Fatalf("append volume: %v", err)
	}
	if err := h.DesktopAppendVolume(volume2); err != nil {
		t.Fatalf("replay append volume: %v", err)
	}
	layered, err := st.Outline.LoadLayeredOutline()
	if err != nil {
		t.Fatalf("load layered outline: %v", err)
	}
	gotProgress, err = st.Progress.Load()
	if err != nil {
		t.Fatalf("load progress after volume: %v", err)
	}
	if len(layered) != 2 || gotProgress == nil || gotProgress.TotalChapters != 4 {
		t.Fatalf("volume replay/coordination failed: volumes=%+v progress=%+v", layered, gotProgress)
	}

	replacement := []domain.OutlineEntry{{Title: "Five", CoreEvent: "Departure"}}
	capacity, err := h.DesktopReviseOutline(5, replacement)
	if err != nil || capacity != 5 {
		t.Fatalf("revise future tail: capacity=%d err=%v", capacity, err)
	}
	capacity, err = h.DesktopReviseOutline(5, replacement)
	if err != nil || capacity != 5 {
		t.Fatalf("replay future tail revision: capacity=%d err=%v", capacity, err)
	}
	flat, err = st.Outline.LoadOutline()
	if err != nil || len(flat) != 5 || flat[4].Title != "Five" {
		t.Fatalf("revised flat projection = %+v err=%v", flat, err)
	}

	gotProgress, err = st.Progress.Load()
	if err != nil {
		t.Fatalf("load progress before protection: %v", err)
	}
	gotProgress.CompletedChapters = []int{1}
	gotProgress.InProgressChapter = 2
	if err := st.Progress.Save(gotProgress); err != nil {
		t.Fatalf("protect progress: %v", err)
	}
	_, err = h.DesktopReviseOutline(2, []domain.OutlineEntry{{Title: "Rewrite", CoreEvent: "Blocked"}})
	if !errors.Is(err, apperrs.ErrToolPrecondition) {
		t.Fatalf("protected outline history was editable: %v", err)
	}
}
