package headless

import (
	"strings"
	"testing"

	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/host"
)

func TestCompletionErrorAcceptsOnlyPersistedCompleteState(t *testing.T) {
	t.Parallel()

	if err := completionError(host.UISnapshot{
		RuntimeState:   "completed",
		Phase:          string(domain.PhaseComplete),
		CurrentChapter: 1,
		CompletedCount: 1,
		TotalChapters:  1,
	}); err != nil {
		t.Fatalf("completed snapshot should succeed: %v", err)
	}
}

func TestCompletionErrorRejectsIncompleteIdleStop(t *testing.T) {
	t.Parallel()

	err := completionError(host.UISnapshot{
		RuntimeState: "idle",
		Phase:        string(domain.PhaseInit),
	})
	if err == nil {
		t.Fatal("incomplete headless stop must fail closed")
	}
	message := err.Error()
	for _, want := range []string{"runtime=idle", "phase=init", "completed=0", "total=0"} {
		if !strings.Contains(message, want) {
			t.Fatalf("completion error %q missing %q", message, want)
		}
	}
}

func TestCompletionErrorRejectsPhaseCompleteWithoutCompletedLifecycle(t *testing.T) {
	t.Parallel()

	err := completionError(host.UISnapshot{
		RuntimeState:   "idle",
		Phase:          string(domain.PhaseComplete),
		CurrentChapter: 1,
		CompletedCount: 1,
		TotalChapters:  1,
	})
	if err == nil {
		t.Fatal("phase complete without completed lifecycle must fail closed")
	}
}

func TestWriteModelGraphProjectionEmitsAuthoritativeStrictRoleGraph(t *testing.T) {
	t.Parallel()

	const graph = "default=web/gemini-web specialist=chatgpt-web/chatgpt-web"
	var out strings.Builder
	if err := writeModelGraphProjection(&out, graph); err != nil {
		t.Fatalf("writeModelGraphProjection: %v", err)
	}
	if got, want := out.String(), "headless model graph: "+graph+"\n"; got != want {
		t.Fatalf("projection = %q, want %q", got, want)
	}
}

func TestWriteModelGraphProjectionRejectsMissingAuthority(t *testing.T) {
	t.Parallel()

	for _, graph := range []string{"", "   ", "default=unavailable"} {
		var out strings.Builder
		if err := writeModelGraphProjection(&out, graph); err == nil {
			t.Fatalf("graph %q must fail closed", graph)
		}
		if out.Len() != 0 {
			t.Fatalf("graph %q wrote non-authoritative projection %q", graph, out.String())
		}
	}
}
