package host

import (
	"testing"

	"github.com/voocel/ainovel-cli/internal/domain"
	storepkg "github.com/voocel/ainovel-cli/internal/store"
)

func TestDesktopProjectReadUsesCanonicalStores(t *testing.T) {
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
	if err := store.Outline.SaveOutline([]domain.OutlineEntry{{Chapter: 1, Title: "One", CoreEvent: "Event"}}); err != nil {
		t.Fatal(err)
	}
	if err := store.Progress.Save(&domain.Progress{
		Phase:             domain.PhaseWriting,
		Flow:              domain.FlowWriting,
		CurrentChapter:    1,
		InProgressChapter: 1,
		TotalChapters:     3,
	}); err != nil {
		t.Fatal(err)
	}

	h := &Host{store: store}
	got, err := h.DesktopProjectRead()
	if err != nil {
		t.Fatal(err)
	}
	if got.FormatVersion != storepkg.CurrentProjectFormatVersion {
		t.Fatalf("format version = %d", got.FormatVersion)
	}
	if got.Book == nil || got.Book.Title != "Book" || got.Book.Synopsis != "Synopsis" {
		t.Fatalf("book = %+v", got.Book)
	}
	if got.Premise != "Premise" {
		t.Fatalf("premise = %q", got.Premise)
	}
	if got.Progress == nil || got.Progress.CurrentChapter != 1 || got.Progress.Flow != domain.FlowWriting {
		t.Fatalf("progress = %+v", got.Progress)
	}
	if len(got.Outline) != 1 || got.Outline[0].Title != "One" {
		t.Fatalf("outline = %+v", got.Outline)
	}
}

func TestDesktopProjectReadPrefersCanonicalLayeredArtifactsWithoutMutation(t *testing.T) {
	store := storepkg.NewStore(t.TempDir())
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}
	volumes := []domain.VolumeOutline{{
		Index: 1,
		Title: "V1",
		Theme: "Theme",
		Arcs: []domain.ArcOutline{{
			Index: 1,
			Title: "A1",
			Goal:  "Goal",
			Chapters: []domain.OutlineEntry{{
				Title: "Layered One", CoreEvent: "Layered Event",
			}},
		}},
	}}
	if err := store.Outline.SaveLayeredOutline(volumes); err != nil {
		t.Fatal(err)
	}
	if err := store.Outline.SaveCompass(domain.StoryCompass{EndingDirection: "Ending", OpenThreads: []string{"thread"}}); err != nil {
		t.Fatal(err)
	}

	h := &Host{store: store}
	got, err := h.DesktopProjectRead()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Volumes) != 1 || got.Volumes[0].Arcs[0].Chapters[0].Title != "Layered One" {
		t.Fatalf("volumes = %+v", got.Volumes)
	}
	if got.Compass == nil || got.Compass.EndingDirection != "Ending" {
		t.Fatalf("compass = %+v", got.Compass)
	}
	// SaveLayeredOutline owns synchronization of the flat compatibility view.
	if len(got.Outline) != 1 || got.Outline[0].Chapter != 1 || got.Outline[0].Title != "Layered One" {
		t.Fatalf("derived flat outline = %+v", got.Outline)
	}
}

func TestDesktopChapterReadKeepsPlanDraftFinalAndAcceptedRecordDistinct(t *testing.T) {
	store := storepkg.NewStore(t.TempDir())
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}
	plan := domain.ChapterPlan{Chapter: 1, Title: "Plan Title", Goal: "Goal"}
	if err := store.Drafts.SaveChapterPlan(plan); err != nil {
		t.Fatal(err)
	}
	if err := store.Drafts.SaveDraft(1, "draft text"); err != nil {
		t.Fatal(err)
	}
	if err := store.Drafts.SaveFinalChapter(1, "published final"); err != nil {
		t.Fatal(err)
	}
	record, err := store.ChapterRecords.Accept(
		1,
		domain.ChapterOriginUser,
		"accepted baseline",
		domain.ChapterFacts{Title: "Accepted Title"},
		domain.StyleDelta{},
	)
	if err != nil {
		t.Fatal(err)
	}

	h := &Host{store: store}
	got, err := h.DesktopChapterRead(1)
	if err != nil {
		t.Fatal(err)
	}
	if got.Plan == nil || got.Plan.Title != "Plan Title" {
		t.Fatalf("plan = %+v", got.Plan)
	}
	if got.Draft != "draft text" {
		t.Fatalf("draft = %q", got.Draft)
	}
	if got.Final != "published final" {
		t.Fatalf("final = %q", got.Final)
	}
	if got.Record == nil || got.Record.Content != "accepted baseline" || got.Record.ContentSHA256 != record.ContentSHA256 {
		t.Fatalf("record = %+v", got.Record)
	}
	if got.Final == got.Record.Content {
		t.Fatal("published final and accepted record must remain independent facts")
	}
}
