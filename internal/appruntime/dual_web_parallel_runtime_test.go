package appruntime

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/voocel/ainovel-cli/internal/domain"
)

type fakeDualWebLanes struct {
	mu       sync.Mutex
	active   map[WebAIProvider]domain.RunID
	releases []string
}

func newFakeDualWebLanes() *fakeDualWebLanes {
	return &fakeDualWebLanes{active: make(map[WebAIProvider]domain.RunID)}
}

func (f *fakeDualWebLanes) Acquire(_ context.Context, provider WebAIProvider, runID domain.RunID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if owner := f.active[provider]; owner != "" && owner != runID {
		return errors.New("provider lane already active")
	}
	f.active[provider] = runID
	return nil
}

func (f *fakeDualWebLanes) Release(provider WebAIProvider, runID domain.RunID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.active[provider] == runID {
		delete(f.active, provider)
	}
	f.releases = append(f.releases, string(provider)+":"+string(runID))
	return nil
}

func TestD04ParallelAuthorityRunsTwoProviderLanesConcurrently(t *testing.T) {
	lanes := newFakeDualWebLanes()
	backend := newFakeRunBackend()
	authority := newDualWebParallelAuthority(lanes, backend)

	type result struct {
		ctx context.Context
		err error
	}
	start := make(chan struct{})
	results := make(chan result, 2)
	for _, spec := range []dualWebJobSpec{
		{JobID: "ai-gemini", RunID: "run-001", Kind: dualWebJobAI, Provider: ProviderGeminiWeb},
		{JobID: "ai-chatgpt", RunID: "run-002", Kind: dualWebJobAI, Provider: ProviderChatGPTWeb},
	} {
		spec := spec
		go func() {
			<-start
			ctx, err := authority.begin(context.Background(), spec)
			results <- result{ctx: ctx, err: err}
		}()
	}
	close(start)
	for i := 0; i < 2; i++ {
		if got := <-results; got.err != nil || got.ctx == nil {
			t.Fatalf("parallel provider admission failed: ctx=%v err=%v", got.ctx, got.err)
		}
	}

	snapshot := authority.snapshot()
	if snapshot.ActiveAI != 2 || snapshot.GeminiActive != 1 || snapshot.ChatGPTActive != 1 {
		t.Fatalf("unexpected provider concurrency snapshot: %+v", snapshot)
	}
	if _, err := authority.begin(context.Background(), dualWebJobSpec{
		JobID: "ai-third", RunID: "run-003", Kind: dualWebJobAI, Provider: ProviderGeminiWeb,
	}); err == nil {
		t.Fatal("third AI admission must fail closed")
	}
	if err := authority.finish("ai-gemini"); err != nil {
		t.Fatalf("finish gemini: %v", err)
	}
	if err := authority.finish("ai-chatgpt"); err != nil {
		t.Fatalf("finish chatgpt: %v", err)
	}
}

func TestD04ParallelAuthorityEnforcesTwoLocalWorkersAndFourTotalJobs(t *testing.T) {
	lanes := newFakeDualWebLanes()
	authority := newDualWebParallelAuthority(lanes, newFakeRunBackend())

	specs := []dualWebJobSpec{
		{JobID: "ai-1", RunID: "run-001", Kind: dualWebJobAI, Provider: ProviderGeminiWeb},
		{JobID: "ai-2", RunID: "run-002", Kind: dualWebJobAI, Provider: ProviderChatGPTWeb},
		{JobID: "local-1", RunID: "run-003", Kind: dualWebJobLocal},
		{JobID: "local-2", RunID: "run-004", Kind: dualWebJobLocal},
	}
	for _, spec := range specs {
		if _, err := authority.begin(context.Background(), spec); err != nil {
			t.Fatalf("begin %s: %v", spec.JobID, err)
		}
	}
	snapshot := authority.snapshot()
	if snapshot.ActiveJobs != DualWebMaxActiveJobs || snapshot.ActiveAI != DualWebAIConcurrency || snapshot.ActiveLocal != DualWebLocalWorkers {
		t.Fatalf("bounded capacity mismatch: %+v", snapshot)
	}
	if _, err := authority.begin(context.Background(), dualWebJobSpec{
		JobID: "local-3", RunID: "run-005", Kind: dualWebJobLocal,
	}); err == nil {
		t.Fatal("fifth active job must be rejected")
	}
	for _, spec := range specs {
		if err := authority.finish(spec.JobID); err != nil {
			t.Fatalf("finish %s: %v", spec.JobID, err)
		}
	}
}

