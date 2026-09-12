package appruntime

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/voocel/ainovel-cli/internal/diag"
	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/host"
	"github.com/voocel/ainovel-cli/internal/stylestat"
)

const defaultReviewHistoryPageSize = 50

func (r *Runtime) queryReviewCatalog() (json.RawMessage, error) {
	return marshalQueryData(ReviewCatalogResultDTO{Contract: CurrentReviewContractCatalog()})
}

func (r *Runtime) queryReviewStatus(req ReviewStatusQuery) (json.RawMessage, error) {
	snapshot, err := r.core.DesktopReviewRead(hostReviewTarget(req.Target))
	if err != nil {
		return nil, err
	}
	result, err := aggregateReviewStatus(req.Target, snapshot)
	if err != nil {
		return nil, err
	}
	return marshalQueryData(result)
}

func (r *Runtime) queryReviewHistory(req ReviewHistoryQuery) (json.RawMessage, error) {
	snapshot, err := r.core.DesktopReviewRead(hostReviewTarget(req.Target))
	if err != nil {
		return nil, err
	}
	current, err := aggregateReviewStatus(req.Target, snapshot)
	if err != nil {
		return nil, err
	}

	total := len(snapshot.ReviewHistory)
	limit := req.Limit
	if limit == 0 {
		limit = defaultReviewHistoryPageSize
	}
	start, end := pageBounds(req.Offset, limit, total)
	items := make([]ReviewHistoryItemDTO, 0, end-start)
	// Checkpoints are canonical seq-ascending; Review history is presented newest first.
	for index := start; index < end; index++ {
		cp := snapshot.ReviewHistory[total-1-index]
		overall := ReviewGateStale
		fingerprint := checkpointReviewFingerprint(req.Target, cp)
		if snapshot.ReviewCheckpoint != nil && cp.Seq == snapshot.ReviewCheckpoint.Seq &&
			cp.Digest == snapshot.ReviewArtifactDigest && reviewSemanticFresh(snapshot) {
			overall = current.Overall
			fingerprint = current.Freshness.Fingerprint
		}
		items = append(items, ReviewHistoryItemDTO{
			ReviewID:    checkpointReviewID(req.Target, cp),
			Target:      req.Target,
			State:       ReviewRunCompleted,
			Overall:     overall,
			Fingerprint: fingerprint,
			EvaluatedAt: cp.OccurredAt.UTC(),
		})
	}

	return marshalQueryData(ReviewHistoryResultDTO{
		Items:  items,
		Offset: req.Offset,
		Limit:  limit,
		Total:  total,
	})
}

func hostReviewTarget(target ReviewTargetDTO) host.DesktopReviewTarget {
	return host.DesktopReviewTarget{
		Scope:          string(target.Scope),
		Chapter:        target.Chapter,
		Volume:         target.Volume,
		Arc:            target.Arc,
		ThroughChapter: target.ThroughChapter,
	}
}

func aggregateReviewStatus(target ReviewTargetDTO, snapshot host.DesktopReviewReadSnapshot) (ReviewStatusResultDTO, error) {
	revisions := reviewRevisionRefs(snapshot.Revisions)
	semanticFresh := reviewSemanticFresh(snapshot)
	fingerprint, err := reviewEvidenceFingerprint(target, revisions, snapshot)
	if err != nil {
		return ReviewStatusResultDTO{}, err
	}
	evaluatedAt := reviewEvaluatedAt(snapshot)
	freshness := ReviewFreshnessDTO{
		Fingerprint: fingerprint,
		Revisions:   revisions,
		EvaluatedAt: evaluatedAt,
	}

	gates := make([]ReviewGateResultDTO, 0, len(ReviewGateCatalog()))
	gates = append(gates, contractGateResult(snapshot.Review, semanticFresh, freshness, revisionChapters(revisions)))
	for _, definition := range ReviewGateCatalog()[1:8] {
		gates = append(gates, dimensionGateResult(definition, snapshot.Review, semanticFresh, freshness))
	}
	gates = append(gates, styleGateResult(snapshot.StyleStatus, snapshot.Style, freshness))
	gates = append(gates,
		diagGateResult(ReviewGateFlowIntegrity, diag.CatFlow, snapshot.Diagnostics.Findings, freshness),
		diagGateResult(ReviewGatePlanningIntegrity, diag.CatPlanning, snapshot.Diagnostics.Findings, freshness),
		diagGateResult(ReviewGateContextIntegrity, diag.CatContext, snapshot.Diagnostics.Findings, freshness),
	)

	result := ReviewStatusResultDTO{
		ReviewID:  liveReviewID(target, snapshot, fingerprint),
		Target:    target,
		State:     ReviewRunCompleted,
		Overall:   aggregateReviewGateState(gates, snapshot.Review != nil),
		Freshness: freshness,
		Gates:     gates,
	}
	if snapshot.Review != nil {
		result.Verdict = snapshot.Review.Verdict
		result.Summary = boundedReviewText(snapshot.Review.Summary, 512)
		result.AffectedChapters = slices.Clone(snapshot.Review.AffectedChapters)
	}
	return result, nil
}

