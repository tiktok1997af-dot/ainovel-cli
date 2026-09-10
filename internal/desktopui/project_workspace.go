package desktopui

import (
	"strings"

	"github.com/voocel/ainovel-cli/internal/appruntime"
)

const ProjectWorkspaceQueryLimit = 100

type ProjectTab string

const (
	ProjectTabOverview ProjectTab = "overview"
	ProjectTabChapters ProjectTab = "chapters"
	ProjectTabOutline  ProjectTab = "outline"
)

type workspaceQueryTicket struct {
	ID               string
	Kind             appruntime.QueryKind
	SnapshotRevision uint64
	Route            RouteID
	Chapter          int
}

type ProjectWorkspaceState struct {
	Load            LoadState
	Tab             ProjectTab
	Overview        *appruntime.ProjectOverviewResultDTO
	Chapters        appruntime.ChaptersListResultDTO
	SelectedChapter *appruntime.ChaptersGetResultDTO
	Outline         *appruntime.OutlineGetResultDTO
	Selected        int
	Error           *ErrorView
	pending         map[appruntime.QueryKind]workspaceQueryTicket
}

func NewProjectWorkspaceState() ProjectWorkspaceState {
	return ProjectWorkspaceState{
		Load:    LoadInitial,
		Tab:     ProjectTabOverview,
		pending: make(map[appruntime.QueryKind]workspaceQueryTicket),
	}
}

func (p *ProjectWorkspaceState) SelectTab(tab ProjectTab) bool {
	switch tab {
	case ProjectTabOverview, ProjectTabChapters, ProjectTabOutline:
		p.Tab = tab
		return true
	default:
		return false
	}
}

func (p *ProjectWorkspaceState) begin(ticket workspaceQueryTicket) {
	if p.pending == nil {
		p.pending = make(map[appruntime.QueryKind]workspaceQueryTicket)
	}
	p.pending[ticket.Kind] = ticket
	p.Error = nil
	if p.hasProjectedData() {
		p.Load = LoadRefreshing
	} else {
		p.Load = LoadLoading
	}
}

func (p *ProjectWorkspaceState) current(ticket workspaceQueryTicket, shell *ShellState) bool {
	if shell == nil || shell.Route != ticket.Route || shell.SnapshotRevision != ticket.SnapshotRevision {
		return false
	}
	pending, ok := p.pending[ticket.Kind]
	return ok && pending.ID == ticket.ID
}

func (p *ProjectWorkspaceState) discard(ticket workspaceQueryTicket) {
	pending, ok := p.pending[ticket.Kind]
	if !ok || pending.ID != ticket.ID {
		return
	}
	delete(p.pending, ticket.Kind)
	if len(p.pending) == 0 {
		if p.Error != nil {
			p.Load = loadStateForWorkspaceError(p.Error)
		} else if p.isEmpty() {
			p.Load = LoadEmpty
		} else {
			p.Load = LoadReady
		}
	}
}

func (p *ProjectWorkspaceState) finish(ticket workspaceQueryTicket) {
	pending, ok := p.pending[ticket.Kind]
	if ok && pending.ID == ticket.ID {
		delete(p.pending, ticket.Kind)
	}
	if len(p.pending) > 0 {
		if p.hasProjectedData() {
			p.Load = LoadRefreshing
		} else {
			p.Load = LoadLoading
		}
		return
	}
	if p.Error != nil {
		p.Load = loadStateForWorkspaceError(p.Error)
		return
	}
	if p.isEmpty() {
		p.Load = LoadEmpty
		return
	}
	p.Load = LoadReady
}

func (p *ProjectWorkspaceState) fail(ticket workspaceQueryTicket, shell *ShellState, viewErr *ErrorView) bool {
	if !p.current(ticket, shell) {
		return false
	}
	if viewErr == nil {
		viewErr = runtimeUnavailableViewError()
	}
	p.Error = viewErr
	p.finish(ticket)
	return true
}

func (p *ProjectWorkspaceState) hasProjectedData() bool {
	return p.Overview != nil || len(p.Chapters.Items) > 0 || p.Chapters.Total > 0 ||
		p.SelectedChapter != nil || p.Outline != nil
}

func (p *ProjectWorkspaceState) isEmpty() bool {
	if p.Overview == nil && p.Outline == nil && len(p.Chapters.Items) == 0 && p.Chapters.Total == 0 {
		return true
	}
	if p.Overview != nil {
		if strings.TrimSpace(p.Overview.Title) != "" || strings.TrimSpace(p.Overview.Synopsis) != "" ||
			strings.TrimSpace(p.Overview.Premise) != "" || p.Overview.CurrentChapter > 0 ||
			p.Overview.CompletedChapters > 0 || p.Overview.TotalWordCount > 0 {
			return false
		}
	}
	if len(p.Chapters.Items) > 0 || p.Chapters.Total > 0 {
		return false
	}
	if p.Outline != nil && (len(p.Outline.Chapters) > 0 || len(p.Outline.Volumes) > 0 || p.Outline.Compass != nil) {
		return false
	}
	return true
}

func loadStateForWorkspaceError(viewErr *ErrorView) LoadState {
	if viewErr == nil {
		return LoadRuntimeError
	}
	switch viewErr.Category {
	case string(appruntime.ErrorCategoryValidation):
		return LoadValidationError
	case string(appruntime.ErrorCategoryConflict):
		return LoadConflict
	default:
		return LoadRuntimeError
	}
}

func queryProtocolViewError() *ErrorView {
	return &ErrorView{
		Code:      string(appruntime.ErrorCodeInternal),
		Category:  string(appruntime.ErrorCategoryInternal),
		Message:   "An internal runtime error occurred.",
		Retryable: true,
	}
}
