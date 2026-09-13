package host

import "github.com/voocel/ainovel-cli/internal/domain"

// DesktopPromoteOfficial is the narrow Host seam for the privileged G06.5
// transition. AppRuntime validates Review eligibility; ChapterRecordStore owns
// the final revision CAS and atomic manifest commit.
func (h *Host) DesktopPromoteOfficial(
	target DesktopReviewTarget,
	revisions []domain.OfficialRevisionRef,
	reviewFingerprint string,
) (domain.OfficialSelection, bool, error) {
	selection := domain.OfficialSelection{
		Target: domain.OfficialTarget{
			Scope:          target.Scope,
			Chapter:        target.Chapter,
			Volume:         target.Volume,
			Arc:            target.Arc,
			ThroughChapter: target.ThroughChapter,
		},
		Revisions:         append([]domain.OfficialRevisionRef(nil), revisions...),
		ReviewFingerprint: reviewFingerprint,
	}
	return h.store.ChapterRecords.PromoteOfficial(selection)
}