func reviewRevisionRefs(records []domain.ChapterRecord) []ReviewRevisionRefDTO {
	refs := make([]ReviewRevisionRefDTO, 0, len(records))
	for _, record := range records {
		refs = append(refs, ReviewRevisionRefDTO{
			Chapter:       record.Chapter,
			Revision:      record.Revision,
			ContentSHA256: record.ContentSHA256,
		})
	}
	return refs
}

func revisionChapters(refs []ReviewRevisionRefDTO) []int {
	chapters := make([]int, 0, len(refs))
	for _, ref := range refs {
		chapters = append(chapters, ref.Chapter)
	}
	return chapters
}

func reviewSemanticFresh(snapshot host.DesktopReviewReadSnapshot) bool {
	if snapshot.Review == nil || snapshot.ReviewCheckpoint == nil || snapshot.ReviewArtifactDigest == "" {
		return false
	}
	checkpoint := snapshot.ReviewCheckpoint
	if checkpoint.Digest == "" || checkpoint.Digest != snapshot.ReviewArtifactDigest || checkpoint.OccurredAt.IsZero() {
		return false
	}
	for _, record := range snapshot.Revisions {
		if record.AcceptedAt.IsZero() || record.AcceptedAt.After(checkpoint.OccurredAt) {
			return false
		}
	}
	return true
}

func reviewEvaluatedAt(snapshot host.DesktopReviewReadSnapshot) time.Time {
	var evaluatedAt time.Time
	for _, record := range snapshot.Revisions {
		if record.AcceptedAt.After(evaluatedAt) {
			evaluatedAt = record.AcceptedAt
		}
	}
	if snapshot.ReviewCheckpoint != nil && snapshot.ReviewCheckpoint.OccurredAt.After(evaluatedAt) {
		evaluatedAt = snapshot.ReviewCheckpoint.OccurredAt
	}
	if evaluatedAt.IsZero() {
		return time.Time{}
	}
	return evaluatedAt.UTC()
}

func contractGateResult(review *domain.ReviewEntry, fresh bool, freshness ReviewFreshnessDTO, chapters []int) ReviewGateResultDTO {
	result := ReviewGateResultDTO{
		GateID:    ReviewGateContractFulfillment,
		State:     ReviewGateNotRun,
		Freshness: freshness,
	}
	if review == nil {
		result.Summary = "No canonical semantic review exists for this target."
		return result
	}
	if !fresh {
		result.State = ReviewGateStale
		result.Summary = "Canonical review predates or cannot be bound to the accepted target revision."
		return result
	}

	status := strings.TrimSpace(review.ContractStatus)
	switch status {
	case "met":
		result.State = ReviewGatePass
	case "partial", "missed":
		// ContractStatus is semantic Editor evidence. It can warn and drive repair,
		// but it never becomes a deterministic hard failure by inventing a threshold.
		result.State = ReviewGateWarn
		result.Actionable = review.Verdict != "accept"
	case "":
		result.State = ReviewGateUnavailable
		result.Summary = "Contract fulfillment is not applicable or was not supplied by the canonical review."
		return result
	default:
		result.State = ReviewGateUnavailable
		result.Summary = "Canonical review contains an unsupported contract status."
		return result
	}

	severity := "info"
	if result.State == ReviewGateWarn {
		severity = "warning"
	}
	result.Summary = "Canonical Editor contract status: " + status
	result.Evidence = append(result.Evidence, ReviewEvidenceDTO{
		Source:   "review_entry.contract",
		Code:     "contract_status." + status,
		Severity: severity,
		Summary:  result.Summary,
		Detail:   boundedReviewText(review.ContractNotes, 384),
		Chapters: slices.Clone(chapters),
	})
	for _, miss := range review.ContractMisses {
		result.Evidence = append(result.Evidence, ReviewEvidenceDTO{
			Source:   "review_entry.contract",
			Code:     "contract_miss",
			Severity: "warning",
			Summary:  boundedReviewText(miss, 240),
			Chapters: slices.Clone(chapters),
		})
	}
	return result
}

