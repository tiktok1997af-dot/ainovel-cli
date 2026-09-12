package appruntime

import "time"

const (
	QueryReviewCatalog QueryKind = "review.catalog"
	QueryReviewStatus  QueryKind = "review.status"
	QueryReviewHistory QueryKind = "review.history"
)

const (
	CommandReviewRun             CommandKind = "review.run"
	CommandReviewRepair          CommandKind = "review.repair"
	CommandReviewRerun           CommandKind = "review.rerun"
	CommandReviewPromoteOfficial CommandKind = "review.promote_official"
)

const (
	ReviewEventCategory   = "REVIEW"
	EventTypeReviewState  = "review_state"
	EventTypeReviewGate   = "review_gate"
	EventTypeReviewAction = "review_action"
)

type ReviewGateID string

const (
	ReviewGateContractFulfillment ReviewGateID = "contract_fulfillment"
	ReviewGateConsistency         ReviewGateID = "consistency"
	ReviewGateCharacter           ReviewGateID = "character"
	ReviewGatePacing              ReviewGateID = "pacing"
	ReviewGateContinuity          ReviewGateID = "continuity"
	ReviewGateForeshadow          ReviewGateID = "foreshadow"
	ReviewGateHook                ReviewGateID = "hook"
	ReviewGateAesthetic           ReviewGateID = "aesthetic"
	ReviewGateStyleRegression     ReviewGateID = "style_regression"
	ReviewGateFlowIntegrity       ReviewGateID = "flow_integrity"
	ReviewGatePlanningIntegrity   ReviewGateID = "planning_integrity"
	ReviewGateContextIntegrity    ReviewGateID = "context_integrity"
)

type ReviewGateClass string

const (
	ReviewGateClassSemantic      ReviewGateClass = "semantic"
	ReviewGateClassDeterministic ReviewGateClass = "deterministic"
)

type ReviewGateState string

const (
	ReviewGateNotRun      ReviewGateState = "not_run"
	ReviewGateRunning     ReviewGateState = "running"
	ReviewGatePass        ReviewGateState = "pass"
	ReviewGateWarn        ReviewGateState = "warn"
	ReviewGateFail        ReviewGateState = "fail"
	ReviewGateStale       ReviewGateState = "stale"
	ReviewGateUnavailable ReviewGateState = "unavailable"
)

type ReviewScope string

const (
	ReviewScopeChapter ReviewScope = "chapter"
	ReviewScopeArc     ReviewScope = "arc"
	ReviewScopeGlobal  ReviewScope = "global"
)

type ReviewRunState string

const (
	ReviewRunQueued    ReviewRunState = "queued"
	ReviewRunRunning   ReviewRunState = "running"
	ReviewRunCompleted ReviewRunState = "completed"
	ReviewRunFailed    ReviewRunState = "failed"
	ReviewRunCancelled ReviewRunState = "cancelled"
)

type ReviewGateDefinitionDTO struct {
	ID            ReviewGateID    `json:"id"`
	Label         string          `json:"label"`
	Class         ReviewGateClass `json:"class"`
	EvidenceOwner string          `json:"evidence_owner"`
	Dimension     string          `json:"dimension,omitempty"`
}

type ReviewContractCatalog struct {
	QueryKinds   []QueryKind               `json:"query_kinds"`
	CommandKinds []CommandKind             `json:"command_kinds"`
	EventTypes   []string                  `json:"event_types"`
	GateStates   []ReviewGateState         `json:"gate_states"`
	Gates        []ReviewGateDefinitionDTO `json:"gates"`
}

