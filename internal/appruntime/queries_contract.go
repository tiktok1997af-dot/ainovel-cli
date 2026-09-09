package appruntime

import "time"

const MaxQueryPageSize = 500

const (
	QueryProjectOverview    QueryKind = "project.overview"
	QueryChaptersList       QueryKind = "chapters.list"
	QueryChaptersGet        QueryKind = "chapters.get"
	QueryOutlineGet         QueryKind = "outline.get"
	QueryDocumentsList      QueryKind = "documents.list"
	QueryDocumentsGet       QueryKind = "documents.get"
	QueryKnowledgeContext   QueryKind = "knowledge.context"
	QueryKnowledgeCanon     QueryKind = "knowledge.canon"
	QueryKnowledgeCharacters QueryKind = "knowledge.characters"
	QueryKnowledgeWorld     QueryKind = "knowledge.world"
	QueryKnowledgeTimeline  QueryKind = "knowledge.timeline"
)

// SupportedQueryKinds returns the stable G03 Project / Knowledge read catalog.
// Keep this order stable for desktop capability discovery and contract tests.
func SupportedQueryKinds() []QueryKind {
	return []QueryKind{
		QueryProjectOverview,
		QueryChaptersList,
		QueryChaptersGet,
		QueryOutlineGet,
		QueryDocumentsList,
		QueryDocumentsGet,
		QueryKnowledgeContext,
		QueryKnowledgeCanon,
		QueryKnowledgeCharacters,
		QueryKnowledgeWorld,
		QueryKnowledgeTimeline,
	}
}

type PageQuery struct {
	Offset int `json:"offset,omitempty"`
	Limit  int `json:"limit,omitempty"`
}

type ProjectOverviewQuery struct{}

type ProjectOverviewResultDTO struct {
	FormatVersion     int    `json:"format_version"`
	Title             string `json:"title,omitempty"`
	Synopsis          string `json:"synopsis,omitempty"`
	Premise           string `json:"premise,omitempty"`
	Phase             string `json:"phase,omitempty"`
	Flow              string `json:"flow,omitempty"`
	Layered           bool   `json:"layered"`
	CurrentChapter    int    `json:"current_chapter"`
	CompletedChapters int    `json:"completed_chapters"`
	TotalWordCount    int    `json:"total_word_count"`
	CurrentVolume     int    `json:"current_volume,omitempty"`
	CurrentArc        int    `json:"current_arc,omitempty"`
}

type ChaptersListQuery struct {
	PageQuery
}

type ChapterListItemDTO struct {
	Chapter   int    `json:"chapter"`
	Title     string `json:"title,omitempty"`
	Status    string `json:"status"`
	WordCount int    `json:"word_count,omitempty"`
	HasPlan   bool   `json:"has_plan"`
	HasDraft  bool   `json:"has_draft"`
	HasFinal  bool   `json:"has_final"`
	Origin    string `json:"origin,omitempty"`
	Revision  int    `json:"revision,omitempty"`
	Volume    int    `json:"volume,omitempty"`
	Arc       int    `json:"arc,omitempty"`
}

type ChaptersListResultDTO struct {
	Items  []ChapterListItemDTO `json:"items"`
	Offset int                  `json:"offset"`
	Limit  int                  `json:"limit"`
	Total  int                  `json:"total"`
}

type ChaptersGetQuery struct {
	Chapter        int  `json:"chapter"`
	IncludeContent bool `json:"include_content,omitempty"`
}

type ChapterPlanViewDTO struct {
	Chapter    int      `json:"chapter"`
	Title      string   `json:"title,omitempty"`
	Goal       string   `json:"goal,omitempty"`
	Conflict   string   `json:"conflict,omitempty"`
	Hook       string   `json:"hook,omitempty"`
	EmotionArc string   `json:"emotion_arc,omitempty"`
	Notes      string   `json:"notes,omitempty"`
	Required   []string `json:"required_beats,omitempty"`
	Forbidden  []string `json:"forbidden_moves,omitempty"`
	Continuity []string `json:"continuity_checks,omitempty"`
}

type ChapterTextViewDTO struct {
	Present   bool   `json:"present"`
	Content   string `json:"content,omitempty"`
	WordCount int    `json:"word_count,omitempty"`
}

