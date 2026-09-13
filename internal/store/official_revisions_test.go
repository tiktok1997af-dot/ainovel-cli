package store

import (
	"errors"
	"testing"
	"time"

	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/errs"
)

func TestPromoteOfficialIsIdempotentAndDoesNotMutateChapterRecord(t *testing.T) {
	st := NewStore(t.TempDir())
	if err := st.Init(); err != nil {
		t.Fatal(err)
	}
	content := "chapter seven\n"
	record := domain.ChapterRecord{
		Version:       domain.ChapterRecordVersion,
		Chapter:       7,
		Revision:      3,
		Origin:        domain.ChapterOriginGenerated,
		Content:       content,
		ContentSHA256: domain.ChapterContentSHA256(content),
		AcceptedAt:    time.Now().Add(-time.Minute),
	}
	if err := st.ChapterRecords.Save(record); err != nil {
		t.Fatal(err)
	}
	selection := domain.OfficialSelection{
		Target: domain.OfficialTarget{Scope: "chapter", Chapter: 7},
		Revisions: []domain.OfficialRevisionRef{{
			Chapter: 7, Revision: 3, ContentSHA256: record.ContentSHA256,
		}},
		ReviewFingerprint: "sha256:review",
	}
	first, changed, err := st.ChapterRecords.PromoteOfficial(selection)
	if err != nil {
		t.Fatal(err)
	}
	if !changed || first.PromotedAt.IsZero() {
		t.Fatalf("first promotion = %+v changed=%v", first, changed)
	}
	second, changed, err := st.ChapterRecords.PromoteOfficial(selection)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("exact promotion replay must be a no-op")
	}
	if !second.PromotedAt.Equal(first.PromotedAt) {
		t.Fatalf("idempotent replay changed promoted_at: %v -> %v", first.PromotedAt, second.PromotedAt)
	}
	got, err := st.ChapterRecords.Load(7)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Revision != record.Revision || got.ContentSHA256 != record.ContentSHA256 || got.Content != record.Content {
		t.Fatalf("promotion mutated chapter record: %+v", got)
	}
}

func TestPromoteOfficialRejectsStaleRevisionCAS(t *testing.T) {
	st := NewStore(t.TempDir())
	if err := st.Init(); err != nil {
		t.Fatal(err)
	}
	content := "current\n"
	record := domain.ChapterRecord{
		Version:       domain.ChapterRecordVersion,
		Chapter:       2,
		Revision:      4,
		Origin:        domain.ChapterOriginUser,
		Content:       content,
		ContentSHA256: domain.ChapterContentSHA256(content),
		AcceptedAt:    time.Now(),
	}
	if err := st.ChapterRecords.Save(record); err != nil {
		t.Fatal(err)
	}
	_, _, err := st.ChapterRecords.PromoteOfficial(domain.OfficialSelection{
		Target: domain.OfficialTarget{Scope: "chapter", Chapter: 2},
		Revisions: []domain.OfficialRevisionRef{{
			Chapter: 2, Revision: 3, ContentSHA256: record.ContentSHA256,
		}},
		ReviewFingerprint: "sha256:review",
	})
	if err == nil || !errors.Is(err, errs.ErrToolConflict) {
		t.Fatalf("stale CAS error = %v, want ErrToolConflict", err)
	}
	manifest, loadErr := st.ChapterRecords.LoadOfficialManifest()
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if len(manifest.Selections) != 0 {
		t.Fatalf("stale promotion wrote manifest: %+v", manifest)
	}
}
