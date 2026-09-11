package store

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/voocel/ainovel-cli/internal/domain"
)

func TestRuntimeResourceKeyCanonicalOpaqueAndStable(t *testing.T) {
	dir := t.TempDir()
	runtimeA := NewRuntimeStore(newIO(dir))
	runtimeB := NewRuntimeStore(newIO(filepath.Join(dir, "child", "..")))

	if err := runtimeA.io.EnsureDirs([]string{"child"}); err != nil {
		t.Fatal(err)
	}
	keyA, err := runtimeA.CanonicalStoryResourceKey()
	if err != nil {
		t.Fatal(err)
	}
	keyB, err := runtimeB.CanonicalStoryResourceKey()
	if err != nil {
		t.Fatal(err)
	}
	if keyA != keyB {
		t.Fatalf("canonical aliases produced different keys: %q != %q", keyA, keyB)
	}
	if !strings.HasPrefix(string(keyA), storyResourceKeyPrefix) || strings.ContainsAny(string(keyA), `/\`) {
		t.Fatalf("resource key is not opaque/path-free: %q", keyA)
	}

	other := NewRuntimeStore(newIO(t.TempDir()))
	keyOther, err := other.CanonicalStoryResourceKey()
	if err != nil {
		t.Fatal(err)
	}
	if keyOther == keyA {
		t.Fatalf("different project roots collided: %q", keyA)
	}
}

func TestRuntimeResourceLockMutualExclusionFIFOAndRestartReplay(t *testing.T) {
	dir := t.TempDir()
	runtime := NewRuntimeStore(newIO(dir))
	for _, runID := range []domain.RunID{"run-a", "run-b", "run-c"} {
		if _, err := runtime.SaveRun(domain.RunRegistryRecord{RunID: runID}); err != nil {
			t.Fatal(err)
		}
	}
	resource, err := runtime.CanonicalStoryResourceKey()
	if err != nil {
		t.Fatal(err)
	}

	a, err := runtime.RequestRunResourceLock("run-a", resource)
	if err != nil {
		t.Fatal(err)
	}
	if a.Status != domain.ResourceLockStatusAcquired || a.OwnerRunID != "run-a" {
		t.Fatalf("run-a did not acquire resource: %#v", a)
	}
	aAgain, err := runtime.RequestRunResourceLock("run-a", resource)
	if err != nil {
		t.Fatal(err)
	}
	if aAgain != a {
		t.Fatalf("idempotent owner request drifted: first=%#v again=%#v", a, aAgain)
	}

	b, err := runtime.RequestRunResourceLock("run-b", resource)
	if err != nil {
		t.Fatal(err)
	}
	c, err := runtime.RequestRunResourceLock("run-c", resource)
	if err != nil {
		t.Fatal(err)
	}
	if b.Status != domain.ResourceLockStatusWaiting || b.WaitPosition != 1 ||
		c.Status != domain.ResourceLockStatusWaiting || c.WaitPosition != 2 ||
		b.RequestSeq >= c.RequestSeq {
		t.Fatalf("unexpected FIFO wait decisions: b=%#v c=%#v", b, c)
	}

	restarted := NewRuntimeStore(newIO(dir))
	states, err := restarted.LoadResourceLocks()
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 1 || states[0].OwnerRunID != "run-a" ||
		len(states[0].Waiters) != 2 ||
		states[0].Waiters[0].RunID != "run-b" ||
		states[0].Waiters[1].RunID != "run-c" {
		t.Fatalf("restart replay drifted: %#v", states)
	}

	if err := restarted.ReleaseRunResourceLock("run-a", resource); err != nil {
		t.Fatal(err)
	}
	cStillWaiting, err := restarted.RequestRunResourceLock("run-c", resource)
	if err != nil {
		t.Fatal(err)
	}
	if cStillWaiting.Status != domain.ResourceLockStatusWaiting || cStillWaiting.WaitPosition != 2 {
		t.Fatalf("run-c bypassed older waiter: %#v", cStillWaiting)
	}
	bAcquire, err := restarted.RequestRunResourceLock("run-b", resource)
	if err != nil {
		t.Fatal(err)
	}
	if bAcquire.Status != domain.ResourceLockStatusAcquired || bAcquire.OwnerRunID != "run-b" {
		t.Fatalf("oldest waiter did not acquire released resource: %#v", bAcquire)
	}
}

func TestRuntimeResourceLockOwnerSafetyCancelAndCleanup(t *testing.T) {
	dir := t.TempDir()
	runtime := NewRuntimeStore(newIO(dir))
	for _, runID := range []domain.RunID{"run-owner", "run-wait"} {
		if _, err := runtime.SaveRun(domain.RunRegistryRecord{RunID: runID}); err != nil {
			t.Fatal(err)
		}
	}
	resource, err := runtime.CanonicalStoryResourceKey()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.RequestRunResourceLock("run-owner", resource); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.RequestRunResourceLock("run-wait", resource); err != nil {
		t.Fatal(err)
	}
	if err := runtime.ReleaseRunResourceLock("run-wait", resource); err == nil {
		t.Fatal("non-owner release unexpectedly succeeded")
	}
	if err := runtime.CancelRunResourceWait("run-wait", resource); err != nil {
		t.Fatal(err)
	}
	if err := runtime.CancelRunResourceWait("run-wait", resource); err != nil {
		t.Fatalf("idempotent wait cancel failed: %v", err)
	}
	if err := runtime.ReleaseAllRunResourceClaims("run-owner"); err != nil {
		t.Fatal(err)
	}
	states, err := runtime.LoadResourceLocks()
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 0 {
		t.Fatalf("run cleanup leaked resource claims: %#v", states)
	}
}

func TestRuntimeResourceLockRecoveryReleasesOwnersPreservesWaiters(t *testing.T) {
	dir := t.TempDir()
	runtime := NewRuntimeStore(newIO(dir))
	for _, runID := range []domain.RunID{"run-owner", "run-wait"} {
		if _, err := runtime.SaveRun(domain.RunRegistryRecord{RunID: runID}); err != nil {
			t.Fatal(err)
		}
	}
	resource, err := runtime.CanonicalStoryResourceKey()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.RequestRunResourceLock("run-owner", resource); err != nil {
		t.Fatal(err)
	}
	wait, err := runtime.RequestRunResourceLock("run-wait", resource)
	if err != nil {
		t.Fatal(err)
	}
	restarted := NewRuntimeStore(newIO(dir))
	released, err := restarted.RecoverResourceLocks()
	if err != nil {
		t.Fatal(err)
	}
	if len(released) != 1 || released[0].OwnerRunID != "run-owner" {
		t.Fatalf("unexpected recovery release set: %#v", released)
	}
	states, err := restarted.LoadResourceLocks()
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 1 || states[0].OwnerRunID != "" ||
		len(states[0].Waiters) != 1 || states[0].Waiters[0].RunID != "run-wait" ||
		states[0].Waiters[0].RequestSeq != wait.RequestSeq {
		t.Fatalf("recovery did not preserve FIFO waiter: %#v", states)
	}
	acquired, err := restarted.RequestRunResourceLock("run-wait", resource)
	if err != nil {
		t.Fatal(err)
	}
	if acquired.Status != domain.ResourceLockStatusAcquired || acquired.OwnerRunID != "run-wait" {
		t.Fatalf("preserved oldest waiter could not reacquire after recovery: %#v", acquired)
	}
}

func TestRuntimeResourceLockRejectsMalformedFacts(t *testing.T) {
	dir := t.TempDir()
	runtime := NewRuntimeStore(newIO(dir))
	for _, runID := range []domain.RunID{"run-a", "run-b"} {
		if _, err := runtime.SaveRun(domain.RunRegistryRecord{RunID: runID}); err != nil {
			t.Fatal(err)
		}
	}
	resource, err := runtime.CanonicalStoryResourceKey()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.RequestRunResourceLock("missing", resource); err == nil {
		t.Fatal("expected unregistered run lock request to fail")
	}
	if _, err := runtime.RequestRunResourceLock(domain.LegacySingleRunID, resource); err == nil {
		t.Fatal("expected legacy synthetic run lock request to fail")
	}
	if _, err := runtime.RequestRunResourceLock("run-a", "path/like"); err == nil {
		t.Fatal("expected path-like resource to fail")
	}

	if _, err := runtime.AppendQueue(domain.RuntimeQueueItem{
		Priority: domain.RuntimePriorityControl,
		RunID:    "run-a",
		Resource: resource,
		Category: domain.RuntimeQueueCategoryResourceAcquired,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.AppendQueue(domain.RuntimeQueueItem{
		Priority: domain.RuntimePriorityControl,
		RunID:    "run-b",
		Resource: resource,
		Category: domain.RuntimeQueueCategoryResourceAcquired,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.LoadResourceLocks(); err == nil {
		t.Fatal("expected persisted double-owner facts to fail closed")
	}
}
