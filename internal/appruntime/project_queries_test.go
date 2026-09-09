package appruntime

import (
	"testing"
	"time"

	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/host"
)

func TestProjectOverviewProjectionUsesCanonicalFacts(t *testing.T) {
	project := host.DesktopProjectReadSnapshot{
		FormatVersion: 2,
		Book:          &domain.BookMetadata{Title: "Book", Synopsis: "Synopsis"},
		Premise:       "  Premise  ",
		Progress: &domain.Progress{
			Phase:             domain.PhaseWriting,
			Flow:              domain.FlowReviewing,
			CurrentChapter:    12,
			CompletedChapters: []int{1, 2, 3},
			TotalWordCount:    4567,
			CurrentVolume:     2,
			CurrentArc:        1,
			Layered:           true,
		},
	}
	got := projectOverviewDTO(project)
	if got.FormatVersion != 2 || got.Title != "Book" || got.Synopsis != "Synopsis" || got.Premise != "Premise" {
		t.Fatalf("overview identity = %+v", got)
	}
	if got.Phase != "writing" || got.Flow != "reviewing" || !got.Layered {
		t.Fatalf("overview runtime = %+v", got)
	}
	if got.CurrentChapter != 12 || got.CompletedChapters != 3 || got.TotalWordCount != 4567 || got.CurrentVolume != 2 || got.CurrentArc != 1 {
		t.Fatalf("overview progress = %+v", got)
	}
}

func TestLayeredCatalogDoesNotExposeEstimatedCapacityAsChapterTotal(t *testing.T) {
	project := host.DesktopProjectReadSnapshot{
		Progress: &domain.Progress{
			Layered:           true,
			TotalChapters:     1500,
			CompletedChapters: []int{1, 2},
			InProgressChapter: 3,
		},
		Volumes: []domain.VolumeOutline{{
			Index: 1,
			Arcs: []domain.ArcOutline{
				{Index: 1, Chapters: []domain.OutlineEntry{{Title: "One"}, {Title: "Two"}}},
				{Index: 2, EstimatedChapters: 1498},
			},
		}},
	}
	outline := effectiveProjectOutline(project)
	if got := projectChapterCatalogSize(project, outline); got != 3 {
		t.Fatalf("layered chapter catalog total = %d, want 3 detailed/completed/in-progress facts", got)
	}
}

func TestFlatCatalogMayUseCanonicalDetailedTotal(t *testing.T) {
	project := host.DesktopProjectReadSnapshot{
		Progress: &domain.Progress{TotalChapters: 12},
		Outline:  []domain.OutlineEntry{{Chapter: 1}, {Chapter: 2}},
	}
	if got := projectChapterCatalogSize(project, effectiveProjectOutline(project)); got != 12 {
		t.Fatalf("flat chapter catalog total = %d, want 12", got)
	}
}

func TestChapterArtifactStatusAndProjectionKeepAuthoritiesDistinct(t *testing.T) {
	acceptedAt := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	artifacts := host.DesktopChapterReadSnapshot{
		Chapter: 4,
		Plan: &domain.ChapterPlan{
			Chapter: 4,
			Title:   "Plan Title",
			Goal:    "Goal",
			Contract: domain.ChapterContract{
				RequiredBeats:    []string{"beat"},
				ForbiddenMoves:   []string{"forbidden"},
				ContinuityChecks: []string{"continuity"},
			},
		},
		Draft: "draft",
		Final: "published final",
		Record: &domain.ChapterRecord{
			Revision:      7,
			Origin:        domain.ChapterOriginUser,
			Content:       "accepted baseline differs",
			ContentSHA256: "sha",
			Facts:         domain.ChapterFacts{Title: "Accepted Title"},
			AcceptedAt:    acceptedAt,
		},
	}
	item := chapterListItemDTO(artifacts, domain.OutlineEntry{Chapter: 4, Title: "Outline Title"})
	if item.Status != "accepted" || !item.HasPlan || !item.HasDraft || !item.HasFinal {
		t.Fatalf("list authority state = %+v", item)
	}
	if item.Title != "Plan Title" || item.Origin != "user" || item.Revision != 7 {
		t.Fatalf("list metadata = %+v", item)
	}
	if item.WordCount != domain.WordCount("published final") {
		t.Fatalf("word count = %d", item.WordCount)
	}

	withoutContent := chapterGetDTO(artifacts, domain.OutlineEntry{Chapter: 4, Title: "Outline Title"}, false)
	if withoutContent.Status != "accepted" || withoutContent.Record == nil || withoutContent.Record.AcceptedAt != acceptedAt {
		t.Fatalf("get authority = %+v", withoutContent)
	}
	if withoutContent.Draft == nil || !withoutContent.Draft.Present || withoutContent.Final == nil || !withoutContent.Final.Present {
		t.Fatalf("presence projection = %+v", withoutContent)
	}
	if withoutContent.Draft.Content != "" || withoutContent.Final.Content != "" {
		t.Fatal("include_content=false leaked chapter text")
	}
	if withoutContent.Plan == nil || len(withoutContent.Plan.Required) != 1 || len(withoutContent.Plan.Forbidden) != 1 || len(withoutContent.Plan.Continuity) != 1 {
		t.Fatalf("plan contract = %+v", withoutContent.Plan)
	}

	withContent := chapterGetDTO(artifacts, domain.OutlineEntry{Chapter: 4}, true)
	if withContent.Draft.Content != "draft" || withContent.Final.Content != "published final" {
		t.Fatalf("include_content=true = %+v", withContent)
	}
}

