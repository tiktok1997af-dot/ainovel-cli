package main

import (
	"strings"
	"testing"

	"github.com/voocel/ainovel-cli/internal/webai"
)

func TestExpectedFinalBindsReceipt(t *testing.T) {
	if got := expectedFinal("abc123"); got != "W4_TOOL_OK:abc123" {
		t.Fatalf("expectedFinal = %q", got)
	}
}

func TestInstructionRequiresExactLocalToolAndChallenge(t *testing.T) {
	instruction := w4Instruction()
	for _, want := range []string{proofToolName, proofChallenge, "exactly once", "Do not invent"} {
		if !strings.Contains(instruction, want) {
			t.Fatalf("instruction missing %q: %s", want, instruction)
		}
	}
}

func TestCountStateRequiresTwoBusyRounds(t *testing.T) {
	states := []webai.SessionState{
		webai.SessionReady,
		webai.SessionBusy,
		webai.SessionReady,
		webai.SessionBusy,
		webai.SessionReady,
	}
	if got := countState(states, webai.SessionBusy); got != 2 {
		t.Fatalf("BUSY count=%d, want 2", got)
	}
	if !containsTransition(states, webai.SessionReady, webai.SessionBusy, webai.SessionReady) {
		t.Fatal("expected READY -> BUSY -> READY transition")
	}
}

func TestReadyWaitTreatsStartupAuthAndDegradedAsTransient(t *testing.T) {
	for _, state := range []webai.SessionState{
		webai.SessionStarting,
		webai.SessionAuthRequired,
		webai.SessionDegraded,
	} {
		if readyWaitTerminalState(state) {
			t.Fatalf("state %s must remain transient during startup", state)
		}
	}
}

func TestReadyWaitFailsImmediatelyForTerminalStates(t *testing.T) {
	for _, state := range []webai.SessionState{webai.SessionFailed, webai.SessionStopped} {
		if !readyWaitTerminalState(state) {
			t.Fatalf("state %s must be terminal", state)
		}
	}
}
