package appruntime

import (
	"encoding/json"
	"strings"

	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/host"
)

const defaultProjectQueryPageSize = 100

type chapterLocation struct {
	Volume int
	Arc    int
}

func (r *Runtime) queryProjectOverview() (json.RawMessage, error) {
	project, err := r.core.DesktopProjectRead()
	if err != nil {
		return nil, err
	}
	result := projectOverviewDTO(project)
	return marshalQueryData(result)
}

func (r *Runtime) queryChaptersList(req ChaptersListQuery) (json.RawMessage, error) {
	project, err := r.core.DesktopProjectRead()
	if err != nil {
		return nil, err
	}

	outline := effectiveProjectOutline(project)
	locations := projectChapterLocations(project.Volumes)
	total := projectChapterCatalogSize(project, outline)
	limit := effectiveProjectQueryLimit(req.Limit)
	start, end := pageBounds(req.Offset, limit, total)

	items := make([]ChapterListItemDTO, 0, end-start)
	outlineByChapter := make(map[int]domain.OutlineEntry, len(outline))
	for _, entry := range outline {
		outlineByChapter[entry.Chapter] = entry
	}

	for chapter := start + 1; chapter <= end; chapter++ {
		artifacts, readErr := r.core.DesktopChapterRead(chapter)
		if readErr != nil {
			return nil, readErr
		}
		item := chapterListItemDTO(artifacts, outlineByChapter[chapter])
		if location, ok := locations[chapter]; ok {
			item.Volume = location.Volume
			item.Arc = location.Arc
		}
		if item.WordCount == 0 && project.Progress != nil && project.Progress.ChapterWordCounts != nil {
			item.WordCount = project.Progress.ChapterWordCounts[chapter]
		}
		items = append(items, item)
	}

	return marshalQueryData(ChaptersListResultDTO{
		Items:  items,
		Offset: req.Offset,
		Limit:  limit,
		Total:  total,
	})
}

func (r *Runtime) queryChaptersGet(req ChaptersGetQuery) (json.RawMessage, error) {
	project, err := r.core.DesktopProjectRead()
	if err != nil {
		return nil, err
	}
	artifacts, err := r.core.DesktopChapterRead(req.Chapter)
	if err != nil {
		return nil, err
	}

	var outlineEntry domain.OutlineEntry
	for _, entry := range effectiveProjectOutline(project) {
		if entry.Chapter == req.Chapter {
			outlineEntry = entry
			break
		}
	}
	result := chapterGetDTO(artifacts, outlineEntry, req.IncludeContent)
	return marshalQueryData(result)
}

func (r *Runtime) queryOutlineGet(req OutlineGetQuery) (json.RawMessage, error) {
	project, err := r.core.DesktopProjectRead()
	if err != nil {
		return nil, err
	}

	outline := effectiveProjectOutline(project)
	limit := effectiveProjectQueryLimit(req.Limit)
	fromChapter := req.FromChapter
	if fromChapter == 0 {
		fromChapter = 1
	}
	selected := selectOutlineWindow(outline, fromChapter, limit)
	selectedSet := make(map[int]bool, len(selected))
	chapters := make([]OutlineChapterDTO, 0, len(selected))
	for _, entry := range selected {
		selectedSet[entry.Chapter] = true
		chapters = append(chapters, outlineChapterDTO(entry))
	}

	result := OutlineGetResultDTO{
		Layered:  len(project.Volumes) > 0,
		Chapters: chapters,
	}
	if len(project.Volumes) > 0 {
		result.Volumes = layeredOutlineDTO(project.Volumes, selectedSet)
	}
	if project.Compass != nil {
		result.Compass = &StoryCompassViewDTO{
			EndingDirection: project.Compass.EndingDirection,
			OpenThreads:     cloneStrings(project.Compass.OpenThreads),
			EstimatedScale:  project.Compass.EstimatedScale,
			LastUpdated:     project.Compass.LastUpdated,
		}
	}
	return marshalQueryData(result)
}

