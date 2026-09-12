package host

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/voocel/ainovel-cli/internal/diag"
	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/stylestat"
)

// DesktopReviewTarget is the Host-side equivalent of the AppRuntime Review
// target. Host owns the Store-facing validation so AppRuntime never receives a
// Store pointer or filesystem path.
type DesktopReviewTarget struct {
	Scope          string
	Chapter        int
	Volume         int
	Arc            int
	ThroughChapter int
}

// DesktopReviewReadSnapshot exposes canonical Review inputs as copied domain
// values. It is deliberately evidence-only: no mutation handles, browser state
// or unrestricted project files cross the Host/AppRuntime seam.
type DesktopReviewReadSnapshot struct {
	Target           DesktopReviewTarget
	Review           *domain.ReviewEntry
	ReviewCheckpoint *domain.Checkpoint
	ReviewHistory    []domain.Checkpoint
	Revisions        []domain.ChapterRecord
	Diagnostics      diag.Report
	StyleStatus      string
	Style            *stylestat.Stats
}

// DesktopReviewRead aggregates the existing canonical Review owners behind a
// read-only Host seam. It does not persist a second Review result database.
func (h *Host) DesktopReviewRead(target DesktopReviewTarget) (DesktopReviewReadSnapshot, error) {
	chapters, scope, artifact, err := h.desktopReviewTargetFacts(target)
	if err != nil {
		return DesktopReviewReadSnapshot{}, err
	}

	revisions, err := h.store.ChapterRecords.LoadCompleted(chapters)
	if err != nil {
		return DesktopReviewReadSnapshot{}, fmt.Errorf("read review chapter revisions: %w", err)
	}

	review, err := h.desktopReviewArtifact(target)
	if err != nil {
		return DesktopReviewReadSnapshot{}, err
	}

	history := desktopReviewCheckpoints(h.store.Checkpoints.All(), scope, artifact)
	var latest *domain.Checkpoint
	if len(history) > 0 {
		cp := history[len(history)-1]
		latest = &cp
	}

	style, styleStatus, err := h.desktopReviewStyle(chapters)
	if err != nil {
		return DesktopReviewReadSnapshot{}, err
	}

	return DesktopReviewReadSnapshot{
		Target:           target,
		Review:           review,
		ReviewCheckpoint: latest,
		ReviewHistory:    history,
		Revisions:        revisions,
		Diagnostics:      diag.Analyze(h.store),
		StyleStatus:      styleStatus,
		Style:            style,
	}, nil
}

func (h *Host) desktopReviewTargetFacts(target DesktopReviewTarget) ([]int, domain.Scope, string, error) {
	progress, err := h.store.Progress.Load()
	if err != nil {
		return nil, domain.Scope{}, "", fmt.Errorf("read review progress: %w", err)
	}
	if progress == nil {
		return nil, domain.Scope{}, "", fmt.Errorf("review target requires project progress")
	}

	switch target.Scope {
	case "chapter":
		if target.Chapter <= 0 || target.Volume != 0 || target.Arc != 0 || target.ThroughChapter != 0 {
			return nil, domain.Scope{}, "", fmt.Errorf("invalid chapter review target")
		}
		if !slices.Contains(progress.CompletedChapters, target.Chapter) {
			return nil, domain.Scope{}, "", fmt.Errorf("review chapter %d is not completed", target.Chapter)
		}
		return []int{target.Chapter}, domain.ChapterScope(target.Chapter), fmt.Sprintf("reviews/%02d.json", target.Chapter), nil

	case "arc":
		if target.Volume <= 0 || target.Arc <= 0 || target.ThroughChapter <= 0 || target.Chapter != 0 {
			return nil, domain.Scope{}, "", fmt.Errorf("invalid arc review target")
		}
		boundary, err := h.store.Outline.CheckArcBoundary(target.ThroughChapter)
		if err != nil {
			return nil, domain.Scope{}, "", fmt.Errorf("read arc review boundary: %w", err)
		}
		if boundary == nil || !boundary.IsArcEnd || boundary.Volume != target.Volume || boundary.Arc != target.Arc || boundary.EndChapter != target.ThroughChapter {
			return nil, domain.Scope{}, "", fmt.Errorf("arc review target does not match canonical outline boundary")
		}
		chapters := make([]int, 0, boundary.EndChapter-boundary.StartChapter+1)
		for chapter := boundary.StartChapter; chapter <= boundary.EndChapter; chapter++ {
			if !slices.Contains(progress.CompletedChapters, chapter) {
				return nil, domain.Scope{}, "", fmt.Errorf("arc review chapter %d is not completed", chapter)
			}
			chapters = append(chapters, chapter)
		}
		return chapters, domain.ArcScope(target.Volume, target.Arc), fmt.Sprintf("reviews/%02d.json", target.ThroughChapter), nil

	case "global":
		if target.ThroughChapter <= 0 || target.Chapter != 0 || target.Volume != 0 || target.Arc != 0 {
			return nil, domain.Scope{}, "", fmt.Errorf("invalid global review target")
		}
		if !slices.Contains(progress.CompletedChapters, target.ThroughChapter) {
			return nil, domain.Scope{}, "", fmt.Errorf("global review endpoint %d is not completed", target.ThroughChapter)
		}
		chapters := make([]int, 0, len(progress.CompletedChapters))
		for _, chapter := range progress.CompletedChapters {
			if chapter > 0 && chapter <= target.ThroughChapter {
				chapters = append(chapters, chapter)
			}
		}
		sort.Ints(chapters)
		return chapters, domain.GlobalScope(), fmt.Sprintf("reviews/%02d-global.json", target.ThroughChapter), nil

	default:
		return nil, domain.Scope{}, "", fmt.Errorf("unsupported review scope %q", target.Scope)
	}
}

