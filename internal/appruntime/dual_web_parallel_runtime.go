package appruntime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/webai"
)

// D04 keeps parallel execution behind the existing Run Center authority. It is
// intentionally internal: D05 owns desktop START/model-gate wiring.
type dualWebJobKind string

const (
	dualWebJobAI    dualWebJobKind = "ai"
	dualWebJobLocal dualWebJobKind = "local"
)

type dualWebJobSpec struct {
	JobID               string
	RunID               domain.RunID
	Kind                dualWebJobKind
	Provider            WebAIProvider
	ClaimsStoryResource bool
}

type dualWebParallelSnapshot struct {
	ActiveJobs    int
	ActiveAI      int
	ActiveLocal   int
	GeminiActive  int
	ChatGPTActive int
	ShuttingDown  bool
}

type dualWebLaneAuthority interface {
	Acquire(context.Context, WebAIProvider, domain.RunID) error
	Release(WebAIProvider, domain.RunID) error
}

type dualWebResourceAuthority interface {
	DesktopStoryResourceKey() (domain.ResourceKey, error)
	DesktopRequestRunResource(domain.RunID, domain.ResourceKey) (domain.ResourceLockDecision, error)
	DesktopReleaseRunResourceClaims(domain.RunID) error
}

type dualWebActiveJob struct {
	spec   dualWebJobSpec
	ctx    context.Context
	cancel context.CancelFunc
}

type dualWebParallelAuthority struct {
	lanes     dualWebLaneAuthority
	resources dualWebResourceAuthority

	mu             sync.Mutex
	active         map[string]*dualWebActiveJob
	providerActive map[WebAIProvider]int
	localActive    int
	resourceRuns   map[domain.RunID]struct{}
	shuttingDown   bool
	wg             sync.WaitGroup
}

func newDualWebParallelAuthority(lanes dualWebLaneAuthority, resources dualWebResourceAuthority) *dualWebParallelAuthority {
	return &dualWebParallelAuthority{
		lanes:          lanes,
		resources:      resources,
		active:         make(map[string]*dualWebActiveJob),
		providerActive: make(map[WebAIProvider]int),
		resourceRuns:   make(map[domain.RunID]struct{}),
	}
}

