package appruntime

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/voocel/ainovel-cli/internal/domain"
	apperrs "github.com/voocel/ainovel-cli/internal/errs"
)

func TestG037LifecycleCommandsStaySeparateFromMutationPlane(t *testing.T) {
	for _, kind := range []CommandKind{CommandStart, CommandPause, CommandResume, CommandStop, CommandCancel, CommandRetry} {
		if !isLifecycleCommand(kind) {
			t.Fatalf("lifecycle command %q routed away from lifecycle plane", kind)
		}
	}
	for _, kind := range G03MutationCommandKinds() {
		if isLifecycleCommand(kind) {
			t.Fatalf("mutation command %q collapsed into lifecycle plane", kind)
		}
	}
	if isLifecycleCommand(CommandKind("documents.write")) {
		t.Fatal("unknown command must not be treated as lifecycle")
	}
}

func TestG037ChapterTextCompareAndRevisionGuards(t *testing.T) {
	current := "current workspace"
	digest := domain.ChapterContentSHA256(current)
	record := &domain.ChapterRecord{Revision: 4}

	valid := ChapterTextSavePayload{Chapter: 3, Content: "next", ExpectedSHA256: digest, ExpectedRecordRevision: 4}
	if err := validateChapterTextGuards(CommandChapterWorkspaceSave, valid, "draft", current, record); err != nil {
		t.Fatalf("matching guards rejected: %v", err)
	}

	staleSHA := valid
	staleSHA.ExpectedSHA256 = domain.ChapterContentSHA256("older")
	if err := validateChapterTextGuards(CommandChapterWorkspaceSave, staleSHA, "draft", current, record); !errors.Is(err, ErrMutationStale) {
		t.Fatalf("stale sha = %v, want ErrMutationStale", err)
	}

	staleRevision := valid
	staleRevision.ExpectedRecordRevision = 3
	if err := validateChapterTextGuards(CommandChapterWorkspaceSave, staleRevision, "draft", current, record); !errors.Is(err, ErrMutationStale) {
		t.Fatalf("stale revision = %v, want ErrMutationStale", err)
	}

	missingArtifact := valid
	if err := validateChapterTextGuards(CommandChapterWorkspaceSave, missingArtifact, "draft", "", record); !errors.Is(err, ErrMutationTargetNotFound) {
		t.Fatalf("missing workspace = %v, want target-not-found", err)
	}

	missingRecord := valid
	missingRecord.ExpectedSHA256 = ""
	if err := validateChapterTextGuards(CommandChapterDraftSave, missingRecord, "draft", "", nil); !errors.Is(err, ErrMutationTargetNotFound) {
		t.Fatalf("missing accepted record = %v, want target-not-found", err)
	}
}

func TestG037OutlineTargetAndReplayGuards(t *testing.T) {
	expansion := domain.ArcExpansion{
		Title: "Second arc",
		Goal:  "Escalate",
		Chapters: []domain.OutlineEntry{
			{Title: "Beat 2", CoreEvent: "Choice"},
			{Title: "Beat 3", CoreEvent: "Cost"},
		},
	}
	skeleton := []domain.VolumeOutline{{
		Index: 1,
		Arcs: []domain.ArcOutline{
			{Index: 1, Title: "First", Goal: "Open", Chapters: []domain.OutlineEntry{{Title: "Beat 1", CoreEvent: "Door"}}},
			{Index: 2, Title: "Second arc", Goal: "Escalate", EstimatedChapters: 2},
		},
	}}
	if err := validateArcExpansionTarget(skeleton, 1, 2, expansion); err != nil {
		t.Fatalf("skeleton target rejected: %v", err)
	}
	if err := validateArcExpansionTarget(skeleton, 1, 9, expansion); !errors.Is(err, ErrMutationTargetNotFound) {
		t.Fatalf("missing arc = %v", err)
	}

	expanded := append([]domain.VolumeOutline(nil), skeleton...)
	expanded[0].Arcs = append([]domain.ArcOutline(nil), skeleton[0].Arcs...)
	expanded[0].Arcs[1] = domain.ArcOutline{Index: 2, Title: expansion.Title, Goal: expansion.Goal, Chapters: expansion.Chapters}
	if err := validateArcExpansionTarget(expanded, 1, 2, expansion); err != nil {
		t.Fatalf("same-payload replay rejected: %v", err)
	}
	changed := expansion
	changed.Goal = "Different"
	if err := validateArcExpansionTarget(expanded, 1, 2, changed); !errors.Is(err, ErrMutationPrecondition) {
		t.Fatalf("conflicting re-expansion = %v", err)
	}

	volume := domain.VolumeOutline{
		Index: 2, Title: "V2", Theme: "Cost",
		Arcs: []domain.ArcOutline{{Index: 1, Title: "A1", Goal: "Enter", Chapters: []domain.OutlineEntry{{Title: "Beat", CoreEvent: "Event"}}}},
	}
	if err := validateVolumeAppendTarget(nil, volume); !errors.Is(err, ErrMutationPrecondition) {
		t.Fatalf("append without layered authority = %v", err)
	}
	if err := validateVolumeAppendTarget([]domain.VolumeOutline{volume}, volume); err != nil {
		t.Fatalf("same last-volume replay rejected: %v", err)
	}
	older := volume
	older.Index = 1
	if err := validateVolumeAppendTarget([]domain.VolumeOutline{volume}, older); !errors.Is(err, ErrMutationPrecondition) {
		t.Fatalf("older volume append = %v", err)
	}
}

