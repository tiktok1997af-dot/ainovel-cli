package appruntime

import (
	"sort"
	"strings"

	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/host"
)

func projectCharacterKnowledge(data host.DesktopCharacterKnowledgeSnapshot, scope string) []CharacterViewDTO {
	return projectCharacterKnowledgeAt(data, scope, 0, false)
}

func projectCharacterKnowledgeAt(data host.DesktopCharacterKnowledgeSnapshot, scope string, chapter int, bounded bool) []CharacterViewDTO {
	coreNames := make(map[string]struct{}, len(data.Characters))
	items := make([]CharacterViewDTO, 0, len(data.Characters)+len(data.Cast))

	for _, character := range data.Characters {
		name := strings.TrimSpace(character.Name)
		if name == "" {
			continue
		}
		coreNames[knowledgeNameKey(name)] = struct{}{}
		if scope == "cast" {
			continue
		}
		items = append(items, CharacterViewDTO{
			Name:        name,
			Aliases:     cloneStrings(character.Aliases),
			Role:        character.Role,
			Description: character.Description,
			Arc:         character.Arc,
			Traits:      cloneStrings(character.Traits),
			Tier:        character.Tier,
			Origin:      "core",
		})
	}

	if scope != "core" {
		for _, entry := range castEntriesAt(data.Cast, chapter, bounded) {
			name := strings.TrimSpace(entry.Name)
			if name == "" || entry.Promoted {
				continue
			}
			if _, duplicate := coreNames[knowledgeNameKey(name)]; duplicate {
				continue
			}
			items = append(items, CharacterViewDTO{
				Name:        name,
				Aliases:     cloneStrings(entry.Aliases),
				Role:        entry.BriefRole,
				Origin:      "cast",
				FirstSeen:   entry.FirstSeenChapter,
				LastSeen:    entry.LastSeenChapter,
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

// castEntriesAt returns a detached cast view valid at a historical chapter.
// When AppearanceChapters are available they are the exact authority used to
// recompute first/last/count so later appearances cannot leak backwards. Legacy
// entries without the chapter list degrade conservatively to the known first
// appearance only when a bounded historical view is requested.
func castEntriesAt(entries []domain.CastEntry, chapter int, bounded bool) []domain.CastEntry {
	out := make([]domain.CastEntry, 0, len(entries))
	for _, source := range entries {
		if source.Promoted || strings.TrimSpace(source.Name) == "" {
			continue
		}
		entry := source
		entry.Aliases = cloneStrings(source.Aliases)
		entry.AppearanceChapters = append([]int(nil), source.AppearanceChapters...)
		if bounded {
			if chapter <= 0 {
				continue
			}
			if len(source.AppearanceChapters) > 0 {
				filtered := make([]int, 0, len(source.AppearanceChapters))
				for _, appearance := range source.AppearanceChapters {
					if appearance > 0 && appearance <= chapter {
						filtered = append(filtered, appearance)
					}
				}
				if len(filtered) == 0 {
					continue
				}
				sort.Ints(filtered)
				entry.AppearanceChapters = filtered
				entry.FirstSeenChapter = filtered[0]
				entry.LastSeenChapter = filtered[len(filtered)-1]
				entry.AppearanceCount = len(filtered)
			} else {
				if source.FirstSeenChapter <= 0 || source.FirstSeenChapter > chapter {
					continue
				}
				entry.FirstSeenChapter = source.FirstSeenChapter
				entry.LastSeenChapter = source.FirstSeenChapter
				entry.AppearanceCount = 1
				entry.AppearanceChapters = nil
			}
		}
		out = append(out, entry)
	}
	sort.SliceStable(out, func(i, j int) bool {
		left, right := knowledgeNameKey(out[i].Name), knowledgeNameKey(out[j].Name)
		if left != right {
			return left < right
		}
		return out[i].FirstSeenChapter < out[j].FirstSeenChapter
	})
	return out
}
