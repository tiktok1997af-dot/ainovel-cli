package appruntime

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/webai"
)

type providerCircuitState string

const (
	providerCircuitHealthy  providerCircuitState = "healthy"
	providerCircuitDegraded providerCircuitState = "degraded"
	providerCircuitOpen     providerCircuitState = "open"
	providerCircuitHalfOpen providerCircuitState = "half_open"

	d06ProviderFailureThreshold = 2
	d06ProviderOpenCooldown     = 30 * time.Second
)

var errD06ProviderCircuitOpen = errors.New("dual-web provider circuit is open")

type providerCircuitSnapshot struct {
	Provider            WebAIProvider
	State               providerCircuitState
	ConsecutiveFailures int
	ProbeInFlight       bool
	OpenedAt            time.Time
	LastSuccessAt       time.Time
	LastFailureAt       time.Time
}

type providerCircuit struct {
	state               providerCircuitState
	consecutiveFailures int
	probeInFlight       bool
	openedAt            time.Time
	lastSuccessAt       time.Time
	lastFailureAt       time.Time
}

type providerResilienceLayer struct {
	mu        sync.Mutex
	providers map[WebAIProvider]*providerCircuit
	now       func() time.Time
}

func newProviderResilienceLayer() *providerResilienceLayer {
	return &providerResilienceLayer{
		providers: make(map[WebAIProvider]*providerCircuit),
		now:       time.Now,
	}
}

// beginAdmission is the only circuit-admission mutation. D04 still owns lane
// capacity, so HEALTHY/DEGRADED admissions are not serialized here. OPEN is
// fail-closed; after cooldown exactly one HALF_OPEN probe is allowed.
func (r *providerResilienceLayer) beginAdmission(provider WebAIProvider) error {
	if r == nil {
		return nil
	}
	if err := provider.Validate(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	circuit := r.circuitLocked(provider)
	now := r.nowLocked()

	switch circuit.state {
	case providerCircuitOpen:
		if now.Sub(circuit.openedAt) < d06ProviderOpenCooldown {
			return errD06ProviderCircuitOpen
		}
		circuit.state = providerCircuitHalfOpen
		circuit.probeInFlight = true
		return nil
	case providerCircuitHalfOpen:
		if circuit.probeInFlight {
			return errD06ProviderCircuitOpen
		}
		circuit.probeInFlight = true
		return nil
	default:
		return nil
	}
}

func (r *providerResilienceLayer) recordSuccess(provider WebAIProvider) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	circuit := r.circuitLocked(provider)
	circuit.state = providerCircuitHealthy
	circuit.consecutiveFailures = 0
	circuit.probeInFlight = false
	circuit.openedAt = time.Time{}
	circuit.lastSuccessAt = r.nowLocked()
}

// recordNeutral completes an admission whose outcome says nothing about
// provider health (auth required, caller cancellation, capacity contention).
// A HALF_OPEN probe therefore returns to OPEN without incrementing failures;
// otherwise an unrelated neutral request could accidentally close/degrade the
// circuit and release a provider for immediate browser churn.
func (r *providerResilienceLayer) recordNeutral(provider WebAIProvider) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	circuit := r.circuitLocked(provider)
	if circuit.state == providerCircuitHalfOpen {
		circuit.state = providerCircuitOpen
		circuit.probeInFlight = false
		circuit.openedAt = r.nowLocked()
	}
}

func (r *providerResilienceLayer) recordFailure(provider WebAIProvider) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	circuit := r.circuitLocked(provider)
	now := r.nowLocked()
	circuit.lastFailureAt = now
	if circuit.state == providerCircuitHalfOpen {
		circuit.consecutiveFailures = d06ProviderFailureThreshold
	} else {
		circuit.consecutiveFailures++
	}
	circuit.probeInFlight = false
	if circuit.consecutiveFailures >= d06ProviderFailureThreshold {
		circuit.state = providerCircuitOpen
		circuit.openedAt = now
		return
	}
	circuit.state = providerCircuitDegraded
}