func TestG037ForeshadowTargetsHonorSequentialPlantThenAdvance(t *testing.T) {
	updates := []domain.ForeshadowUpdate{
		{ID: "f1", Action: "plant", Description: "seal"},
		{ID: "f1", Action: "advance"},
		{ID: "f1", Action: "resolve"},
	}
	if err := validateForeshadowTargets(nil, updates); err != nil {
		t.Fatalf("plant->advance->resolve rejected: %v", err)
	}
	if err := validateForeshadowTargets(nil, []domain.ForeshadowUpdate{{ID: "missing", Action: "advance"}}); !errors.Is(err, ErrMutationTargetNotFound) {
		t.Fatalf("unknown foreshadow target = %v", err)
	}
}

func TestG037CoreErrorsMapToMutationContract(t *testing.T) {
	precondition := mapMutationCoreError(fmt.Errorf("core conflict: %w", apperrs.ErrToolPrecondition))
	if !errors.Is(precondition, ErrMutationPrecondition) || !errors.Is(precondition, apperrs.ErrToolPrecondition) {
		t.Fatalf("precondition chain = %v", precondition)
	}
	invalid := mapMutationCoreError(fmt.Errorf("bad args: %w", apperrs.ErrToolArgs))
	if !errors.Is(invalid, ErrInvalidMutation) || !errors.Is(invalid, apperrs.ErrToolArgs) {
		t.Fatalf("invalid chain = %v", invalid)
	}
	readErr := fmt.Errorf("read: %w", apperrs.ErrStoreRead)
	if got := mapMutationCoreError(readErr); !errors.Is(got, apperrs.ErrStoreRead) {
		t.Fatalf("store read mapping lost: %v", got)
	}
}

func TestG037MutationDTOConversionsAreDetachedAndNormalized(t *testing.T) {
	payload := ChapterPlanSavePayload{
		Chapter: 2, Title: "  Title  ", Goal: " Goal ",
		Contract: ChapterContractMutationDTO{RequiredBeats: []string{" beat ", " "}},
	}
	plan := chapterPlanFromMutation(payload)
	if plan.Title != "Title" || plan.Goal != "Goal" || !reflect.DeepEqual(plan.Contract.RequiredBeats, []string{"beat"}) {
		t.Fatalf("plan conversion = %+v", plan)
	}

	acceptedAt := time.Date(2026, 9, 10, 1, 2, 3, 0, time.UTC)
	record := &domain.ChapterRecord{Revision: 7, AcceptedAt: acceptedAt}
	if err := validateChapterTextGuards(CommandChapterWorkspaceSave, ChapterTextSavePayload{Chapter: 2, Content: "changed"}, "", "old", record); err != nil {
		t.Fatalf("unguarded workspace update unexpectedly rejected: %v", err)
	}
	if record.Revision != 7 || !record.AcceptedAt.Equal(acceptedAt) {
		t.Fatal("guard evaluation must never mutate the accepted record")
	}
}
