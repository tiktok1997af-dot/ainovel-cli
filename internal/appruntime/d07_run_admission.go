package appruntime

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/voocel/ainovel-cli/internal/domain"
)

// d07AdmissionRunBackend is a narrow adapter around the already-locked Run
// Center backend. Run Center still owns queue selection and the canonical story
// resource claim. Immediately before core Resume, this adapter admits the AI
// execution through the D04 dual-provider authority so Balanced/Turbo, provider
// lane capacity and D06 resilience are on the production path.
type d07AdmissionRunBackend struct {
	runBackend

	mu       sync.Mutex
	parallel *dualWebParallelAuthority
	active   map[domain.RunID]string
}

func newD07AdmissionRunBackend(base runBackend) *d07AdmissionRunBackend {
	return &d07AdmissionRunBackend{runBackend: base, active: make(map[domain.RunID]string)}
}

func (b *d07AdmissionRunBackend) bindParallel(parallel *dualWebParallelAuthority) {
	if b == nil {
		return
	}
	b.mu.Lock()
	b.parallel = parallel
	b.mu.Unlock()
}

func (b *d07AdmissionRunBackend) Resume() (string, error) {
	if b == nil || b.runBackend == nil {
		return "", ErrRuntimeUnavailable
	}
	runID, record, err := b.currentResourceOwner()
	if err != nil {
		return "", err
	}
	if runID == "" || record == nil {
		return "", fmt.Errorf("%w: Run Center Resume requires canonical resource ownership", ErrMutationPrecondition)
	}

	provider := ProviderGeminiWeb
	if record.ReviewWork != nil {
		provider = ProviderChatGPTWeb
	}
	jobID := d07RunAdmissionJobID(runID)

	b.mu.Lock()
	parallel := b.parallel
	_, alreadyActive := b.active[runID]
	b.mu.Unlock()
	if parallel == nil {
		return "", ErrRuntimeUnavailable
	}
	if alreadyActive {
		return "", fmt.Errorf("%w: run %q already owns D07 AI admission", ErrMutationPrecondition, runID)
	}

	if _, err := parallel.begin(context.Background(), dualWebJobSpec{
		JobID:               jobID,
		RunID:               runID,
		Kind:                dualWebJobAI,
		Provider:            provider,
		ClaimsStoryResource: false, // Run Center already owns the canonical lock.
	}); err != nil {
		return "", err
	}
	b.mu.Lock()
	b.active[runID] = jobID
	b.mu.Unlock()

	label, resumeErr := b.runBackend.Resume()
	if resumeErr != nil || label == "" {
		finishErr := b.finishAdmission(runID)
		return label, errors.Join(resumeErr, finishErr)
	}
	return label, nil
}

// DesktopReleaseRunResourceClaims is the common Run Center terminal/rollback
// seam. Release D04 provider admission before relinquishing the story lock so a
// following run cannot observe a free story resource while the previous AI lane
// is still counted active.
func (b *d07AdmissionRunBackend) DesktopReleaseRunResourceClaims(runID domain.RunID) error {
	if b == nil || b.runBackend == nil {
		return ErrRuntimeUnavailable
	}
	return errors.Join(b.finishAdmission(runID), b.runBackend.DesktopReleaseRunResourceClaims(runID))
}

func (b *d07AdmissionRunBackend) finishAdmission(runID domain.RunID) error {
	b.mu.Lock()
	jobID, ok := b.active[runID]
	parallel := b.parallel
	if ok {
		delete(b.active, runID)
	}
	b.mu.Unlock()
	if !ok {
		return nil
	}
	if parallel == nil {
		return ErrRuntimeUnavailable
	}
	return parallel.finish(jobID)
}

func (b *d07AdmissionRunBackend) currentResourceOwner() (domain.RunID, *domain.RunRegistryRecord, error) {
	resource, err := b.runBackend.DesktopStoryResourceKey()
	if err != nil {
		return "", nil, err
	}
	locks, err := b.runBackend.DesktopResourceLocks()
	if err != nil {
		return "", nil, err
	}
	var owner domain.RunID
	for _, lock := range locks {
		if lock.Resource == resource {
			owner = lock.OwnerRunID
			break
		}
	}
	if owner == "" {
		return "", nil, nil
	}
	record, err := b.runBackend.DesktopRunLoad(owner)
	return owner, record, err
}

func d07RunAdmissionJobID(runID domain.RunID) string {
	return "run-center:" + string(runID)
}
