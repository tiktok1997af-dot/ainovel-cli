package desktopui

import (
	"strings"

	"github.com/voocel/ainovel-cli/internal/appruntime"
)

const (
	ThemeDark        = "dark"
	MaxActivityItems = 100
)

type RouteID string

const (
	RouteOverview  RouteID = "overview"
	RouteProject   RouteID = "project"
	RouteCreative  RouteID = "creative"
	RouteKnowledge RouteID = "knowledge"
	RouteReview    RouteID = "review"
	RouteRunCenter RouteID = "run_center"
	RouteSettings  RouteID = "settings"
)

type RouteItem struct {
	ID           RouteID
	Label        string
	Enabled      bool
	Availability string
}

// PrimaryNavigation reserves the complete G01 product navigation. G04.3 only
// makes the snapshot-backed Overview shell functional; successor workspaces
// remain explicit disabled placeholders until their owning child gate opens.
func PrimaryNavigation() []RouteItem {
	return []RouteItem{
		{ID: RouteOverview, Label: "Tổng quan", Enabled: true, Availability: "G04.3"},
		{ID: RouteProject, Label: "Dự án", Enabled: false, Availability: "G04.4"},
		{ID: RouteCreative, Label: "Sáng tác", Enabled: false, Availability: "G04.7"},
		{ID: RouteKnowledge, Label: "Tri thức", Enabled: false, Availability: "G04.5"},
		{ID: RouteReview, Label: "Review", Enabled: false, Availability: "later gate"},
		{ID: RouteRunCenter, Label: "Run Center", Enabled: false, Availability: "G05+"},
		{ID: RouteSettings, Label: "Cài đặt", Enabled: false, Availability: "later gate"},
	}
}

type LoadState string

const (
	LoadInitial         LoadState = "initial"
	LoadLoading         LoadState = "loading"
	LoadReady           LoadState = "ready"
	LoadEmpty           LoadState = "empty"
	LoadRefreshing      LoadState = "refreshing"
	LoadSaving          LoadState = "saving"
	LoadCommandPending  LoadState = "command_pending"
	LoadValidationError LoadState = "validation_error"
	LoadConflict        LoadState = "conflict"
	LoadRuntimeError    LoadState = "runtime_error"
	LoadUnsupported     LoadState = "unsupported"
)

type ViewportClass string

const (
	ViewportMobile  ViewportClass = "mobile"
	ViewportTablet  ViewportClass = "tablet"
	ViewportDesktop ViewportClass = "desktop"
)

type LayoutState struct {
	Class             ViewportClass
	Width             int
	NavigationVisible bool
	NavigationWidth   int
	MainVisible       bool
	InspectorVisible  bool
	InspectorWidth    int
	InspectorDrawer   bool
}

func LayoutForWidth(width int, inspectorOpen bool) LayoutState {
	if width < 0 {
		width = 0
	}
	switch {
	case width >= 1200:
		return LayoutState{
			Class:             ViewportDesktop,
			Width:             width,
			NavigationVisible: true,
			NavigationWidth:   256,
			MainVisible:       true,
			InspectorVisible:  inspectorOpen,
			InspectorWidth:    360,
		}
	case width >= 760:
		return LayoutState{
			Class:             ViewportTablet,
			Width:             width,
			NavigationVisible: true,
			NavigationWidth:   256,
			MainVisible:       true,
			InspectorDrawer:   inspectorOpen,
		}
	default:
		return LayoutState{
			Class:           ViewportMobile,
			Width:           width,
			MainVisible:     true,
			InspectorDrawer: inspectorOpen,
		}
	}
}

type HeaderView struct {
	ProductName    string
	ProjectTitle   string
	ChapterCurrent int
	ChapterTotal   int
	Revision       uint64
}

type RuntimeStatusView struct {
	State         string
	Status        string
	Phase         string
	Flow          string
	BrowserState  string
	BrowserSite   string
	RecoveryLabel string
	CanResume     bool
}

type LifecycleControlView struct {
	ID       string
	Label    string
	Eligible bool
	Enabled  bool
	Reason   string
}

