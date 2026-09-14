package flow

import (
	"testing"

	"github.com/voocel/ainovel-cli/internal/domain"
	storepkg "github.com/voocel/ainovel-cli/internal/store"
)

func TestD07LoadStateUsesBoundRuntimeReviewInterval(t *testing.T) {
	store := storepkg.NewStore(t.TempDir())
	if err := BindRuntimeReviewInterval(store, 7); err != nil {
		t.Fatalf("bind review interval: %v", err)
	}
	t.Cleanup(func() { UnbindRuntimeReviewInterval(store) })

	state, err := LoadState(store)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if state.ReviewInterval != 7 {
		t.Fatalf("ReviewInterval=%d, want 7", state.ReviewInterval)
	}
}

func TestD07RouteUsesConfiguredReviewCheckpoint(t *testing.T) {
	progress := &domain.Progress{
		Phase:             domain.PhaseWriting,
		Flow:              domain.FlowWriting,
		TotalChapters:     20,
		CompletedChapters: []int{1, 2, 3, 4, 5, 6, 7},
	}
	state := State{Progress: progress, LastCompleted: 7, ReviewInterval: 7}

	inst := Route(state)
	if inst == nil || inst.Agent != "editor" {
		t.Fatalf("checkpoint 7 route=%+v, want editor global review", inst)
	}

	state.ReviewInterval = 10
	inst = Route(state)
	if inst == nil || inst.Agent != "writer" || inst.Chapter != 8 {
		t.Fatalf("checkpoint 10 route=%+v, want writer chapter 8", inst)
	}
}