func (r *providerResilienceLayer) snapshot(provider WebAIProvider) providerCircuitSnapshot {
	if r == nil {
		return providerCircuitSnapshot{Provider: provider, State: providerCircuitHealthy}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	circuit := r.circuitLocked(provider)
	return providerCircuitSnapshot{
		Provider:            provider,
		State:               circuit.state,
		ConsecutiveFailures: circuit.consecutiveFailures,
		ProbeInFlight:       circuit.probeInFlight,
		OpenedAt:            circuit.openedAt,
		LastSuccessAt:       circuit.lastSuccessAt,
		LastFailureAt:       circuit.lastFailureAt,
	}
}

func (r *providerResilienceLayer) circuitLocked(provider WebAIProvider) *providerCircuit {
	circuit := r.providers[provider]
	if circuit == nil {
		circuit = &providerCircuit{state: providerCircuitHealthy}
		r.providers[provider] = circuit
	}
	return circuit
}

func (r *providerResilienceLayer) nowLocked() time.Time {
	if r.now == nil {
		return time.Now()
	}
	return r.now()
}

type runLaneRecoverer interface {
	Recover(context.Context, domain.BrowserLaneID) (webai.BrowserLaneProjection, error)
}

// resilientDualWebLaneAuthority is the D06 provider-health wrapper around the
// already-locked D04 lane authorities. It creates no scheduler, browser pool,
// resource lock, provider fallback, or Store truth of its own.
type resilientDualWebLaneAuthority struct {
	rt     *Runtime
	health *providerResilienceLayer
}

func newResilientDualWebLaneAuthority(rt *Runtime) *resilientDualWebLaneAuthority {
	return &resilientDualWebLaneAuthority{rt: rt, health: newProviderResilienceLayer()}
}

func (a *resilientDualWebLaneAuthority) Acquire(ctx context.Context, provider WebAIProvider, runID domain.RunID) error {
	if a == nil || a.rt == nil || a.rt.run == nil || a.health == nil {
		return ErrRuntimeUnavailable
	}
	if err := provider.Validate(); err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		// This request never acquired circuit admission, so it must not mutate a
		// HALF_OPEN probe that may belong to another request.
		return err
	}
	switch provider {
	case ProviderGeminiWeb:
		if a.rt.run.lanes == nil {
			return ErrRuntimeUnavailable
		}
	case ProviderChatGPTWeb:
		if a.rt.chatGPTLane == nil || a.rt.dualWeb == nil {
			return ErrRuntimeUnavailable
		}
	}
	if err := a.health.beginAdmission(provider); err != nil {
		return fmt.Errorf("%w: %s", ErrMutationPrecondition, err)
	}

	switch provider {
	case ProviderGeminiWeb:
		return a.acquireGemini(ctx, runID)
	case ProviderChatGPTWeb:
		return a.acquireChatGPT(ctx)
	default:
		return fmt.Errorf("unsupported provider lane %q", provider)
	}
}

func (a *resilientDualWebLaneAuthority) Release(provider WebAIProvider, runID domain.RunID) error {
	if a == nil || a.rt == nil {
		return ErrRuntimeUnavailable
	}
	return (runtimeDualWebLanes{rt: a.rt}).Release(provider, runID)
}

