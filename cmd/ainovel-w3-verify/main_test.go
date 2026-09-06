package main

import (
	"testing"

	"github.com/voocel/ainovel-cli/internal/webai"
)

func TestContainsTransition(t *testing.T) {
	states := []webai.SessionState{webai.SessionReady, webai.SessionBusy, webai.SessionReady}
	if !containsTransition(states, webai.SessionReady, webai.SessionBusy, webai.SessionReady) {
		t.Fatal("expected transition")
	}
}

func TestRunRejectsBadTimeout(t *testing.T) {
	if code := run([]string{"--timeout", "bad"}); code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
}

func TestReadyWaitTreatsStartupAuthAndDegradedAsTransient(t *testing.T) {
	for _, state := range []webai.SessionState{
		webai.SessionStarting,
		webai.SessionAuthRequired,
		webai.SessionDegraded,
	} {
		if readyWaitTerminalState(state) {
			t.Fatalf("state %s must remain retryable during Chrome/Gemini startup", state)
		}
	}
}

func TestReadyWaitFailsImmediatelyOnlyForTerminalSessionStates(t *testing.T) {
	for _, state := range []webai.SessionState{
		webai.SessionFailed,
		webai.SessionStopped,
	} {
		if !readyWaitTerminalState(state) {
			t.Fatalf("state %s must be terminal while waiting for READY", state)
		}
	}
}