func TestChapterStatusPrecedence(t *testing.T) {
	plan := &domain.ChapterPlan{Chapter: 1}
	record := &domain.ChapterRecord{}
	cases := []struct {
		name string
		in   host.DesktopChapterReadSnapshot
		want string
	}{
		{"none", host.DesktopChapterReadSnapshot{}, "not_started"},
		{"plan", host.DesktopChapterReadSnapshot{Plan: plan}, "planned"},
		{"draft", host.DesktopChapterReadSnapshot{Plan: plan, Draft: "d"}, "draft"},
		{"final", host.DesktopChapterReadSnapshot{Plan: plan, Draft: "d", Final: "f"}, "final"},
		{"accepted", host.DesktopChapterReadSnapshot{Plan: plan, Draft: "d", Final: "f", Record: record}, "accepted"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := chapterArtifactStatus(tc.in); got != tc.want {
				t.Fatalf("status = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestLayeredOutlineProjectionAppliesChapterWindowButPreservesStructure(t *testing.T) {
	volumes := []domain.VolumeOutline{{
		Index: 1,
		Title: "V1",
		Arcs: []domain.ArcOutline{
			{Index: 1, Title: "A1", Chapters: []domain.OutlineEntry{{Title: "One"}, {Title: "Two"}}},
			{Index: 2, Title: "A2", EstimatedChapters: 5},
		},
	}}
	flat := domain.FlattenOutline(volumes)
	selected := selectOutlineWindow(flat, 2, 1)
	if len(selected) != 1 || selected[0].Chapter != 2 {
		t.Fatalf("selected = %+v", selected)
	}
	view := layeredOutlineDTO(volumes, map[int]bool{2: true})
	if len(view) != 1 || len(view[0].Arcs) != 2 {
		t.Fatalf("layered structure = %+v", view)
	}
	if len(view[0].Arcs[0].Chapters) != 1 || view[0].Arcs[0].Chapters[0].Chapter != 2 || view[0].Arcs[0].Chapters[0].Title != "Two" {
		t.Fatalf("filtered detailed chapters = %+v", view[0].Arcs[0].Chapters)
	}
	if view[0].Arcs[1].EstimatedChapters != 5 || len(view[0].Arcs[1].Chapters) != 0 {
		t.Fatalf("skeleton arc metadata lost = %+v", view[0].Arcs[1])
	}
}

func TestProjectQueryPaginationBounds(t *testing.T) {
	if got := effectiveProjectQueryLimit(0); got != defaultProjectQueryPageSize {
		t.Fatalf("default limit = %d", got)
	}
	start, end := pageBounds(5, 10, 8)
	if start != 5 || end != 8 {
		t.Fatalf("page bounds = %d..%d", start, end)
	}
	start, end = pageBounds(12, 10, 8)
	if start != 8 || end != 8 {
		t.Fatalf("past-end bounds = %d..%d", start, end)
	}
}
