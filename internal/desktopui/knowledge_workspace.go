package desktopui

import "github.com/voocel/ainovel-cli/internal/appruntime"

const KnowledgeWorkspaceQueryLimit = 100

type KnowledgeTab string

const (
	KnowledgeTabContext    KnowledgeTab = "context"
	KnowledgeTabCanon      KnowledgeTab = "canon"
	KnowledgeTabCharacters KnowledgeTab = "characters"
	KnowledgeTabWorld      KnowledgeTab = "world"
	KnowledgeTabTimeline   KnowledgeTab = "timeline"
)

type knowledgeQueryTicket struct {
	ID               string
	Kind             appruntime.QueryKind
	SnapshotRevision uint64
	Route            RouteID
	Tab              KnowledgeTab
}

type KnowledgeWorkspaceState struct {
	Load       LoadState
	Tab        KnowledgeTab
	Context    *appruntime.KnowledgeContextResultDTO
	Canon      *appruntime.KnowledgeCanonResultDTO
	Characters *appruntime.KnowledgeCharactersResultDTO
	World      *appruntime.KnowledgeWorldResultDTO
	Timeline   *appruntime.KnowledgeTimelineResultDTO
	Error      *ErrorView
	pending    map[appruntime.QueryKind]knowledgeQueryTicket
}

func NewKnowledgeWorkspaceState() KnowledgeWorkspaceState {
	return KnowledgeWorkspaceState{
		Load:    LoadInitial,
		Tab:     KnowledgeTabContext,
		pending: make(map[appruntime.QueryKind]knowledgeQueryTicket),
	}
}

func (k *KnowledgeWorkspaceState) SelectTab(tab KnowledgeTab) bool {
	switch tab {
	case KnowledgeTabContext, KnowledgeTabCanon, KnowledgeTabCharacters, KnowledgeTabWorld, KnowledgeTabTimeline:
		k.Tab = tab
		k.Error = nil
		k.syncLoadForSelectedTab()
		return true
	default:
		return false
	}
}

func (k *KnowledgeWorkspaceState) begin(ticket knowledgeQueryTicket) {
	if k.pending == nil {
		k.pending = make(map[appruntime.QueryKind]knowledgeQueryTicket)
	}
	k.Tab = ticket.Tab
	k.pending[ticket.Kind] = ticket
	k.Error = nil
	if k.hasProjectedData(ticket.Tab) {
		k.Load = LoadRefreshing
	} else {
		k.Load = LoadLoading
	}
}

func (k *KnowledgeWorkspaceState) current(ticket knowledgeQueryTicket, shell *ShellState) bool {
	if shell == nil || shell.Route != ticket.Route || shell.SnapshotRevision != ticket.SnapshotRevision || k.Tab != ticket.Tab {
		return false
	}
	pending, ok := k.pending[ticket.Kind]
	return ok && pending.ID == ticket.ID
}

func (k *KnowledgeWorkspaceState) discard(ticket knowledgeQueryTicket) {
	pending, ok := k.pending[ticket.Kind]
	if !ok || pending.ID != ticket.ID {
		return
	}
	delete(k.pending, ticket.Kind)
	k.syncLoadForSelectedTab()
}

func (k *KnowledgeWorkspaceState) finish(ticket knowledgeQueryTicket) {
	pending, ok := k.pending[ticket.Kind]
	if ok && pending.ID == ticket.ID {
		delete(k.pending, ticket.Kind)
	}
	k.syncLoadForSelectedTab()
}

func (k *KnowledgeWorkspaceState) fail(ticket knowledgeQueryTicket, shell *ShellState, viewErr *ErrorView) bool {
	if !k.current(ticket, shell) {
		return false
	}
	if viewErr == nil {
		viewErr = runtimeUnavailableViewError()
	}
	k.Error = viewErr
	k.finish(ticket)
	return true
}

func (k *KnowledgeWorkspaceState) syncLoadForSelectedTab() {
	for _, pending := range k.pending {
		if pending.Tab == k.Tab {
			if k.hasProjectedData(k.Tab) {
				k.Load = LoadRefreshing
			} else {
				k.Load = LoadLoading
			}
			return
		}
	}
	if k.Error != nil {
		k.Load = loadStateForWorkspaceError(k.Error)
		return
	}
	if !k.hasProjectedData(k.Tab) || k.isEmpty(k.Tab) {
		k.Load = LoadEmpty
		return
	}
	k.Load = LoadReady
}

func (k *KnowledgeWorkspaceState) hasProjectedData(tab KnowledgeTab) bool {
	switch tab {
	case KnowledgeTabContext:
		return k.Context != nil
	case KnowledgeTabCanon:
		return k.Canon != nil
	case KnowledgeTabCharacters:
		return k.Characters != nil
	case KnowledgeTabWorld:
		return k.World != nil
	case KnowledgeTabTimeline:
		return k.Timeline != nil
	default:
		return false
	}
}

func (k *KnowledgeWorkspaceState) isEmpty(tab KnowledgeTab) bool {
	switch tab {
	case KnowledgeTabContext:
		return k.Context == nil || len(k.Context.Sections) == 0
	case KnowledgeTabCanon:
		return k.Canon == nil || (len(k.Canon.Facts) == 0 && k.Canon.Total == 0)
	case KnowledgeTabCharacters:
		return k.Characters == nil || (len(k.Characters.Items) == 0 && k.Characters.Total == 0)
	case KnowledgeTabWorld:
		return k.World == nil || (len(k.World.Rules) == 0 && len(k.World.Foreshadow) == 0 &&
			len(k.World.Relationships) == 0 && len(k.World.StateChanges) == 0)
	case KnowledgeTabTimeline:
		return k.Timeline == nil || (len(k.Timeline.Events) == 0 && k.Timeline.Total == 0)
	default:
		return true
	}
}
