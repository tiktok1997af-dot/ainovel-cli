package webai

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/voocel/ainovel-cli/internal/domain"
)

const MaxBrowserLaneCount = 8

var ErrNoBrowserLaneAvailable = errors.New("webai: no browser lane available")

type BrowserLaneState string

const (
	BrowserLaneStarting     BrowserLaneState = "STARTING"
	BrowserLaneAuthRequired BrowserLaneState = "AUTH_REQUIRED"
	BrowserLaneReady        BrowserLaneState = "READY"
	BrowserLaneBusy         BrowserLaneState = "BUSY"
	BrowserLaneDegraded     BrowserLaneState = "DEGRADED"
	BrowserLaneFailed       BrowserLaneState = "FAILED"
	BrowserLaneRecovering   BrowserLaneState = "RECOVERING"
	BrowserLaneStopped      BrowserLaneState = "STOPPED"
)

// BrowserLaneProjection is deliberately safe for later AppRuntime projection.
// It never contains browser/profile paths, PIDs, credentials, cookies, browser
// storage, raw provider payloads, or raw SessionManager error strings.
type BrowserLaneProjection struct {
	LaneID        domain.BrowserLaneID `json:"lane_id"`
	State         BrowserLaneState     `json:"state"`
	RunID         domain.RunID         `json:"run_id,omitempty"`
	RecoveryCount int                  `json:"recovery_count,omitempty"`
	ChangedAt     time.Time            `json:"changed_at"`
}

type BrowserLanePoolConfig struct {
	Capacity int
	Session  SessionConfig
}

type browserLane struct {
	id            domain.BrowserLaneID
	manager       *SessionManager
	owner         domain.RunID
	recovering    bool
	recoveryCount int
	changedAt     time.Time
}

// BrowserLanePool owns a bounded set of isolated visible Chrome/Gemini Web
// sessions. Lifecycle control is serialized deterministically, while actual
// model work may run concurrently in separately allocated lanes at a later
// orchestration gate.
type BrowserLanePool struct {
	control sync.Mutex
	mu      sync.RWMutex
	lanes   []*browserLane
	byID    map[domain.BrowserLaneID]*browserLane
	byRun   map[domain.RunID]*browserLane
}

func NewBrowserLanePool(cfg BrowserLanePoolConfig) (*BrowserLanePool, error) {
	if cfg.Capacity < 1 || cfg.Capacity > MaxBrowserLaneCount {
		return nil, fmt.Errorf("webai: browser lane capacity must be between 1 and %d", MaxBrowserLaneCount)
	}

	pool := &BrowserLanePool{
		lanes: make([]*browserLane, 0, cfg.Capacity),
		byID:  make(map[domain.BrowserLaneID]*browserLane, cfg.Capacity),
		byRun: make(map[domain.RunID]*browserLane, cfg.Capacity),
	}
	now := time.Now()
	for i := 0; i < cfg.Capacity; i++ {
		laneID := domain.BrowserLaneID(fmt.Sprintf("lane-%03d", i+1))
		laneCfg := isolatedLaneSessionConfig(cfg.Session, laneID)
		lane := &browserLane{
			id:        laneID,
			manager:   NewSessionManager(laneCfg),
			changedAt: now,
		}
		pool.lanes = append(pool.lanes, lane)
		pool.byID[laneID] = lane
	}
	return pool, nil
}

