package appruntime

import (
	"fmt"
	"sort"
	"strings"

	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/host"
)

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
				Kind:       "character",
				Subject:    character.Name,
				Field:      "profile",
				Value:      value,
				Provenance: []ProvenanceDTO{artifactProvenance("knowledge.characters", "characters", "characters.json", 0, 0)},
			})
		}
		for _, cast := range projectCharacterKnowledgeAt(data.Characters, "cast", chapter, bounded) {
			facts = append(facts, CanonFactDTO{
				Kind:    "cast",
				Subject: cast.Name,
				Field:   "role",
				Value:   firstNonEmpty(cast.Role, "supporting cast"),
				Chapter: cast.FirstSeen,
				Provenance: []ProvenanceDTO{artifactProvenance(
					"knowledge.cast", "cast", "meta/cast_ledger.json", cast.FirstSeen, 0,
				)},
			})
		}
	}

	if include("world_rule") || include("world_boundary") {
		for _, rule := range data.World.Rules {
			if include("world_rule") && strings.TrimSpace(rule.Rule) != "" {
				facts = append(facts, CanonFactDTO{
					Kind:       "world_rule",
					Subject:    rule.Category,
					Field:      "rule",
					Value:      rule.Rule,
					Provenance: []ProvenanceDTO{artifactProvenance("knowledge.world-rules", "world_rules", "world_rules.json", 0, 0)},
				})
			}
			if include("world_boundary") && strings.TrimSpace(rule.Boundary) != "" {
				facts = append(facts, CanonFactDTO{
					Kind:       "world_boundary",
					Subject:    rule.Category,
					Field:      "boundary",
					Value:      rule.Boundary,
					Provenance: []ProvenanceDTO{artifactProvenance("knowledge.world-rules", "world_rules", "world_rules.json", 0, 0)},
				})
			}
		}
	}

	if include("chapter_summary") || include("chapter_event") {
		for _, record := range recordsAt(data.Records, chapter, bounded) {
			prov := chapterRecordProvenance(record)
			if include("chapter_summary") && strings.TrimSpace(record.Facts.Summary) != "" {
				facts = append(facts, CanonFactDTO{
					Kind:       "chapter_summary",
					Subject:    fmt.Sprintf("chapter:%d", record.Chapter),
					Field:      "summary",
					Value:      record.Facts.Summary,
					Chapter:    record.Chapter,
					Provenance: []ProvenanceDTO{prov},
				})
			}
			if include("chapter_event") {
				for _, event := range record.Facts.KeyEvents {
					if strings.TrimSpace(event) == "" {
						continue
					}
					facts = append(facts, CanonFactDTO{
						Kind:       "chapter_event",
						Subject:    fmt.Sprintf("chapter:%d", record.Chapter),
						Field:      "event",
						Value:      event,
						Chapter:    record.Chapter,
						Provenance: []ProvenanceDTO{prov},
					})
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
				Kind:       "timeline",
				Subject:    strings.Join(event.Characters, ", "),
				Field:      event.Time,
				Value:      event.Event,
				Chapter:    event.Chapter,
				Provenance: []ProvenanceDTO{artifactProvenance("knowledge.timeline-log", "timeline", "timeline.jsonl", event.Chapter, 0)},
			})
		}
	}

	if include("state") {
		for _, change := range latestStateAt(data.World.StateChanges, chapter, bounded) {
			facts = append(facts, CanonFactDTO{
				Kind:       "state",
				Subject:    change.Entity,
				Field:      change.Field,
				Value:      change.NewValue,
				Chapter:    change.Chapter,
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
				Kind:       "relationship",
				Subject:    relation.CharacterA + " ↔ " + relation.CharacterB,
				Field:      "relation",
				Value:      relation.Relation,
				Chapter:    relation.Chapter,
				Provenance: []ProvenanceDTO{prov},
			})
		}
	}

	if include("foreshadow") {
		for _, entry := range foreshadowAt(data, chapter, bounded) {
			facts = append(facts, CanonFactDTO{
				Kind:       "foreshadow",
				Subject:    entry.ID,
				Field:      entry.Status,
				Value:      entry.Description,
				Chapter:    entry.PlantedAt,
				Provenance: []ProvenanceDTO{artifactProvenance("knowledge.foreshadow", "foreshadow", "foreshadow_ledger.json", entry.PlantedAt, 0)},
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
		previous, ok := latest[key]
		if !ok || change.Chapter >= previous.Chapter {
			latest[key] = change
		}
	}
	out := make([]domain.StateChange, 0, len(latest))
	for _, change := range latest {
		out = append(out, change)
	}
	sort.SliceStable(out, func(i, j int) bool {
		left, right := knowledgeNameKey(out[i].Entity), knowledgeNameKey(out[j].Entity)
		if left != right {
			return left < right
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
		for _, source := range record.Facts.RelationshipChanges {
			relation := source
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
			id := strings.TrimSpace(update.ID)
			if id == "" {
				continue
			}
			entry, exists := entries[id]
			switch update.Action {
			case "plant":
				if !exists {
					entry = domain.ForeshadowEntry{ID: id, PlantedAt: record.Chapter}
				}
				if entry.Description == "" {
					entry.Description = update.Description
				}
				entry.Status = "planted"
				entries[id] = entry
			case "advance":
				if exists {
					entry.Status = "advanced"
					entries[id] = entry
				}
			case "resolve":
				if exists {
					entry.Status = "resolved"
					entry.ResolvedAt = record.Chapter
					entries[id] = entry
				}
			}
		}
	}
	out := make([]domain.ForeshadowEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func artifactProvenance(id, kind, path string, chapter, revision int) ProvenanceDTO {
	return ProvenanceDTO{
		ArtifactID: id,
		Kind:       kind,
		Path:       path,
		Chapter:    chapter,
		Revision:   revision,
	}
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
