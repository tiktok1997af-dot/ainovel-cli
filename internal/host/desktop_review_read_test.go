package host

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/voocel/ainovel-cli/internal/domain"
)

func TestG063DesktopReviewArtifactDigestMatchesStoreJSONEncoding(t *testing.T) {
	review := &domain.ReviewEntry{
		Chapter:        7,
		Scope:          "chapter",
		Dimensions:     []domain.DimensionScore{{Dimension: "consistency", Score: 88, Comment: "ok"}},
		ContractStatus: "met",
		Verdict:        "accept",
		Summary:        "canonical",
	}
	got, err := desktopReviewArtifactDigest(review)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.MarshalIndent(review, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	want := "sha256:" + hex.EncodeToString(sum[:])
	if got != want {
		t.Fatalf("digest = %q, want %q", got, want)
	}
	if empty, err := desktopReviewArtifactDigest(nil); err != nil || empty != "" {
		t.Fatalf("nil review digest = %q, %v", empty, err)
	}
}

func TestG063DesktopReviewCheckpointsFilterExactScopeAndArtifact(t *testing.T) {
	scope := domain.ArcScope(2, 3)
	now := time.Date(2026, 9, 13, 1, 0, 0, 0, time.UTC)
	all := []domain.Checkpoint{
		{Seq: 1, Scope: scope, Step: "review", Artifact: "reviews/12.json", Digest: "sha256:a", OccurredAt: now},
		{Seq: 2, Scope: domain.ChapterScope(12), Step: "review", Artifact: "reviews/12.json", Digest: "sha256:b", OccurredAt: now},
		{Seq: 3, Scope: scope, Step: "commit", Artifact: "reviews/12.json", Digest: "sha256:c", OccurredAt: now},
		{Seq: 4, Scope: scope, Step: "review", Artifact: "reviews/12-global.json", Digest: "sha256:d", OccurredAt: now},
		{Seq: 5, Scope: scope, Step: "review", Artifact: "reviews/12.json", Digest: "sha256:e", OccurredAt: now.Add(time.Minute)},
	}
	got := desktopReviewCheckpoints(all, scope, "reviews/12.json")
	want := []domain.Checkpoint{all[0], all[4]}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("filtered checkpoints = %#v, want %#v", got, want)
	}
}
