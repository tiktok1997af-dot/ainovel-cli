package appruntime

import (
	"strings"
	"testing"

	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/host"
)

func TestCharacterKnowledgePreservesOriginAndCorePrecedence(t *testing.T) {
	data := host.DesktopCharacterKnowledgeSnapshot{
		Characters: []domain.Character{
			{Name: "Hero", Role: "lead", Tier: "core"},
			{Name: "Mentor", Role: "guide", Tier: "important"},
		},
		Cast: []domain.CastEntry{
			{Name: "Hero", BriefRole: "duplicate", FirstSeenChapter: 1},
			{Name: "Guard", BriefRole: "gate guard", FirstSeenChapter: 2, LastSeenChapter: 4, AppearanceCount: 2},
			{Name: "Promoted", BriefRole: "old cast", Promoted: true},
		},
	}

	all := projectCharacterKnowledge(data, "all")
	if len(all) != 3 {
		t.Fatalf("all characters = %+v", all)
	}
	seen := map[string]string{}
	for _, item := range all {
		seen[item.Name] = item.Origin
	}
	if seen["Hero"] != "core" || seen["Mentor"] != "core" || seen["Guard"] != "cast" {
		t.Fatalf("origin projection = %+v", seen)
	}
	if _, duplicate := seen["Promoted"]; duplicate {
		t.Fatal("promoted cast entry must not remain in the cast projection")
	}

	core := projectCharacterKnowledge(data, "core")
	if len(core) != 2 || core[0].Origin != "core" || core[1].Origin != "core" {
		t.Fatalf("core scope = %+v", core)
	}
	cast := projectCharacterKnowledge(data, "cast")
	if len(cast) != 1 || cast[0].Name != "Guard" || cast[0].Origin != "cast" {
		t.Fatalf("cast scope = %+v", cast)
	}
}

func TestHistoricalCastProjectionDoesNotLeakFutureAppearances(t *testing.T) {
	data := host.DesktopCharacterKnowledgeSnapshot{
		Cast: []domain.CastEntry{
			{
				Name:               "Guard",
				BriefRole:          "gate guard",
				FirstSeenChapter:   2,
				LastSeenChapter:    8,
				AppearanceCount:    3,
				AppearanceChapters: []int{2, 4, 8},
			},
			{
				Name:               "Messenger",
				BriefRole:          "late messenger",
				FirstSeenChapter:   7,
				LastSeenChapter:    7,
				AppearanceCount:    1,
				AppearanceChapters: []int{7},
			},
		},
	}

	items := projectCharacterKnowledgeAt(data, "cast", 4, true)
	if len(items) != 1 {
		t.Fatalf("historical cast = %+v", items)
	}
	guard := items[0]
	if guard.Name != "Guard" || guard.FirstSeen != 2 || guard.LastSeen != 4 || guard.Appearances != 2 {
		t.Fatalf("historical guard projection = %+v", guard)
	}
}

