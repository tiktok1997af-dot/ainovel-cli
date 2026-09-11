package store

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/voocel/ainovel-cli/internal/domain"
)

const storyResourceKeyPrefix = "story:"

type resourceLockReplay struct {
	states map[domain.ResourceKey]domain.ResourceLockState
}

// CanonicalStoryResourceKey derives the one conservative G05.5 story resource
// for this project Store. The persisted key is opaque and never contains a raw
// project path. Finer-grained resource authority remains closed until proven safe.
func (s *RuntimeStore) CanonicalStoryResourceKey() (domain.ResourceKey, error) {
	return canonicalStoryResourceKey(s.io.dir)
}

func canonicalStoryResourceKey(root string) (domain.ResourceKey, error) {
	if strings.TrimSpace(root) == "" {
		return "", fmt.Errorf("project root is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve project root: %w", err)
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("canonicalize project root: %w", err)
	}
	canonical := filepath.Clean(real)
	if runtime.GOOS == "windows" {
		canonical = strings.ToLower(canonical)
	}
	canonical = filepath.ToSlash(canonical)
	sum := sha256.Sum256([]byte("ainovel.story.resource.v1\x00" + canonical))
	key := domain.ResourceKey(storyResourceKeyPrefix + hex.EncodeToString(sum[:]))
	if err := (domain.RunIdentity{RunID: "resource-key-check", Resource: key}).Validate(); err != nil {
		return "", fmt.Errorf("derived resource key: %w", err)
	}
	return key, nil
}

// LoadResourceLocks reconstructs lock ownership and deterministic FIFO waiters
// entirely from append-only queue.jsonl facts.
func (s *RuntimeStore) LoadResourceLocks() ([]domain.ResourceLockState, error) {
	items, err := s.LoadQueue()
	if err != nil {
		return nil, err
	}
	replay, err := replayResourceLocks(items)
	if err != nil {
		return nil, err
	}
	return sortedResourceLockStates(replay), nil
}

