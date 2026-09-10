package appruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/voocel/ainovel-cli/internal/domain"
	apperrs "github.com/voocel/ainovel-cli/internal/errs"
)

const mutationStatusCompleted = "completed"

func isLifecycleCommand(kind CommandKind) bool {
	switch kind {
	case CommandStart, CommandPause, CommandResume, CommandStop, CommandCancel, CommandRetry:
		return true
	default:
		return false
	}
}

// dispatchMutation is the executable G03.7 write plane. commandMu deliberately
// serializes it with lifecycle Dispatch calls, but this is not G05 resource
// locking: resource remains a logical conflict identity only.
func (r *Runtime) dispatchMutation(ctx context.Context, cmd CommandRequest) (CommandResult, error) {
	r.commandMu.Lock()
	defer r.commandMu.Unlock()

	cmd.ID = r.ensureCommandID(cmd.ID)
	result := CommandResult{
		CommandID: cmd.ID,
		RunID:     cmd.RunID,
		TaskID:    cmd.TaskID,
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}

	route, err := decodeMutationContract(cmd)
	if err != nil {
		return result, err
	}
	result.Resource = route.Resource

	if err := r.ensureMutationState(); err != nil {
		return result, err
	}

	data, err := r.applyMutationRoute(route)
	if err != nil {
		return result, mapMutationCoreError(err)
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		return result, fmt.Errorf("marshal mutation result: %w", err)
	}
	result.Accepted = true
	result.Status = mutationStatusCompleted
	result.Data = encoded
	return result, nil
}

// Mutating story/project truth while the engine is physically active can race
// the existing writer/editor workflows. Paused/stopped/failed/completed states
// are not globally forbidden; command-specific Store preconditions still apply.
func (r *Runtime) ensureMutationState() error {
	r.reconcileLifecycle(r.core.Snapshot())
	if r.core.DesktopEngineRunning() {
		return ErrMutationPrecondition
	}
	switch r.currentLifecycleState() {
	case LifecycleStarting, LifecycleRunning, LifecyclePausing, LifecycleResuming,
		LifecycleStopping, LifecycleCancelling, LifecycleRecovering:
		return ErrMutationPrecondition
	default:
		return nil
	}
}

