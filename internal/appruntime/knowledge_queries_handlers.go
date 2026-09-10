package appruntime

import (
	"encoding/json"
	"sort"

	"github.com/voocel/ainovel-cli/internal/domain"
)

const (
	knowledgeScopeWriter   = "writer"
	knowledgeScopeEditor   = "editor"
	knowledgeScopeOverview = "overview"
	knowledgeScopeAll      = "all"

	knowledgeSectionRules         = "rules"
	knowledgeSectionForeshadow    = "foreshadow"
	knowledgeSectionRelationships = "relationships"
	knowledgeSectionStateChanges  = "state_changes"
)

var knowledgeWorldSectionOrder = []string{
	knowledgeSectionRules,
	knowledgeSectionForeshadow,
	knowledgeSectionRelationships,
	knowledgeSectionStateChanges,
}

func (r *Runtime) queryKnowledgeCharacters(req KnowledgeCharactersQuery) (json.RawMessage, error) {
	data, err := r.core.DesktopCharacterKnowledgeRead()
	if err != nil {
		return nil, err
	}
	items := projectCharacterKnowledge(data, normalizedCharacterScope(req.Scope))
	limit := effectiveProjectQueryLimit(req.Limit)
	start, end := pageBounds(req.Offset, limit, len(items))
	return marshalQueryData(KnowledgeCharactersResultDTO{
		Items:  append([]CharacterViewDTO(nil), items[start:end]...),
		Offset: req.Offset,
		Limit:  limit,
		Total:  len(items),
	})
}

func (r *Runtime) queryKnowledgeWorld(req KnowledgeWorldQuery) (json.RawMessage, error) {
	data, err := r.core.DesktopWorldKnowledgeRead()
	if err != nil {
		return nil, err
	}
	limit := effectiveProjectQueryLimit(req.Limit)
	selected := selectedWorldSections(req.Sections)
	remaining := limit
	result := KnowledgeWorldResultDTO{
		Rules:         make([]WorldRuleViewDTO, 0),
		Foreshadow:    make([]ForeshadowViewDTO, 0),
		Relationships: make([]RelationshipViewDTO, 0),
		StateChanges:  make([]StateChangeViewDTO, 0),
	}
	for _, section := range knowledgeWorldSectionOrder {
		if remaining == 0 || !selected[section] {
			continue
		}
		switch section {
		case knowledgeSectionRules:
			for _, rule := range data.Rules {
				if remaining == 0 {
					break
				}
				result.Rules = append(result.Rules, WorldRuleViewDTO{
					Category: rule.Category,
					Rule:     rule.Rule,
					Boundary: rule.Boundary,
				})
				remaining--
			}
		case knowledgeSectionForeshadow:
			for _, entry := range data.Foreshadow {
				if remaining == 0 {
					break
				}
				result.Foreshadow = append(result.Foreshadow, ForeshadowViewDTO{
					ID:          entry.ID,
					Description: entry.Description,
					PlantedAt:   entry.PlantedAt,
					Status:      entry.Status,
					ResolvedAt:  entry.ResolvedAt,
				})
				remaining--
			}
		case knowledgeSectionRelationships:
			for _, entry := range data.Relationships {
				if remaining == 0 {
					break
				}
				result.Relationships = append(result.Relationships, RelationshipViewDTO{
					CharacterA: entry.CharacterA,
					CharacterB: entry.CharacterB,
					Relation:   entry.Relation,
					Chapter:    entry.Chapter,
				})
				remaining--
			}
		case knowledgeSectionStateChanges:
			for _, change := range data.StateChanges {
				if remaining == 0 {
					break
				}
				result.StateChanges = append(result.StateChanges, StateChangeViewDTO{
					Chapter:  change.Chapter,
					Entity:   change.Entity,
					Field:    change.Field,
					OldValue: change.OldValue,
					NewValue: change.NewValue,
					Reason:   change.Reason,
				})
				remaining--
			}
		}
	}
	return marshalQueryData(result)
}

