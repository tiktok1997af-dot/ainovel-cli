package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/voocel/ainovel-cli/internal/domain"
)

func TestRuntimeRunRegistryRoundTripAndDeterministicList(t *testing.T) {
	dir := t.TempDir()
	runtime := NewRuntimeStore(newIO(dir))

	runB, err := runtime.SaveRun(domain.RunRegistryRecord{
		RunID:  domain.RunID("run-b"),
		State:  "queued",
		Status: "waiting",
	})
	if err != nil {
		t.Fatal(err)
	}
	if runB.Version != domain.CurrentRunRegistryRecordVersion || runB.Source != domain.RunRecordSourceRegistry {
		t.Fatalf("unexpected defaults: %#v", runB)
	}
	if runB.CreatedAt.IsZero() || runB.UpdatedAt.IsZero() {
		t.Fatalf("timestamps were not assigned: %#v", runB)
	}

	if _, err := runtime.SaveRun(domain.RunRegistryRecord{RunID: domain.RunID("run-a")}); err != nil {
		t.Fatal(err)
	}
	runs, err := runtime.ListRuns()
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 || runs[0].RunID != "run-a" || runs[1].RunID != "run-b" {
		t.Fatalf("runs not deterministically sorted: %#v", runs)
	}

	loaded, err := runtime.LoadRun(domain.RunID("run-b"))
	if err != nil {
		t.Fatal(err)
	}
	if loaded == nil || loaded.Status != "waiting" || loaded.CreatedAt != runB.CreatedAt {
		t.Fatalf("unexpected loaded record: %#v", loaded)
	}

	updated, err := runtime.SaveRun(domain.RunRegistryRecord{
		RunID:  domain.RunID("run-b"),
		Status: "still-waiting",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.CreatedAt != runB.CreatedAt {
		t.Fatalf("created_at changed across update: before=%v after=%v", runB.CreatedAt, updated.CreatedAt)
	}
}

func TestRuntimeRunRegistryRejectsPathLikeAndReservedIDs(t *testing.T) {
	dir := t.TempDir()
	runtime := NewRuntimeStore(newIO(dir))

	if _, err := runtime.SaveRun(domain.RunRegistryRecord{RunID: domain.RunID("../escape")}); err == nil {
		t.Fatal("expected path-like run id to be rejected")
	}
	if _, err := runtime.SaveRun(domain.RunRegistryRecord{RunID: domain.LegacySingleRunID}); err == nil {
		t.Fatal("expected reserved legacy run id to be rejected")
	}
	if _, err := os.Stat(filepath.Join(dir, "meta", "runtime", "escape")); !os.IsNotExist(err) {
		t.Fatalf("invalid run id escaped registry root: err=%v", err)
	}
}

func TestRuntimeRunHistoryIsPerRunAppendOnlyAndSequenced(t *testing.T) {
	dir := t.TempDir()
	runtime := NewRuntimeStore(newIO(dir))

	if _, err := runtime.SaveRun(domain.RunRegistryRecord{RunID: domain.RunID("run-001")}); err != nil {
		t.Fatal(err)
	}
	first, err := runtime.AppendRunHistory(domain.RunHistoryRecord{
		RunID:    domain.RunID("run-001"),
		Category: "RUN",
		Type:     "created",
		Summary:  "run created",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := runtime.AppendRunHistory(domain.RunHistoryRecord{
		RunID:    domain.RunID("run-001"),
		TaskID:   domain.TaskID("task-001"),
		Category: "RUN",
		Type:     "task",
		Summary:  "task projected",
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.Seq != 1 || second.Seq != 2 || first.Time.IsZero() || second.Time.IsZero() {
		t.Fatalf("unexpected history sequencing: first=%#v second=%#v", first, second)
	}

	after, err := runtime.LoadRunHistoryAfter(domain.RunID("run-001"), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 1 || after[0].Seq != 2 || after[0].TaskID != "task-001" {
		t.Fatalf("unexpected history tail: %#v", after)
	}

	if _, err := runtime.AppendRunHistory(domain.RunHistoryRecord{
		RunID:    domain.RunID("missing-run"),
		Category: "RUN",
		Type:     "created",
	}); err == nil {
		t.Fatal("expected history for an unregistered run to be rejected")
	}
}

func TestRuntimeLegacyProjectionIsReadOnlyAndNonDestructive(t *testing.T) {
	dir := t.TempDir()
	io := newIO(dir)
	runtime := NewRuntimeStore(io)
	legacy := NewRunMetaStore(newIO(dir))
	progress := NewProgressStore(newIO(dir))

	started := time.Date(2026, time.September, 11, 1, 2, 3, 0, time.UTC)
	if err := legacy.Save(domain.RunMeta{
		StartedAt:   started.Format(time.RFC3339),
		Style:       "legacy-style",
		Model:       "legacy-model",
		AdvanceMode: domain.ChapterAdvanceAuto,
	}); err != nil {
		t.Fatal(err)
	}
	if err := progress.Save(&domain.Progress{
		Phase:          domain.PhaseWriting,
		CurrentChapter: 7,
		TotalChapters:  20,
	}); err != nil {
		t.Fatal(err)
	}

	runs, err := runtime.ListRunsWithLegacyProjection()
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("legacy projection count = %d, want 1", len(runs))
	}
	got := runs[0]
	if got.RunID != domain.LegacySingleRunID || got.Source != domain.RunRecordSourceLegacySingleRun ||
		got.LegacyPhase != domain.PhaseWriting || !got.StartedAt.Equal(started) {
		t.Fatalf("unexpected legacy projection: %#v", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "meta", "runtime", "runs")); !os.IsNotExist(err) {
		t.Fatalf("legacy projection materialized additive registry: err=%v", err)
	}

	if _, err := runtime.SaveRun(domain.RunRegistryRecord{RunID: domain.RunID("run-new")}); err != nil {
		t.Fatal(err)
	}
	runs, err = runtime.ListRunsWithLegacyProjection()
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].RunID != "run-new" || runs[0].Source != domain.RunRecordSourceRegistry {
		t.Fatalf("persisted registry should supersede synthetic projection: %#v", runs)
	}

	var reloaded domain.RunMeta
	if err := io.ReadJSON("meta/run.json", &reloaded); err != nil {
		t.Fatal(err)
	}
	if reloaded.Style != "legacy-style" || reloaded.Model != "legacy-model" {
		t.Fatalf("legacy meta/run.json was modified: %#v", reloaded)
	}
}

func TestRuntimeResetPreservesRunRegistryHistory(t *testing.T) {
	dir := t.TempDir()
	runtime := NewRuntimeStore(newIO(dir))

	if _, err := runtime.SaveRun(domain.RunRegistryRecord{RunID: domain.RunID("run-keep")}); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.AppendRunHistory(domain.RunHistoryRecord{
		RunID:    domain.RunID("run-keep"),
		Category: "RUN",
		Type:     "created",
	}); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Reset(); err != nil {
		t.Fatal(err)
	}
	record, err := runtime.LoadRun(domain.RunID("run-keep"))
	if err != nil {
		t.Fatal(err)
	}
	history, err := runtime.LoadRunHistory(domain.RunID("run-keep"))
	if err != nil {
		t.Fatal(err)
	}
	if record == nil || len(history) != 1 {
		t.Fatalf("reset removed G05.3 durable facts: record=%#v history=%#v", record, history)
	}
}
