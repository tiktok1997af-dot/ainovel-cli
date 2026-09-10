package host

import (
	"testing"

	"github.com/voocel/ainovel-cli/internal/domain"
	storepkg "github.com/voocel/ainovel-cli/internal/store"
)

func TestDesktopKnowledgeReadUsesExistingCanonicalStores(t *testing.T) {
	store := storepkg.NewStore(t.TempDir())
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveProjectFormatVersion(storepkg.CurrentProjectFormatVersion); err != nil {
		t.Fatal(err)
	}
	if err := store.Book.Save(domain.BookMetadata{Title: "Book", Synopsis: "Synopsis"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Outline.SavePremise("Premise"); err != nil {
		t.Fatal(err)
	}
	if err := store.Progress.Save(&domain.Progress{
		Phase: domain.PhaseWriting, CurrentChapter: 3, CompletedChapters: []int{2, 1, 2},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Characters.Save([]domain.Character{{Name: "Hero", Role: "lead", Tier: "core"}}); err != nil {
		t.Fatal(err)
	}
	if err := store.Cast.Save([]domain.CastEntry{{Name: "Guard", BriefRole: "guard", FirstSeenChapter: 1, LastSeenChapter: 1, AppearanceCount: 1, AppearanceChapters: []int{1}}}); err != nil {
		t.Fatal(err)
	}
	if err := store.World.SaveWorldRules([]domain.WorldRule{{Category: "magic", Rule: "Rule", Boundary: "Boundary"}}); err != nil {
		t.Fatal(err)
	}
	if err := store.World.SaveForeshadowLedger([]domain.ForeshadowEntry{{ID: "seal", Description: "sealed door", PlantedAt: 1, Status: "planted"}}); err != nil {
		t.Fatal(err)
	}
	if err := store.World.SaveRelationships([]domain.RelationshipEntry{{CharacterA: "Hero", CharacterB: "Guard", Relation: "known", Chapter: 1}}); err != nil {
		t.Fatal(err)
	}
	if err := store.World.SaveStateChanges([]domain.StateChange{{Chapter: 1, Entity: "Hero", Field: "status", NewValue: "ready"}}); err != nil {
		t.Fatal(err)
	}
	if err := store.World.SaveTimeline([]domain.TimelineEvent{{Chapter: 1, Time: "dawn", Event: "Start", Characters: []string{"Hero"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ChapterRecords.Accept(1, domain.ChapterOriginGenerated, "chapter one", domain.ChapterFacts{Title: "One", Summary: "Summary one"}, domain.StyleDelta{}); err != nil {
		t.Fatal(err)
	}
	if err := store.Summaries.SaveSummary(domain.ChapterSummary{Chapter: 1, Title: "One", Summary: "Summary one"}); err != nil {
		t.Fatal(err)
	}
	// Chapter 2 is completed in Progress but intentionally has no optional
	// accepted record/summary. The desktop read must preserve that as missing,
	// not synthesize a record and not fail the unrelated knowledge domains.

	h := &Host{store: store}
	got, err := h.DesktopKnowledgeRead()
	if err != nil {
		t.Fatal(err)
	}
	if got.Project.Book == nil || got.Project.Book.Title != "Book" || got.Project.Premise != "Premise" {
		t.Fatalf("project facts = %+v", got.Project)
	}
	if len(got.Characters.Characters) != 1 || got.Characters.Characters[0].Name != "Hero" {
		t.Fatalf("characters = %+v", got.Characters.Characters)
	}
	if len(got.Characters.Cast) != 1 || got.Characters.Cast[0].Name != "Guard" {
		t.Fatalf("cast = %+v", got.Characters.Cast)
	}
	if len(got.World.Rules) != 1 || len(got.World.Foreshadow) != 1 || len(got.World.Relationships) != 1 || len(got.World.StateChanges) != 1 {
		t.Fatalf("world facts = %+v", got.World)
	}
	if len(got.Timeline) != 1 || got.Timeline[0].Event != "Start" {
		t.Fatalf("timeline = %+v", got.Timeline)
	}
	if len(got.Records) != 1 || got.Records[0].Chapter != 1 || len(got.Summaries) != 1 || got.Summaries[0].Chapter != 1 {
		t.Fatalf("optional chapter knowledge = records:%+v summaries:%+v", got.Records, got.Summaries)
	}
}

func TestUniqueSortedCompletedChaptersIsDeterministic(t *testing.T) {
	got := uniqueSortedCompletedChapters(&domain.Progress{CompletedChapters: []int{4, 2, 4, -1, 1, 0}})
	want := []int{1, 2, 4}
	if len(got) != len(want) {
		t.Fatalf("completed = %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("completed = %+v", got)
		}
	}
}