// ReservedLifecycleControls makes the required global controls visible to a
// renderer without wiring behavior before G04.6. Eligibility is derived from
// the authoritative lifecycle state, while Enabled stays false until the
// command-binding child gate opens.
func ReservedLifecycleControls(state ...string) []LifecycleControlView {
	current := ""
	if len(state) > 0 {
		current = strings.ToLower(strings.TrimSpace(state[0]))
	}
	const reason = "Lifecycle command wiring opens in G04.6."
	return []LifecycleControlView{
		{ID: "start", Label: "Start", Eligible: allowsLifecyclePresentation("start", current), Reason: reason},
		{ID: "pause", Label: "Pause", Eligible: allowsLifecyclePresentation("pause", current), Reason: reason},
		{ID: "resume", Label: "Resume", Eligible: allowsLifecyclePresentation("resume", current), Reason: reason},
		{ID: "stop", Label: "Stop", Eligible: allowsLifecyclePresentation("stop", current), Reason: reason},
		{ID: "cancel", Label: "Cancel", Eligible: allowsLifecyclePresentation("cancel", current), Reason: reason},
		{ID: "retry", Label: "Retry", Eligible: allowsLifecyclePresentation("retry", current), Reason: reason},
	}
}

func allowsLifecyclePresentation(action, state string) bool {
	switch action {
	case "start":
		return state == string(appruntime.LifecycleReady) || state == string(appruntime.LifecycleStopped) ||
			state == string(appruntime.LifecycleCancelled) || state == string(appruntime.LifecycleFailed)
	case "pause":
		return state == string(appruntime.LifecycleRunning)
	case "resume":
		return state == string(appruntime.LifecyclePaused)
	case "stop", "cancel":
		return state == string(appruntime.LifecycleRunning) || state == string(appruntime.LifecyclePaused)
	case "retry":
		return state == string(appruntime.LifecycleFailed) || state == string(appruntime.LifecycleCancelled) ||
			state == string(appruntime.LifecycleStopped)
	default:
		return false
	}
}

type InspectorState struct {
	Open    bool
	Section string
}

type ActivityItem struct {
	Seq      int64
	Category string
	Type     string
	Level    string
	Summary  string
}

type ErrorView struct {
	Code      string
	Category  string
	Message   string
	Retryable bool
}

type ShellState struct {
	Theme            string
	Route            RouteID
	Load             LoadState
	Layout           LayoutState
	Header           HeaderView
	Runtime          RuntimeStatusView
	Controls         []LifecycleControlView
	Inspector        InspectorState
	Activity         []ActivityItem
	Error            *ErrorView
	HasSnapshot      bool
	Snapshot         appruntime.DesktopSnapshot
	SnapshotRevision uint64
	LastEventSeq     int64
	pendingSnapshot  string
}

func NewShell(width int) *ShellState {
	inspectorOpen := width >= 1200
	return &ShellState{
		Theme:     ThemeDark,
		Route:     RouteOverview,
		Load:      LoadInitial,
		Layout:    LayoutForWidth(width, inspectorOpen),
		Controls:  ReservedLifecycleControls(),
		Inspector: InspectorState{Open: inspectorOpen, Section: "status"},
	}
}

func (s *ShellState) SelectRoute(route RouteID) bool {
	for _, item := range PrimaryNavigation() {
		if item.ID != route {
			continue
		}
		if !item.Enabled {
			return false
		}
		s.Route = route
		return true
	}
	return false
}

func (s *ShellState) Resize(width int) {
	s.Layout = LayoutForWidth(width, s.Inspector.Open)
}

func (s *ShellState) SetInspectorOpen(open bool) {
	s.Inspector.Open = open
	s.Layout = LayoutForWidth(s.Layout.Width, open)
}

func (s *ShellState) SetInspectorSection(section string) {
	section = strings.TrimSpace(section)
	if section == "" {
		section = "status"
	}
	s.Inspector.Section = section
}

func (s *ShellState) BeginSnapshot(requestID string) bool {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return false
	}
	s.pendingSnapshot = requestID
	s.Error = nil
	if s.HasSnapshot {
		s.Load = LoadRefreshing
	} else {
		s.Load = LoadLoading
	}
	return true
}

