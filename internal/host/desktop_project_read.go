package host

import (
	"fmt"

	"github.com/voocel/ainovel-cli/internal/domain"
	apperrs "github.com/voocel/ainovel-cli/internal/errs"
)

// DesktopProjectReadSnapshot is a read-only Host seam for AppRuntime project
// queries. It contains freshly loaded canonical values, never Store pointers or
// mutation handles. AppRuntime must still project these domain values into its
// own serialization-safe desktop DTOs before they cross the desktop boundary.
type DesktopProjectReadSnapshot struct {
	FormatVersion int
	Book          *domain.BookMetadata
	Premise       string
	Progress      *domain.Progress
	Outline       []domain.OutlineEntry
	Volumes       []domain.VolumeOutline
	Compass       *domain.StoryCompass
}

// DesktopChapterReadSnapshot keeps the three editable/published chapter
// artifacts separate from the accepted chapter record. In particular, Final
// is chapters/*.md while Record is the accepted revision authority; neither is
// synthesized from the other.
type DesktopChapterReadSnapshot struct {
	Chapter int
	Plan    *domain.ChapterPlan
	Draft   string
	Final   string
	Record  *domain.ChapterRecord
}

// DesktopProjectRead loads the canonical project facts needed by the desktop
// Project workspace. It is deliberately read-only and does not expose the
// underlying Store or sub-stores.
func (h *Host) DesktopProjectRead() (DesktopProjectReadSnapshot, error) {
	if h == nil || h.store == nil {
		return DesktopProjectReadSnapshot{}, fmt.Errorf("desktop project store unavailable: %w", apperrs.ErrStoreRead)
	}

	formatVersion, err := h.store.LoadProjectFormatVersion()
	if err != nil {
		return DesktopProjectReadSnapshot{}, desktopProjectReadError("project format", err)
	}
	book, err := h.store.Book.Load()
	if err != nil {
		return DesktopProjectReadSnapshot{}, desktopProjectReadError("book metadata", err)
	}
	premise, err := h.store.Outline.LoadPremise()
	if err != nil {
		return DesktopProjectReadSnapshot{}, desktopProjectReadError("premise", err)
	}
	progress, err := h.store.Progress.Load()
	if err != nil {
		return DesktopProjectReadSnapshot{}, desktopProjectReadError("progress", err)
	}
	outline, err := h.store.Outline.LoadOutline()
	if err != nil {
		return DesktopProjectReadSnapshot{}, desktopProjectReadError("outline", err)
	}
	volumes, err := h.store.Outline.LoadLayeredOutline()
	if err != nil {
		return DesktopProjectReadSnapshot{}, desktopProjectReadError("layered outline", err)
	}
	compass, err := h.store.Outline.LoadCompass()
	if err != nil {
		return DesktopProjectReadSnapshot{}, desktopProjectReadError("story compass", err)
	}

	return DesktopProjectReadSnapshot{
		FormatVersion: formatVersion,
		Book:          book,
		Premise:       premise,
		Progress:      progress,
		Outline:       outline,
		Volumes:       volumes,
		Compass:       compass,
	}, nil
}

// DesktopChapterRead loads chapter-local artifacts from their canonical stores.
// Missing artifacts are represented by nil/empty values exactly as the stores
// define them; a missing draft/final is not promoted from another artifact.
func (h *Host) DesktopChapterRead(chapter int) (DesktopChapterReadSnapshot, error) {
	if h == nil || h.store == nil {
		return DesktopChapterReadSnapshot{}, fmt.Errorf("desktop project store unavailable: %w", apperrs.ErrStoreRead)
	}
	if chapter <= 0 {
		return DesktopChapterReadSnapshot{}, fmt.Errorf("invalid desktop chapter read: %w", apperrs.ErrToolArgs)
	}

	plan, err := h.store.Drafts.LoadChapterPlan(chapter)
	if err != nil {
		return DesktopChapterReadSnapshot{}, desktopProjectReadError("chapter plan", err)
	}
	draft, err := h.store.Drafts.LoadDraft(chapter)
	if err != nil {
		return DesktopChapterReadSnapshot{}, desktopProjectReadError("chapter draft", err)
	}
	final, err := h.store.Drafts.LoadChapterText(chapter)
	if err != nil {
		return DesktopChapterReadSnapshot{}, desktopProjectReadError("chapter final", err)
	}
	record, err := h.store.ChapterRecords.Load(chapter)
	if err != nil {
		return DesktopChapterReadSnapshot{}, desktopProjectReadError("chapter record", err)
	}

	return DesktopChapterReadSnapshot{
		Chapter: chapter,
		Plan:    plan,
		Draft:   draft,
		Final:   final,
		Record:  record,
	}, nil
}

func desktopProjectReadError(kind string, err error) error {
	return fmt.Errorf("read desktop %s: %w: %w", kind, apperrs.ErrStoreRead, err)
}
