package webai

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/voocel/ainovel-cli/internal/webai/sites"
)

const (
	ChatGPTWebSite            = "chatgpt-web"
	ChatGPTWebLaneID          = "chatgpt-web"
	DefaultChatGPTProfileName = "ainovel-chatgpt-web"
)

type ChatGPTLaneConfig struct {
	BrowserPath string
	ProfileDir  string
	ProfileName string
	StartURL    string
	Launcher    BrowserLauncher
	Probe       ReadinessProbe
	Session     *SessionManager

	evaluatorFactory func(context.Context, SessionSnapshot, sites.Adapter) (interactionEvaluator, error)
}

type ChatGPTLaneSnapshot struct {
	LaneID        string                     `json:"lane_id"`
	State         SessionState               `json:"state"`
	Authenticated bool                       `json:"authenticated"`
	Ready         bool                       `json:"ready"`
	TurnActive    bool                       `json:"turn_active"`
	Catalog       sites.ModelCatalogSnapshot `json:"catalog"`
}

// ChatGPTLane owns exactly one visible ChatGPT web session and at most one
// active model turn. D07 also makes the same lane a WEB-only Transport so role
// routing never creates a second ChatGPT profile/session/tab.
type ChatGPTLane struct {
	mu               sync.Mutex
	session          *SessionManager
	adapter          sites.ModelCatalogObserver
	transport        *GeminiWebTransport
	evaluatorFactory func(context.Context, SessionSnapshot, sites.Adapter) (interactionEvaluator, error)
	turnActive       bool
	catalog          sites.ModelCatalogSnapshot
}

var _ Transport = (*ChatGPTLane)(nil)

func NewChatGPTLane(cfg ChatGPTLaneConfig) *ChatGPTLane {
	profileName := strings.TrimSpace(cfg.ProfileName)
	if strings.TrimSpace(cfg.ProfileDir) == "" && profileName == "" {
		profileName = DefaultChatGPTProfileName
	}
	session := cfg.Session
	if session == nil {
		session = NewSessionManager(SessionConfig{
			Site:        ChatGPTWebSite,
			BrowserPath: strings.TrimSpace(cfg.BrowserPath),
			ProfileDir:  strings.TrimSpace(cfg.ProfileDir),
			ProfileName: profileName,
			StartURL:    strings.TrimSpace(cfg.StartURL),
			Launcher:    cfg.Launcher,
			Probe:       cfg.Probe,
		})
	}
	factory := cfg.evaluatorFactory
	if factory == nil {
		factory = openInteractionEvaluator
	}
	interaction := sites.ChatGPT{}
	transport, _ := NewGeminiWebTransport(GeminiWebTransportConfig{
		Session:          session,
		adapter:          interaction,
		evaluatorFactory: factory,
	})
	return &ChatGPTLane{
		session:          session,
		adapter:          interaction,
		transport:        transport,
		evaluatorFactory: factory,
	}
}

func (l *ChatGPTLane) Start(ctx context.Context) (ChatGPTLaneSnapshot, error) {
	if l == nil || l.session == nil {
		return ChatGPTLaneSnapshot{}, fmt.Errorf("webai: ChatGPT lane is unavailable")
	}
	snap, err := l.session.Start(ctx)
	if err != nil && snap.State != SessionAuthRequired && snap.State != SessionDegraded {
		return l.Snapshot(), err
	}
	if snap.State != SessionReady {
		return l.Snapshot(), err
	}
	if observeErr := l.observeModels(ctx, snap); observeErr != nil {
		return l.Snapshot(), observeErr
	}
	return l.Snapshot(), err
}

func (l *ChatGPTLane) Refresh(ctx context.Context) (ChatGPTLaneSnapshot, error) {
	if l == nil || l.session == nil {
		return ChatGPTLaneSnapshot{}, fmt.Errorf("webai: ChatGPT lane is unavailable")
	}
	snap, err := l.session.Refresh(ctx)
	if err != nil {
		return l.Snapshot(), err
	}
	if snap.State != SessionReady {
		l.clearCatalog()
		return l.Snapshot(), nil
	}
	if err := l.observeModels(ctx, snap); err != nil {
		return l.Snapshot(), err
	}
	return l.Snapshot(), nil
}

func (l *ChatGPTLane) Stop() error {
	if l == nil || l.session == nil {
		return nil
	}
	l.mu.Lock()
	l.turnActive = false
	l.catalog = sites.ModelCatalogSnapshot{}
	l.mu.Unlock()
	return l.session.Stop()
}