func (r *Runtime) applyMutationRoute(route mutationContractRoute) (any, error) {
	switch route.Kind {
	case CommandProjectMetadataUpdate:
		payload := route.Request.(ProjectMetadataUpdatePayload)
		book := domain.BookMetadata{Title: payload.Title, Synopsis: payload.Synopsis}.Normalized()
		if err := r.core.DesktopSaveBookMetadata(book); err != nil {
			return nil, err
		}
		return ProjectMetadataUpdateResultDTO{Title: book.Title, Synopsis: book.Synopsis}, nil

	case CommandProjectPremiseUpdate:
		payload := route.Request.(ProjectPremiseUpdatePayload)
		if err := r.core.DesktopSavePremise(strings.TrimSpace(payload.Premise)); err != nil {
			return nil, err
		}
		return ProjectPremiseUpdateResultDTO{Updated: true}, nil

	case CommandChapterPlanSave:
		payload := route.Request.(ChapterPlanSavePayload)
		plan := chapterPlanFromMutation(payload)
		if err := r.core.DesktopSaveChapterPlan(plan); err != nil {
			return nil, err
		}
		return ChapterPlanSaveResultDTO{Chapter: plan.Chapter}, nil

	case CommandChapterDraftSave, CommandChapterWorkspaceSave:
		payload := route.Request.(ChapterTextSavePayload)
		return r.applyChapterTextMutation(route.Kind, payload)

	case CommandOutlineTailRevise:
		payload := route.Request.(OutlineTailRevisePayload)
		replacement := outlineEntriesFromMutation(payload.Replacement, payload.FromChapter)
		capacity, err := r.core.DesktopReviseOutline(payload.FromChapter, replacement)
		if err != nil {
			return nil, err
		}
		return OutlineTailReviseResultDTO{FromChapter: payload.FromChapter, PlannedCapacity: capacity}, nil

	case CommandOutlineArcExpand:
		payload := route.Request.(OutlineArcExpandPayload)
		expansion := arcExpansionFromMutation(payload.Expansion)
		project, err := r.core.DesktopProjectRead()
		if err != nil {
			return nil, err
		}
		if err := validateArcExpansionTarget(project.Volumes, payload.Volume, payload.Arc, expansion); err != nil {
			return nil, err
		}
		if err := r.core.DesktopExpandArc(payload.Volume, payload.Arc, expansion); err != nil {
			return nil, err
		}
		return OutlineArcExpandResultDTO{Volume: payload.Volume, Arc: payload.Arc, DetailedChapters: len(expansion.Chapters)}, nil

	case CommandOutlineVolumeAppend:
		payload := route.Request.(OutlineVolumeAppendPayload)
		volume := volumeOutlineFromMutation(payload.Volume)
		project, err := r.core.DesktopProjectRead()
		if err != nil {
			return nil, err
		}
		if err := validateVolumeAppendTarget(project.Volumes, volume); err != nil {
			return nil, err
		}
		if err := r.core.DesktopAppendVolume(volume); err != nil {
			return nil, err
		}
		return OutlineVolumeAppendResultDTO{Volume: volume.Index}, nil

	case CommandOutlineCompassUpdate:
		payload := route.Request.(OutlineCompassUpdatePayload)
		compass := storyCompassFromMutation(payload.Compass)
		if err := r.core.DesktopSaveCompass(compass); err != nil {
			return nil, err
		}
		return OutlineCompassUpdateResultDTO{LastUpdated: compass.LastUpdated}, nil

	case CommandKnowledgeCharactersReplace:
		payload := route.Request.(KnowledgeCharactersReplacePayload)
		characters := coreCharactersFromMutation(payload.Characters)
		if err := r.core.DesktopReplaceCoreCharacters(characters); err != nil {
			return nil, err
		}
		return KnowledgeCharactersReplaceResultDTO{Count: len(characters)}, nil

	case CommandKnowledgeWorldRulesReplace:
		payload := route.Request.(KnowledgeWorldRulesReplacePayload)
		rules := worldRulesFromMutation(payload.Rules)
		if err := r.core.DesktopReplaceWorldRules(rules); err != nil {
			return nil, err
		}
		return KnowledgeWorldRulesReplaceResultDTO{Count: len(rules)}, nil

	case CommandKnowledgeTimelineAppend:
		payload := route.Request.(KnowledgeTimelineAppendPayload)
		events := timelineEventsFromMutation(payload.Events)
		if err := r.core.DesktopAppendTimelineEvents(events); err != nil {
			return nil, err
		}
		return KnowledgeTimelineAppendResultDTO{Submitted: len(events)}, nil

	case CommandKnowledgeRelationshipsUpdate:
		payload := route.Request.(KnowledgeRelationshipsUpdatePayload)
		changes := relationshipsFromMutation(payload.Changes)
		if err := r.core.DesktopUpdateRelationships(changes); err != nil {
			return nil, err
		}
		return KnowledgeRelationshipsUpdateResultDTO{Count: len(changes)}, nil

	case CommandKnowledgeForeshadowUpdate:
		payload := route.Request.(KnowledgeForeshadowUpdatePayload)
		updates := foreshadowFromMutation(payload.Updates)
		world, err := r.core.DesktopWorldKnowledgeRead()
		if err != nil {
			return nil, err
		}
		if err := validateForeshadowTargets(world.Foreshadow, updates); err != nil {
			return nil, err
		}
		if err := r.core.DesktopUpdateForeshadow(payload.Chapter, updates); err != nil {
			return nil, err
		}
		return KnowledgeForeshadowUpdateResultDTO{Count: len(updates)}, nil
	default:
		return nil, ErrUnsupportedMutation
	}
}

func (r *Runtime) applyChapterTextMutation(kind CommandKind, payload ChapterTextSavePayload) (ChapterTextSaveResultDTO, error) {
	current, err := r.core.DesktopChapterRead(payload.Chapter)
	if err != nil {
		return ChapterTextSaveResultDTO{}, err
	}
	if err := validateChapterTextGuards(kind, payload, current.Draft, current.Final, current.Record); err != nil {
		return ChapterTextSaveResultDTO{}, err
	}

	content := domain.NormalizeChapterContent(payload.Content)
	artifact := "draft"
	if kind == CommandChapterWorkspaceSave {
		artifact = "workspace"
		err = r.core.DesktopSaveChapterWorkspace(payload.Chapter, content)
	} else {
		err = r.core.DesktopSaveChapterDraft(payload.Chapter, content)
	}
	if err != nil {
		return ChapterTextSaveResultDTO{}, err
	}
	return ChapterTextSaveResultDTO{
		Chapter:       payload.Chapter,
		Artifact:      artifact,
		ContentSHA256: domain.ChapterContentSHA256(content),
		WordCount:     domain.WordCount(content),
	}, nil
}