func TestCanonHistoricalAnchorDoesNotLeakFutureContinuityOrCast(t *testing.T) {
	data := host.DesktopKnowledgeReadSnapshot{
		Characters: host.DesktopCharacterKnowledgeSnapshot{
			Characters: []domain.Character{{Name: "Hero", Role: "lead"}},
			Cast: []domain.CastEntry{
				{Name: "Guard", BriefRole: "gate guard", FirstSeenChapter: 1, LastSeenChapter: 4, AppearanceCount: 2, AppearanceChapters: []int{1, 4}},
				{Name: "Oracle", BriefRole: "late oracle", FirstSeenChapter: 5, LastSeenChapter: 5, AppearanceCount: 1, AppearanceChapters: []int{5}},
			},
		},
		World: host.DesktopWorldKnowledgeSnapshot{
			StateChanges: []domain.StateChange{
				{Chapter: 1, Entity: "Hero", Field: "location", NewValue: "village"},
				{Chapter: 3, Entity: "Hero", Field: "location", NewValue: "capital"},
			},
			Relationships: []domain.RelationshipEntry{{CharacterA: "Hero", CharacterB: "Rival", Relation: "enemy", Chapter: 3}},
			Foreshadow:    []domain.ForeshadowEntry{{ID: "seal", Description: "sealed door", PlantedAt: 1, Status: "resolved", ResolvedAt: 3}},
		},
		Timeline: []domain.TimelineEvent{
			{Chapter: 1, Time: "morning", Event: "Hero reaches the village"},
			{Chapter: 3, Time: "night", Event: "Hero enters the capital"},
		},
		Records: []domain.ChapterRecord{
			{
				Chapter:  1,
				Revision: 2,
				Facts: domain.ChapterFacts{
					Summary:             "Village arrival",
					KeyEvents:           []string{"met Rival"},
					RelationshipChanges: []domain.RelationshipEntry{{CharacterA: "Hero", CharacterB: "Rival", Relation: "ally", Chapter: 1}},
					ForeshadowUpdates:   []domain.ForeshadowUpdate{{ID: "seal", Action: "plant", Description: "sealed door"}},
				},
			},
			{
				Chapter:  3,
				Revision: 1,
				Facts: domain.ChapterFacts{
					Summary:             "Capital conflict",
					RelationshipChanges: []domain.RelationshipEntry{{CharacterA: "Hero", CharacterB: "Rival", Relation: "enemy", Chapter: 3}},
					ForeshadowUpdates:   []domain.ForeshadowUpdate{{ID: "seal", Action: "resolve"}},
				},
			},
		},
	}

	facts := projectCanonFacts(data, knowledgeScopeAll, 2)
	if len(facts) == 0 {
		t.Fatal("expected canon facts")
	}
	var state, relation, foreshadow string
	seenGuard := false
	for _, fact := range facts {
		if fact.Chapter > 2 {
			t.Fatalf("future fact leaked across chapter anchor: %+v", fact)
		}
		if len(fact.Provenance) == 0 || fact.Provenance[0].ArtifactID == "" {
			t.Fatalf("fact lacks provenance: %+v", fact)
		}
		switch fact.Kind {
		case "state":
			state = fact.Value
		case "relationship":
			relation = fact.Value
		case "foreshadow":
			foreshadow = fact.Field
		case "timeline":
			if fact.Value == "Hero enters the capital" {
				t.Fatal("future timeline event leaked")
			}
		case "cast":
			if fact.Subject == "Oracle" {
				t.Fatal("future supporting cast leaked")
			}
			if fact.Subject == "Guard" {
				seenGuard = true
			}
		}
	}
	if state != "village" {
		t.Fatalf("state at chapter 2 = %q", state)
	}
	if relation != "ally" {
		t.Fatalf("relationship at chapter 2 = %q", relation)
	}
	if foreshadow != "planted" {
		t.Fatalf("foreshadow at chapter 2 = %q", foreshadow)
	}
	if !seenGuard {
		t.Fatal("historically valid cast entry missing")
	}
}

func TestContextWriterUsesHistoryBeforeTargetChapter(t *testing.T) {
	data := host.DesktopKnowledgeReadSnapshot{
		Project: host.DesktopProjectReadSnapshot{
			Book:    &domain.BookMetadata{Title: "Book", Synopsis: "Synopsis"},
			Premise: "Premise",
			Progress: &domain.Progress{
				Phase:             domain.PhaseWriting,
				CurrentChapter:    3,
				CompletedChapters: []int{1, 2},
			},
			Outline: []domain.OutlineEntry{{Chapter: 2, Title: "Target", CoreEvent: "Target event"}},
		},
		World: host.DesktopWorldKnowledgeSnapshot{
			StateChanges: []domain.StateChange{
				{Chapter: 1, Entity: "Hero", Field: "status", NewValue: "ready"},
				{Chapter: 2, Entity: "Hero", Field: "status", NewValue: "injured"},
			},
		},
		Timeline: []domain.TimelineEvent{
			{Chapter: 1, Event: "past"},
			{Chapter: 2, Event: "target"},
		},
		Summaries: []domain.ChapterSummary{
			{Chapter: 1, Title: "One", Summary: "past summary"},
			{Chapter: 2, Title: "Two", Summary: "target summary"},
		},
	}

	cutoff, bounded := contextHistoryCutoff(data, 2, knowledgeScopeWriter)
	if !bounded || cutoff != 1 {
		t.Fatalf("writer cutoff = %d bounded=%v", cutoff, bounded)
	}
	sections := projectContextSections(data, 2, knowledgeScopeWriter)
	if len(sections["planning"]) == 0 || sections["planning"][0].Chapter != 2 {
		t.Fatalf("target planning context missing = %+v", sections["planning"])
	}
	for _, item := range sections["continuity"] {
		if item.Chapter >= 2 {
			t.Fatalf("writer continuity leaked target/future chapter: %+v", item)
		}
	}
}