func CurrentReviewContractCatalog() ReviewContractCatalog {
	return ReviewContractCatalog{
		QueryKinds: []QueryKind{
			QueryReviewCatalog,
			QueryReviewStatus,
			QueryReviewHistory,
		},
		CommandKinds: []CommandKind{
			CommandReviewRun,
			CommandReviewRepair,
			CommandReviewRerun,
			CommandReviewPromoteOfficial,
		},
		EventTypes: []string{
			EventTypeReviewState,
			EventTypeReviewGate,
			EventTypeReviewAction,
		},
		GateStates: []ReviewGateState{
			ReviewGateNotRun,
			ReviewGateRunning,
			ReviewGatePass,
			ReviewGateWarn,
			ReviewGateFail,
			ReviewGateStale,
			ReviewGateUnavailable,
		},
		Gates: ReviewGateCatalog(),
	}
}

func ReviewGateCatalog() []ReviewGateDefinitionDTO {
	return []ReviewGateDefinitionDTO{
		{ID: ReviewGateContractFulfillment, Label: "Contract fulfillment", Class: ReviewGateClassSemantic, EvidenceOwner: "review_entry.contract"},
		{ID: ReviewGateConsistency, Label: "Consistency", Class: ReviewGateClassSemantic, EvidenceOwner: "review_entry.dimension", Dimension: "consistency"},
		{ID: ReviewGateCharacter, Label: "Character", Class: ReviewGateClassSemantic, EvidenceOwner: "review_entry.dimension", Dimension: "character"},
		{ID: ReviewGatePacing, Label: "Pacing", Class: ReviewGateClassSemantic, EvidenceOwner: "review_entry.dimension", Dimension: "pacing"},
		{ID: ReviewGateContinuity, Label: "Continuity", Class: ReviewGateClassSemantic, EvidenceOwner: "review_entry.dimension", Dimension: "continuity"},
		{ID: ReviewGateForeshadow, Label: "Foreshadow", Class: ReviewGateClassSemantic, EvidenceOwner: "review_entry.dimension", Dimension: "foreshadow"},
		{ID: ReviewGateHook, Label: "Hook", Class: ReviewGateClassSemantic, EvidenceOwner: "review_entry.dimension", Dimension: "hook"},
		{ID: ReviewGateAesthetic, Label: "Aesthetic", Class: ReviewGateClassSemantic, EvidenceOwner: "review_entry.dimension", Dimension: "aesthetic"},
		{ID: ReviewGateStyleRegression, Label: "Style regression", Class: ReviewGateClassDeterministic, EvidenceOwner: "stylestat"},
		{ID: ReviewGateFlowIntegrity, Label: "Flow integrity", Class: ReviewGateClassDeterministic, EvidenceOwner: "diag.flow"},
		{ID: ReviewGatePlanningIntegrity, Label: "Planning integrity", Class: ReviewGateClassDeterministic, EvidenceOwner: "diag.planning"},
		{ID: ReviewGateContextIntegrity, Label: "Context integrity", Class: ReviewGateClassDeterministic, EvidenceOwner: "diag.context"},
	}
}

type ReviewTargetDTO struct {
	Scope          ReviewScope `json:"scope"`
	Chapter        int         `json:"chapter,omitempty"`
	Volume         int         `json:"volume,omitempty"`
	Arc            int         `json:"arc,omitempty"`
	ThroughChapter int         `json:"through_chapter,omitempty"`
}

type ReviewRevisionRefDTO struct {
	Chapter       int    `json:"chapter"`
	Revision      int    `json:"revision"`
	ContentSHA256 string `json:"content_sha256"`
}

type ReviewFreshnessDTO struct {
	Fingerprint string                 `json:"fingerprint,omitempty"`
	Revisions   []ReviewRevisionRefDTO `json:"revisions,omitempty"`
	EvaluatedAt time.Time              `json:"evaluated_at,omitempty"`
}

type ReviewEvidenceDTO struct {
	Source   string `json:"source"`
	Code     string `json:"code,omitempty"`
	Severity string `json:"severity,omitempty"`
	Summary  string `json:"summary"`
	Detail   string `json:"detail,omitempty"`
	Chapters []int  `json:"chapters,omitempty"`
}

