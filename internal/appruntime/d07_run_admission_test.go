package appruntime

import (
	"testing"

	"github.com/voocel/ainovel-cli/internal/domain"
)

func TestD07RunCenterAdmissionRoutesDurableReviewWorkToChatGPT(t *testing.T) {
	base := newFakeRunBackend()
	lanes := newFakeDualWebLanes()
	parallel := newDualWebParallelAuthority(lanes, base, "turbo")
	backend := newD07AdmissionRunBackend(base)
	backend.bindParallel(parallel)

	runID := domain.RunID("run-review")
	base.resourceOwner = runID
	base.runs[runID] = domain.RunRegistryRecord{
		RunID:      runID,
		Source:     domain.RunRecordSourceRegistry,
		ReviewWork: &domain.ReviewRunWork{Action: domain.ReviewWorkRun},
	}

	label, err := backend.Resume()
	if err != nil || label == "" {
		t.Fatalf("review admission Resume label=%q err=%v", label, err)
	}
	if got := lanes.active[ProviderChatGPTWeb]; got != runID {
		t.Fatalf("ChatGPT lane owner=%q, want %q", got, runID)
	}
	if got := lanes.active[ProviderGeminiWeb]; got != "" {
		t.Fatalf("D07 admission unexpectedly routed review work to Gemini owner=%q", got)
	}
	if got := parallel.snapshot(); got.ActiveAI != 1 || got.ChatGPTActive != 1 || got.GeminiActive != 0 {
		t.Fatalf("review admission snapshot=%+v", got)
	}

	if err := backend.DesktopReleaseRunResourceClaims(runID); err != nil {
		t.Fatalf("release review run: %v", err)
	}
	if got := parallel.snapshot(); got.ActiveJobs != 0 {
		t.Fatalf("review admission leaked after resource release: %+v", got)
	}
}

func TestD07RunCenterAdmissionRoutesOrdinaryRunToGemini(t *testing.T) {
	base := newFakeRunBackend()
	lanes := newFakeDualWebLanes()
	parallel := newDualWebParallelAuthority(lanes, base, "balanced")
	backend := newD07AdmissionRunBackend(base)
	backend.bindParallel(parallel)

	runID := domain.RunID("run-writer")
	base.resourceOwner = runID
	base.runs[runID] = domain.RunRegistryRecord{RunID: runID, Source: domain.RunRecordSourceRegistry}

	label, err := backend.Resume()
	if err != nil || label == "" {
		t.Fatalf("writer admission Resume label=%q err=%v", label, err)
	}
	if got := lanes.active[ProviderGeminiWeb]; got != runID {
		t.Fatalf("Gemini lane owner=%q, want %q", got, runID)
	}
	if got := lanes.active[ProviderChatGPTWeb]; got != "" {
		t.Fatalf("ordinary run unexpectedly routed to ChatGPT owner=%q", got)
	}
	if err := backend.DesktopReleaseRunResourceClaims(runID); err != nil {
		t.Fatalf("release writer run: %v", err)
	}
}

func TestD07RunCenterAdmissionFailsClosedWithoutCanonicalResourceOwner(t *testing.T) {
	base := newFakeRunBackend()
	parallel := newDualWebParallelAuthority(newFakeDualWebLanes(), base, "turbo")
	backend := newD07AdmissionRunBackend(base)
	backend.bindParallel(parallel)

	if _, err := backend.Resume(); err == nil {
		t.Fatal("production admission must fail closed without Run Center resource ownership")
	}
	if got := parallel.snapshot(); got.ActiveJobs != 0 {
		t.Fatalf("failed admission leaked capacity: %+v", got)
	}
}