func (a *dualWebParallelAuthority) begin(ctx context.Context, spec dualWebJobSpec) (context.Context, error) {
	if a == nil || a.lanes == nil || a.resources == nil {
		return nil, ErrRuntimeUnavailable
	}
	if err := validateDualWebJobSpec(spec); err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	a.mu.Lock()
	if a.shuttingDown {
		a.mu.Unlock()
		return nil, ErrClosed
	}
	if _, exists := a.active[spec.JobID]; exists {
		a.mu.Unlock()
		return nil, fmt.Errorf("dual-web job %q is already active", spec.JobID)
	}
	if len(a.active) >= DualWebMaxActiveJobs {
		a.mu.Unlock()
		return nil, fmt.Errorf("dual-web active job capacity reached")
	}
	if spec.Kind == dualWebJobAI {
		if a.activeAICountLocked() >= DualWebAIConcurrency {
			a.mu.Unlock()
			return nil, fmt.Errorf("dual-web AI concurrency reached")
		}
		if a.providerActive[spec.Provider] >= DualWebAILaneCapacity {
			a.mu.Unlock()
			return nil, fmt.Errorf("provider lane %q is busy", spec.Provider)
		}
	} else if a.localActive >= DualWebLocalWorkers {
		a.mu.Unlock()
		return nil, fmt.Errorf("dual-web local worker capacity reached")
	}
	if spec.ClaimsStoryResource {
		if _, exists := a.resourceRuns[spec.RunID]; exists {
			a.mu.Unlock()
			return nil, fmt.Errorf("run %q already owns a D04 resource-writing job", spec.RunID)
		}
	}
	// Reserve capacity before touching external lane/resource authorities. This
	// makes simultaneous admission deterministic and prevents oversubscription.
	if spec.Kind == dualWebJobAI {
		a.providerActive[spec.Provider]++
	} else {
		a.localActive++
	}
	if spec.ClaimsStoryResource {
		a.resourceRuns[spec.RunID] = struct{}{}
	}
	a.mu.Unlock()

	rollback := func() {
		a.mu.Lock()
		if spec.Kind == dualWebJobAI {
			a.providerActive[spec.Provider]--
		} else {
			a.localActive--
		}
		if spec.ClaimsStoryResource {
			delete(a.resourceRuns, spec.RunID)
		}
		a.mu.Unlock()
	}

	resourceClaimed := false
	if spec.ClaimsStoryResource {
		resource, err := a.resources.DesktopStoryResourceKey()
		if err != nil {
			rollback()
			return nil, err
		}
		decision, err := a.resources.DesktopRequestRunResource(spec.RunID, resource)
		if err != nil {
			rollback()
			return nil, err
		}
		if decision.Status == domain.ResourceLockStatusWaiting {
			_ = a.resources.DesktopReleaseRunResourceClaims(spec.RunID)
			rollback()
			return nil, fmt.Errorf("run %q is waiting for canonical story resource", spec.RunID)
		}
		resourceClaimed = true
	}

	laneAcquired := false
	if spec.Kind == dualWebJobAI {
		if err := a.lanes.Acquire(ctx, spec.Provider, spec.RunID); err != nil {
			if resourceClaimed {
				_ = a.resources.DesktopReleaseRunResourceClaims(spec.RunID)
			}
			rollback()
			return nil, err
		}
		laneAcquired = true
	}

	jobCtx, cancel := context.WithCancel(ctx)
	job := &dualWebActiveJob{spec: spec, ctx: jobCtx, cancel: cancel}
	a.mu.Lock()
	if a.shuttingDown {
		a.mu.Unlock()
		cancel()
		if laneAcquired {
			_ = a.lanes.Release(spec.Provider, spec.RunID)
		}
		if resourceClaimed {
			_ = a.resources.DesktopReleaseRunResourceClaims(spec.RunID)
		}
		rollback()
		return nil, ErrClosed
	}
	a.active[spec.JobID] = job
	a.wg.Add(1)
	a.mu.Unlock()
	return jobCtx, nil
}

func (a *dualWebParallelAuthority) finish(jobID string) error {
	return a.release(jobID, false)
}

func (a *dualWebParallelAuthority) cancel(jobID string) error {
	return a.release(jobID, true)
}

func (a *dualWebParallelAuthority) release(jobID string, cancel bool) error {
	if a == nil {
		return ErrRuntimeUnavailable
	}
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return fmt.Errorf("dual-web job id is required")
	}

	a.mu.Lock()
	job, ok := a.active[jobID]
	if !ok {
		a.mu.Unlock()
		return fmt.Errorf("dual-web job %q is not active", jobID)
	}
	delete(a.active, jobID)
	if job.spec.Kind == dualWebJobAI {
		a.providerActive[job.spec.Provider]--
	} else {
		a.localActive--
	}
	if job.spec.ClaimsStoryResource {
		delete(a.resourceRuns, job.spec.RunID)
	}
	a.mu.Unlock()

	if cancel {
		job.cancel()
	}
	var errs []error
	if job.spec.Kind == dualWebJobAI {
		if err := a.lanes.Release(job.spec.Provider, job.spec.RunID); err != nil {
			errs = append(errs, err)
		}
	}
	if job.spec.ClaimsStoryResource {
		if err := a.resources.DesktopReleaseRunResourceClaims(job.spec.RunID); err != nil {
			errs = append(errs, err)
		}
	}
	job.cancel()
	a.wg.Done()
	return errors.Join(errs...)
}