func dimensionGateResult(definition ReviewGateDefinitionDTO, review *domain.ReviewEntry, fresh bool, freshness ReviewFreshnessDTO) ReviewGateResultDTO {
	result := ReviewGateResultDTO{
		GateID:    definition.ID,
		State:     ReviewGateNotRun,
		Freshness: freshness,
	}
	if review == nil {
		result.Summary = "No canonical semantic review exists for this target."
		return result
	}
	if !fresh {
		result.State = ReviewGateStale
		result.Summary = "Canonical review predates or cannot be bound to the accepted target revision."
		return result
	}

	dimension := review.Dimension(definition.Dimension)
	if dimension == nil {
		result.State = ReviewGateUnavailable
		result.Summary = "Canonical review did not provide this native dimension."
		return result
	}
	score := dimension.Score
	result.Score = &score
	result.State = semanticDimensionState(*dimension, review.Issues)
	result.Summary = boundedReviewText(dimension.Comment, 384)
	if result.Summary == "" {
		result.Summary = "Canonical Editor dimension assessment is available."
	}
	result.Evidence = append(result.Evidence, ReviewEvidenceDTO{
		Source:   "review_entry.dimension",
		Code:     definition.Dimension,
		Severity: semanticStateSeverity(result.State),
		Summary:  result.Summary,
	})

	for _, issue := range review.Issues {
		if !issueMatchesDimension(issue, definition.Dimension) {
			continue
		}
		result.Evidence = append(result.Evidence, semanticIssueEvidence(issue))
		if issue.RequiresChange {
			result.Actionable = true
		}
	}
	return result
}

func semanticDimensionState(dimension domain.DimensionScore, issues []domain.ConsistencyIssue) ReviewGateState {
	// Legacy DimensionScore.Verdict is honored when present. Current save_review
	// intentionally does not synthesize verdicts from numeric scores.
	switch strings.ToLower(strings.TrimSpace(dimension.Verdict)) {
	case "accept", "pass", "ok":
		return ReviewGatePass
	case "polish", "rewrite", "warn", "warning", "fail", "error":
		return ReviewGateWarn
	}
	for _, issue := range issues {
		if issueMatchesDimension(issue, dimension.Dimension) {
			return ReviewGateWarn
		}
	}
	return ReviewGatePass
}

func issueMatchesDimension(issue domain.ConsistencyIssue, dimension string) bool {
	return strings.EqualFold(strings.TrimSpace(issue.Type), strings.TrimSpace(dimension))
}

func semanticIssueEvidence(issue domain.ConsistencyIssue) ReviewEvidenceDTO {
	return ReviewEvidenceDTO{
		Source:   "review_entry.issue",
		Code:     boundedReviewCode(issue.Type),
		Severity: boundedReviewCode(issue.Severity),
		Summary:  boundedReviewText(issue.Description, 384),
		Detail:   boundedReviewText(issue.Suggestion, 384),
		Chapters: slices.Clone(issue.Chapters),
	}
}

func semanticStateSeverity(state ReviewGateState) string {
	if state == ReviewGateWarn {
		return "warning"
	}
	return "info"
}