func TestD04ParallelAuthorityResourceIsolationUsesExistingRunLocks(t *testing.T) {
	backend := newFakeRunBackend()
	authority := newDualWebParallelAuthority(newFakeDualWebLanes(), backend)

	ctx1, err := authority.begin(context.Background(), dualWebJobSpec{
		JobID: "writer-1", RunID: "run-001", Kind: dualWebJobLocal, ClaimsStoryResource: true,
	})
	if err != nil || ctx1 == nil {
		t.Fatalf("first resource writer: ctx=%v err=%v", ctx1, err)
	}
	if backend.resourceOwner != "run-001" {
		t.Fatalf("resource owner=%q, want run-001", backend.resourceOwner)
	}
	if _, err := authority.begin(context.Background(), dualWebJobSpec{
		JobID: "writer-2", RunID: "run-002", Kind: dualWebJobLocal, ClaimsStoryResource: true,
	}); err == nil {
		t.Fatal("conflicting writer must wait/fail admission")
	}
	if authority.snapshot().ActiveJobs != 1 || backend.resourceOwner != "run-001" {
		t.Fatalf("failed admission disturbed owner: snapshot=%+v owner=%q", authority.snapshot(), backend.resourceOwner)
	}
	if err := authority.finish("writer-1"); err != nil {
		t.Fatalf("finish writer-1: %v", err)
	}
	if _, err := authority.begin(context.Background(), dualWebJobSpec{
		JobID: "writer-2", RunID: "run-002", Kind: dualWebJobLocal, ClaimsStoryResource: true,
	}); err != nil {
		t.Fatalf("writer-2 should acquire after release: %v", err)
	}
	if err := authority.finish("writer-2"); err != nil {
		t.Fatalf("finish writer-2: %v", err)
	}
}

func TestD04ParallelAuthorityCancelIsJobScoped(t *testing.T) {
	authority := newDualWebParallelAuthority(newFakeDualWebLanes(), newFakeRunBackend())
	ctx1, err := authority.begin(context.Background(), dualWebJobSpec{JobID: "local-1", RunID: "run-001", Kind: dualWebJobLocal})
	if err != nil {
		t.Fatalf("begin local-1: %v", err)
	}
	ctx2, err := authority.begin(context.Background(), dualWebJobSpec{JobID: "local-2", RunID: "run-002", Kind: dualWebJobLocal})
	if err != nil {
		t.Fatalf("begin local-2: %v", err)
	}
	if err := authority.cancel("local-1"); err != nil {
		t.Fatalf("cancel local-1: %v", err)
	}
	select {
	case <-ctx1.Done():
	case <-time.After(time.Second):
		t.Fatal("cancelled job context did not close")
	}
	select {
	case <-ctx2.Done():
		t.Fatal("cancel crossed into unrelated job")
	default:
	}
	if got := authority.snapshot(); got.ActiveJobs != 1 || got.ActiveLocal != 1 {
		t.Fatalf("cancel isolation snapshot: %+v", got)
	}
	if err := authority.finish("local-2"); err != nil {
		t.Fatalf("finish local-2: %v", err)
	}
}

func TestD04ParallelAuthorityShutdownCancelsAllAndRejectsNewWork(t *testing.T) {
	authority := newDualWebParallelAuthority(newFakeDualWebLanes(), newFakeRunBackend())
	contexts := make([]context.Context, 0, 4)
	for _, spec := range []dualWebJobSpec{
		{JobID: "ai-1", RunID: "run-001", Kind: dualWebJobAI, Provider: ProviderGeminiWeb},
		{JobID: "ai-2", RunID: "run-002", Kind: dualWebJobAI, Provider: ProviderChatGPTWeb},
		{JobID: "local-1", RunID: "run-003", Kind: dualWebJobLocal},
		{JobID: "local-2", RunID: "run-004", Kind: dualWebJobLocal},
	} {
		ctx, err := authority.begin(context.Background(), spec)
		if err != nil {
			t.Fatalf("begin %s: %v", spec.JobID, err)
		}
		contexts = append(contexts, ctx)
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := authority.shutdown(shutdownCtx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	for i, ctx := range contexts {
		select {
		case <-ctx.Done():
		default:
			t.Fatalf("job context %d survived shutdown", i)
		}
	}
	snapshot := authority.snapshot()
	if !snapshot.ShuttingDown || snapshot.ActiveJobs != 0 || snapshot.ActiveAI != 0 || snapshot.ActiveLocal != 0 {
		t.Fatalf("shutdown leaked capacity: %+v", snapshot)
	}
	if _, err := authority.begin(context.Background(), dualWebJobSpec{
		JobID: "after-shutdown", RunID: "run-005", Kind: dualWebJobLocal,
	}); !errors.Is(err, ErrClosed) {
		t.Fatalf("new work after shutdown err=%v, want ErrClosed", err)
	}
}