func (a *resilientDualWebLaneAuthority) acquireGemini(ctx context.Context, runID domain.RunID) error {
	lane, allocateErr := a.rt.run.lanes.Allocate(ctx, runID)
	if errors.Is(allocateErr, webai.ErrNoBrowserLaneAvailable) {
		a.health.recordNeutral(ProviderGeminiWeb)
		return allocateErr
	}
	if err := ctx.Err(); err != nil {
		a.health.recordNeutral(ProviderGeminiWeb)
		_ = a.rt.run.lanes.Release(runID)
		return err
	}
	if lane.State == webai.BrowserLaneBusy {
		a.health.recordSuccess(ProviderGeminiWeb)
		return nil
	}
	if lane.State == webai.BrowserLaneAuthRequired {
		a.health.recordNeutral(ProviderGeminiWeb)
		_ = a.rt.run.lanes.Release(runID)
		return fmt.Errorf("%w: Gemini Web authentication is required", ErrMutationPrecondition)
	}

	refreshed, refreshErr := a.rt.run.lanes.Refresh(ctx, runID)
	if refreshed.LaneID != "" {
		lane = refreshed
	}
	if err := ctx.Err(); err != nil {
		a.health.recordNeutral(ProviderGeminiWeb)
		_ = a.rt.run.lanes.Release(runID)
		return err
	}
	if lane.State == webai.BrowserLaneBusy {
		a.health.recordSuccess(ProviderGeminiWeb)
		return nil
	}
	if lane.State == webai.BrowserLaneAuthRequired {
		a.health.recordNeutral(ProviderGeminiWeb)
		_ = a.rt.run.lanes.Release(runID)
		return fmt.Errorf("%w: Gemini Web authentication is required", ErrMutationPrecondition)
	}

	var recoveryErr error
	if recoverer, ok := a.rt.run.lanes.(runLaneRecoverer); ok && lane.LaneID != "" {
		recovered, err := recoverer.Recover(ctx, lane.LaneID)
		recoveryErr = err
		if err := ctx.Err(); err != nil {
			a.health.recordNeutral(ProviderGeminiWeb)
			_ = a.rt.run.lanes.Release(runID)
			return err
		}
		if recovered.State == webai.BrowserLaneBusy {
			a.health.recordSuccess(ProviderGeminiWeb)
			return nil
		}
		if recovered.State == webai.BrowserLaneAuthRequired {
			a.health.recordNeutral(ProviderGeminiWeb)
			_ = a.rt.run.lanes.Release(runID)
			return fmt.Errorf("%w: Gemini Web authentication is required", ErrMutationPrecondition)
		}
		lane = recovered
	} else {
		recoveryErr = fmt.Errorf("Gemini Web lane recovery is unavailable")
	}

	_ = a.rt.run.lanes.Release(runID)
	finalErr := errors.Join(allocateErr, refreshErr, recoveryErr)
	if finalErr == nil {
		finalErr = fmt.Errorf("Gemini Web lane is not ready after bounded recovery (state=%s)", lane.State)
	}
	if errors.Is(finalErr, context.Canceled) || errors.Is(finalErr, context.DeadlineExceeded) {
		a.health.recordNeutral(ProviderGeminiWeb)
		return finalErr
	}
	a.health.recordFailure(ProviderGeminiWeb)
	return finalErr
}

func (a *resilientDualWebLaneAuthority) acquireChatGPT(ctx context.Context) error {
	snapshot := a.rt.chatGPTLane.Snapshot()
	var primaryErr error
	if snapshot.State == webai.SessionStopped {
		snapshot, primaryErr = a.rt.startChatGPTLane(ctx)
	} else {
		snapshot, primaryErr = a.rt.refreshChatGPTLane(ctx)
	}
	if err := ctx.Err(); err != nil {
		a.health.recordNeutral(ProviderChatGPTWeb)
		return err
	}
	if snapshot.Authenticated && snapshot.Ready {
		a.health.recordSuccess(ProviderChatGPTWeb)
		return nil
	}
	if snapshot.State == webai.SessionAuthRequired {
		a.health.recordNeutral(ProviderChatGPTWeb)
		return fmt.Errorf("%w: ChatGPT Web authentication is required", ErrMutationPrecondition)
	}

	stopErr := a.rt.stopChatGPTLane()
	if err := ctx.Err(); err != nil {
		a.health.recordNeutral(ProviderChatGPTWeb)
		return err
	}
	recovered, recoveryErr := a.rt.startChatGPTLane(ctx)
	if err := ctx.Err(); err != nil {
		a.health.recordNeutral(ProviderChatGPTWeb)
		return err
	}
	if recovered.Authenticated && recovered.Ready {
		a.health.recordSuccess(ProviderChatGPTWeb)
		return nil
	}
	if recovered.State == webai.SessionAuthRequired {
		a.health.recordNeutral(ProviderChatGPTWeb)
		return fmt.Errorf("%w: ChatGPT Web authentication is required", ErrMutationPrecondition)
	}

	finalErr := errors.Join(primaryErr, stopErr, recoveryErr)
	if finalErr == nil {
		finalErr = fmt.Errorf("ChatGPT Web lane is not ready after bounded recovery (state=%s)", recovered.State)
	}
	if errors.Is(finalErr, context.Canceled) || errors.Is(finalErr, context.DeadlineExceeded) {
		a.health.recordNeutral(ProviderChatGPTWeb)
		return finalErr
	}
	a.health.recordFailure(ProviderChatGPTWeb)
	return finalErr
}
