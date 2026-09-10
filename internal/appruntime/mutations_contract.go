package appruntime

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

const MaxMutationPayloadBytes = 4 << 20

const (
	CommandProjectMetadataUpdate        CommandKind = "project.metadata.update"
	CommandProjectPremiseUpdate         CommandKind = "project.premise.update"
	CommandChapterPlanSave              CommandKind = "chapter.plan.save"
	CommandChapterDraftSave             CommandKind = "chapter.draft.save"
	CommandChapterWorkspaceSave         CommandKind = "chapter.workspace.save"
	CommandOutlineTailRevise            CommandKind = "outline.tail.revise"
	CommandOutlineArcExpand             CommandKind = "outline.arc.expand"
	CommandOutlineVolumeAppend          CommandKind = "outline.volume.append"
	CommandOutlineCompassUpdate         CommandKind = "outline.compass.update"
	CommandKnowledgeCharactersReplace   CommandKind = "knowledge.characters.core.replace"
	CommandKnowledgeWorldRulesReplace   CommandKind = "knowledge.world.rules.replace"
	CommandKnowledgeTimelineAppend      CommandKind = "knowledge.timeline.append"
	CommandKnowledgeRelationshipsUpdate CommandKind = "knowledge.relationships.update"
	CommandKnowledgeForeshadowUpdate    CommandKind = "knowledge.foreshadow.update"
)

// G03MutationCommandKinds returns the forward-locked G03.6 mutation catalog.
// These contracts are staged until G03.7 wires Store-backed handlers.
func G03MutationCommandKinds() []CommandKind {
	return []CommandKind{
		CommandProjectMetadataUpdate,
		CommandProjectPremiseUpdate,
		CommandChapterPlanSave,
		CommandChapterDraftSave,
		CommandChapterWorkspaceSave,
		CommandOutlineTailRevise,
		CommandOutlineArcExpand,
		CommandOutlineVolumeAppend,
		CommandOutlineCompassUpdate,
		CommandKnowledgeCharactersReplace,
		CommandKnowledgeWorldRulesReplace,
		CommandKnowledgeTimelineAppend,
		CommandKnowledgeRelationshipsUpdate,
		CommandKnowledgeForeshadowUpdate,
	}
}

type ProjectMetadataUpdatePayload struct {
	Title    string `json:"title"`
	Synopsis string `json:"synopsis"`
}

type ProjectMetadataUpdateResultDTO struct {
	Title    string `json:"title"`
	Synopsis string `json:"synopsis"`
}

type ProjectPremiseUpdatePayload struct {
	Premise string `json:"premise"`
}

type ProjectPremiseUpdateResultDTO struct {
	Updated bool `json:"updated"`
}

type ChapterContractMutationDTO struct {
	RequiredBeats    []string `json:"required_beats,omitempty"`
	ForbiddenMoves   []string `json:"forbidden_moves,omitempty"`
	ContinuityChecks []string `json:"continuity_checks,omitempty"`
	EvaluationFocus  []string `json:"evaluation_focus,omitempty"`
	EmotionTarget    string   `json:"emotion_target,omitempty"`
	PayoffPoints     []string `json:"payoff_points,omitempty"`
	HookGoal         string   `json:"hook_goal,omitempty"`
}

type ChapterPlanSavePayload struct {
	Chapter    int                        `json:"chapter"`
	Title      string                     `json:"title"`
	Goal       string                     `json:"goal"`
	Conflict   string                     `json:"conflict,omitempty"`
	Hook       string                     `json:"hook,omitempty"`
	EmotionArc string                     `json:"emotion_arc,omitempty"`
	Notes      string                     `json:"notes,omitempty"`
	Contract   ChapterContractMutationDTO `json:"contract,omitempty"`
}

type ChapterPlanSaveResultDTO struct {
	Chapter int `json:"chapter"`
}

type ChapterTextSavePayload struct {
	Chapter                int    `json:"chapter"`
	Content                string `json:"content"`
	ExpectedSHA256         string `json:"expected_sha256,omitempty"`
	ExpectedRecordRevision int    `json:"expected_record_revision,omitempty"`
}