func styleGateResult(status string, stats *stylestat.Stats, freshness ReviewFreshnessDTO) ReviewGateResultDTO {
	result := ReviewGateResultDTO{
		GateID:    ReviewGateStyleRegression,
		Freshness: freshness,
	}
	if status != "ok" || stats == nil {
		result.State = ReviewGateUnavailable
		result.Summary = "Style regression needs at least five chapters in the bounded target."
		result.Evidence = []ReviewEvidenceDTO{{
			Source:   "stylestat",
			Code:     "insufficient_sample",
			Severity: "info",
			Summary:  result.Summary,
		}}
		return result
	}

	mixedTitles := stats.TitleFormats != nil && stats.TitleFormats.WithPrefix > 0 && stats.TitleFormats.WithoutPrefix > 0
	preThresholdedSignal := len(stats.TopPhrases) > 0 || len(stats.RepeatedSentences) > 0 || mixedTitles
	if preThresholdedSignal {
		result.State = ReviewGateWarn
		result.Actionable = true
		result.Summary = "Deterministic style statistics contain regression signals already identified by stylestat."
	} else {
		result.State = ReviewGatePass
		result.Summary = "No pre-thresholded style regression signal is present in the bounded sample."
	}

	result.Evidence = append(result.Evidence, ReviewEvidenceDTO{
		Source:   "stylestat",
		Code:     "sample",
		Severity: "info",
		Summary:  fmt.Sprintf("%d chapters; %d pattern classes; %d high-frequency phrases; %d repeated sentences", stats.Chapters, len(stats.Patterns), len(stats.TopPhrases), len(stats.RepeatedSentences)),
	})
	if len(stats.TopPhrases) > 0 {
		result.Evidence = append(result.Evidence, ReviewEvidenceDTO{
			Source: "stylestat", Code: "top_phrases", Severity: "warning",
			Summary: fmt.Sprintf("%d high-frequency phrase signals", len(stats.TopPhrases)),
		})
	}
	if len(stats.RepeatedSentences) > 0 {
		result.Evidence = append(result.Evidence, ReviewEvidenceDTO{
			Source: "stylestat", Code: "repeated_sentences", Severity: "warning",
			Summary: fmt.Sprintf("%d cross-chapter repeated-sentence signals", len(stats.RepeatedSentences)),
		})
	}
	if mixedTitles {
		result.Evidence = append(result.Evidence, ReviewEvidenceDTO{
			Source: "stylestat", Code: "mixed_title_format", Severity: "warning",
			Summary: fmt.Sprintf("mixed title formats: prefixed=%d unprefixed=%d", stats.TitleFormats.WithPrefix, stats.TitleFormats.WithoutPrefix),
		})
	}
	return result
}

func diagGateResult(gateID ReviewGateID, category diag.Category, findings []diag.Finding, freshness ReviewFreshnessDTO) ReviewGateResultDTO {
	result := ReviewGateResultDTO{
		GateID:    gateID,
		State:     ReviewGatePass,
		Freshness: freshness,
	}
	var critical, warnings int
	for _, finding := range findings {
		if finding.Category != category {
			continue
		}
		switch finding.Severity {
		case diag.SevCritical:
			critical++
		case diag.SevWarning:
			warnings++
		}
		result.Evidence = append(result.Evidence, ReviewEvidenceDTO{
			Source:   "diag." + string(category),
			Code:     boundedReviewCode(finding.Rule),
			Severity: string(finding.Severity),
			Summary:  boundedReviewText(finding.Title, 384),
			Detail:   boundedReviewText(finding.Suggestion, 384),
		})
		if finding.AutoLevel != diag.AutoNone {
			result.Actionable = true
		}
	}
	switch {
	case critical > 0:
		result.State = ReviewGateFail
		result.Summary = fmt.Sprintf("%d critical and %d warning deterministic findings", critical, warnings)
	case warnings > 0:
		result.State = ReviewGateWarn
		result.Summary = fmt.Sprintf("%d deterministic warning findings", warnings)
	case len(result.Evidence) > 0:
		result.State = ReviewGatePass
		result.Summary = "Only informational deterministic findings are present."
	default:
		result.Summary = "No deterministic findings in this category."
	}
	return result
}

func aggregateReviewGateState(gates []ReviewGateResultDTO, hasSemanticReview bool) ReviewGateState {
	if hasReviewGateState(gates, ReviewGateFail) {
		return ReviewGateFail
	}
	if hasReviewGateState(gates, ReviewGateStale) {
		return ReviewGateStale
	}
	if hasReviewGateState(gates, ReviewGateWarn) {
		return ReviewGateWarn
	}
	if hasReviewGateState(gates, ReviewGateRunning) {
		return ReviewGateRunning
	}
	if !hasSemanticReview {
		return ReviewGateNotRun
	}
	if hasReviewGateState(gates, ReviewGateUnavailable) {
		return ReviewGateUnavailable
	}
	if hasReviewGateState(gates, ReviewGateNotRun) {
		return ReviewGateNotRun
	}
	return ReviewGatePass
}

