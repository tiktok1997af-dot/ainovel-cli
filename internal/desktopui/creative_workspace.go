package desktopui

import (
	"strings"

	"github.com/voocel/ainovel-cli/internal/appruntime"
)

type CreativeArtifact string

const (
	CreativeArtifactPlan      CreativeArtifact = "plan"
	CreativeArtifactDraft     CreativeArtifact = "draft"
	CreativeArtifactWorkspace CreativeArtifact = "workspace"
	CreativeArtifactAccepted  CreativeArtifact = "accepted"
)

type PlanEditorState struct {
	Value               appruntime.ChapterPlanSavePayload
	Dirty               bool
	Existing            bool
	CompleteReplacement bool
}

type TextEditorState struct {
	Content        string
	Dirty          bool
	BaselineSHA256 string
}

type AcceptedRecordState struct {
	Present bool
	Record  *appruntime.ChapterRecordViewDTO
}

type CreativeWorkspaceState struct {
	Load      LoadState
	Chapter   int
	Artifact  CreativeArtifact
	Canonical *appruntime.ChaptersGetResultDTO
	Plan      PlanEditorState
	Draft     TextEditorState
	Workspace TextEditorState
	Accepted  AcceptedRecordState
	Error     *ErrorView
}

func NewCreativeWorkspaceState() CreativeWorkspaceState {
	return CreativeWorkspaceState{Load: LoadInitial, Artifact: CreativeArtifactDraft}
}

func (s *CreativeWorkspaceState) SelectArtifact(artifact CreativeArtifact) bool {
	switch artifact {
	case CreativeArtifactPlan, CreativeArtifactDraft, CreativeArtifactWorkspace, CreativeArtifactAccepted:
		s.Artifact = artifact
		return true
	default:
		return false
	}
}

func (s *CreativeWorkspaceState) EditPlan(value appruntime.ChapterPlanSavePayload, completeReplacement bool) bool {
	if s.Chapter <= 0 || value.Chapter != s.Chapter {
		return false
	}
	s.Plan.Value = value
	s.Plan.Dirty = true
	s.Plan.CompleteReplacement = completeReplacement
	return true
}

func (s *CreativeWorkspaceState) EditDraft(content string) bool {
	if s.Chapter <= 0 {
		return false
	}
	s.Draft.Content = content
	s.Draft.Dirty = true
	return true
}

func (s *CreativeWorkspaceState) EditWorkspace(content string) bool {
	if s.Chapter <= 0 {
		return false
	}
	s.Workspace.Content = content
	s.Workspace.Dirty = true
	return true
}

func (s *CreativeWorkspaceState) applyCanonical(dto appruntime.ChaptersGetResultDTO, preserveDirty bool) {
	s.Chapter = dto.Chapter
	s.Canonical = &dto
	s.Error = nil
	s.Accepted = AcceptedRecordState{Present: dto.Record != nil, Record: cloneRecord(dto.Record)}

	if !preserveDirty || !s.Plan.Dirty {
		s.Plan = PlanEditorState{Value: planPayloadFromView(dto.Chapter, dto.Plan), Existing: dto.Plan != nil}
		// G03 chapters.get deliberately omits several ChapterContract fields.
		// An existing plan therefore cannot be treated as a complete replacement
		// unless the renderer supplies the complete typed payload explicitly.
		s.Plan.CompleteReplacement = dto.Plan == nil
	}
	if !preserveDirty || !s.Draft.Dirty {
		s.Draft.Content = ""
		if dto.Draft != nil {
			s.Draft.Content = dto.Draft.Content
		}
		s.Draft.Dirty = false
	}
	if !preserveDirty || !s.Workspace.Dirty {
		s.Workspace.Content = ""
		if dto.Final != nil {
			s.Workspace.Content = dto.Final.Content
		}
		s.Workspace.Dirty = false
	}
	s.Load = creativeLoadState(dto)
}

func planPayloadFromView(chapter int, plan *appruntime.ChapterPlanViewDTO) appruntime.ChapterPlanSavePayload {
	value := appruntime.ChapterPlanSavePayload{Chapter: chapter}
	if plan == nil {
		return value
	}
	value.Title = plan.Title
	value.Goal = plan.Goal
	value.Conflict = plan.Conflict
	value.Hook = plan.Hook
	value.EmotionArc = plan.EmotionArc
	value.Notes = plan.Notes
	value.Contract.RequiredBeats = append([]string(nil), plan.Required...)
	value.Contract.ForbiddenMoves = append([]string(nil), plan.Forbidden...)
	value.Contract.ContinuityChecks = append([]string(nil), plan.Continuity...)
	return value
}

func creativeLoadState(dto appruntime.ChaptersGetResultDTO) LoadState {
	if dto.Plan == nil && (dto.Draft == nil || !dto.Draft.Present) &&
		(dto.Final == nil || !dto.Final.Present) && dto.Record == nil {
		return LoadEmpty
	}
	return LoadReady
}

func cloneRecord(record *appruntime.ChapterRecordViewDTO) *appruntime.ChapterRecordViewDTO {
	if record == nil {
		return nil
	}
	copyRecord := *record
	return &copyRecord
}

func (s *CreativeWorkspaceState) expectedRecordRevision() int {
	if s.Canonical == nil || s.Canonical.Record == nil {
		return 0
	}
	return s.Canonical.Record.Revision
}

func (s *CreativeWorkspaceState) canSavePlan() bool {
	return s.Plan.Dirty && s.Plan.CompleteReplacement && s.Plan.Value.Chapter == s.Chapter &&
		strings.TrimSpace(s.Plan.Value.Title) != "" && strings.TrimSpace(s.Plan.Value.Goal) != ""
}