type ChapterTextSaveResultDTO struct {
	Chapter       int    `json:"chapter"`
	Artifact      string `json:"artifact"`
	ContentSHA256 string `json:"content_sha256"`
	WordCount     int    `json:"word_count"`
}

type OutlineEntryMutationDTO struct {
	Title     string   `json:"title"`
	CoreEvent string   `json:"core_event"`
	Hook      string   `json:"hook,omitempty"`
	Scenes    []string `json:"scenes,omitempty"`
}

type OutlineTailRevisePayload struct {
	FromChapter int                       `json:"from_chapter"`
	Replacement []OutlineEntryMutationDTO `json:"replacement"`
}

type OutlineTailReviseResultDTO struct {
	FromChapter     int `json:"from_chapter"`
	PlannedCapacity int `json:"planned_capacity"`
}

type ArcExpansionMutationDTO struct {
	Title    string                    `json:"title"`
	Goal     string                    `json:"goal"`
	Chapters []OutlineEntryMutationDTO `json:"chapters"`
}

type OutlineArcExpandPayload struct {
	Volume    int                     `json:"volume"`
	Arc       int                     `json:"arc"`
	Expansion ArcExpansionMutationDTO `json:"expansion"`
}

type OutlineArcExpandResultDTO struct {
	Volume           int `json:"volume"`
	Arc              int `json:"arc"`
	DetailedChapters int `json:"detailed_chapters"`
}

type ArcOutlineMutationDTO struct {
	Index             int                       `json:"index"`
	Title             string                    `json:"title"`
	Goal              string                    `json:"goal"`
	EstimatedChapters int                       `json:"estimated_chapters,omitempty"`
	Chapters          []OutlineEntryMutationDTO `json:"chapters,omitempty"`
}

type VolumeOutlineMutationDTO struct {
	Index int                     `json:"index"`
	Title string                  `json:"title"`
	Theme string                  `json:"theme"`
	Final bool                    `json:"final,omitempty"`
	Arcs  []ArcOutlineMutationDTO `json:"arcs"`
}

type OutlineVolumeAppendPayload struct {
	Volume VolumeOutlineMutationDTO `json:"volume"`
}

type OutlineVolumeAppendResultDTO struct {
	Volume int `json:"volume"`
}

type StoryCompassMutationDTO struct {
	EndingDirection string   `json:"ending_direction"`
	OpenThreads     []string `json:"open_threads,omitempty"`
	EstimatedScale  string   `json:"estimated_scale,omitempty"`
	LastUpdated     int      `json:"last_updated,omitempty"`
}

type OutlineCompassUpdatePayload struct {
	Compass StoryCompassMutationDTO `json:"compass"`
}

type OutlineCompassUpdateResultDTO struct {
	LastUpdated int `json:"last_updated,omitempty"`
}

type CoreCharacterMutationDTO struct {
	Name        string   `json:"name"`
	Aliases     []string `json:"aliases,omitempty"`
	Role        string   `json:"role"`
	Description string   `json:"description"`
	Arc         string   `json:"arc,omitempty"`
	Traits      []string `json:"traits,omitempty"`
	Tier        string   `json:"tier,omitempty"`
}

type KnowledgeCharactersReplacePayload struct {
	Characters []CoreCharacterMutationDTO `json:"characters"`
}

type KnowledgeCharactersReplaceResultDTO struct {
	Count int `json:"count"`
}

type WorldRuleMutationDTO struct {
	Category string `json:"category,omitempty"`
	Rule     string `json:"rule"`
	Boundary string `json:"boundary,omitempty"`
}

type KnowledgeWorldRulesReplacePayload struct {
	Rules []WorldRuleMutationDTO `json:"rules"`
}

type KnowledgeWorldRulesReplaceResultDTO struct {
	Count int `json:"count"`
}

type TimelineEventMutationDTO struct {
	Chapter    int      `json:"chapter"`
	Time       string   `json:"time,omitempty"`
	Event      string   `json:"event"`
	Characters []string `json:"characters,omitempty"`
}

type KnowledgeTimelineAppendPayload struct {
	Events []TimelineEventMutationDTO `json:"events"`
}

type KnowledgeTimelineAppendResultDTO struct {
	Submitted int `json:"submitted"`
}

