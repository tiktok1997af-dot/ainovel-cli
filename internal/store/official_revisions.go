package store

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/errs"
)

const officialRevisionsPath = "meta/official_revisions.json"

// LoadOfficialManifest returns the canonical revision-selection manifest. It is
// additive pointer metadata owned by ChapterRecordStore, never chapter content.
func (s *ChapterRecordStore) LoadOfficialManifest() (domain.OfficialManifest, error) {
	s.io.mu.RLock()
	defer s.io.mu.RUnlock()
	return s.loadOfficialManifestUnlocked()
}

func (s *ChapterRecordStore) loadOfficialManifestUnlocked() (domain.OfficialManifest, error) {
	var manifest domain.OfficialManifest
	if err := s.io.ReadJSONUnlocked(officialRevisionsPath, &manifest); err != nil {
		if os.IsNotExist(err) {
			return domain.OfficialManifest{
				Version:    domain.OfficialManifestVersion,
				Selections: make(map[string]domain.OfficialSelection),
			}, nil
		}
		return domain.OfficialManifest{}, fmt.Errorf("read official revision manifest: %w: %w", errs.ErrStoreRead, err)
	}
	if manifest.Version != domain.OfficialManifestVersion {
		return domain.OfficialManifest{}, fmt.Errorf("unsupported official manifest version %d: %w", manifest.Version, errs.ErrToolPrecondition)
	}
	if manifest.Selections == nil {
		manifest.Selections = make(map[string]domain.OfficialSelection)
	}
	return manifest, nil
}

// PromoteOfficial atomically revalidates the exact current ChapterRecord CAS
// and commits one Official selection. It never mutates ChapterRecord content or
// revision history. The returned bool is true only when the manifest changed.
func (s *ChapterRecordStore) PromoteOfficial(selection domain.OfficialSelection) (domain.OfficialSelection, bool, error) {
	key, err := officialTargetKey(selection.Target)
	if err != nil {
		return domain.OfficialSelection{}, false, err
	}
	selection.ReviewFingerprint = strings.TrimSpace(selection.ReviewFingerprint)
	if selection.ReviewFingerprint == "" || len(selection.Revisions) == 0 {
		return domain.OfficialSelection{}, false, fmt.Errorf("official promotion requires review fingerprint and revisions: %w", errs.ErrToolArgs)
	}

	seen := make(map[int]struct{}, len(selection.Revisions))
	for _, ref := range selection.Revisions {
		if ref.Chapter <= 0 || ref.Revision <= 0 || strings.TrimSpace(ref.ContentSHA256) == "" {
			return domain.OfficialSelection{}, false, fmt.Errorf("invalid official revision ref for chapter %d: %w", ref.Chapter, errs.ErrToolArgs)
		}
		if _, exists := seen[ref.Chapter]; exists {
			return domain.OfficialSelection{}, false, fmt.Errorf("duplicate official revision ref for chapter %d: %w", ref.Chapter, errs.ErrToolArgs)
		}
		seen[ref.Chapter] = struct{}{}
	}

	var committed domain.OfficialSelection
	var changed bool
	err = s.io.WithWriteLock(func() error {
		// Re-read every canonical ChapterRecord while holding the same owning
		// Store lock used to commit the manifest. A stale caller can never replace
		// the Official pointer for an older revision.
		for _, expected := range selection.Revisions {
			var current domain.ChapterRecord
			if err := s.io.ReadJSONUnlocked(ChapterRecordPath(expected.Chapter), &current); err != nil {
				if os.IsNotExist(err) {
					return fmt.Errorf("chapter %d has no canonical record: %w", expected.Chapter, errs.ErrToolConflict)
				}
				return fmt.Errorf("read chapter %d record: %w: %w", expected.Chapter, errs.ErrStoreRead, err)
			}
			if current.Revision != expected.Revision || current.ContentSHA256 != expected.ContentSHA256 {
				return fmt.Errorf("chapter %d revision changed from expected r%d/%s to r%d/%s: %w",
					expected.Chapter, expected.Revision, expected.ContentSHA256,
					current.Revision, current.ContentSHA256, errs.ErrToolConflict)
			}
		}

		manifest, err := s.loadOfficialManifestUnlocked()
		if err != nil {
			return err
		}
		if existing, ok := manifest.Selections[key]; ok && sameOfficialSelection(existing, selection) {
			committed = cloneOfficialSelection(existing)
			changed = false
			return nil
		}

		selection.PromotedAt = time.Now().UTC()
		selection = cloneOfficialSelection(selection)
		manifest.Selections[key] = selection
		if err := s.io.WriteJSONUnlocked(officialRevisionsPath, manifest); err != nil {
			return fmt.Errorf("write official revision manifest: %w: %w", errs.ErrStoreWrite, err)
		}
		committed = cloneOfficialSelection(selection)
		changed = true
		return nil
	})
	if err != nil {
		return domain.OfficialSelection{}, false, err
	}
	return committed, changed, nil
}

func officialTargetKey(target domain.OfficialTarget) (string, error) {
	switch target.Scope {
	case "chapter":
		if target.Chapter <= 0 || target.Volume != 0 || target.Arc != 0 || target.ThroughChapter != 0 {
			return "", fmt.Errorf("invalid chapter official target: %w", errs.ErrToolArgs)
		}
		return fmt.Sprintf("chapter:%d", target.Chapter), nil
	case "arc":
		if target.Chapter != 0 || target.Volume <= 0 || target.Arc <= 0 || target.ThroughChapter <= 0 {
			return "", fmt.Errorf("invalid arc official target: %w", errs.ErrToolArgs)
		}
		return fmt.Sprintf("arc:%d:%d:%d", target.Volume, target.Arc, target.ThroughChapter), nil
	case "global":
		if target.Chapter != 0 || target.Volume != 0 || target.Arc != 0 || target.ThroughChapter <= 0 {
			return "", fmt.Errorf("invalid global official target: %w", errs.ErrToolArgs)
		}
		return fmt.Sprintf("global:%d", target.ThroughChapter), nil
	default:
		return "", fmt.Errorf("unsupported official target scope %q: %w", target.Scope, errs.ErrToolArgs)
	}
}

func sameOfficialSelection(existing, requested domain.OfficialSelection) bool {
	return existing.Target == requested.Target &&
		existing.ReviewFingerprint == requested.ReviewFingerprint &&
		reflect.DeepEqual(existing.Revisions, requested.Revisions)
}

func cloneOfficialSelection(in domain.OfficialSelection) domain.OfficialSelection {
	out := in
	out.Revisions = append([]domain.OfficialRevisionRef(nil), in.Revisions...)
	return out
}