// RequestRunResourceLock either acquires the resource or persists one FIFO wait
// claim. Repeating the same request is idempotent. A free resource with existing
// waiters may only be acquired by the oldest persisted waiter.
func (s *RuntimeStore) RequestRunResourceLock(runID domain.RunID, resource domain.ResourceKey) (domain.ResourceLockDecision, error) {
	if err := validateResourceLockIdentity(runID, resource); err != nil {
		return domain.ResourceLockDecision{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	record, err := s.loadRunRecord(runID)
	if err != nil {
		return domain.ResourceLockDecision{}, err
	}
	if record == nil {
		return domain.ResourceLockDecision{}, fmt.Errorf("run %q is not registered", runID)
	}
	replay, err := s.loadResourceLocksLocked()
	if err != nil {
		return domain.ResourceLockDecision{}, err
	}
	state := replay.states[resource]

	if state.OwnerRunID == runID {
		return domain.ResourceLockDecision{
			Status:     domain.ResourceLockStatusAcquired,
			Resource:   resource,
			RunID:      runID,
			OwnerRunID: runID,
			AcquireSeq: state.AcquireSeq,
		}, nil
	}

	waitIndex := resourceWaiterIndex(state.Waiters, runID)
	if state.OwnerRunID == "" && len(state.Waiters) == 0 {
		return s.acquireResourceLockLocked(runID, resource)
	}
	if state.OwnerRunID == "" && waitIndex == 0 {
		return s.acquireResourceLockLocked(runID, resource)
	}
	if waitIndex >= 0 {
		waiter := state.Waiters[waitIndex]
		return domain.ResourceLockDecision{
			Status:       domain.ResourceLockStatusWaiting,
			Resource:     resource,
			RunID:        runID,
			OwnerRunID:   state.OwnerRunID,
			WaitPosition: waitIndex + 1,
			RequestSeq:   waiter.RequestSeq,
		}, nil
	}

	item, err := s.appendResourceQueueLocked(domain.RuntimeQueueItem{
		Priority: domain.RuntimePriorityControl,
		RunID:    runID,
		Resource: resource,
		Category: domain.RuntimeQueueCategoryResourceWaitRequested,
		Summary:  "run waiting for story resource",
	})
	if err != nil {
		return domain.ResourceLockDecision{}, err
	}
	return domain.ResourceLockDecision{
		Status:       domain.ResourceLockStatusWaiting,
		Resource:     resource,
		RunID:        runID,
		OwnerRunID:   state.OwnerRunID,
		WaitPosition: len(state.Waiters) + 1,
		RequestSeq:   item.Seq,
	}, nil
}

// ReleaseRunResourceLock releases only the current owner. Releasing an already
// free resource is idempotent only when the caller is not still queued for it.
func (s *RuntimeStore) ReleaseRunResourceLock(runID domain.RunID, resource domain.ResourceKey) error {
	if err := validateResourceLockIdentity(runID, resource); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	replay, err := s.loadResourceLocksLocked()
	if err != nil {
		return err
	}
	state := replay.states[resource]
	if state.OwnerRunID == "" {
		if resourceWaiterIndex(state.Waiters, runID) >= 0 {
			return fmt.Errorf("run %q is waiting for resource %q and does not own it", runID, resource)
		}
		return nil
	}
	if state.OwnerRunID != runID {
		return fmt.Errorf("resource %q is owned by run %q", resource, state.OwnerRunID)
	}
	_, err = s.appendResourceQueueLocked(domain.RuntimeQueueItem{
		Priority: domain.RuntimePriorityControl,
		RunID:    runID,
		Resource: resource,
		Category: domain.RuntimeQueueCategoryResourceReleased,
		Summary:  "story resource released",
	})
	return err
}

// CancelRunResourceWait removes one persisted waiting claim without disturbing
// the owner or other waiters. Missing wait claims are idempotent.
func (s *RuntimeStore) CancelRunResourceWait(runID domain.RunID, resource domain.ResourceKey) error {
	if err := validateResourceLockIdentity(runID, resource); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	replay, err := s.loadResourceLocksLocked()
	if err != nil {
		return err
	}
	state := replay.states[resource]
	if resourceWaiterIndex(state.Waiters, runID) < 0 {
		return nil
	}
	_, err = s.appendResourceQueueLocked(domain.RuntimeQueueItem{
		Priority: domain.RuntimePriorityControl,
		RunID:    runID,
		Resource: resource,
		Category: domain.RuntimeQueueCategoryResourceWaitCancelled,
		Summary:  "resource wait cancelled",
	})
	return err
}

// ReleaseAllRunResourceClaims is the terminal-run cleanup primitive for later
// orchestration: held resources are released and queued claims are cancelled.
func (s *RuntimeStore) ReleaseAllRunResourceClaims(runID domain.RunID) error {
	if err := validateSchedulableRunIdentity(runID); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	replay, err := s.loadResourceLocksLocked()
	if err != nil {
		return err
	}
	states := sortedResourceLockStates(replay)
	for _, state := range states {
		if state.OwnerRunID == runID {
			if _, err := s.appendResourceQueueLocked(domain.RuntimeQueueItem{
				Priority: domain.RuntimePriorityControl,
				RunID:    runID,
				Resource: state.Resource,
				Category: domain.RuntimeQueueCategoryResourceReleased,
				Summary:  "story resource released during run cleanup",
			}); err != nil {
				return err
			}
		}
		if resourceWaiterIndex(state.Waiters, runID) >= 0 {
			if _, err := s.appendResourceQueueLocked(domain.RuntimeQueueItem{
				Priority: domain.RuntimePriorityControl,
				RunID:    runID,
				Resource: state.Resource,
				Category: domain.RuntimeQueueCategoryResourceWaitCancelled,
				Summary:  "resource wait cancelled during run cleanup",
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

// RecoverResourceLocks performs crash/restart cleanup. Because no mutating work
// survives the owning desktop process, every replayed owner is released with an
// explicit recovery fact. FIFO wait claims are preserved for later orchestration.
func (s *RuntimeStore) RecoverResourceLocks() ([]domain.ResourceLockState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	replay, err := s.loadResourceLocksLocked()
	if err != nil {
		return nil, err
	}
	states := sortedResourceLockStates(replay)
	released := make([]domain.ResourceLockState, 0)
	for _, state := range states {
		if state.OwnerRunID == "" {
			continue
		}
		if _, err := s.appendResourceQueueLocked(domain.RuntimeQueueItem{
			Priority: domain.RuntimePriorityControl,
			RunID:    state.OwnerRunID,
			Resource: state.Resource,
			Category: domain.RuntimeQueueCategoryResourceRecoveryReleased,
			Summary:  "stale story resource released during recovery",
		}); err != nil {
			return nil, err
		}
		released = append(released, state)
	}
	return released, nil
}

func (s *RuntimeStore) acquireResourceLockLocked(runID domain.RunID, resource domain.ResourceKey) (domain.ResourceLockDecision, error) {
	item, err := s.appendResourceQueueLocked(domain.RuntimeQueueItem{
		Priority: domain.RuntimePriorityControl,
		RunID:    runID,
		Resource: resource,
		Category: domain.RuntimeQueueCategoryResourceAcquired,
		Summary:  "story resource acquired",
	})
	if err != nil {
		return domain.ResourceLockDecision{}, err
	}
	return domain.ResourceLockDecision{
		Status:     domain.ResourceLockStatusAcquired,
		Resource:   resource,
		RunID:      runID,
		OwnerRunID: runID,
		AcquireSeq: item.Seq,
	}, nil
}

func (s *RuntimeStore) loadResourceLocksLocked() (resourceLockReplay, error) {
	items, err := loadJSONLines[domain.RuntimeQueueItem](s.io, runtimeQueuePath)
	if err != nil {
		return resourceLockReplay{}, err
	}
	return replayResourceLocks(items)
}

func replayResourceLocks(items []domain.RuntimeQueueItem) (resourceLockReplay, error) {
	replay := resourceLockReplay{states: make(map[domain.ResourceKey]domain.ResourceLockState)}
	for _, item := range items {
		if !domain.IsResourceLockQueueCategory(item.Category) {
			continue
		}
		if item.Seq <= 0 {
			return resourceLockReplay{}, fmt.Errorf("resource lock record has invalid seq %d", item.Seq)
		}
		if item.Priority != domain.RuntimePriorityControl {
			return resourceLockReplay{}, fmt.Errorf("resource lock record %d must use control audit priority", item.Seq)
		}
		if err := validateResourceLockIdentity(item.RunID, item.Resource); err != nil {
			return resourceLockReplay{}, fmt.Errorf("resource lock record %d: %w", item.Seq, err)
		}

		state := replay.states[item.Resource]
		state.Resource = item.Resource
		switch item.Category {
		case domain.RuntimeQueueCategoryResourceWaitRequested:
			if state.OwnerRunID == item.RunID {
				return resourceLockReplay{}, fmt.Errorf("resource wait %d is owned already by run %q", item.Seq, item.RunID)
			}
			if resourceWaiterIndex(state.Waiters, item.RunID) >= 0 {
				return resourceLockReplay{}, fmt.Errorf("resource wait %d duplicates run %q", item.Seq, item.RunID)
			}
			state.Waiters = append(state.Waiters, domain.ResourceLockWaiter{
				RunID:      item.RunID,
				RequestSeq: item.Seq,
			})
		case domain.RuntimeQueueCategoryResourceAcquired:
			if state.OwnerRunID != "" {
				return resourceLockReplay{}, fmt.Errorf("resource acquire %d would double-own %q", item.Seq, item.Resource)
			}
			if len(state.Waiters) > 0 {
				if state.Waiters[0].RunID != item.RunID {
					return resourceLockReplay{}, fmt.Errorf("resource acquire %d bypasses oldest waiter %q", item.Seq, state.Waiters[0].RunID)
				}
				state.Waiters = append([]domain.ResourceLockWaiter(nil), state.Waiters[1:]...)
			}
			state.OwnerRunID = item.RunID
			state.AcquireSeq = item.Seq
		case domain.RuntimeQueueCategoryResourceReleased, domain.RuntimeQueueCategoryResourceRecoveryReleased:
			if state.OwnerRunID != item.RunID {
				return resourceLockReplay{}, fmt.Errorf("resource release %d owner mismatch: owner=%q run=%q", item.Seq, state.OwnerRunID, item.RunID)
			}
			state.OwnerRunID = ""
			state.AcquireSeq = 0
		case domain.RuntimeQueueCategoryResourceWaitCancelled:
			idx := resourceWaiterIndex(state.Waiters, item.RunID)
			if idx < 0 {
				return resourceLockReplay{}, fmt.Errorf("resource wait cancel %d references non-waiting run %q", item.Seq, item.RunID)
			}
			state.Waiters = append(state.Waiters[:idx], state.Waiters[idx+1:]...)
		}
		replay.states[item.Resource] = state
	}
	return replay, nil
}

func validateResourceLockIdentity(runID domain.RunID, resource domain.ResourceKey) error {
	if resource == "" {
		return fmt.Errorf("resource is required")
	}
	if err := (domain.RunIdentity{RunID: runID, Resource: resource}).Validate(); err != nil {
		return err
	}
	if runID == domain.LegacySingleRunID {
		return fmt.Errorf("run_id %q is a read-only legacy projection", runID)
	}
	return nil
}

func resourceWaiterIndex(waiters []domain.ResourceLockWaiter, runID domain.RunID) int {
	for i := range waiters {
		if waiters[i].RunID == runID {
			return i
		}
	}
	return -1
}

func sortedResourceLockStates(replay resourceLockReplay) []domain.ResourceLockState {
	out := make([]domain.ResourceLockState, 0, len(replay.states))
	for _, state := range replay.states {
		if state.OwnerRunID == "" && len(state.Waiters) == 0 {
			continue
		}
		state.Waiters = append([]domain.ResourceLockWaiter(nil), state.Waiters...)
		out = append(out, state)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Resource < out[j].Resource })
	return out
}

func (s *RuntimeStore) appendResourceQueueLocked(item domain.RuntimeQueueItem) (domain.RuntimeQueueItem, error) {
	if err := s.ensureSeqLoadedLocked(); err != nil {
		return item, err
	}
	s.nextSeqNum++
	item.Seq = s.nextSeqNum
	if item.Time.IsZero() {
		item.Time = time.Now().UTC()
	}
	if err := s.appendJSONLine(runtimeQueuePath, item); err != nil {
		return item, err
	}
	return item, nil
}