func validateChapterTextGuards(kind CommandKind, payload ChapterTextSavePayload, draft, workspace string, record *domain.ChapterRecord) error {
	current := draft
	if kind == CommandChapterWorkspaceSave {
		current = workspace
	}
	if payload.ExpectedSHA256 != "" {
		if current == "" {
			return ErrMutationTargetNotFound
		}
		if domain.ChapterContentSHA256(current) != payload.ExpectedSHA256 {
			return ErrMutationStale
		}
	}
	if payload.ExpectedRecordRevision > 0 {
		if record == nil {
			return ErrMutationTargetNotFound
		}
		if record.Revision != payload.ExpectedRecordRevision {
			return ErrMutationStale
		}
	}
	return nil
}

func validateArcExpansionTarget(volumes []domain.VolumeOutline, volume, arc int, expansion domain.ArcExpansion) error {
	for _, currentVolume := range volumes {
		if currentVolume.Index != volume {
			continue
		}
		for _, currentArc := range currentVolume.Arcs {
			if currentArc.Index != arc {
				continue
			}
			if !currentArc.IsExpanded() {
				return nil
			}
			current := domain.ArcExpansion{Title: currentArc.Title, Goal: currentArc.Goal, Chapters: currentArc.Chapters}
			if reflect.DeepEqual(current, expansion) {
				return nil // same-payload replay; Store repairs any partial derived views
			}
			return ErrMutationPrecondition
		}
		return ErrMutationTargetNotFound
	}
	return ErrMutationTargetNotFound
}

func validateVolumeAppendTarget(volumes []domain.VolumeOutline, volume domain.VolumeOutline) error {
	if len(volumes) == 0 {
		return ErrMutationPrecondition
	}
	last := volumes[len(volumes)-1]
	if reflect.DeepEqual(last, volume) {
		return nil // idempotent replay after a partial cross-domain write
	}
	if volume.Index <= last.Index {
		return ErrMutationPrecondition
	}
	return nil
}

func validateForeshadowTargets(current []domain.ForeshadowEntry, updates []domain.ForeshadowUpdate) error {
	known := make(map[string]struct{}, len(current)+len(updates))
	for _, entry := range current {
		known[strings.TrimSpace(entry.ID)] = struct{}{}
	}
	for _, update := range updates {
		id := strings.TrimSpace(update.ID)
		switch update.Action {
		case "plant":
			known[id] = struct{}{}
		case "advance", "resolve":
			if _, ok := known[id]; !ok {
				return ErrMutationTargetNotFound
			}
		}
	}
	return nil
}

func mapMutationCoreError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, ErrInvalidMutation), errors.Is(err, ErrUnsupportedMutation),
		errors.Is(err, ErrMutationTargetNotFound), errors.Is(err, ErrMutationPrecondition), errors.Is(err, ErrMutationStale):
		return err
	case errors.Is(err, apperrs.ErrToolPrecondition), errors.Is(err, apperrs.ErrToolConflict),
		errors.Is(err, apperrs.ErrPhaseTransition), errors.Is(err, apperrs.ErrFlowTransition):
		return fmt.Errorf("%w: %w", ErrMutationPrecondition, err)
	case errors.Is(err, apperrs.ErrToolArgs):
		return fmt.Errorf("%w: %w", ErrInvalidMutation, err)
	case errors.Is(err, apperrs.ErrStoreRead), errors.Is(err, apperrs.ErrStoreWrite):
		return err
	default:
		return err
	}
}

func chapterPlanFromMutation(payload ChapterPlanSavePayload) domain.ChapterPlan {
	return domain.ChapterPlan{
		Chapter:    payload.Chapter,
		Title:      strings.TrimSpace(payload.Title),
		Goal:       strings.TrimSpace(payload.Goal),
		Conflict:   strings.TrimSpace(payload.Conflict),
		Hook:       strings.TrimSpace(payload.Hook),
		EmotionArc: strings.TrimSpace(payload.EmotionArc),
		Notes:      strings.TrimSpace(payload.Notes),
		Contract: domain.ChapterContract{
			RequiredBeats:    trimStrings(payload.Contract.RequiredBeats),
			ForbiddenMoves:   trimStrings(payload.Contract.ForbiddenMoves),
			ContinuityChecks: trimStrings(payload.Contract.ContinuityChecks),
			EvaluationFocus:  trimStrings(payload.Contract.EvaluationFocus),
			EmotionTarget:    strings.TrimSpace(payload.Contract.EmotionTarget),
			PayoffPoints:     trimStrings(payload.Contract.PayoffPoints),
			HookGoal:         strings.TrimSpace(payload.Contract.HookGoal),
		},
	}
}