func (h *Host) desktopReviewArtifact(target DesktopReviewTarget) (*domain.ReviewEntry, error) {
	var (
		review *domain.ReviewEntry
		err    error
	)
	switch target.Scope {
	case "chapter":
		review, err = h.store.World.LoadReview(target.Chapter)
		if err == nil && review != nil && review.Scope != "chapter" {
			review = nil
		}
	case "arc":
		review, err = h.store.World.LoadReview(target.ThroughChapter)
		if err == nil && review != nil && review.Scope != "arc" {
			review = nil
		}
	case "global":
		review, err = h.store.World.LoadGlobalReview(target.ThroughChapter)
	}
	if err != nil {
		return nil, fmt.Errorf("read review artifact: %w", err)
	}
	return review, nil
}

func desktopReviewCheckpoints(all []domain.Checkpoint, scope domain.Scope, artifact string) []domain.Checkpoint {
	out := make([]domain.Checkpoint, 0)
	for _, cp := range all {
		if cp.Step != "review" || !cp.Scope.Matches(scope) || cp.Artifact != artifact {
			continue
		}
		out = append(out, cp)
	}
	return out
}

func (h *Host) desktopReviewStyle(chapters []int) (*stylestat.Stats, string, error) {
	outline, err := h.store.Outline.LoadOutline()
	if err != nil {
		return nil, "", fmt.Errorf("read review outline: %w", err)
	}
	titleByChapter := make(map[int]string, len(outline))
	for _, entry := range outline {
		titleByChapter[entry.Chapter] = strings.TrimSpace(entry.Title)
	}
	titles := make([]string, 0, len(chapters))
	for _, chapter := range chapters {
		title := titleByChapter[chapter]
		if summary, loadErr := h.store.Summaries.LoadSummary(chapter); loadErr != nil {
			return nil, "", fmt.Errorf("read review summary %d: %w", chapter, loadErr)
		} else if summary != nil && strings.TrimSpace(summary.Title) != "" {
			title = strings.TrimSpace(summary.Title)
		}
		titles = append(titles, title)
	}

	characters, err := h.store.Characters.Load()
	if err != nil {
		return nil, "", fmt.Errorf("read review characters: %w", err)
	}
	stopwords := make([]string, 0, len(characters)*2)
	for _, character := range characters {
		if strings.TrimSpace(character.Name) != "" {
			stopwords = append(stopwords, character.Name)
		}
		stopwords = append(stopwords, character.Aliases...)
	}

	stats, err := h.styleStats.Snapshot(chapters, titles, stopwords)
	if err != nil {
		return nil, "", fmt.Errorf("collect review style stats: %w", err)
	}
	if stats == nil {
		return nil, "insufficient_sample", nil
	}
	return stats, "ok", nil
}