type ChapterRecordViewDTO struct {
	Revision      int       `json:"revision,omitempty"`
	Origin        string    `json:"origin,omitempty"`
	ContentSHA256 string    `json:"content_sha256,omitempty"`
	AcceptedAt    time.Time `json:"accepted_at,omitempty"`
}

type ChaptersGetResultDTO struct {
	Chapter int                   `json:"chapter"`
	Title   string                `json:"title,omitempty"`
	Status  string                `json:"status"`
	Plan    *ChapterPlanViewDTO   `json:"plan,omitempty"`
	Draft   *ChapterTextViewDTO   `json:"draft,omitempty"`
	Final   *ChapterTextViewDTO   `json:"final,omitempty"`
	Record  *ChapterRecordViewDTO `json:"record,omitempty"`
}

type OutlineGetQuery struct {
	FromChapter int `json:"from_chapter,omitempty"`
	Limit       int `json:"limit,omitempty"`
}

type OutlineChapterDTO struct {
	Chapter   int      `json:"chapter"`
	Title     string   `json:"title,omitempty"`
	CoreEvent string   `json:"core_event,omitempty"`
	Hook      string   `json:"hook,omitempty"`
	Scenes    []string `json:"scenes,omitempty"`
}

type ArcOutlineViewDTO struct {
	Index             int                 `json:"index"`
	Title             string              `json:"title,omitempty"`
	Goal              string              `json:"goal,omitempty"`
	EstimatedChapters int                 `json:"estimated_chapters,omitempty"`
	Chapters          []OutlineChapterDTO `json:"chapters,omitempty"`
}

type VolumeOutlineViewDTO struct {
	Index int                 `json:"index"`
	Title string              `json:"title,omitempty"`
	Theme string              `json:"theme,omitempty"`
	Final bool                `json:"final"`
	Arcs  []ArcOutlineViewDTO `json:"arcs,omitempty"`
}

type StoryCompassViewDTO struct {
	EndingDirection string   `json:"ending_direction,omitempty"`
	OpenThreads     []string `json:"open_threads,omitempty"`
	EstimatedScale  string   `json:"estimated_scale,omitempty"`
	LastUpdated     int      `json:"last_updated,omitempty"`
}

type OutlineGetResultDTO struct {
	Layered  bool                   `json:"layered"`
	Chapters []OutlineChapterDTO    `json:"chapters,omitempty"`
	Volumes  []VolumeOutlineViewDTO `json:"volumes,omitempty"`
	Compass  *StoryCompassViewDTO   `json:"compass,omitempty"`
}

type DocumentsListQuery struct {
	PageQuery
	Kind   string `json:"kind,omitempty"`
	Prefix string `json:"prefix,omitempty"`
}