func TestContextHistoricalViewOmitsFutureCompassAndCast(t *testing.T) {
	data := host.DesktopKnowledgeReadSnapshot{
		Project: host.DesktopProjectReadSnapshot{
			Progress: &domain.Progress{CompletedChapters: []int{1, 2, 3, 4, 5}},
			Outline:  []domain.OutlineEntry{{Chapter: 3, Title: "Target", CoreEvent: "target"}},
			Compass: &domain.StoryCompass{
				EndingDirection: "future ending direction",
				OpenThreads:     []string{"future secret"},
				LastUpdated:     5,
			},
		},
		Characters: host.DesktopCharacterKnowledgeSnapshot{
			Characters: []domain.Character{{Name: "Hero", Role: "lead"}},
			Cast: []domain.CastEntry{
				{Name: "Guard", BriefRole: "early guard", AppearanceChapters: []int{1, 4}, FirstSeenChapter: 1, LastSeenChapter: 4, AppearanceCount: 2},
				{Name: "Oracle", BriefRole: "future oracle", AppearanceChapters: []int{5}, FirstSeenChapter: 5, LastSeenChapter: 5, AppearanceCount: 1},
			},
		},
	}

	sections := projectContextSections(data, 3, knowledgeScopeWriter)
	for _, item := range sections["planning"] {
		if item.Kind == "compass" || strings.Contains(item.Summary, "future") {
			t.Fatalf("future compass leaked into historical writer context: %+v", item)
		}
	}
	seenGuard := false
	for _, item := range sections["characters"] {
		if item.Key == "Oracle" {
			t.Fatalf("future cast leaked into historical writer context: %+v", item)
		}
		if item.Key == "Guard" {
			seenGuard = true
			if item.Chapter != 1 || !strings.Contains(item.Summary, "appearances=1") || !strings.Contains(item.Summary, "last_seen=1") {
				t.Fatalf("historical cast metadata leaked future appearances: %+v", item)
			}
		}
	}
	if !seenGuard {
		t.Fatal("historically valid cast entry missing from context")
	}
}

func TestContextEditorMayUseTargetChapterHistory(t *testing.T) {
	data := host.DesktopKnowledgeReadSnapshot{
		Project: host.DesktopProjectReadSnapshot{
			Outline: []domain.OutlineEntry{{Chapter: 2, Title: "Target", CoreEvent: "target"}},
		},
		Timeline: []domain.TimelineEvent{
			{Chapter: 1, Event: "past"},
			{Chapter: 2, Event: "target event"},
			{Chapter: 3, Event: "future event"},
		},
	}
	sections := projectContextSections(data, 2, knowledgeScopeEditor)
	seenTarget := false
	for _, item := range sections["continuity"] {
		if item.Chapter > 2 {
			t.Fatalf("editor context leaked future chapter: %+v", item)
		}
		if item.Chapter == 2 && strings.Contains(item.Summary, "target event") {
			seenTarget = true
		}
	}
	if !seenTarget {
		t.Fatal("editor context should include target chapter continuity")
	}
}

func TestLatestStateAtReturnsSemanticCurrentValue(t *testing.T) {
	changes := []domain.StateChange{
		{Chapter: 1, Entity: "Hero", Field: "realm", NewValue: "one"},
		{Chapter: 2, Entity: "Hero", Field: "status", NewValue: "hurt"},
		{Chapter: 3, Entity: "Hero", Field: "realm", NewValue: "two"},
	}
	atTwo := latestStateAt(changes, 2, true)
	if len(atTwo) != 2 {
		t.Fatalf("state facts at two = %+v", atTwo)
	}
	for _, change := range atTwo {
		if change.Field == "realm" && change.NewValue != "one" {
			t.Fatalf("future realm leaked: %+v", change)
		}
	}
	current := latestStateAt(changes, 0, false)
	for _, change := range current {
		if change.Field == "realm" && change.NewValue != "two" {
			t.Fatalf("current realm = %+v", change)
		}
	}
}

func TestWorldSectionSelectionDefaultsToAll(t *testing.T) {
	all := selectedWorldSections(nil)
	for _, section := range knowledgeWorldSectionOrder {
		if !all[section] {
			t.Fatalf("default world selection missing %q", section)
		}
	}
	only := selectedWorldSections([]string{"rules", "relationships"})
	if !only["rules"] || !only["relationships"] || only["foreshadow"] || only["state_changes"] {
		t.Fatalf("selected world sections = %+v", only)
	}
}