func (a *dualWebParallelAuthority) snapshot() dualWebParallelSnapshot {
	if a == nil {
		return dualWebParallelSnapshot{}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return dualWebParallelSnapshot{
		ActiveJobs:    len(a.active),
		ActiveAI:      a.activeAICountLocked(),
		ActiveLocal:   a.localActive,
		GeminiActive:  a.providerActive[ProviderGeminiWeb],
		ChatGPTActive: a.providerActive[ProviderChatGPTWeb],
		ShuttingDown:  a.shuttingDown,
	}
}

func (a *dualWebParallelAuthority) shutdown(ctx context.Context) error {
	if a == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	a.mu.Lock()
	if a.shuttingDown && len(a.active) == 0 {
		a.mu.Unlock()
		return nil
	}
	a.shuttingDown = true
	ids := make([]string, 0, len(a.active))
	for id := range a.active {
		ids = append(ids, id)
	}
	a.mu.Unlock()

	var errs []error
	for _, id := range ids {
		if err := a.cancel(id); err != nil {
			errs = append(errs, err)
		}
	}
	done := make(chan struct{})
	go func() {
		a.wg.Wait()
		close(done)
	}()
	select {
	case <-ctx.Done():
		errs = append(errs, ctx.Err())
	case <-done:
	}
	return errors.Join(errs...)
}

func (a *dualWebParallelAuthority) activeAICountLocked() int {
	return a.providerActive[ProviderGeminiWeb] + a.providerActive[ProviderChatGPTWeb]
}

func validateDualWebJobSpec(spec dualWebJobSpec) error {
	if strings.TrimSpace(spec.JobID) == "" {
		return fmt.Errorf("dual-web job id is required")
	}
	if err := (domain.RunIdentity{RunID: spec.RunID}).Validate(); err != nil || spec.RunID == domain.LegacySingleRunID {
		return fmt.Errorf("invalid dual-web run identity")
	}
	switch spec.Kind {
	case dualWebJobAI:
		if err := spec.Provider.Validate(); err != nil {
			return err
		}
	case dualWebJobLocal:
		if spec.Provider != "" {
			return fmt.Errorf("local worker job may not own a provider lane")
		}
	default:
		return fmt.Errorf("invalid dual-web job kind %q", spec.Kind)
	}
	return nil
}

// runtimeDualWebLanes binds D04 to the exact provider lanes already owned by
// AppRuntime/Run Center. It does not create another scheduler or browser pool.
type runtimeDualWebLanes struct {
	rt *Runtime
}

func (l runtimeDualWebLanes) Acquire(ctx context.Context, provider WebAIProvider, runID domain.RunID) error {
	if l.rt == nil || l.rt.run == nil {
		return ErrRuntimeUnavailable
	}
	switch provider {
	case ProviderGeminiWeb:
		lane, err := l.rt.run.lanes.Allocate(ctx, runID)
		if err != nil {
			return err
		}
		if lane.State != webai.BrowserLaneBusy {
			refreshed, refreshErr := l.rt.run.lanes.Refresh(ctx, runID)
			if refreshErr != nil {
				_ = l.rt.run.lanes.Release(runID)
				return refreshErr
			}
			lane = refreshed
		}
		if lane.State != webai.BrowserLaneBusy {
			_ = l.rt.run.lanes.Release(runID)
			return fmt.Errorf("Gemini Web lane is not ready for execution")
		}
		return nil
	case ProviderChatGPTWeb:
		snapshot, err := l.rt.startChatGPTLane(ctx)
		if err != nil {
			return err
		}
		if !snapshot.Ready {
			return fmt.Errorf("ChatGPT Web lane is not ready for execution")
		}
		return nil
	default:
		return fmt.Errorf("unsupported provider lane %q", provider)
	}
}

func (l runtimeDualWebLanes) Release(provider WebAIProvider, runID domain.RunID) error {
	if l.rt == nil || l.rt.run == nil {
		return ErrRuntimeUnavailable
	}
	switch provider {
	case ProviderGeminiWeb:
		return l.rt.run.lanes.Release(runID)
	case ProviderChatGPTWeb:
		// Keep the authenticated ChatGPT browser alive between turns. The D04
		// authority releases only the capacity token; Runtime.Close owns browser
		// shutdown and D06 will own provider recovery policy.
		return nil
	default:
		return fmt.Errorf("unsupported provider lane %q", provider)
	}
}
