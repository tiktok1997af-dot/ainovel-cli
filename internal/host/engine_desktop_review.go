package host

import (
	"context"
	"fmt"

	"github.com/voocel/ainovel-cli/internal/flow"
)

// runDesktopReviewInstruction is the bounded G06.4 Engine hook. It intentionally
// reuses Engine precheck + runWorker instead of creating a second Worker runtime.
// A bounded review job must never accept a deterministic reroute (for example an
// architect expansion) because that would let the job escape its authorized
// Review target/repair chapter set.
func (e *engine) runDesktopReviewInstruction(ctx context.Context, inst *flow.Instruction) error {
	if e == nil || inst == nil || inst.Agent == "" {
		return fmt.Errorf("desktop review instruction is required")
	}
	reroute, err := e.precheck(inst)
	if err != nil {
		return err
	}
	if reroute != nil && reroute.Agent != "" {
		return fmt.Errorf("bounded desktop review instruction requires out-of-scope reroute to %q", reroute.Agent)
	}

	// D07 strict-role repair keeps the existing writer precheck/queue semantics,
	// then executes the exact bounded repair task on the ChatGPT specialist
	// worker. Ordinary writer routing remains Gemini and no provider fallback is
	// introduced. Preserve writer's chapter-start bookkeeping because runWorker
	// keys that bookkeeping by worker identity.
	if inst.Agent == "writer" && inst.Reason == "G06.4 bounded review repair" {
		if inst.Chapter > 0 {
			if err := e.store.Progress.ValidateChapterWork(inst.Chapter); err != nil {
				return err
			}
			if err := e.store.Progress.StartChapter(inst.Chapter); err != nil {
				return fmt.Errorf("pre-mark review repair chapter %d: %w", inst.Chapter, err)
			}
		}
		repair := *inst
		repair.Agent = "repair"
		return e.runWorker(ctx, &repair)
	}
	return e.runWorker(ctx, inst)
}