func (s *ShellState) AcceptSnapshot(requestID string, snapshot appruntime.DesktopSnapshot) bool {
	if requestID == "" || requestID != s.pendingSnapshot {
		return false
	}
	s.pendingSnapshot = ""
	if snapshot.Contract.Version != appruntime.ContractVersion || snapshot.Contract.SchemaVersion != appruntime.SchemaVersion {
		s.Load = LoadRuntimeError
		s.Error = &ErrorView{
			Code:     string(appruntime.ErrorCodeContractMismatch),
			Category: string(appruntime.ErrorCategoryValidation),
			Message:  "The desktop and core contract versions are incompatible.",
		}
		return false
	}
	if s.HasSnapshot && snapshot.Revision < s.SnapshotRevision {
		s.Load = loadStateForSnapshot(s.Snapshot)
		return false
	}

	s.Snapshot = snapshot
	s.HasSnapshot = true
	s.SnapshotRevision = snapshot.Revision
	s.Header = headerFromSnapshot(snapshot)
	s.Runtime = runtimeFromSnapshot(snapshot)
	s.Controls = ReservedLifecycleControls(snapshot.Runtime.State)
	s.Load = loadStateForSnapshot(snapshot)
	s.Error = nil
	return true
}

func (s *ShellState) FailSnapshot(requestID string, viewErr *ErrorView) bool {
	if requestID == "" || requestID != s.pendingSnapshot {
		return false
	}
	s.pendingSnapshot = ""
	if viewErr == nil {
		viewErr = runtimeUnavailableViewError()
	}
	s.Error = viewErr
	s.Load = LoadRuntimeError
	return true
}

func (s *ShellState) ApplyEvent(event appruntime.DesktopEvent) bool {
	if event.ContractVersion != "" && event.ContractVersion != appruntime.ContractVersion {
		return false
	}
	if event.Seq > 0 {
		if event.Seq <= s.LastEventSeq {
			return false
		}
		s.LastEventSeq = event.Seq
	}

	item := ActivityItem{
		Seq:      event.Seq,
		Category: event.Category,
		Type:     event.Type,
		Level:    event.Level,
		Summary:  event.Summary,
	}
	if item.Category != "" || item.Type != "" || item.Summary != "" || item.Level != "" {
		s.Activity = append(s.Activity, item)
		if len(s.Activity) > MaxActivityItems {
			s.Activity = append([]ActivityItem(nil), s.Activity[len(s.Activity)-MaxActivityItems:]...)
		}
	}
	if event.Error != nil {
		s.Error = viewErrorFromAppError(event.Error)
	}
	return true
}

func headerFromSnapshot(snapshot appruntime.DesktopSnapshot) HeaderView {
	projectTitle := strings.TrimSpace(snapshot.Project.Title)
	if projectTitle == "" {
		projectTitle = "Chưa mở dự án"
	}
	productName := strings.TrimSpace(snapshot.Product.Name)
	if productName == "" {
		productName = "AINOVEL"
	}
	return HeaderView{
		ProductName:    productName,
		ProjectTitle:   projectTitle,
		ChapterCurrent: snapshot.CurrentChapter.Current,
		ChapterTotal:   snapshot.CurrentChapter.Total,
		Revision:       snapshot.Revision,
	}
}

func runtimeFromSnapshot(snapshot appruntime.DesktopSnapshot) RuntimeStatusView {
	return RuntimeStatusView{
		State:         snapshot.Runtime.State,
		Status:        snapshot.Runtime.Status,
		Phase:         snapshot.Runtime.Phase,
		Flow:          snapshot.Runtime.Flow,
		BrowserState:  snapshot.Browser.State,
		BrowserSite:   snapshot.Browser.Site,
		RecoveryLabel: snapshot.Recovery.Label,
		CanResume:     snapshot.Recovery.CanResume,
	}
}

func loadStateForSnapshot(snapshot appruntime.DesktopSnapshot) LoadState {
	if strings.TrimSpace(snapshot.Project.Title) == "" && strings.TrimSpace(snapshot.Project.OutputDir) == "" {
		return LoadEmpty
	}
	return LoadReady
}

func viewErrorFromAppError(appErr *appruntime.AppError) *ErrorView {
	if appErr == nil {
		return nil
	}
	return &ErrorView{
		Code:      string(appErr.Code),
		Category:  string(appErr.Category),
		Message:   appErr.Message,
		Retryable: appErr.Retryable,
	}
}

func runtimeUnavailableViewError() *ErrorView {
	return &ErrorView{
		Code:      string(appruntime.ErrorCodeRuntimeUnavailable),
		Category:  string(appruntime.ErrorCategoryRuntime),
		Message:   "The application runtime is unavailable.",
		Retryable: true,
	}
}
