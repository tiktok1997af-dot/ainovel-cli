package host

import (
	"fmt"

	"github.com/voocel/ainovel-cli/internal/domain"
	apperrs "github.com/voocel/ainovel-cli/internal/errs"
)

// Desktop mutation seams are intentionally narrow. They expose canonical
// domain operations to AppRuntime without exposing Store pointers, filesystem
// paths, or generic write handles.

func (h *Host) DesktopSaveBookMetadata(book domain.BookMetadata) error {
	if err := h.desktopMutationReady(); err != nil {
		return err
	}
	if err := h.store.Book.Save(book); err != nil {
		return desktopProjectWriteError("book metadata", err)
	}
	return nil
}

func (h *Host) DesktopSavePremise(premise string) error {
	if err := h.desktopMutationReady(); err != nil {
		return err
	}
	if err := h.store.Outline.SavePremise(premise); err != nil {
		return desktopProjectWriteError("premise", err)
	}
	return nil
}

func (h *Host) DesktopSaveChapterPlan(plan domain.ChapterPlan) error {
	if err := h.desktopMutationReady(); err != nil {
		return err
	}
	if err := h.store.Drafts.SaveChapterPlan(plan); err != nil {
		return desktopProjectWriteError("chapter plan", err)
	}
	return nil
}

func (h *Host) DesktopSaveChapterDraft(chapter int, content string) error {
	if err := h.desktopMutationReady(); err != nil {
		return err
	}
	if err := h.store.Drafts.SaveDraft(chapter, content); err != nil {
		return desktopProjectWriteError("chapter draft", err)
	}
	return nil
}

// DesktopSaveChapterWorkspace updates chapters/*.md only. It deliberately does
// not accept or rewrite ChapterRecord; external/user edits remain divergence
// that the existing revision workflow may reconcile later.
func (h *Host) DesktopSaveChapterWorkspace(chapter int, content string) error {
	if err := h.desktopMutationReady(); err != nil {
		return err
	}
	if err := h.store.Drafts.SaveFinalChapter(chapter, content); err != nil {
		return desktopProjectWriteError("chapter workspace", err)
	}
	return nil
}

func (h *Host) DesktopReviseOutline(fromChapter int, replacement []domain.OutlineEntry) (int, error) {
	if err := h.desktopMutationReady(); err != nil {
		return 0, err
	}
	capacity, err := h.store.ReviseOutline(fromChapter, replacement)
	if err != nil {
		return 0, desktopProjectWriteError("outline tail", err)
	}
	return capacity, nil
}

func (h *Host) DesktopExpandArc(volume, arc int, expansion domain.ArcExpansion) error {
	if err := h.desktopMutationReady(); err != nil {
		return err
	}
	if err := h.store.ExpandArc(volume, arc, expansion); err != nil {
		return desktopProjectWriteError("outline arc", err)
	}
	return nil
}

func (h *Host) DesktopAppendVolume(volume domain.VolumeOutline) error {
	if err := h.desktopMutationReady(); err != nil {
		return err
	}
	if err := h.store.AppendVolume(volume); err != nil {
		return desktopProjectWriteError("outline volume", err)
	}
	return nil
}

func (h *Host) DesktopSaveCompass(compass domain.StoryCompass) error {
	if err := h.desktopMutationReady(); err != nil {
		return err
	}
	if err := h.store.Outline.SaveCompass(compass); err != nil {
		return desktopProjectWriteError("story compass", err)
	}
	return nil
}

// DesktopReplaceCoreCharacters writes only CharacterStore. CastStore remains a
// separate supporting-cast authority and is never rewritten here.
func (h *Host) DesktopReplaceCoreCharacters(characters []domain.Character) error {
	if err := h.desktopMutationReady(); err != nil {
		return err
	}
	if err := h.store.Characters.Save(characters); err != nil {
		return desktopProjectWriteError("core characters", err)
	}
	return nil
}

func (h *Host) DesktopReplaceWorldRules(rules []domain.WorldRule) error {
	if err := h.desktopMutationReady(); err != nil {
		return err
	}
	if err := h.store.World.SaveWorldRules(rules); err != nil {
		return desktopProjectWriteError("world rules", err)
	}
	return nil
}

func (h *Host) DesktopAppendTimelineEvents(events []domain.TimelineEvent) error {
	if err := h.desktopMutationReady(); err != nil {
		return err
	}
	// Append is read-modify-write. Refuse to append over a corrupt canonical log
	// and classify that failure as a read error rather than silently replacing it.
	if _, err := h.store.World.LoadTimeline(); err != nil {
		return desktopProjectReadError("timeline", err)
	}
	if err := h.store.World.AppendTimelineEvents(events); err != nil {
		return desktopProjectWriteError("timeline", err)
	}
	return nil
}

func (h *Host) DesktopUpdateRelationships(changes []domain.RelationshipEntry) error {
	if err := h.desktopMutationReady(); err != nil {
		return err
	}
	if _, err := h.store.World.LoadRelationships(); err != nil {
		return desktopProjectReadError("relationships", err)
	}
	if err := h.store.World.UpdateRelationships(changes); err != nil {
		return desktopProjectWriteError("relationships", err)
	}
	return nil
}

func (h *Host) DesktopUpdateForeshadow(chapter int, updates []domain.ForeshadowUpdate) error {
	if err := h.desktopMutationReady(); err != nil {
		return err
	}
	if _, err := h.store.World.LoadForeshadowLedger(); err != nil {
		return desktopProjectReadError("foreshadow", err)
	}
	if err := h.store.World.UpdateForeshadow(chapter, updates); err != nil {
		return desktopProjectWriteError("foreshadow", err)
	}
	return nil
}

func (h *Host) desktopMutationReady() error {
	if h == nil || h.store == nil {
		return fmt.Errorf("desktop project store unavailable: %w", apperrs.ErrStoreWrite)
	}
	return nil
}

func desktopProjectWriteError(kind string, err error) error {
	return fmt.Errorf("write desktop %s: %w: %w", kind, apperrs.ErrStoreWrite, err)
}