type RelationshipMutationDTO struct {
	CharacterA string `json:"character_a"`
	CharacterB string `json:"character_b"`
	Relation   string `json:"relation"`
	Chapter    int    `json:"chapter"`
}

type KnowledgeRelationshipsUpdatePayload struct {
	Changes []RelationshipMutationDTO `json:"changes"`
}

type KnowledgeRelationshipsUpdateResultDTO struct {
	Count int `json:"count"`
}

type ForeshadowMutationDTO struct {
	ID          string `json:"id"`
	Action      string `json:"action"`
	Description string `json:"description,omitempty"`
}

type KnowledgeForeshadowUpdatePayload struct {
	Chapter int                     `json:"chapter"`
	Updates []ForeshadowMutationDTO `json:"updates"`
}

type KnowledgeForeshadowUpdateResultDTO struct {
	Count int `json:"count"`
}

type mutationContractRoute struct {
	Kind     CommandKind
	Resource string
	Request  any
}

// decodeMutationContract validates a G03.6 mutation envelope and returns its
// canonical logical resource. It performs no Store/Host mutation.
func decodeMutationContract(cmd CommandRequest) (mutationContractRoute, error) {
	var route mutationContractRoute
	route.Kind = cmd.Kind

	switch cmd.Kind {
	case CommandProjectMetadataUpdate:
		var payload ProjectMetadataUpdatePayload
		if err := decodeMutationPayload(cmd.Payload, &payload); err != nil {
			return mutationContractRoute{}, err
		}
		payload.Title = strings.TrimSpace(payload.Title)
		payload.Synopsis = strings.TrimSpace(payload.Synopsis)
		if payload.Title == "" || payload.Synopsis == "" {
			return mutationContractRoute{}, invalidMutation("project metadata requires title and synopsis")
		}
		route.Resource, route.Request = "project:metadata", payload
	case CommandProjectPremiseUpdate:
		var payload ProjectPremiseUpdatePayload
		if err := decodeMutationPayload(cmd.Payload, &payload); err != nil {
			return mutationContractRoute{}, err
		}
		payload.Premise = strings.TrimSpace(payload.Premise)
		if payload.Premise == "" {
			return mutationContractRoute{}, invalidMutation("premise is required")
		}
		route.Resource, route.Request = "project:premise", payload
	case CommandChapterPlanSave:
		var payload ChapterPlanSavePayload
		if err := decodeMutationPayload(cmd.Payload, &payload); err != nil {
			return mutationContractRoute{}, err
		}
		if payload.Chapter <= 0 || strings.TrimSpace(payload.Title) == "" || strings.TrimSpace(payload.Goal) == "" {
			return mutationContractRoute{}, invalidMutation("chapter plan is invalid")
		}
		route.Resource, route.Request = chapterResource(payload.Chapter, "plan"), payload
	case CommandChapterDraftSave, CommandChapterWorkspaceSave:
		var payload ChapterTextSavePayload
		if err := decodeMutationPayload(cmd.Payload, &payload); err != nil {
			return mutationContractRoute{}, err
		}
		if payload.Chapter <= 0 || strings.TrimSpace(payload.Content) == "" || payload.ExpectedRecordRevision < 0 {
			return mutationContractRoute{}, invalidMutation("chapter text mutation is invalid")
		}
		if payload.ExpectedSHA256 != "" && !validSHA256(payload.ExpectedSHA256) {
			return mutationContractRoute{}, invalidMutation("expected sha256 is invalid")
		}
		artifact := "draft"
		if cmd.Kind == CommandChapterWorkspaceSave {
			artifact = "workspace"
		}
		route.Resource, route.Request = chapterResource(payload.Chapter, artifact), payload
	case CommandOutlineTailRevise:
		var payload OutlineTailRevisePayload
		if err := decodeMutationPayload(cmd.Payload, &payload); err != nil {
			return mutationContractRoute{}, err
		}
		if payload.FromChapter <= 0 || len(payload.Replacement) == 0 || !validOutlineEntries(payload.Replacement) {
			return mutationContractRoute{}, invalidMutation("outline replacement is invalid")
		}
		route.Resource, route.Request = "outline:structure", payload
	case CommandOutlineArcExpand:
		var payload OutlineArcExpandPayload
		if err := decodeMutationPayload(cmd.Payload, &payload); err != nil {
			return mutationContractRoute{}, err
		}
		if payload.Volume <= 0 || payload.Arc <= 0 || strings.TrimSpace(payload.Expansion.Title) == "" || strings.TrimSpace(payload.Expansion.Goal) == "" || len(payload.Expansion.Chapters) == 0 || !validOutlineEntries(payload.Expansion.Chapters) {
			return mutationContractRoute{}, invalidMutation("arc expansion is invalid")
		}
		route.Resource, route.Request = "outline:structure", payload
	case CommandOutlineVolumeAppend:
		var payload OutlineVolumeAppendPayload
		if err := decodeMutationPayload(cmd.Payload, &payload); err != nil {
			return mutationContractRoute{}, err
		}
		if !validVolumeMutation(payload.Volume) {
			return mutationContractRoute{}, invalidMutation("volume append is invalid")
		}
		route.Resource, route.Request = "outline:structure", payload
	case CommandOutlineCompassUpdate:
		var payload OutlineCompassUpdatePayload
		if err := decodeMutationPayload(cmd.Payload, &payload); err != nil {
			return mutationContractRoute{}, err
		}
		if strings.TrimSpace(payload.Compass.EndingDirection) == "" || payload.Compass.LastUpdated < 0 {
			return mutationContractRoute{}, invalidMutation("story compass is invalid")
		}
		route.Resource, route.Request = "outline:compass", payload
	case CommandKnowledgeCharactersReplace:
		var payload KnowledgeCharactersReplacePayload
		if err := decodeMutationPayload(cmd.Payload, &payload); err != nil {
			return mutationContractRoute{}, err
		}
		if len(payload.Characters) == 0 || !validCoreCharacters(payload.Characters) {
			return mutationContractRoute{}, invalidMutation("core characters are invalid")
		}
		route.Resource, route.Request = "knowledge:characters:core", payload
	case CommandKnowledgeWorldRulesReplace:
		var payload KnowledgeWorldRulesReplacePayload
		if err := decodeMutationPayload(cmd.Payload, &payload); err != nil {
			return mutationContractRoute{}, err
		}
		if len(payload.Rules) == 0 || !validWorldRuleMutations(payload.Rules) {
			return mutationContractRoute{}, invalidMutation("world rules are invalid")
		}
		route.Resource, route.Request = "knowledge:world:rules", payload
	case CommandKnowledgeTimelineAppend:
		var payload KnowledgeTimelineAppendPayload
		if err := decodeMutationPayload(cmd.Payload, &payload); err != nil {
			return mutationContractRoute{}, err
		}
		if len(payload.Events) == 0 || !validTimelineMutations(payload.Events) {
			return mutationContractRoute{}, invalidMutation("timeline events are invalid")
		}
		route.Resource, route.Request = "knowledge:timeline", payload
	case CommandKnowledgeRelationshipsUpdate:
		var payload KnowledgeRelationshipsUpdatePayload
		if err := decodeMutationPayload(cmd.Payload, &payload); err != nil {
			return mutationContractRoute{}, err
		}
		if len(payload.Changes) == 0 || !validRelationshipMutations(payload.Changes) {
			return mutationContractRoute{}, invalidMutation("relationship changes are invalid")
		}
		route.Resource, route.Request = "knowledge:relationships", payload
	case CommandKnowledgeForeshadowUpdate:
		var payload KnowledgeForeshadowUpdatePayload
		if err := decodeMutationPayload(cmd.Payload, &payload); err != nil {
			return mutationContractRoute{}, err
		}
		if payload.Chapter <= 0 || len(payload.Updates) == 0 || !validForeshadowMutations(payload.Updates) {
			return mutationContractRoute{}, invalidMutation("foreshadow updates are invalid")
		}
		route.Resource, route.Request = "knowledge:foreshadow", payload
	default:
		return mutationContractRoute{}, unsupportedMutation()
	}

	if resource := strings.TrimSpace(cmd.Resource); resource != "" && resource != route.Resource {
		return mutationContractRoute{}, invalidMutation("resource does not match canonical mutation resource")
	}
	return route, nil
}