func outlineEntriesFromMutation(entries []OutlineEntryMutationDTO, firstChapter int) []domain.OutlineEntry {
	out := make([]domain.OutlineEntry, 0, len(entries))
	for i, entry := range entries {
		chapter := 0
		if firstChapter > 0 {
			chapter = firstChapter + i
		}
		out = append(out, domain.OutlineEntry{
			Chapter:   chapter,
			Title:     strings.TrimSpace(entry.Title),
			CoreEvent: strings.TrimSpace(entry.CoreEvent),
			Hook:      strings.TrimSpace(entry.Hook),
			Scenes:    trimStrings(entry.Scenes),
		})
	}
	return out
}

func arcExpansionFromMutation(expansion ArcExpansionMutationDTO) domain.ArcExpansion {
	return domain.ArcExpansion{
		Title:    strings.TrimSpace(expansion.Title),
		Goal:     strings.TrimSpace(expansion.Goal),
		Chapters: outlineEntriesFromMutation(expansion.Chapters, 0),
	}
}

func volumeOutlineFromMutation(volume VolumeOutlineMutationDTO) domain.VolumeOutline {
	out := domain.VolumeOutline{
		Index: volume.Index,
		Title: strings.TrimSpace(volume.Title),
		Theme: strings.TrimSpace(volume.Theme),
		Final: volume.Final,
		Arcs:  make([]domain.ArcOutline, 0, len(volume.Arcs)),
	}
	for _, arc := range volume.Arcs {
		out.Arcs = append(out.Arcs, domain.ArcOutline{
			Index:             arc.Index,
			Title:             strings.TrimSpace(arc.Title),
			Goal:              strings.TrimSpace(arc.Goal),
			EstimatedChapters: arc.EstimatedChapters,
			Chapters:          outlineEntriesFromMutation(arc.Chapters, 0),
		})
	}
	return out
}

func storyCompassFromMutation(compass StoryCompassMutationDTO) domain.StoryCompass {
	return domain.StoryCompass{
		EndingDirection: strings.TrimSpace(compass.EndingDirection),
		OpenThreads:     trimStrings(compass.OpenThreads),
		EstimatedScale:  strings.TrimSpace(compass.EstimatedScale),
		LastUpdated:     compass.LastUpdated,
	}
}

func coreCharactersFromMutation(characters []CoreCharacterMutationDTO) []domain.Character {
	out := make([]domain.Character, 0, len(characters))
	for _, character := range characters {
		out = append(out, domain.Character{
			Name:        strings.TrimSpace(character.Name),
			Aliases:     trimStrings(character.Aliases),
			Role:        strings.TrimSpace(character.Role),
			Description: strings.TrimSpace(character.Description),
			Arc:         strings.TrimSpace(character.Arc),
			Traits:      trimStrings(character.Traits),
			Tier:        strings.TrimSpace(character.Tier),
		})
	}
	return out
}

func worldRulesFromMutation(rules []WorldRuleMutationDTO) []domain.WorldRule {
	out := make([]domain.WorldRule, 0, len(rules))
	for _, rule := range rules {
		out = append(out, domain.WorldRule{
			Category: strings.TrimSpace(rule.Category),
			Rule:     strings.TrimSpace(rule.Rule),
			Boundary: strings.TrimSpace(rule.Boundary),
		})
	}
	return out
}

func timelineEventsFromMutation(events []TimelineEventMutationDTO) []domain.TimelineEvent {
	out := make([]domain.TimelineEvent, 0, len(events))
	for _, event := range events {
		out = append(out, domain.TimelineEvent{
			Chapter:    event.Chapter,
			Time:       strings.TrimSpace(event.Time),
			Event:      strings.TrimSpace(event.Event),
			Characters: trimStrings(event.Characters),
		})
	}
	return out
}

func relationshipsFromMutation(changes []RelationshipMutationDTO) []domain.RelationshipEntry {
	out := make([]domain.RelationshipEntry, 0, len(changes))
	for _, change := range changes {
		out = append(out, domain.RelationshipEntry{
			CharacterA: strings.TrimSpace(change.CharacterA),
			CharacterB: strings.TrimSpace(change.CharacterB),
			Relation:   strings.TrimSpace(change.Relation),
			Chapter:    change.Chapter,
		})
	}
	return out
}

func foreshadowFromMutation(updates []ForeshadowMutationDTO) []domain.ForeshadowUpdate {
	out := make([]domain.ForeshadowUpdate, 0, len(updates))
	for _, update := range updates {
		out = append(out, domain.ForeshadowUpdate{
			ID:          strings.TrimSpace(update.ID),
			Action:      strings.TrimSpace(update.Action),
			Description: strings.TrimSpace(update.Description),
		})
	}
	return out
}

func trimStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			out = append(out, value)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