func hasReviewGateState(gates []ReviewGateResultDTO, state ReviewGateState) bool {
	for _, gate := range gates {
		if gate.State == state {
			return true
		}
	}
	return false
}

type reviewFingerprintFinding struct {
	Rule       string          `json:"rule"`
	Category   diag.Category   `json:"category"`
	Severity   diag.Severity   `json:"severity"`
	Confidence diag.Confidence `json:"confidence"`
	AutoLevel  diag.AutoLevel  `json:"auto_level"`
	Target     string          `json:"target"`
	Title      string          `json:"title"`
	Suggestion string          `json:"suggestion"`
}

func reviewEvidenceFingerprint(target ReviewTargetDTO, revisions []ReviewRevisionRefDTO, snapshot host.DesktopReviewReadSnapshot) (string, error) {
	findings := make([]reviewFingerprintFinding, 0)
	for _, finding := range snapshot.Diagnostics.Findings {
		if finding.Category != diag.CatFlow && finding.Category != diag.CatPlanning && finding.Category != diag.CatContext {
			continue
		}
		findings = append(findings, reviewFingerprintFinding{
			Rule: finding.Rule, Category: finding.Category, Severity: finding.Severity,
			Confidence: finding.Confidence, AutoLevel: finding.AutoLevel, Target: finding.Target,
			Title: finding.Title, Suggestion: finding.Suggestion,
		})
	}
	payload := struct {
		Target       ReviewTargetDTO            `json:"target"`
		Revisions    []ReviewRevisionRefDTO     `json:"revisions"`
		ReviewDigest string                     `json:"review_digest,omitempty"`
		Findings     []reviewFingerprintFinding `json:"findings,omitempty"`
		StyleStatus  string                     `json:"style_status"`
		Style        *stylestat.Stats           `json:"style,omitempty"`
	}{
		Target: target, Revisions: revisions, ReviewDigest: snapshot.ReviewArtifactDigest,
		Findings: findings, StyleStatus: snapshot.StyleStatus, Style: snapshot.Style,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal review fingerprint: %w", err)
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func checkpointReviewFingerprint(target ReviewTargetDTO, checkpoint domain.Checkpoint) string {
	payload := struct {
		Target ReviewTargetDTO `json:"target"`
		Seq    int64           `json:"seq"`
		Digest string          `json:"digest"`
	}{Target: target, Seq: checkpoint.Seq, Digest: checkpoint.Digest}
	data, _ := json.Marshal(payload)
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func liveReviewID(target ReviewTargetDTO, snapshot host.DesktopReviewReadSnapshot, fingerprint string) string {
	if snapshot.ReviewCheckpoint != nil && snapshot.ReviewArtifactDigest != "" && snapshot.ReviewCheckpoint.Digest == snapshot.ReviewArtifactDigest {
		return checkpointReviewID(target, *snapshot.ReviewCheckpoint)
	}
	return "aggregate:" + shortFingerprint(fingerprint)
}

func checkpointReviewID(target ReviewTargetDTO, checkpoint domain.Checkpoint) string {
	return fmt.Sprintf("review:%s:%d:%d", target.Scope, reviewTargetEndpoint(target), checkpoint.Seq)
}

func reviewTargetEndpoint(target ReviewTargetDTO) int {
	if target.Scope == ReviewScopeChapter {
		return target.Chapter
	}
	return target.ThroughChapter
}

func shortFingerprint(fingerprint string) string {
	fingerprint = strings.TrimPrefix(fingerprint, "sha256:")
	if len(fingerprint) <= 16 {
		return fingerprint
	}
	return fingerprint[:16]
}

func boundedReviewCode(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unspecified"
	}
	if utf8.RuneCountInString(value) > 96 {
		return boundedReviewText(value, 96)
	}
	return value
}

func boundedReviewText(value string, maxRunes int) string {
	value = strings.Join(strings.Fields(value), " ")
	if value == "" || maxRunes <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes]) + "…"
}