type ReviewGateResultDTO struct {
	GateID     ReviewGateID        `json:"gate_id"`
	State      ReviewGateState     `json:"state"`
	Score      *int                `json:"score,omitempty"`
	Summary    string              `json:"summary,omitempty"`
	Evidence   []ReviewEvidenceDTO `json:"evidence,omitempty"`
	Actionable bool                `json:"actionable"`
	Freshness  ReviewFreshnessDTO  `json:"freshness"`
}

type ReviewCatalogQuery struct{}

type ReviewCatalogResultDTO struct {
	Contract ReviewContractCatalog `json:"contract"`
}

type ReviewStatusQuery struct {
	Target ReviewTargetDTO `json:"target"`
}

type ReviewStatusResultDTO struct {
	ReviewID         string                `json:"review_id,omitempty"`
	Target           ReviewTargetDTO       `json:"target"`
	State            ReviewRunState        `json:"state,omitempty"`
	Overall          ReviewGateState       `json:"overall"`
	Verdict          string                `json:"verdict,omitempty"`
	Summary          string                `json:"summary,omitempty"`
	AffectedChapters []int                 `json:"affected_chapters,omitempty"`
	Freshness        ReviewFreshnessDTO    `json:"freshness"`
	Gates            []ReviewGateResultDTO `json:"gates"`
}

type ReviewHistoryQuery struct {
	PageQuery
	Target ReviewTargetDTO `json:"target"`
}

type ReviewHistoryItemDTO struct {
	ReviewID    string          `json:"review_id"`
	Target      ReviewTargetDTO `json:"target"`
	State       ReviewRunState  `json:"state"`
	Overall     ReviewGateState `json:"overall"`
	Fingerprint string          `json:"fingerprint,omitempty"`
	EvaluatedAt time.Time       `json:"evaluated_at,omitempty"`
}

type ReviewHistoryResultDTO struct {
	Items  []ReviewHistoryItemDTO `json:"items"`
	Offset int                    `json:"offset"`
	Limit  int                    `json:"limit"`
	Total  int                    `json:"total"`
}

type ReviewRunCommandPayload struct {
	Target              ReviewTargetDTO `json:"target"`
	ExpectedFingerprint string          `json:"expected_fingerprint,omitempty"`
}

type ReviewRepairCommandPayload struct {
	Target              ReviewTargetDTO `json:"target"`
	GateIDs             []ReviewGateID  `json:"gate_ids,omitempty"`
	Chapters            []int           `json:"chapters"`
	ExpectedFingerprint string          `json:"expected_fingerprint"`
}

type ReviewRerunCommandPayload struct {
	Target              ReviewTargetDTO `json:"target"`
	ExpectedFingerprint string          `json:"expected_fingerprint"`
}

type ReviewPromoteOfficialCommandPayload struct {
	Target              ReviewTargetDTO        `json:"target"`
	ExpectedFingerprint string                 `json:"expected_fingerprint"`
	ExpectedRevisions   []ReviewRevisionRefDTO `json:"expected_revisions"`
}

type ReviewCommandResultDTO struct {
	ReviewID string          `json:"review_id,omitempty"`
	Target   ReviewTargetDTO `json:"target"`
	State    ReviewRunState  `json:"state,omitempty"`
	Action   string          `json:"action,omitempty"`
}

type ReviewStateEventPayloadDTO struct {
	ReviewID string          `json:"review_id"`
	Target   ReviewTargetDTO `json:"target"`
	State    ReviewRunState  `json:"state"`
	Overall  ReviewGateState `json:"overall,omitempty"`
}

type ReviewGateEventPayloadDTO struct {
	ReviewID string              `json:"review_id"`
	Target   ReviewTargetDTO     `json:"target"`
	Gate     ReviewGateResultDTO `json:"gate"`
}

type ReviewActionEventPayloadDTO struct {
	ReviewID string          `json:"review_id"`
	Target   ReviewTargetDTO `json:"target"`
	Action   string          `json:"action"`
	State    string          `json:"state,omitempty"`
	Chapters []int           `json:"chapters,omitempty"`
}