func (r *Runtime) queryKnowledgeTimeline(req KnowledgeTimelineQuery) (json.RawMessage, error) {
	events, err := r.core.DesktopTimelineKnowledgeRead()
	if err != nil {
		return nil, err
	}
	filtered := make([]domain.TimelineEvent, 0, len(events))
	for _, event := range events {
		if req.FromChapter > 0 && event.Chapter < req.FromChapter {
			continue
		}
		if req.ToChapter > 0 && event.Chapter > req.ToChapter {
			continue
		}
		filtered = append(filtered, event)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].Chapter != filtered[j].Chapter {
			return filtered[i].Chapter < filtered[j].Chapter
		}
		if filtered[i].Time != filtered[j].Time {
			return filtered[i].Time < filtered[j].Time
		}
		return filtered[i].Event < filtered[j].Event
	})
	total := len(filtered)
	limit := effectiveProjectQueryLimit(req.Limit)
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}
	items := make([]TimelineEventViewDTO, 0, len(filtered))
	for _, event := range filtered {
		items = append(items, TimelineEventViewDTO{
			Chapter:    event.Chapter,
			Time:       event.Time,
			Event:      event.Event,
			Characters: cloneStrings(event.Characters),
		})
	}
	return marshalQueryData(KnowledgeTimelineResultDTO{Events: items, Total: total})
}

func (r *Runtime) queryKnowledgeCanon(req KnowledgeCanonQuery) (json.RawMessage, error) {
	data, err := r.core.DesktopKnowledgeRead()
	if err != nil {
		return nil, err
	}
	facts := projectCanonFacts(data, normalizedCanonScope(req.Scope), req.Chapter)
	limit := effectiveProjectQueryLimit(req.Limit)
	start, end := pageBounds(req.Offset, limit, len(facts))
	return marshalQueryData(KnowledgeCanonResultDTO{
		Facts:  append([]CanonFactDTO(nil), facts[start:end]...),
		Offset: req.Offset,
		Limit:  limit,
		Total:  len(facts),
	})
}

func (r *Runtime) queryKnowledgeContext(req KnowledgeContextQuery) (json.RawMessage, error) {
	data, err := r.core.DesktopKnowledgeRead()
	if err != nil {
		return nil, err
	}
	scope := normalizedContextScope(req.Scope)
	maxItems := effectiveProjectQueryLimit(req.MaxItems)
	candidates := projectContextSections(data, req.Chapter, scope)
	order := contextSectionOrder(scope)
	sections := make([]KnowledgeSectionDTO, 0, len(order))
	remaining := maxItems
	for _, name := range order {
		if remaining == 0 {
			break
		}
		items := candidates[name]
		if len(items) == 0 {
			continue
		}
		if len(items) > remaining {
			items = items[:remaining]
		}
		sections = append(sections, KnowledgeSectionDTO{
			Name:  name,
			Items: append([]KnowledgeItemDTO(nil), items...),
		})
		remaining -= len(items)
	}
	return marshalQueryData(KnowledgeContextResultDTO{
		Chapter:  req.Chapter,
		Scope:    scope,
		Sections: sections,
	})
}

func selectedWorldSections(sections []string) map[string]bool {
	selected := make(map[string]bool, len(knowledgeWorldSectionOrder))
	if len(sections) == 0 {
		for _, section := range knowledgeWorldSectionOrder {
			selected[section] = true
		}
		return selected
	}
	for _, section := range sections {
		selected[section] = true
	}
	return selected
}

func normalizedContextScope(scope string) string {
	if scope == "" {
		return knowledgeScopeWriter
	}
	return scope
}

func normalizedCanonScope(scope string) string {
	if scope == "" {
		return knowledgeScopeAll
	}
	return scope
}

func normalizedCharacterScope(scope string) string {
	if scope == "" {
		return knowledgeScopeAll
	}
	return scope
}