// Allocate deterministically reserves the first free lane for runID and starts
// that lane's visible browser session. Repeated allocation for the same run is
// idempotent. AUTH_REQUIRED is a valid allocated state. A transient readiness
// error may return DEGRADED while preserving the allocation so recovery can own
// the same isolated lane. Fatal launch failure releases the reservation.
func (p *BrowserLanePool) Allocate(ctx context.Context, runID domain.RunID) (BrowserLaneProjection, error) {
	if err := validatePoolRunID(runID); err != nil {
		return BrowserLaneProjection{}, err
	}
	if err := ctx.Err(); err != nil {
		return BrowserLaneProjection{}, err
	}

	p.control.Lock()
	defer p.control.Unlock()

	p.mu.Lock()
	if lane := p.byRun[runID]; lane != nil {
		projection := p.projectLaneLocked(lane)
		p.mu.Unlock()
		return projection, nil
	}
	var lane *browserLane
	for _, candidate := range p.lanes {
		if candidate.owner == "" && !candidate.recovering {
			lane = candidate
			break
		}
	}
	if lane == nil {
		p.mu.Unlock()
		return BrowserLaneProjection{}, ErrNoBrowserLaneAvailable
	}
	lane.owner = runID
	lane.changedAt = time.Now()
	p.byRun[runID] = lane
	p.mu.Unlock()

	snap, err := lane.manager.Start(ctx)
	if err != nil && fatalSessionStart(snap) {
		_ = lane.manager.Stop()
		p.mu.Lock()
		if lane.owner == runID {
			delete(p.byRun, runID)
			lane.owner = ""
			lane.changedAt = time.Now()
		}
		projection := p.projectLaneLocked(lane)
		p.mu.Unlock()
		return projection, err
	}

	p.mu.RLock()
	projection := p.projectLaneLocked(lane)
	p.mu.RUnlock()
	return projection, err
}

// Release stops the owned browser process before making the lane available to a
// different run. The persistent lane profile is deliberately left on disk so
// browser-owned login state can survive future lane reuse without sharing one
// profile concurrently between lanes.
func (p *BrowserLanePool) Release(runID domain.RunID) error {
	if err := validatePoolRunID(runID); err != nil {
		return err
	}

	p.control.Lock()
	defer p.control.Unlock()

	p.mu.RLock()
	lane := p.byRun[runID]
	p.mu.RUnlock()
	if lane == nil {
		return nil
	}

	stopErr := lane.manager.Stop()
	p.mu.Lock()
	if lane.owner == runID {
		delete(p.byRun, runID)
		lane.owner = ""
		lane.recovering = false
		lane.changedAt = time.Now()
	}
	p.mu.Unlock()
	return stopErr
}

// Recover performs a bounded stop/restart of one allocated lane while retaining
// its run ownership and isolated profile. Recovery never changes lane identity
// and never falls back to an AI API or hidden browser.
func (p *BrowserLanePool) Recover(ctx context.Context, laneID domain.BrowserLaneID) (BrowserLaneProjection, error) {
	if err := validatePoolLaneID(laneID); err != nil {
		return BrowserLaneProjection{}, err
	}
	if err := ctx.Err(); err != nil {
		return BrowserLaneProjection{}, err
	}

	p.control.Lock()
	defer p.control.Unlock()

	p.mu.Lock()
	lane := p.byID[laneID]
	if lane == nil {
		p.mu.Unlock()
		return BrowserLaneProjection{}, fmt.Errorf("webai: unknown browser lane %q", laneID)
	}
	if lane.owner == "" {
		projection := p.projectLaneLocked(lane)
		p.mu.Unlock()
		return projection, fmt.Errorf("webai: browser lane %q is not allocated", laneID)
	}
	lane.recovering = true
	lane.recoveryCount++
	lane.changedAt = time.Now()
	p.mu.Unlock()

	if err := lane.manager.Stop(); err != nil {
		p.mu.Lock()
		lane.recovering = false
		lane.changedAt = time.Now()
		projection := p.projectLaneLocked(lane)
		p.mu.Unlock()
		return projection, err
	}

	_, startErr := lane.manager.Start(ctx)
	p.mu.Lock()
	lane.recovering = false
	lane.changedAt = time.Now()
	projection := p.projectLaneLocked(lane)
	p.mu.Unlock()
	return projection, startErr
}

