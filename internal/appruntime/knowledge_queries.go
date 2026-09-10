package appruntime

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/host"
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
				result.Rules = append(result.Rules, WorldRuleViewDTO{Category: rule.Category, Rule: rule.Rule, Boundary: rule.Boundary})
				remaining--
			}
		case knowledgeSectionForeshadow:
			for _, entry := range data.Foreshadow {
				if remaining == 0 {
					break
				}
				result.Foreshadow = append(result.Foreshadow, ForeshadowViewDTO{
					ID: entry.ID, Description: entry.Description, PlantedAt: entry.PlantedAt,
					Status: entry.Status, ResolvedAt: entry.ResolvedAt,
				})
				remaining--
			}
		case knowledgeSectionRelationships:
			for _, entry := range data.Relationships {
				if remaining == 0 {
					break
				}
				result.Relationships = append(result.Relationships, RelationshipViewDTO{
					CharacterA: entry.CharacterA, CharacterB: entry.CharacterB,
					Relation: entry.Relation, Chapter: entry.Chapter,
				})
				remaining--
			}
		case knowledgeSectionStateChanges:
			for _, change := range data.StateChanges {
				if remaining == 0 {
					break
				}
				result.StateChanges = append(result.StateChanges, StateChangeViewDTO{
					Chapter: change.Chapter, Entity: change.Entity, Field: change.Field,
					OldValue: change.OldValue, NewValue: change.NewValue, Reason: change.Reason,
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
			Chapter: event.Chapter, Time: event.Time, Event: event.Event,
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
	scope := normalizedCanonScope(req.Scope)
	facts := projectCanonFacts(data, scope, req.Chapter)
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
		sections = append(sections, KnowledgeSectionDTO{Name: name, Items: append([]KnowledgeItemDTO(nil), items...)})
		remaining -= len(items)
	}
	return marshalQueryData(KnowledgeContextResultDTO{Chapter: req.Chapter, Scope: scope, Sections: sections})
}

func projectCharacterKnowledge(data host.DesktopCharacterKnowledgeSnapshot, scope string) []CharacterViewDTO {
	coreNames := make(map[string]struct{}, len(data.Characters))
	items := make([]CharacterViewDTO, 0, len(data.Characters)+len(data.Cast))
	if scope != "cast" {
		for _, character := range data.Characters {
			name := strings.TrimSpace(character.Name)
			if name == "" {
				continue
			}
			coreNames[knowledgeNameKey(name)] = struct{}{}
			items = append(items, CharacterViewDTO{
				Name: name, Aliases: cloneStrings(character.Aliases), Role: character.Role,
				Description: character.Description, Arc: character.Arc, Traits: cloneStrings(character.Traits),
				Tier: character.Tier, Origin: "core",
			})
		}
	} else {
		for _, character := range data.Characters {
			if name := strings.TrimSpace(character.Name); name != "" {
				coreNames[knowledgeNameKey(name)] = struct{}{}
			}
		}
	}
	if scope != "core" {
		for _, entry := range data.Cast {
			name := strings.TrimSpace(entry.Name)
			if name == "" || entry.Promoted {
				continue
			}
			if _, duplicate := coreNames[knowledgeNameKey(name)]; duplicate {
				continue
			}
			items = append(items, CharacterViewDTO{
				Name: name, Aliases: cloneStrings(entry.Aliases), Role: entry.BriefRole,
				Origin: "cast", FirstSeen: entry.FirstSeenChapter, LastSeen: entry.LastSeenChapter,
				Appearances: entry.AppearanceCount,
			})
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		left, right := knowledgeNameKey(items[i].Name), knowledgeNameKey(items[j].Name)
		if left != right {
			return left < right
		}
		return items[i].Origin < items[j].Origin
	})
	return items
}

func projectCanonFacts(data host.DesktopKnowledgeReadSnapshot, scope string, chapter int) []CanonFactDTO {
	bounded := chapter > 0
	include := func(kind string) bool {
		switch scope {
		case "state":
			return kind == "state"
		case "relationship":
			return kind == "relationship"
		case "foreshadow":
			return kind == "foreshadow"
		case "continuity":
			return kind == "world_rule" || kind == "world_boundary" || kind == "chapter_summary" || kind == "chapter_event" || kind == "timeline" || kind == "state" || kind == "relationship" || kind == "foreshadow"
		default:
			return true
		}
	}
	facts := make([]CanonFactDTO, 0)
	if include("character") {
		for _, character := range data.Characters.Characters {
			value := firstNonEmpty(character.Description, character.Role, character.Arc)
			if strings.TrimSpace(character.Name) == "" || strings.TrimSpace(value) == "" {
				continue
			}
			facts = append(facts, CanonFactDTO{
				Kind: "character", Subject: character.Name, Field: "profile", Value: value,
				Provenance: []ProvenanceDTO{artifactProvenance("knowledge.characters", "characters", "characters.json", 0, 0)},
			})
		}
		coreNames := make(map[string]struct{}, len(data.Characters.Characters))
		for _, character := range data.Characters.Characters {
			coreNames[knowledgeNameKey(character.Name)] = struct{}{}
		}
		for _, entry := range data.Characters.Cast {
			if entry.Promoted || strings.TrimSpace(entry.Name) == "" {
				continue
			}
			if _, duplicate := coreNames[knowledgeNameKey(entry.Name)]; duplicate {
				continue
			}
			facts = append(facts, CanonFactDTO{
				Kind: "cast", Subject: entry.Name, Field: "role", Value: firstNonEmpty(entry.BriefRole, "supporting cast"), Chapter: entry.FirstSeenChapter,
				Provenance: []ProvenanceDTO{artifactProvenance("knowledge.cast", "cast", "meta/cast_ledger.json", entry.FirstSeenChapter, 0)},
			})
		}
	}
	if include("world_rule") || include("world_boundary") {
		for _, rule := range data.World.Rules {
			if include("world_rule") && strings.TrimSpace(rule.Rule) != "" {
				facts = append(facts, CanonFactDTO{
					Kind: "world_rule", Subject: rule.Category, Field: "rule", Value: rule.Rule,
					Provenance: []ProvenanceDTO{artifactProvenance("knowledge.world-rules", "world_rules", "world_rules.json", 0, 0)},
				})
			}
			if include("world_boundary") && strings.TrimSpace(rule.Boundary) != "" {
				facts = append(facts, CanonFactDTO{
					Kind: "world_boundary", Subject: rule.Category, Field: "boundary", Value: rule.Boundary,
					Provenance: []ProvenanceDTO{artifactProvenance("knowledge.world-rules", "world_rules", "world_rules.json", 0, 0)},
				})
			}
		}
	}
	if include("chapter_summary") || include("chapter_event") {
		for _, record := range recordsAt(data.Records, chapter, bounded) {
			prov := chapterRecordProvenance(record)
			if include("chapter_summary") && strings.TrimSpace(record.Facts.Summary) != "" {
				facts = append(facts, CanonFactDTO{Kind: "chapter_summary", Subject: fmt.Sprintf("chapter:%d", record.Chapter), Field: "summary", Value: record.Facts.Summary, Chapter: record.Chapter, Provenance: []ProvenanceDTO{prov}})
			}
			if include("chapter_event") {
				for _, event := range record.Facts.KeyEvents {
					if strings.TrimSpace(event) == "" {
						continue
					}
					facts = append(facts, CanonFactDTO{Kind: "chapter_event", Subject: fmt.Sprintf("chapter:%d", record.Chapter), Field: "event", Value: event, Chapter: record.Chapter, Provenance: []ProvenanceDTO{prov}})
				}
			}
		}
	}
	if include("timeline") {
		for _, event := range timelineAt(data.Timeline, chapter, bounded) {
			if strings.TrimSpace(event.Event) == "" {
				continue
			}
			facts = append(facts, CanonFactDTO{
				Kind: "timeline", Subject: strings.Join(event.Characters, ", "), Field: event.Time,
				Value: event.Event, Chapter: event.Chapter,
				Provenance: []ProvenanceDTO{artifactProvenance("knowledge.timeline-log", "timeline", "timeline.jsonl", event.Chapter, 0)},
			})
		}
	}
	if include("state") {
		for _, change := range latestStateAt(data.World.StateChanges, chapter, bounded) {
			facts = append(facts, CanonFactDTO{
				Kind: "state", Subject: change.Entity, Field: change.Field, Value: change.NewValue, Chapter: change.Chapter,
				Provenance: []ProvenanceDTO{artifactProvenance("knowledge.state-changes-log", "state_changes", "meta/state_changes.jsonl", change.Chapter, 0)},
			})
		}
	}
	if include("relationship") {
		for _, relation := range relationshipsAt(data, chapter, bounded) {
			prov := artifactProvenance("knowledge.relationships", "relationships", "relationship_state.json", relation.Chapter, 0)
			if bounded {
				if record := recordForChapter(data.Records, relation.Chapter); record != nil {
					prov = chapterRecordProvenance(*record)
				}
			}
			facts = append(facts, CanonFactDTO{
				Kind: "relationship", Subject: relation.CharacterA + " ↔ " + relation.CharacterB,
				Field: "relation", Value: relation.Relation, Chapter: relation.Chapter, Provenance: []ProvenanceDTO{prov},
			})
		}
	}
	if include("foreshadow") {
		for _, entry := range foreshadowAt(data, chapter, bounded) {
			prov := artifactProvenance("knowledge.foreshadow", "foreshadow", "foreshadow_ledger.json", entry.PlantedAt, 0)
			facts = append(facts, CanonFactDTO{
				Kind: "foreshadow", Subject: entry.ID, Field: entry.Status,
				Value: entry.Description, Chapter: entry.PlantedAt, Provenance: []ProvenanceDTO{prov},
			})
		}
	}
	sort.SliceStable(facts, func(i, j int) bool {
		if facts[i].Chapter != facts[j].Chapter {
			return facts[i].Chapter < facts[j].Chapter
		}
		if facts[i].Kind != facts[j].Kind {
			return facts[i].Kind < facts[j].Kind
		}
		if facts[i].Subject != facts[j].Subject {
			return facts[i].Subject < facts[j].Subject
		}
		if facts[i].Field != facts[j].Field {
			return facts[i].Field < facts[j].Field
		}
		return facts[i].Value < facts[j].Value
	})
	return facts
}

func projectContextSections(data host.DesktopKnowledgeReadSnapshot, chapter int, scope string) map[string][]KnowledgeItemDTO {
	sections := map[string][]KnowledgeItemDTO{
		"project":    {},
		"planning":   {},
		"continuity": {},
		"characters": {},
		"world":      {},
	}
	project := data.Project
	if project.Book != nil {
		sections["project"] = append(sections["project"], KnowledgeItemDTO{
			Kind: "book", Key: "book", Title: project.Book.Title,
			Summary: compactKnowledgeText(project.Book.Synopsis, 600),
			Provenance: []ProvenanceDTO{artifactProvenance("project.book", "book", "meta/book.json", 0, 0)},
		})
	}
	if strings.TrimSpace(project.Premise) != "" {
		sections["project"] = append(sections["project"], KnowledgeItemDTO{
			Kind: "premise", Key: "premise", Summary: compactKnowledgeText(project.Premise, 600),
			Provenance: []ProvenanceDTO{artifactProvenance("project.premise", "premise", "premise.md", 0, 0)},
		})
	}
	if project.Progress != nil {
		sections["project"] = append(sections["project"], KnowledgeItemDTO{
			Kind: "progress", Key: "progress",
			Summary: fmt.Sprintf("phase=%s flow=%s current_chapter=%d completed=%d words=%d", project.Progress.Phase, project.Progress.Flow, project.Progress.CurrentChapter, len(project.Progress.CompletedChapters), project.Progress.TotalWordCount),
			Provenance: []ProvenanceDTO{artifactProvenance("project.progress", "progress", "meta/progress.json", 0, 0)},
		})
	}

	targetChapter := chapter
	if targetChapter == 0 && project.Progress != nil {
		targetChapter = project.Progress.CurrentChapter
		if targetChapter == 0 {
			targetChapter = project.Progress.LatestCompleted() + 1
		}
	}
	outline := effectiveProjectOutline(project)
	for _, entry := range outline {
		if entry.Chapter != targetChapter {
			continue
		}
		artifactID, artifactPath := "outline.flat", "outline.json"
		if len(project.Volumes) > 0 {
			artifactID, artifactPath = "outline.layered", "layered_outline.json"
		}
		summary := strings.TrimSpace(entry.CoreEvent)
		if strings.TrimSpace(entry.Hook) != "" {
			summary = strings.TrimSpace(summary + " | hook: " + entry.Hook)
		}
		sections["planning"] = append(sections["planning"], KnowledgeItemDTO{
			Kind: "outline", Key: fmt.Sprintf("chapter:%d", entry.Chapter), Title: entry.Title,
			Summary: compactKnowledgeText(summary, 600), Chapter: entry.Chapter,
			Provenance: []ProvenanceDTO{artifactProvenance(artifactID, "outline", artifactPath, entry.Chapter, 0)},
		})
		break
	}
	if project.Compass != nil {
		summary := project.Compass.EndingDirection
		if len(project.Compass.OpenThreads) > 0 {
			summary = strings.TrimSpace(summary + " | open: " + strings.Join(project.Compass.OpenThreads, "; "))
		}
		sections["planning"] = append(sections["planning"], KnowledgeItemDTO{
			Kind: "compass", Key: "compass", Summary: compactKnowledgeText(summary, 600), Chapter: project.Compass.LastUpdated,
			Provenance: []ProvenanceDTO{artifactProvenance("outline.compass", "compass", "meta/compass.json", project.Compass.LastUpdated, 0)},
		})
	}

	characterItems := projectCharacterKnowledge(data.Characters, "all")
	for _, character := range characterItems {
		summary := character.Role
		if character.Description != "" {
			summary = strings.TrimSpace(summary + " | " + character.Description)
		}
		if character.Origin == "cast" {
			summary = strings.TrimSpace(summary + fmt.Sprintf(" | appearances=%d last_seen=%d", character.Appearances, character.LastSeen))
		}
		id, kind, path := "knowledge.characters", "characters", "characters.json"
		if character.Origin == "cast" {
			id, kind, path = "knowledge.cast", "cast", "meta/cast_ledger.json"
		}
		sections["characters"] = append(sections["characters"], KnowledgeItemDTO{
			Kind: "character", Key: character.Name, Title: character.Name,
			Summary: compactKnowledgeText(summary, 450), Chapter: character.LastSeen,
			Provenance: []ProvenanceDTO{artifactProvenance(id, kind, path, character.LastSeen, 0)},
		})
	}
	for _, rule := range data.World.Rules {
		summary := rule.Rule
		if strings.TrimSpace(rule.Boundary) != "" {
			summary = strings.TrimSpace(summary + " | boundary: " + rule.Boundary)
		}
		sections["world"] = append(sections["world"], KnowledgeItemDTO{
			Kind: "world_rule", Key: rule.Category, Title: rule.Category,
			Summary: compactKnowledgeText(summary, 600),
			Provenance: []ProvenanceDTO{artifactProvenance("knowledge.world-rules", "world_rules", "world_rules.json", 0, 0)},
		})
	}

	cutoff, bounded := contextHistoryCutoff(data, chapter, scope)
	sections["continuity"] = append(sections["continuity"], recentContextSummaries(data, cutoff, bounded, 12)...)
	for _, change := range latestStateAt(data.World.StateChanges, cutoff, bounded) {
		sections["continuity"] = append(sections["continuity"], KnowledgeItemDTO{
			Kind: "state", Key: change.Entity + ":" + change.Field, Title: change.Entity,
			Summary: compactKnowledgeText(change.Field+" = "+change.NewValue, 400), Chapter: change.Chapter,
			Provenance: []ProvenanceDTO{artifactProvenance("knowledge.state-changes-log", "state_changes", "meta/state_changes.jsonl", change.Chapter, 0)},
		})
	}
	for _, relation := range relationshipsAt(data, cutoff, bounded) {
		prov := artifactProvenance("knowledge.relationships", "relationships", "relationship_state.json", relation.Chapter, 0)
		if bounded {
			if record := recordForChapter(data.Records, relation.Chapter); record != nil {
				prov = chapterRecordProvenance(*record)
			}
		}
		sections["continuity"] = append(sections["continuity"], KnowledgeItemDTO{
			Kind: "relationship", Key: relationPairKey(relation), Title: relation.CharacterA + " ↔ " + relation.CharacterB,
			Summary: compactKnowledgeText(relation.Relation, 400), Chapter: relation.Chapter, Provenance: []ProvenanceDTO{prov},
		})
	}
	for _, entry := range foreshadowAt(data, cutoff, bounded) {
		sections["continuity"] = append(sections["continuity"], KnowledgeItemDTO{
			Kind: "foreshadow", Key: entry.ID, Title: entry.ID,
			Summary: compactKnowledgeText(entry.Description+" | status="+entry.Status, 450), Chapter: entry.PlantedAt,
			Provenance: []ProvenanceDTO{artifactProvenance("knowledge.foreshadow", "foreshadow", "foreshadow_ledger.json", entry.PlantedAt, 0)},
		})
	}
	for _, event := range recentTimelineAt(data.Timeline, cutoff, bounded, 12) {
		sections["continuity"] = append(sections["continuity"], KnowledgeItemDTO{
			Kind: "timeline", Key: fmt.Sprintf("chapter:%d:%s", event.Chapter, event.Time), Title: event.Time,
			Summary: compactKnowledgeText(event.Event, 500), Chapter: event.Chapter,
			Provenance: []ProvenanceDTO{artifactProvenance("knowledge.timeline-log", "timeline", "timeline.jsonl", event.Chapter, 0)},
		})
	}
	return sections
}

func recordsAt(records []domain.ChapterRecord, chapter int, bounded bool) []domain.ChapterRecord {
	out := make([]domain.ChapterRecord, 0, len(records))
	for _, record := range records {
		if bounded && record.Chapter > chapter {
			continue
		}
		out = append(out, record)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Chapter < out[j].Chapter })
	return out
}