func (l *ChatGPTLane) Snapshot() ChatGPTLaneSnapshot {
	if l == nil || l.session == nil {
		return ChatGPTLaneSnapshot{LaneID: ChatGPTWebLaneID, State: SessionStopped}
	}
	session := l.session.Snapshot()
	l.mu.Lock()
	catalog := cloneSiteModelCatalog(l.catalog)
	turnActive := l.turnActive
	l.mu.Unlock()
	authenticated := session.State == SessionReady || session.State == SessionBusy
	ready := authenticated && !turnActive && catalog.ActiveModelID != "" && catalog.Revision != ""
	return ChatGPTLaneSnapshot{
		LaneID:        ChatGPTWebLaneID,
		State:         session.State,
		Authenticated: authenticated,
		Ready:         ready,
		TurnActive:    turnActive,
		Catalog:       catalog,
	}
}

func (l *ChatGPTLane) BeginTurn() error {
	if l == nil || l.session == nil {
		return fmt.Errorf("webai: ChatGPT lane is unavailable")
	}
	if l.session.Snapshot().State != SessionReady {
		return fmt.Errorf("webai: ChatGPT lane is not READY")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.turnActive {
		return fmt.Errorf("webai: ChatGPT lane already has an active turn")
	}
	if l.catalog.ActiveModelID == "" || l.catalog.Revision == "" {
		return fmt.Errorf("webai: ChatGPT model observation is not verified")
	}
	l.turnActive = true
	return nil
}

func (l *ChatGPTLane) EndTurn() {
	if l == nil {
		return
	}
	l.mu.Lock()
	l.turnActive = false
	l.mu.Unlock()
}

// RoundTrip executes one prompt through the same authenticated lane. It first
// revalidates readiness/model observation, then holds the lane-local turn lease
// for the entire DOM round trip. There is no provider fallback.
func (l *ChatGPTLane) RoundTrip(ctx context.Context, prompt string) (string, error) {
	if l == nil || l.session == nil || l.transport == nil {
		return "", fmt.Errorf("webai: ChatGPT transport is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	snap := l.Snapshot()
	if snap.TurnActive {
		return "", fmt.Errorf("webai: ChatGPT lane already has an active turn")
	}
	var err error
	if snap.State == SessionStopped {
		snap, err = l.Start(ctx)
	} else if !snap.Ready {
		snap, err = l.Refresh(ctx)
	}
	if err != nil {
		return "", err
	}
	if !snap.Ready {
		return "", fmt.Errorf("webai: ChatGPT lane is not verified READY")
	}
	if err := l.BeginTurn(); err != nil {
		return "", err
	}
	defer l.EndTurn()
	return l.transport.RoundTrip(ctx, prompt)
}

func (l *ChatGPTLane) observeModels(ctx context.Context, snap SessionSnapshot) error {
	if snap.State != SessionReady {
		l.clearCatalog()
		return fmt.Errorf("webai: ChatGPT model observation requires READY session")
	}
	evaluator, err := l.evaluatorFactory(ctx, snap, l.adapter)
	if err != nil {
		l.clearCatalog()
		return &Error{Kind: ErrorTransport, Op: "observe ChatGPT models", Cause: err, Retry: true}
	}
	defer evaluator.Close()
	catalog, err := l.adapter.ObserveModels(ctx, evaluator)
	if err != nil {
		l.clearCatalog()
		return &Error{Kind: ErrorTransport, Op: "observe ChatGPT models", Cause: err, Retry: true}
	}
	if catalog.ActiveModelID == "" || catalog.Revision == "" || len(catalog.Models) == 0 {
		l.clearCatalog()
		return &Error{Kind: ErrorProtocol, Op: "observe ChatGPT models", Cause: fmt.Errorf("active model catalog is incomplete")}
	}
	l.mu.Lock()
	l.catalog = cloneSiteModelCatalog(catalog)
	l.mu.Unlock()
	return nil
}

func (l *ChatGPTLane) clearCatalog() {
	l.mu.Lock()
	l.catalog = sites.ModelCatalogSnapshot{}
	l.mu.Unlock()
}

func cloneSiteModelCatalog(in sites.ModelCatalogSnapshot) sites.ModelCatalogSnapshot {
	out := in
	out.Models = append([]sites.ModelOption(nil), in.Models...)
	return out
}