type DocumentSummaryDTO struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Path        string `json:"path"`
	Title       string `json:"title,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	ReadOnly    bool   `json:"read_only"`
	SizeBytes   int64  `json:"size_bytes,omitempty"`
}

type DocumentsListResultDTO struct {
	Items  []DocumentSummaryDTO `json:"items"`
	Offset int                  `json:"offset"`
	Limit  int                  `json:"limit"`
	Total  int                  `json:"total"`
}

type DocumentsGetQuery struct {
	ID string `json:"id"`
}

type DocumentsGetResultDTO struct {
	Document DocumentSummaryDTO `json:"document"`
	Content  string             `json:"content,omitempty"`
}

type KnowledgeContextQuery struct {
	Chapter  int    `json:"chapter,omitempty"`
	Scope    string `json:"scope,omitempty"`
	MaxItems int    `json:"max_items,omitempty"`
}

type ProvenanceDTO struct {
	ArtifactID string `json:"artifact_id"`
	Kind       string `json:"kind"`
	Path       string `json:"path,omitempty"`
	Chapter    int    `json:"chapter,omitempty"`
	Revision   int    `json:"revision,omitempty"`
}

type KnowledgeItemDTO struct {
	Kind       string          `json:"kind"`
	Key        string          `json:"key,omitempty"`
	Title      string          `json:"title,omitempty"`
	Summary    string          `json:"summary,omitempty"`
	Chapter    int             `json:"chapter,omitempty"`
	Provenance []ProvenanceDTO `json:"provenance"`
}

type KnowledgeSectionDTO struct {
	Name  string             `json:"name"`
	Items []KnowledgeItemDTO `json:"items"`
}

type KnowledgeContextResultDTO struct {
	Chapter  int                   `json:"chapter,omitempty"`
	Scope    string                `json:"scope,omitempty"`
	Sections []KnowledgeSectionDTO `json:"sections"`
}

type KnowledgeCanonQuery struct {
	PageQuery
	Chapter int    `json:"chapter,omitempty"`
	Scope   string `json:"scope,omitempty"`
}

type CanonFactDTO struct {
	Kind       string          `json:"kind"`
	Subject    string          `json:"subject,omitempty"`
	Field      string          `json:"field,omitempty"`
	Value      string          `json:"value"`
	Chapter    int             `json:"chapter,omitempty"`
	Provenance []ProvenanceDTO `json:"provenance"`
}

type KnowledgeCanonResultDTO struct {
	Facts  []CanonFactDTO `json:"facts"`
	Offset int            `json:"offset"`
	Limit  int            `json:"limit"`
	Total  int            `json:"total"`
}

type KnowledgeCharactersQuery struct {
	PageQuery
	Scope string `json:"scope,omitempty"` // all / core / cast
}

type CharacterViewDTO struct {
	Name        string   `json:"name"`
	Aliases     []string `json:"aliases,omitempty"`
	Role        string   `json:"role,omitempty"`
	Description string   `json:"description,omitempty"`
	Arc         string   `json:"arc,omitempty"`
	Traits      []string `json:"traits,omitempty"`
	Tier        string   `json:"tier,omitempty"`
	Origin      string   `json:"origin"` // core / cast
	FirstSeen   int      `json:"first_seen,omitempty"`
	LastSeen    int      `json:"last_seen,omitempty"`
	Appearances int      `json:"appearances,omitempty"`
}

type KnowledgeCharactersResultDTO struct {
	Items  []CharacterViewDTO `json:"items"`
	Offset int                `json:"offset"`
	Limit  int                `json:"limit"`
	Total  int                `json:"total"`
}

type KnowledgeWorldQuery struct {
	Sections []string `json:"sections,omitempty"`
	Limit    int      `json:"limit,omitempty"`
}

type WorldRuleViewDTO struct {
	Category string `json:"category,omitempty"`
	Rule     string `json:"rule"`
	Boundary string `json:"boundary,omitempty"`
}

type ForeshadowViewDTO struct {
	ID          string `json:"id"`
	Description string `json:"description,omitempty"`
	PlantedAt   int    `json:"planted_at,omitempty"`
	Status      string `json:"status,omitempty"`
	ResolvedAt  int    `json:"resolved_at,omitempty"`
}

type RelationshipViewDTO struct {
	CharacterA string `json:"character_a"`
	CharacterB string `json:"character_b"`
	Relation   string `json:"relation,omitempty"`
	Chapter    int    `json:"chapter,omitempty"`
}

type StateChangeViewDTO struct {
	Chapter  int    `json:"chapter"`
	Entity   string `json:"entity"`
	Field    string `json:"field"`
	OldValue string `json:"old_value,omitempty"`
	NewValue string `json:"new_value"`
	Reason   string `json:"reason,omitempty"`
}

type KnowledgeWorldResultDTO struct {
	Rules         []WorldRuleViewDTO     `json:"rules"`
	Foreshadow    []ForeshadowViewDTO    `json:"foreshadow"`
	Relationships []RelationshipViewDTO  `json:"relationships"`
	StateChanges  []StateChangeViewDTO   `json:"state_changes"`
}

type KnowledgeTimelineQuery struct {
	FromChapter int `json:"from_chapter,omitempty"`
	ToChapter   int `json:"to_chapter,omitempty"`
	Limit       int `json:"limit,omitempty"`
}

type TimelineEventViewDTO struct {
	Chapter    int      `json:"chapter"`
	Time       string   `json:"time,omitempty"`
	Event      string   `json:"event"`
	Characters []string `json:"characters,omitempty"`
}

type KnowledgeTimelineResultDTO struct {
	Events []TimelineEventViewDTO `json:"events"`
	Total  int                    `json:"total"`
}
