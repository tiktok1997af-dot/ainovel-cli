package appruntime

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/voocel/ainovel-cli/internal/diag"
	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/host"
	"github.com/voocel/ainovel-cli/internal/stylestat"
)

func TestG063AggregateReviewStatusFrozenOrderAndDeterministicFingerprint(t *testing.T) {
	target := ReviewTargetDTO{Scope: ReviewScopeChapter, Chapter: 7}
	snapshot := freshReviewSnapshot(t, target, 88)

	first, err := aggregateReviewStatus(target, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	second, err := aggregateReviewStatus(target, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if first.Overall != ReviewGatePass {
		t.Fatalf("overall = %s, want pass", first.Overall)
	}
	if first.Freshness.Fingerprint == "" || first.Freshness.Fingerprint != second.Freshness.Fingerprint {
		t.Fatalf("fingerprint must be deterministic: %q vs %q", first.Freshness.Fingerprint, second.Freshness.Fingerprint)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("same canonical snapshot produced different projection:\nfirst=%#v\nsecond=%#v", first, second)
	}

	catalog := ReviewGateCatalog()
	if len(first.Gates) != len(catalog) {
		t.Fatalf("gate count = %d, want %d", len(first.Gates), len(catalog))
	}
	for index, definition := range catalog {
		if first.Gates[index].GateID != definition.ID {
			t.Fatalf("gate[%d] = %s, want %s", index, first.Gates[index].GateID, definition.ID)
		}
	}
}

func TestG063SemanticReviewStalesAfterAcceptedRevision(t *testing.T) {
	target := ReviewTargetDTO{Scope: ReviewScopeChapter, Chapter: 7}
	snapshot := freshReviewSnapshot(t, target, 88)
	snapshot.Revisions[0].AcceptedAt = snapshot.ReviewCheckpoint.OccurredAt.Add(time.Second)

	got, err := aggregateReviewStatus(target, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if got.Overall != ReviewGateStale {
		t.Fatalf("overall = %s, want stale", got.Overall)
	}
	for index := 0; index < 8; index++ {
		if got.Gates[index].State != ReviewGateStale {
			t.Fatalf("semantic gate[%d] = %s, want stale", index, got.Gates[index].State)
		}
	}
}

func TestG063SemanticScoreNeverInventsHardThreshold(t *testing.T) {
	target := ReviewTargetDTO{Scope: ReviewScopeChapter, Chapter: 7}
	snapshot := freshReviewSnapshot(t, target, 1)

	got, err := aggregateReviewStatus(target, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	consistency := gateResult(t, got.Gates, ReviewGateConsistency)
	if consistency.Score == nil || *consistency.Score != 1 {
		t.Fatalf("consistency score = %v, want 1", consistency.Score)
	}
	if consistency.State != ReviewGatePass {
		t.Fatalf("score-only semantic gate = %s, want pass without invented threshold", consistency.State)
	}
	if got.Overall != ReviewGatePass {
		t.Fatalf("overall = %s, want pass", got.Overall)
	}
}

func TestG063CriticalDeterministicFindingHardFails(t *testing.T) {
	target := ReviewTargetDTO{Scope: ReviewScopeChapter, Chapter: 7}
	snapshot := freshReviewSnapshot(t, target, 1)
	snapshot.Diagnostics.Findings = []diag.Finding{{
		Rule: "ChapterGaps", Category: diag.CatFlow, Severity: diag.SevCritical,
		Confidence: diag.ConfHigh, AutoLevel: diag.AutoNone,
		Title: "canonical gap", Suggestion: "repair canonical sequence",
	}}

	got, err := aggregateReviewStatus(target, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	flow := gateResult(t, got.Gates, ReviewGateFlowIntegrity)
	if flow.State != ReviewGateFail {
		t.Fatalf("flow = %s, want fail", flow.State)
	}
	if got.Overall != ReviewGateFail {
		t.Fatalf("overall = %s, want fail", got.Overall)
	}
	if gateResult(t, got.Gates, ReviewGateConsistency).State != ReviewGatePass {
		t.Fatal("semantic score must not be promoted into deterministic hard failure")
	}
}

func TestG063ProjectionDoesNotLeakRawReviewOrStyleEvidence(t *testing.T) {
	const reviewSecret = "RAW NOVEL EXCERPT MUST NOT ESCAPE"
	const styleSecret = "RAW REPEATED SENTENCE MUST NOT ESCAPE"
	target := ReviewTargetDTO{Scope: ReviewScopeChapter, Chapter: 7}
	snapshot := freshReviewSnapshot(t, target, 88)
	snapshot.Review.Issues = []domain.ConsistencyIssue{{
		Type: "consistency", Severity: "warning", Description: "bounded issue",
		Evidence: reviewSecret, Suggestion: "bounded suggestion", Chapters: []int{7},
	}}
	snapshot.StyleStatus = "ok"
	snapshot.Style = &stylestat.Stats{
		Chapters:          5,
		TopPhrases:        []stylestat.PhraseStat{{Text: reviewSecret, Count: 9}},
		RepeatedSentences: []stylestat.SentenceStat{{Text: styleSecret, Chapters: 3, Count: 3}},
	}

	got, err := aggregateReviewStatus(target, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	style := gateResult(t, got.Gates, ReviewGateStyleRegression)
	if style.State != ReviewGateWarn {
		t.Fatalf("style = %s, want warn", style.State)
	}
	data, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	serialized := string(data)
	if strings.Contains(serialized, reviewSecret) || strings.Contains(serialized, styleSecret) {
		t.Fatalf("bounded projection leaked raw evidence: %s", serialized)
	}
}

func TestG063ReviewQueryTargetsAreTypedAndFailClosed(t *testing.T) {
	valid := []QueryRequest{
		{Kind: QueryReviewCatalog, Payload: json.RawMessage(`{}`)},
		{Kind: QueryReviewStatus, Payload: json.RawMessage(`{"target":{"scope":"chapter","chapter":3}}`)},
		{Kind: QueryReviewStatus, Payload: json.RawMessage(`{"target":{"scope":"arc","volume":2,"arc":1,"through_chapter":12}}`)},
		{Kind: QueryReviewHistory, Payload: json.RawMessage(`{"target":{"scope":"global","through_chapter":20},"offset":0,"limit":10}`)},
	}
	for _, req := range valid {
		if _, err := decodeQueryRoute(req); err != nil {
			t.Fatalf("valid %s rejected: %v", req.Kind, err)
		}
	}

	invalid := []QueryRequest{
		{Kind: QueryReviewStatus, Payload: json.RawMessage(`{"target":{"scope":"chapter","chapter":0}}`)},
		{Kind: QueryReviewStatus, Payload: json.RawMessage(`{"target":{"scope":"chapter","chapter":3,"through_chapter":3}}`)},
		{Kind: QueryReviewStatus, Payload: json.RawMessage(`{"target":{"scope":"arc","volume":2,"arc":1}}`)},
		{Kind: QueryReviewStatus, Payload: json.RawMessage(`{"target":{"scope":"global","through_chapter":20,"chapter":20}}`)},
		{Kind: QueryReviewStatus, Payload: json.RawMessage(`{"target":{"scope":"chapter","chapter":3},"unexpected":true}`)},
	}
	for _, req := range invalid {
		if _, err := decodeQueryRoute(req); err == nil {
			t.Fatalf("invalid %s payload accepted: %s", req.Kind, req.Payload)
		}
	}
}

func freshReviewSnapshot(t *testing.T, target ReviewTargetDTO, semanticScore int) host.DesktopReviewReadSnapshot {
	t.Helper()
	acceptedAt := time.Date(2026, 9, 13, 1, 0, 0, 0, time.UTC)
	checkpointAt := acceptedAt.Add(time.Minute)
	dimensions := make([]domain.DimensionScore, 0, 7)
	for _, name := range []string{"consistency", "character", "pacing", "continuity", "foreshadow", "hook", "aesthetic"} {
		dimensions = append(dimensions, domain.DimensionScore{Dimension: name, Score: semanticScore, Comment: "canonical semantic assessment"})
	}
	review := &domain.ReviewEntry{
		Chapter: target.Chapter, Scope: string(target.Scope), Dimensions: dimensions,
		ContractStatus: "met", Verdict: "accept", Summary: "canonical review",
	}
	if target.Scope != ReviewScopeChapter {
		review.Chapter = target.ThroughChapter
	}
	return host.DesktopReviewReadSnapshot{
		Target:               hostReviewTarget(target),
		Review:               review,
		ReviewArtifactDigest: "sha256:review",
		ReviewCheckpoint: &domain.Checkpoint{
			Seq: 9, Step: "review", Digest: "sha256:review", OccurredAt: checkpointAt,
		},
		Revisions: []domain.ChapterRecord{{
			Chapter: review.Chapter, Revision: 2, ContentSHA256: "sha256:chapter", AcceptedAt: acceptedAt,
		}},
		StyleStatus: "insufficient_sample",
	}
}

func gateResult(t *testing.T, gates []ReviewGateResultDTO, id ReviewGateID) ReviewGateResultDTO {
	t.Helper()
	for _, gate := range gates {
		if gate.GateID == id {
			return gate
		}
	}
	t.Fatalf("gate %s not found", id)
	return ReviewGateResultDTO{}
}
