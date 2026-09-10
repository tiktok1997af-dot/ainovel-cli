package appruntime

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/host"
)

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
			Kind:       "book",
			Key:        "book",
			Title:      project.Book.Title,
			Summary:    compactKnowledgeText(project.Book.Synopsis, 600),
			Provenance: []ProvenanceDTO{artifactProvenance("project.book", "book", "meta/book.json", 0, 0)},
		})
	}
	if strings.TrimSpace(project.Premise) != "" {
		sections["project"] = append(sections["project"], KnowledgeItemDTO{
			Kind:       "premise",
			Key:        "premise",
			Summary:    compactKnowledgeText(project.Premise, 600),
			Provenance: []ProvenanceDTO{artifactProvenance("project.premise", "premise", "premise.md", 0, 0)},
		})
	}
	if project.Progress != nil {
		sections["project"] = append(sections["project"], KnowledgeItemDTO{
			Kind: "progress",
			Key:  "progress",
			Summary: fmt.Sprintf(
				"phase=%s flow=%s current_chapter=%d completed=%d words=%d",
				project.Progress.Phase,
				project.Progress.Flow,
				project.Progress.CurrentChapter,
				len(project.Progress.CompletedChapters),
				project.Progress.TotalWordCount,
			),
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
			Kind:       "outline",
			Key:        fmt.Sprintf("chapter:%d", entry.Chapter),
			Title:      entry.Title,
			Summary:    compactKnowledgeText(summary, 600),
			Chapter:    entry.Chapter,
			Provenance: []ProvenanceDTO{artifactProvenance(artifactID, "outline", artifactPath, entry.Chapter, 0)},
		})
		break
	}

	cutoff, bounded := contextHistoryCutoff(data, chapter, scope)
	if project.Compass != nil && (!bounded || project.Compass.LastUpdated <= cutoff) {
		summary := project.Compass.EndingDirection
		if len(project.Compass.OpenThreads) > 0 {
			summary = strings.TrimSpace(summary + " | open: " + strings.Join(project.Compass.OpenThreads, "; "))
		}
		sections["planning"] = append(sections["planning"], KnowledgeItemDTO{
			Kind:       "compass",
			Key:        "compass",
			Summary:    compactKnowledgeText(summary, 600),
			Chapter:    project.Compass.LastUpdated,
			Provenance: []ProvenanceDTO{artifactProvenance("outline.compass", "compass", "meta/compass.json", project.Compass.LastUpdated, 0)},
		})
	}

	characterItems := projectCharacterKnowledgeAt(data.Characters, "all", cutoff, bounded)
	for _, character := range characterItems {
		summary := character.Role
		if character.Description != "" {
			summary = strings.TrimSpace(summary + " | " + character.Description)
		}
		if character.Origin == "cast" {
			summary = strings.TrimSpace(summary + fmt.Sprintf(" | appearances=%d last_seen=%d", character.Appearances, character.LastSeen))
		}
		id, kind, artifactPath := "knowledge.characters", "characters", "characters.json"
		if character.Origin == "cast" {
			id, kind, artifactPath = "knowledge.cast", "cast", "meta/cast_ledger.json"
		}
		sections["characters"] = append(sections["characters"], KnowledgeItemDTO{
			Kind:       "character",
			Key:        character.Name,
			Title:      character.Name,
			Summary:    compactKnowledgeText(summary, 450),
			Chapter:    character.LastSeen,
			Provenance: []ProvenanceDTO{artifactProvenance(id, kind, artifactPath, character.LastSeen, 0)},
		})
	}

	for _, rule := range data.World.Rules {
		summary := rule.Rule
		if strings.TrimSpace(rule.Boundary) != "" {
			summary = strings.TrimSpace(summary + " | boundary: " + rule.Boundary)
		}
		sections["world"] = append(sections["world"], KnowledgeItemDTO{
			Kind:       "world_rule",
			Key:        rule.Category,
			Title:      rule.Category,
			Summary:    compactKnowledgeText(summary, 600),
			Provenance: []ProvenanceDTO{artifactProvenance("knowledge.world-rules", "world_rules", "world_rules.json", 0, 0)},
		})
	}

	sections["continuity"] = append(sections["continuity"], recentContextSummaries(data, cutoff, bounded, 12)...)
	for _, change := range latestStateAt(data.World.StateChanges, cutoff, bounded) {
		sections["continuity"] = append(sections["continuity"], KnowledgeItemDTO{
			Kind:       "state",
			Key:        change.Entity + ":" + change.Field,
			Title:      change.Entity,
			Summary:    compactKnowledgeText(change.Field+" = "+change.NewValue, 400),
			Chapter:    change.Chapter,
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
			Kind:       "relationship",
			Key:        relationPairKey(relation),
			Title:      relation.CharacterA + " ↔ " + relation.CharacterB,
			Summary:    compactKnowledgeText(relation.Relation, 400),
			Chapter:    relation.Chapter,
			Provenance: []ProvenanceDTO{prov},
		})
	}
	for _, entry := range foreshadowAt(data, cutoff, bounded) {
		sections["continuity"] = append(sections["continuity"], KnowledgeItemDTO{
			Kind:       "foreshadow",
			Key:        entry.ID,
			Title:      entry.ID,
			Summary:    compactKnowledgeText(entry.Description+" | status="+entry.Status, 450),
			Chapter:    entry.PlantedAt,
			Provenance: []ProvenanceDTO{artifactProvenance("knowledge.foreshadow", "foreshadow", "foreshadow_ledger.json", entry.PlantedAt, 0)},
		})
	}
	for _, event := range recentTimelineAt(data.Timeline, cutoff, bounded, 12) {
		sections["continuity"] = append(sections["continuity"], KnowledgeItemDTO{
			Kind:       "timeline",
			Key:        fmt.Sprintf("chapter:%d:%s", event.Chapter, event.Time),
			Title:      event.Time,
			Summary:    compactKnowledgeText(event.Event, 500),
			Chapter:    event.Chapter,
			Provenance: []ProvenanceDTO{artifactProvenance("knowledge.timeline-log", "timeline", "timeline.jsonl", event.Chapter, 0)},
		})
	}
	return sections
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
			Kind:    "chapter_summary",
			Key:     fmt.Sprintf("chapter:%d", summary.Chapter),
			Title:   summary.Title,
			Summary: compactKnowledgeText(summary.Summary, 600),
			Chapter: summary.Chapter,
			Provenance: []ProvenanceDTO{artifactProvenance(
				fmt.Sprintf("summary.chapter:%d", summary.Chapter),
				"summary",
				fmt.Sprintf("summaries/%02d.json", summary.Chapter),
				summary.Chapter,
				0,
			)},
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
			Kind:       "chapter_summary",
			Key:        fmt.Sprintf("chapter:%d", record.Chapter),
			Title:      record.Facts.Title,
			Summary:    compactKnowledgeText(record.Facts.Summary, 600),
			Chapter:    record.Chapter,
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

func compactKnowledgeText(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	if maxRunes <= 0 || utf8.RuneCountInString(value) <= maxRunes {
		return value
	}
	runes := []rune(value)
	return strings.TrimSpace(string(runes[:maxRunes])) + "…"
}