func projectOverviewDTO(project host.DesktopProjectReadSnapshot) ProjectOverviewResultDTO {
	result := ProjectOverviewResultDTO{
		FormatVersion: project.FormatVersion,
		Premise:       strings.TrimSpace(project.Premise),
		Layered:       len(project.Volumes) > 0,
	}
	if project.Book != nil {
		result.Title = project.Book.Title
		result.Synopsis = project.Book.Synopsis
	}
	if project.Progress != nil {
		result.Phase = string(project.Progress.Phase)
		result.Flow = string(project.Progress.Flow)
		result.Layered = project.Progress.Layered || result.Layered
		result.CurrentChapter = project.Progress.CurrentChapter
		result.CompletedChapters = len(project.Progress.CompletedChapters)
		result.TotalWordCount = project.Progress.TotalWordCount
		result.CurrentVolume = project.Progress.CurrentVolume
		result.CurrentArc = project.Progress.CurrentArc
	}
	return result
}

func effectiveProjectOutline(project host.DesktopProjectReadSnapshot) []domain.OutlineEntry {
	if len(project.Volumes) > 0 {
		// In layered mode layered_outline.json is the canonical structure and the
		// flat outline is only a derived compatibility view.
		return domain.FlattenOutline(project.Volumes)
	}
	return project.Outline
}

func projectChapterCatalogSize(project host.DesktopProjectReadSnapshot, outline []domain.OutlineEntry) int {
	maxChapter := 0
	for _, entry := range outline {
		if entry.Chapter > maxChapter {
			maxChapter = entry.Chapter
		}
	}
	if project.Progress == nil {
		return maxChapter
	}
	for _, chapter := range project.Progress.CompletedChapters {
		if chapter > maxChapter {
			maxChapter = chapter
		}
	}
	if project.Progress.InProgressChapter > maxChapter {
		maxChapter = project.Progress.InProgressChapter
	}
	// TotalChapters is a detailed chapter count only in flat mode. In layered
	// mode it includes estimated skeleton capacity and must not become a desktop
	// "total chapters" fact.
	if !project.Progress.Layered && len(project.Volumes) == 0 && project.Progress.TotalChapters > maxChapter {
		maxChapter = project.Progress.TotalChapters
	}
	return maxChapter
}

func projectChapterLocations(volumes []domain.VolumeOutline) map[int]chapterLocation {
	locations := make(map[int]chapterLocation)
	chapter := 1
	for _, volume := range volumes {
		for _, arc := range volume.Arcs {
			for range arc.Chapters {
				locations[chapter] = chapterLocation{Volume: volume.Index, Arc: arc.Index}
				chapter++
			}
		}
	}
	return locations
}

func chapterListItemDTO(artifacts host.DesktopChapterReadSnapshot, outline domain.OutlineEntry) ChapterListItemDTO {
	item := ChapterListItemDTO{
		Chapter:  artifacts.Chapter,
		Title:    outline.Title,
		HasPlan:  artifacts.Plan != nil,
		HasDraft: artifacts.Draft != "",
		HasFinal: artifacts.Final != "",
	}
	if artifacts.Plan != nil && strings.TrimSpace(artifacts.Plan.Title) != "" {
		item.Title = artifacts.Plan.Title
	}
	if artifacts.Record != nil {
		item.Origin = string(artifacts.Record.Origin)
		item.Revision = artifacts.Record.Revision
		if strings.TrimSpace(item.Title) == "" {
			item.Title = artifacts.Record.Facts.Title
		}
	}
	item.Status = chapterArtifactStatus(artifacts)
	item.WordCount = chapterArtifactWordCount(artifacts)
	return item
}

