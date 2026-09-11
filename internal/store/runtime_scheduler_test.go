package store

import (
	"fmt"
	"testing"

	"github.com/voocel/ainovel-cli/internal/domain"
)

func TestRuntimeSchedulerPriorityOrderAndRestartReplay(t *testing.T) {
	dir := t.TempDir()
	runtime := NewRuntimeStore(newIO(dir))

	for _, runID := range []domain.RunID{"run-low", "run-normal", "run-high"} {
		if _, err := runtime.SaveRun(domain.RunRegistryRecord{RunID: runID}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := runtime.AppendQueue(domain.RuntimeQueueItem{
		Priority: domain.RuntimePriorityBackground,
		Category: "legacy.task",
		Summary:  "legacy queue item must remain compatible",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.EnqueueScheduledRun("run-low", domain.RunSchedulerPriorityLow); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.EnqueueScheduledRun("run-normal", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.EnqueueScheduledRun("run-high", domain.RunSchedulerPriorityHigh); err != nil {
		t.Fatal(err)
	}

	got, err := runtime.LoadScheduledRuns()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].RunID != "run-high" || got[1].RunID != "run-normal" || got[2].RunID != "run-low" {
		t.Fatalf("unexpected deterministic priority order: %#v", got)
	}
	if got[1].Priority != domain.DefaultRunSchedulerPriority {
		t.Fatalf("empty priority did not normalize to default: %#v", got[1])
	}

	restarted := NewRuntimeStore(newIO(dir))
	replayed, err := restarted.LoadScheduledRuns()
	if err != nil {
		t.Fatal(err)
	}
	if len(replayed) != len(got) {
		t.Fatalf("restart replay count = %d, want %d", len(replayed), len(got))
	}
	for i := range got {
		if replayed[i] != got[i] {
			t.Fatalf("restart replay drift at %d: got=%#v want=%#v", i, replayed[i], got[i])
		}
	}
}

func TestRuntimeSchedulerPriorityChangePreservesAgeAndIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	runtime := NewRuntimeStore(newIO(dir))
	if _, err := runtime.SaveRun(domain.RunRegistryRecord{RunID: "run-1"}); err != nil {
		t.Fatal(err)
	}
	original, err := runtime.EnqueueScheduledRun("run-1", domain.RunSchedulerPriorityLow)
	if err != nil {
		t.Fatal(err)
	}
	again, err := runtime.EnqueueScheduledRun("run-1", domain.RunSchedulerPriorityLow)
	if err != nil {
		t.Fatal(err)
	}
	if again != original {
		t.Fatalf("idempotent enqueue changed ticket: original=%#v again=%#v", original, again)
	}
	if _, err := runtime.EnqueueScheduledRun("run-1", domain.RunSchedulerPriorityHigh); err == nil {
		t.Fatal("expected enqueue with a different priority to require explicit reprioritization")
	}

	updated, err := runtime.SetScheduledRunPriority("run-1", domain.RunSchedulerPriorityHigh)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Priority != domain.RunSchedulerPriorityHigh || updated.EnqueueSeq != original.EnqueueSeq || updated.EnqueueTurn != original.EnqueueTurn {
		t.Fatalf("priority change reset queue age/order: original=%#v updated=%#v", original, updated)
	}
	unchanged, err := runtime.SetScheduledRunPriority("run-1", domain.RunSchedulerPriorityHigh)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged != updated {
		t.Fatalf("idempotent priority update drifted: updated=%#v unchanged=%#v", updated, unchanged)
	}
}

func TestRuntimeSchedulerAgingPreventsLowPriorityStarvation(t *testing.T) {
	dir := t.TempDir()
	runtime := NewRuntimeStore(newIO(dir))
	if _, err := runtime.SaveRun(domain.RunRegistryRecord{RunID: "run-low"}); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.EnqueueScheduledRun("run-low", domain.RunSchedulerPriorityLow); err != nil {
		t.Fatal(err)
	}

	selectedLow := false
	for i := 1; i <= 12; i++ {
		runID := domain.RunID(fmt.Sprintf("run-high-%02d", i))
		if _, err := runtime.SaveRun(domain.RunRegistryRecord{RunID: runID}); err != nil {
			t.Fatal(err)
		}
		if _, err := runtime.EnqueueScheduledRun(runID, domain.RunSchedulerPriorityHigh); err != nil {
			t.Fatal(err)
		}
		selected, err := runtime.DequeueNextScheduledRun()
		if err != nil {
			t.Fatal(err)
		}
		if selected == nil {
			t.Fatal("scheduler unexpectedly returned no run")
		}
		if selected.RunID == "run-low" {
			selectedLow = true
			break
		}
		if selected.RunID != runID {
			t.Fatalf("before aging catches up, expected newest high run %q, got %#v", runID, selected)
		}
	}
	if !selectedLow {
		t.Fatalf("low priority run starved beyond deterministic aging bound; quantum=%d", domain.RunSchedulerAgingQuantum)
	}
}

func TestRuntimeSchedulerDequeueRemoveAndReplay(t *testing.T) {
	dir := t.TempDir()
	runtime := NewRuntimeStore(newIO(dir))
	for _, runID := range []domain.RunID{"run-a", "run-b"} {
		if _, err := runtime.SaveRun(domain.RunRegistryRecord{RunID: runID}); err != nil {
			t.Fatal(err)
		}
		if _, err := runtime.EnqueueScheduledRun(runID, domain.RunSchedulerPriorityNormal); err != nil {
			t.Fatal(err)
		}
	}

	selected, err := runtime.DequeueNextScheduledRun()
	if err != nil {
		t.Fatal(err)
	}
	if selected == nil || selected.RunID != "run-a" {
		t.Fatalf("FIFO tie-break failed: %#v", selected)
	}
	if err := runtime.RemoveScheduledRun("run-b"); err != nil {
		t.Fatal(err)
	}
	if err := runtime.RemoveScheduledRun("run-b"); err != nil {
		t.Fatalf("idempotent remove failed: %v", err)
	}

	restarted := NewRuntimeStore(newIO(dir))
	remaining, err := restarted.LoadScheduledRuns()
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 0 {
		t.Fatalf("dequeue/remove replay left queued runs: %#v", remaining)
	}
}

func TestRuntimeSchedulerRejectsInvalidOrUnregisteredFacts(t *testing.T) {
	dir := t.TempDir()
	runtime := NewRuntimeStore(newIO(dir))

	if _, err := runtime.EnqueueScheduledRun("missing", domain.RunSchedulerPriorityNormal); err == nil {
		t.Fatal("expected unregistered run enqueue to fail")
	}
	if _, err := runtime.SaveRun(domain.RunRegistryRecord{RunID: "run-valid"}); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.EnqueueScheduledRun(domain.LegacySingleRunID, domain.RunSchedulerPriorityNormal); err == nil {
		t.Fatal("expected legacy synthetic run to be unschedulable")
	}
	if _, err := runtime.EnqueueScheduledRun("run-valid", "urgent"); err == nil {
		t.Fatal("expected unknown priority to fail")
	}

	if _, err := runtime.AppendQueue(domain.RuntimeQueueItem{
		Priority:    domain.RuntimePriorityControl,
		RunID:       "run-valid",
		RunPriority: "urgent",
		Category:    domain.RuntimeQueueCategoryRunEnqueued,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.LoadScheduledRuns(); err == nil {
		t.Fatal("expected malformed persisted scheduler fact to fail closed")
	}
}