func decodeMutationPayload(payload json.RawMessage, dst any) error {
	if len(payload) == 0 || string(payload) == "null" {
		return invalidMutation("mutation payload is required")
	}
	if len(payload) > MaxMutationPayloadBytes || !json.Valid(payload) {
		return invalidMutation("mutation payload is invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return invalidMutation("mutation payload does not match the typed contract")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return invalidMutation("mutation payload contains trailing values")
	}
	return nil
}

func chapterResource(chapter int, artifact string) string {
	return fmt.Sprintf("chapter:%06d:%s", chapter, artifact)
}

func validSHA256(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func validOutlineEntries(entries []OutlineEntryMutationDTO) bool {
	for _, entry := range entries {
		if strings.TrimSpace(entry.Title) == "" || strings.TrimSpace(entry.CoreEvent) == "" {
			return false
		}
	}
	return true
}

func validVolumeMutation(volume VolumeOutlineMutationDTO) bool {
	if volume.Index <= 0 || strings.TrimSpace(volume.Title) == "" || strings.TrimSpace(volume.Theme) == "" || len(volume.Arcs) == 0 {
		return false
	}
	for i, arc := range volume.Arcs {
		if arc.Index <= 0 || strings.TrimSpace(arc.Title) == "" || strings.TrimSpace(arc.Goal) == "" || arc.EstimatedChapters < 0 {
			return false
		}
		if i == 0 && len(arc.Chapters) == 0 {
			return false
		}
		if len(arc.Chapters) > 0 && !validOutlineEntries(arc.Chapters) {
			return false
		}
		if len(arc.Chapters) == 0 && arc.EstimatedChapters <= 0 {
			return false
		}
	}
	return true
}

func validCoreCharacters(characters []CoreCharacterMutationDTO) bool {
	seen := make(map[string]struct{}, len(characters))
	for _, character := range characters {
		name := strings.TrimSpace(character.Name)
		if name == "" || strings.TrimSpace(character.Role) == "" || strings.TrimSpace(character.Description) == "" {
			return false
		}
		key := strings.ToLower(name)
		if _, exists := seen[key]; exists {
			return false
		}
		seen[key] = struct{}{}
	}
	return true
}

func validWorldRuleMutations(rules []WorldRuleMutationDTO) bool {
	for _, rule := range rules {
		if strings.TrimSpace(rule.Rule) == "" {
			return false
		}
	}
	return true
}

func validTimelineMutations(events []TimelineEventMutationDTO) bool {
	for _, event := range events {
		if event.Chapter <= 0 || strings.TrimSpace(event.Event) == "" {
			return false
		}
	}
	return true
}

func validRelationshipMutations(changes []RelationshipMutationDTO) bool {
	for _, change := range changes {
		if strings.TrimSpace(change.CharacterA) == "" || strings.TrimSpace(change.CharacterB) == "" || strings.TrimSpace(change.Relation) == "" || change.Chapter <= 0 || strings.EqualFold(strings.TrimSpace(change.CharacterA), strings.TrimSpace(change.CharacterB)) {
			return false
		}
	}
	return true
}

func validForeshadowMutations(updates []ForeshadowMutationDTO) bool {
	for _, update := range updates {
		if strings.TrimSpace(update.ID) == "" {
			return false
		}
		switch update.Action {
		case "plant":
			if strings.TrimSpace(update.Description) == "" {
				return false
			}
		case "advance", "resolve":
		default:
			return false
		}
	}
	return true
}

func invalidMutation(_ string) error {
	return &AppError{
		Code:     ErrorCodeInvalidArgument,
		Category: ErrorCategoryValidation,
		Message:  safeErrorMessage(ErrorCodeInvalidArgument),
		Cause:    ErrInvalidMutation,
	}
}

func unsupportedMutation() error {
	return &AppError{
		Code:     ErrorCodeUnsupportedOperation,
		Category: ErrorCategoryValidation,
		Message:  safeErrorMessage(ErrorCodeUnsupportedOperation),
		Cause:    ErrUnsupportedMutation,
	}
}