// StopAll is the pool shutdown boundary. It stops every owned/idle lane and
// clears transient run allocations; it does not delete any browser profile.
func (p *BrowserLanePool) StopAll() error {
	p.control.Lock()
	defer p.control.Unlock()

	var firstErr error
	for _, lane := range p.lanes {
		if err := lane.manager.Stop(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	p.mu.Lock()
	clear(p.byRun)
	for _, lane := range p.lanes {
		lane.owner = ""
		lane.recovering = false
		lane.changedAt = time.Now()
	}
	p.mu.Unlock()
	return firstErr
}

func (p *BrowserLanePool) Snapshot(laneID domain.BrowserLaneID) (BrowserLaneProjection, error) {
	if err := validatePoolLaneID(laneID); err != nil {
		return BrowserLaneProjection{}, err
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	lane := p.byID[laneID]
	if lane == nil {
		return BrowserLaneProjection{}, fmt.Errorf("webai: unknown browser lane %q", laneID)
	}
	return p.projectLaneLocked(lane), nil
}

func (p *BrowserLanePool) List() []BrowserLaneProjection {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]BrowserLaneProjection, 0, len(p.lanes))
	for _, lane := range p.lanes {
		out = append(out, p.projectLaneLocked(lane))
	}
	return out
}

func (p *BrowserLanePool) projectLaneLocked(lane *browserLane) BrowserLaneProjection {
	snap := lane.manager.Snapshot()
	changedAt := lane.changedAt
	if snap.ChangedAt.After(changedAt) {
		changedAt = snap.ChangedAt
	}
	state := projectBrowserLaneState(snap.State, lane.owner != "", lane.recovering)
	return BrowserLaneProjection{
		LaneID:        lane.id,
		State:         state,
		RunID:         lane.owner,
		RecoveryCount: lane.recoveryCount,
		ChangedAt:     changedAt,
	}
}

func projectBrowserLaneState(sessionState SessionState, owned, recovering bool) BrowserLaneState {
	if recovering {
		return BrowserLaneRecovering
	}
	switch sessionState {
	case SessionStarting:
		return BrowserLaneStarting
	case SessionAuthRequired:
		return BrowserLaneAuthRequired
	case SessionReady:
		if owned {
			return BrowserLaneBusy
		}
		return BrowserLaneReady
	case SessionBusy:
		return BrowserLaneBusy
	case SessionDegraded:
		return BrowserLaneDegraded
	case SessionFailed:
		return BrowserLaneFailed
	case SessionStopped:
		return BrowserLaneStopped
	default:
		return BrowserLaneFailed
	}
}

func isolatedLaneSessionConfig(base SessionConfig, laneID domain.BrowserLaneID) SessionConfig {
	cfg := base
	cfg.ExtraArgs = append([]string(nil), base.ExtraArgs...)
	if profileDir := strings.TrimSpace(base.ProfileDir); profileDir != "" {
		cfg.ProfileDir = filepath.Join(profileDir, string(laneID))
		cfg.ProfileName = ""
		return cfg
	}
	profileName := strings.TrimSpace(base.ProfileName)
	if profileName == "" {
		profileName = "default"
	}
	cfg.ProfileName = profileName + "-" + string(laneID)
	cfg.ProfileDir = ""
	return cfg
}

func fatalSessionStart(snap SessionSnapshot) bool {
	return snap.PID == 0 || snap.State == SessionFailed || snap.State == SessionStopped
}

func validatePoolRunID(runID domain.RunID) error {
	identity := domain.RunIdentity{RunID: runID}
	if err := identity.Validate(); err != nil {
		return fmt.Errorf("webai: invalid browser lane run identity: %w", err)
	}
	return nil
}

func validatePoolLaneID(laneID domain.BrowserLaneID) error {
	if laneID == "" {
		return fmt.Errorf("webai: browser lane id is required")
	}
	identity := domain.RunIdentity{RunID: domain.RunID("lane-pool-validation"), LaneID: laneID}
	if err := identity.Validate(); err != nil {
		return fmt.Errorf("webai: invalid browser lane identity: %w", err)
	}
	return nil
}