func recordForChapter(records []domain.ChapterRecord, chapter int) *domain.ChapterRecord {
	for i := range records {
		if records[i].Chapter == chapter {
			return &records[i]
		}
	}
	return nil
}

func timelineAt(events []domain.TimelineEvent, chapter int, bounded bool) []domain.TimelineEvent {
	out := make([]domain.TimelineEvent, 0, len(events))
	for _, event := range events {
		if bounded && event.Chapter > chapter {
			continue
		}
		out = append(out, event)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Chapter != out[j].Chapter {
			return out[i].Chapter < out[j].Chapter
		}
		if out[i].Time != out[j].Time {
			return out[i].Time < out[j].Time
		}
		return out[i].Event < out[j].Event
	})
	return out
}

func latestStateAt(changes []domain.StateChange, chapter int, bounded bool) []domain.StateChange {
	latest := make(map[string]domain.StateChange)
	for _, change := range changes {
		if bounded && change.Chapter > chapter {
			continue
		}
		key := knowledgeNameKey(change.Entity) + "\x00" + strings.TrimSpace(change.Field)
		prev, ok := latest[key]
		if !ok || change.Chapter >= prev.Chapter {
			latest[key] = change
		}
	}
	out := make([]domain.StateChange, 0, len(latest))
	for _, change := range latest {
		out = append(out, change)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if knowledgeNameKey(out[i].Entity) != knowledgeNameKey(out[j].Entity) {
			return knowledgeNameKey(out[i].Entity) < knowledgeNameKey(out[j].Entity)
		}
		if out[i].Field != out[j].Field {
			return out[i].Field < out[j].Field
		}
		return out[i].Chapter < out[j].Chapter
	})
	return out
}

