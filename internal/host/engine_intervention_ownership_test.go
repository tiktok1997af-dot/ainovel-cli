package host

import (
	"context"
	"fmt"
	"testing"

	"github.com/voocel/ainovel-cli/internal/arbiter"
	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/flow"
	storepkg "github.com/voocel/ainovel-cli/internal/store"
)

func newInterventionOwnershipTestEngine(t *testing.T) (*engine, *storepkg.Store, arbiter.InterventionFacts) {
	t.Helper()
	st := storepkg.NewStore(t.TempDir())
	if err := st.Init(); err != nil {
		t.Fatalf("init store: %v", err)
	}
	if err := st.RunMeta.Init("default", "test", "test"); err != nil {
		t.Fatalf("init run meta: %v", err)
	}
	if err := st.Progress.Init(2); err != nil {
		t.Fatalf("init progress: %v", err)
	}
	if err := st.Progress.UpdatePhase(domain.PhaseWriting); err != nil {
		t.Fatalf("phase: %v", err)
	}
	facts, err := arbiter.CollectInterventionFacts(st)
	if err != nil {
		t.Fatalf("facts: %v", err)
	}
	e := &engine{store: st, emitEvent: func(Event) {}}
	return e, st, facts
}

func TestEngine_InterventionOwnershipRestoresAfterTakeNext(t *testing.T) {
	e, st, facts := newInterventionOwnershipTestEngine(t)
	const steer = "重写第1章然后停下来"
	if err := e.applyControlOp(context.Background(), controlOp{
		dispatch: &arbiter.DispatchOp{Agent: "editor", Task: "复核并重写第1章"},
		text:     steer,
		facts:    facts,
	}); err != nil {
		t.Fatalf("apply control op: %v", err)
	}
	inst := e.takeNext()
	if inst == nil {
		t.Fatal("intervention dispatch must reach next before the simulated exit")
	}
	if e.next != nil {
		t.Fatal("takeNext must consume next to reproduce the lost-dispatch window")
	}

	// Simulate run()'s exit defer after the dispatch has left both pending and next.
	e.restoreOwnedInterventionDispatch()
	meta, err := st.RunMeta.Load()
	if err != nil || meta == nil {
		t.Fatalf("load meta: %v", err)
	}
	if meta.PendingSteer != steer {
		t.Fatalf("owned dispatch must restore PendingSteer after exit, got %q", meta.PendingSteer)
	}
}

func TestEngine_InterventionOwnershipClearsOnlyOnExactSuccessfulDispatch(t *testing.T) {
	e, st, facts := newInterventionOwnershipTestEngine(t)
	const steer = "只修改第一章语气"
	if err := e.applyControlOp(context.Background(), controlOp{
		dispatch: &arbiter.DispatchOp{Agent: "editor", Task: "调整第一章语气"},
		text:     steer,
		facts:    facts,
	}); err != nil {
		t.Fatalf("apply control op: %v", err)
	}
	inst := e.takeNext()
	if inst == nil {
		t.Fatal("missing intervention dispatch")
	}

	// An unrelated successful route must not consume the intervention token.
	e.completeOwnedInterventionDispatch(&flow.Instruction{Agent: "writer", Task: "写下一章"})
	if e.ownedDispatchText != steer {
		t.Fatalf("unrelated instruction consumed ownership: %q", e.ownedDispatchText)
	}

	// The exact successful dispatch releases replay ownership.
	e.completeOwnedInterventionDispatch(inst)
	if e.ownedDispatchKey != "" || e.ownedDispatchText != "" {
		t.Fatalf("exact dispatch success must clear ownership: key=%q text=%q", e.ownedDispatchKey, e.ownedDispatchText)
	}
	e.restoreOwnedInterventionDispatch()
	meta, err := st.RunMeta.Load()
	if err != nil || meta == nil {
		t.Fatalf("load meta: %v", err)
	}
	if meta.PendingSteer != "" {
		t.Fatalf("completed intervention must not be replayed, got %q", meta.PendingSteer)
	}
}

func TestEngine_InterventionOwnershipExitStress(t *testing.T) {
	for i := 0; i < 100; i++ {
		t.Run(fmt.Sprintf("iteration-%03d", i), func(t *testing.T) {
			e, st, facts := newInterventionOwnershipTestEngine(t)
			steer := fmt.Sprintf("exit-race-%03d", i)
			if err := e.applyControlOp(context.Background(), controlOp{
				dispatch: &arbiter.DispatchOp{Agent: "editor", Task: "stress dispatch"},
				text:     steer,
				facts:    facts,
			}); err != nil {
				t.Fatalf("apply control op: %v", err)
			}
			if e.takeNext() == nil {
				t.Fatal("missing dispatch")
			}
			e.restoreOwnedInterventionDispatch()
			meta, err := st.RunMeta.Load()
			if err != nil || meta == nil {
				t.Fatalf("load meta: %v", err)
			}
			if meta.PendingSteer != steer {
				t.Fatalf("iteration lost steer: want %q got %q", steer, meta.PendingSteer)
			}
		})
	}
}