func chapterGetDTO(artifacts host.DesktopChapterReadSnapshot, outline domain.OutlineEntry, includeContent bool) ChaptersGetResultDTO {
	result := ChaptersGetResultDTO{
		Chapter: artifacts.Chapter,
		Title:   outline.Title,
		Status:  chapterArtifactStatus(artifacts),
		Draft: &ChapterTextViewDTO{
			Present:   artifacts.Draft != "",
			WordCount: domain.WordCount(artifacts.Draft),
		},
		Final: &ChapterTextViewDTO{
			Present:   artifacts.Final != "",
			WordCount: domain.WordCount(artifacts.Final),
		},
	}
	if includeContent {
		result.Draft.Content = artifacts.Draft
		result.Final.Content = artifacts.Final
	}
	if artifacts.Plan != nil {
		result.Title = artifacts.Plan.Title
		result.Plan = &ChapterPlanViewDTO{
			Chapter:    artifacts.Plan.Chapter,
			Title:      artifacts.Plan.Title,
			Goal:       artifacts.Plan.Goal,
			Conflict:   artifacts.Plan.Conflict,
			Hook:       artifacts.Plan.Hook,
			EmotionArc: artifacts.Plan.EmotionArc,
			Notes:      artifacts.Plan.Notes,
			Required:   cloneStrings(artifacts.Plan.Contract.RequiredBeats),
			Forbidden:  cloneStrings(artifacts.Plan.Contract.ForbiddenMoves),
			Continuity: cloneStrings(artifacts.Plan.Contract.ContinuityChecks),
		}
	}
	if artifacts.Record != nil {
		if strings.TrimSpace(result.Title) == "" {
			result.Title = artifacts.Record.Facts.Title
		}
		result.Record = &ChapterRecordViewDTO{
			Revision:      artifacts.Record.Revision,
			Origin:        string(artifacts.Record.Origin),
			ContentSHA256: artifacts.Record.ContentSHA256,
			AcceptedAt:    artifacts.Record.AcceptedAt,
		}
	}
	return result
}

func chapterArtifactStatus(artifacts host.DesktopChapterReadSnapshot) string {
	switch {
	case artifacts.Record != nil:
		return "accepted"
	case artifacts.Final != "":
		return "final"
	case artifacts.Draft != "":
		return "draft"
	case artifacts.Plan != nil:
		return "planned"
	default:
		return "not_started"
	}
}

func chapterArtifactWordCount(artifacts host.DesktopChapterReadSnapshot) int {
	switch {
	case artifacts.Final != "":
		return domain.WordCount(artifacts.Final)
	case artifacts.Draft != "":
		return domain.WordCount(artifacts.Draft)
	case artifacts.Record != nil:
		return domain.WordCount(artifacts.Record.Content)
	default:
		return 0
	}
}

func effectiveProjectQueryLimit(limit int) int {
	if limit == 0 {
		return defaultProjectQueryPageSize
	}
	return limit
}

func pageBounds(offset, limit, total int) (int, int) {
	if offset >= total {
		return total, total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return offset, end
}

func selectOutlineWindow(outline []domain.OutlineEntry, fromChapter, limit int) []domain.OutlineEntry {
	selected := make([]domain.OutlineEntry, 0, limit)
	for _, entry := range outline {
		if entry.Chapter < fromChapter {
			continue
		}
		selected = append(selected, entry)
		if len(selected) == limit {
			break
		}
	}
	return selected
}

func outlineChapterDTO(entry domain.OutlineEntry) OutlineChapterDTO {
	return OutlineChapterDTO{
		Chapter:   entry.Chapter,
		Title:     entry.Title,
		CoreEvent: entry.CoreEvent,
		Hook:      entry.Hook,
		Scenes:    cloneStrings(entry.Scenes),
	}
}

func layeredOutlineDTO(volumes []domain.VolumeOutline, selected map[int]bool) []VolumeOutlineViewDTO {
	result := make([]VolumeOutlineViewDTO, 0, len(volumes))
	globalChapter := 1
	for _, volume := range volumes {
		volumeDTO := VolumeOutlineViewDTO{
			Index: volume.Index,
			Title: volume.Title,
			Theme: volume.Theme,
			Final: volume.Final,
			Arcs:  make([]ArcOutlineViewDTO, 0, len(volume.Arcs)),
		}
		for _, arc := range volume.Arcs {
			arcDTO := ArcOutlineViewDTO{
				Index:             arc.Index,
				Title:             arc.Title,
				Goal:              arc.Goal,
				EstimatedChapters: arc.EstimatedChapters,
			}
			for _, chapter := range arc.Chapters {
				chapter.Chapter = globalChapter
				if selected[globalChapter] {
					arcDTO.Chapters = append(arcDTO.Chapters, outlineChapterDTO(chapter))
				}
				globalChapter++
			}
			volumeDTO.Arcs = append(volumeDTO.Arcs, arcDTO)
		}
		result = append(result, volumeDTO)
	}
	return result
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return append([]string(nil), values...)
}

func marshalQueryData(value any) (json.RawMessage, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(data), nil
}