func relationshipsAt(data host.DesktopKnowledgeReadSnapshot, chapter int, bounded bool) []domain.RelationshipEntry {
	if !bounded {
		out := append([]domain.RelationshipEntry(nil), data.World.Relationships...)
		sort.SliceStable(out, func(i, j int) bool { return relationPairKey(out[i]) < relationPairKey(out[j]) })
		return out
	}
	latest := make(map[string]domain.RelationshipEntry)
	for _, record := range recordsAt(data.Records, chapter, true) {
		for _, relation := range record.Facts.RelationshipChanges {
			if relation.Chapter == 0 {
				relation.Chapter = record.Chapter
			}
			if relation.Chapter > chapter {
				continue
			}
			latest[relationPairKey(relation)] = relation
		}
	}
	out := make([]domain.RelationshipEntry, 0, len(latest))
	for _, relation := range latest {
		out = append(out, relation)
	}
	sort.SliceStable(out, func(i, j int) bool { return relationPairKey(out[i]) < relationPairKey(out[j]) })
	return out
}

func foreshadowAt(data host.DesktopKnowledgeReadSnapshot, chapter int, bounded bool) []domain.ForeshadowEntry {
	if !bounded {
		out := append([]domain.ForeshadowEntry(nil), data.World.Foreshadow...)
		sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
		return out
	}
	entries := make(map[string]domain.ForeshadowEntry)
	for _, record := range recordsAt(data.Records, chapter, true) {
		for _, update := range record.Facts.ForeshadowUpdates {
			entry := entries[update.ID]
			entry.ID = update.ID
			switch update.Action {
			case "plant":
				if entry.PlantedAt == 0 {
					entry.PlantedAt = record.Chapter
				}
				if strings.TrimSpace(entry.Description) == "" {
					entry.Description = update.Description
				}
				entry.Status = "planted"
			case "advance":
				if entry.ID != "" {
					entry.Status = "advanced"
				}
			case "resolve":
				if entry.ID != "" {
					entry.Status = "resolved"
					entry.ResolvedAt = record.Chapter
				}
			}
			entries[update.ID] = entry
		}
	}
	out := make([]domain.ForeshadowEntry, 0, len(entries))
	for _, entry := range entries {
		if strings.TrimSpace(entry.ID) != "" {
			out = append(out, entry)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func recentContextSummaries(data host.DesktopKnowledgeReadSnapshot, chapter int, bounded bool, limit int) []KnowledgeItemDTO {
	items := make([]KnowledgeItemDTO, 0, limit)
	seen := make(map[int]struct{})
	for i := len(data.Summaries) - 1; i >= 0 && len(items) < limit; i-- {
		summary := data.Summaries[i]
		if bounded && summary.Chapter > chapter {
			continue
		}
		if strings.TrimSpace(summary.Summary) == "" {
			continue
		}
		seen[summary.Chapter] = struct{}{}
		items = append(items, KnowledgeItemDTO{
			Kind: "chapter_summary", Key: fmt.Sprintf("chapter:%d", summary.Chapter), Title: summary.Title,
			Summary: compactKnowledgeText(summary.Summary, 600), Chapter: summary.Chapter,
			Provenance: []ProvenanceDTO{artifactProvenance(fmt.Sprintf("summary.chapter:%d", summary.Chapter), "summary", fmt.Sprintf("summaries/%02d.json", summary.Chapter), summary.Chapter, 0)},
		})
	}
	for i := len(data.Records) - 1; i >= 0 && len(items) < limit; i-- {
		record := data.Records[i]
		if bounded && record.Chapter > chapter {
			continue
		}
		if _, ok := seen[record.Chapter]; ok || strings.TrimSpace(record.Facts.Summary) == "" {
			continue
		}
		items = append(items, KnowledgeItemDTO{
			Kind: "chapter_summary", Key: fmt.Sprintf("chapter:%d", record.Chapter), Title: record.Facts.Title,
			Summary: compactKnowledgeText(record.Facts.Summary, 600), Chapter: record.Chapter,
			Provenance: []ProvenanceDTO{chapterRecordProvenance(record)},
		})
	}
	return items
}

func recentTimelineAt(events []domain.TimelineEvent, chapter int, bounded bool, limit int) []domain.TimelineEvent {
	filtered := timelineAt(events, chapter, bounded)
	if len(filtered) > limit {
		filtered = filtered[len(filtered)-limit:]
	}
	for left, right := 0, len(filtered)-1; left < right; left, right = left+1, right-1 {
		filtered[left], filtered[right] = filtered[right], filtered[left]
	}
	return filtered
}

func contextHistoryCutoff(data host.DesktopKnowledgeReadSnapshot, chapter int, scope string) (int, bool) {
	if chapter > 0 {
		if scope == knowledgeScopeWriter {
			return chapter - 1, true
		}
		return chapter, true
	}
	if data.Project.Progress != nil && len(data.Project.Progress.CompletedChapters) > 0 {
		return data.Project.Progress.LatestCompleted(), true
	}
	return 0, false
}

func contextSectionOrder(scope string) []string {
	switch scope {
	case knowledgeScopeEditor:
		return []string{"continuity", "planning", "world", "characters", "project"}
	case knowledgeScopeOverview:
		return []string{"project", "planning", "world", "characters", "continuity"}
	default:
		return []string{"planning", "continuity", "characters", "world", "project"}
	}
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
		selected[strings.TrimSpace(section)] = true
	}
	return selected
}

func normalizedContextScope(scope string) string {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		return knowledgeScopeWriter
	}
	return scope
}

func normalizedCanonScope(scope string) string {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		return knowledgeScopeAll
	}
	return scope
}

func normalizedCharacterScope(scope string) string {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		return knowledgeScopeAll
	}
	return scope
}

func artifactProvenance(id, kind, path string, chapter, revision int) ProvenanceDTO {
	return ProvenanceDTO{ArtifactID: id, Kind: kind, Path: path, Chapter: chapter, Revision: revision}
}

func chapterRecordProvenance(record domain.ChapterRecord) ProvenanceDTO {
	return artifactProvenance(
		fmt.Sprintf("chapter.record:%d", record.Chapter),
		"chapter_record",
		fmt.Sprintf("meta/chapter_records/%06d.json", record.Chapter),
		record.Chapter,
		record.Revision,
	)
}

func relationPairKey(entry domain.RelationshipEntry) string {
	left, right := knowledgeNameKey(entry.CharacterA), knowledgeNameKey(entry.CharacterB)
	if left > right {
		left, right = right, left
	}
	return left + "\x00" + right
}

func knowledgeNameKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func compactKnowledgeText(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	if maxRunes <= 0 || utf8.RuneCountInString(value) <= maxRunes {
		return value
	}
	runes := []rune(value)
	return strings.TrimSpace(string(runes[:maxRunes])) + "…"
}
