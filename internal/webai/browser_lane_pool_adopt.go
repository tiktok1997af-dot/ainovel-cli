package webai

import (
	"context"
	"fmt"
	"time"

	"github.com/voocel/ainovel-cli/internal/domain"
)

// AdoptedBrowserLanePool binds G05.7 orchestration to the SessionManager that
// already backs the Host's WebChatModel. It intentionally owns one physical
// execution lane because one Host/Engine execution context exists per project.
type AdoptedBrowserLanePool struct {
	pool *BrowserLanePool
}

func NewAdoptedBrowserLanePool(primary *SessionManager) (*AdoptedBrowserLanePool, error) {
	if primary == nil {
		return nil, fmt.Errorf("webai: primary browser session is required")
	}
	laneID := domain.BrowserLaneID("lane-001")
	now := time.Now()
	lane := &browserLane{
		id:        laneID,
		manager:   primary,
		changedAt: now,
	}
	pool := &BrowserLanePool{
		lanes: []*browserLane{lane},
		byID: map[domain.BrowserLaneID]*browserLane{
			laneID: lane,
		},
		byRun: make(map[domain.RunID]*browserLane, 1),
	}
	return &AdoptedBrowserLanePool{pool: pool}, nil
}

// Allocate reserves lane-001 without launching a second Chrome when the adopted
// SessionManager is already active. If it was stopped by a prior release, the
// same manager/profile is restarted.
func (p *AdoptedBrowserLanePool) Allocate(ctx context.Context, runID domain.RunID) (BrowserLaneProjection, error) {
	if p == nil || p.pool == nil {
		return BrowserLaneProjection{}, fmt.Errorf("webai: adopted browser lane pool is unavailable")
	}
	if err := validatePoolRunID(runID); err != nil {
		return BrowserLaneProjection{}, err
	}
	if err := ctx.Err(); err != nil {
		return BrowserLaneProjection{}, err
	}

	p.pool.control.Lock()
	defer p.pool.control.Unlock()

	p.pool.mu.Lock()
	if lane := p.pool.byRun[runID]; lane != nil {
		projection := p.pool.projectLaneLocked(lane)
		p.pool.mu.Unlock()
		return projection, nil
	}
	lane := p.pool.lanes[0]
	if lane.owner != "" || lane.recovering {
		p.pool.mu.Unlock()
		return BrowserLaneProjection{}, ErrNoBrowserLaneAvailable
	}
	lane.owner = runID
	lane.changedAt = time.Now()
	p.pool.byRun[runID] = lane
	p.pool.mu.Unlock()

	snap := lane.manager.Snapshot()
	var err error
	if snap.PID == 0 || snap.State == SessionStopped || snap.State == SessionFailed {
		snap, err = lane.manager.Start(ctx)
	}
	if err != nil && fatalSessionStart(snap) {
		_ = lane.manager.Stop()
		p.pool.mu.Lock()
		delete(p.pool.byRun, runID)
		lane.owner = ""
		lane.changedAt = time.Now()
		projection := p.pool.projectLaneLocked(lane)
		p.pool.mu.Unlock()
		return projection, err
	}

	p.pool.mu.RLock()
	projection := p.pool.projectLaneLocked(lane)
	p.pool.mu.RUnlock()
	return projection, err
}

func (p *AdoptedBrowserLanePool) Refresh(ctx context.Context, runID domain.RunID) (BrowserLaneProjection, error) {
	if p == nil || p.pool == nil {
		return BrowserLaneProjection{}, fmt.Errorf("webai: adopted browser lane pool is unavailable")
	}
	if err := validatePoolRunID(runID); err != nil {
		return BrowserLaneProjection{}, err
	}
	p.pool.control.Lock()
	defer p.pool.control.Unlock()

	p.pool.mu.RLock()
	lane := p.pool.byRun[runID]
	p.pool.mu.RUnlock()
	if lane == nil {
		return BrowserLaneProjection{}, fmt.Errorf("webai: run %q does not own a browser lane", runID)
	}
	_, err := lane.manager.Refresh(ctx)
	p.pool.mu.RLock()
	projection := p.pool.projectLaneLocked(lane)
	p.pool.mu.RUnlock()
	return projection, err
}

func (p *AdoptedBrowserLanePool) Release(runID domain.RunID) error {
	if p == nil || p.pool == nil {
		return nil
	}
	return p.pool.Release(runID)
}

func (p *AdoptedBrowserLanePool) List() []BrowserLaneProjection {
	if p == nil || p.pool == nil {
		return nil
	}
	return p.pool.List()
}

func (p *AdoptedBrowserLanePool) StopAll() error {
	if p == nil || p.pool == nil {
		return nil
	}
	return p.pool.StopAll()
}
