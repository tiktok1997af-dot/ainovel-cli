package main

import (
	"testing"
	"time"

	"github.com/voocel/ainovel-cli/internal/desktopui"
)

func TestProjectSessionFailsClosedWithoutRuntime(t *testing.T) {
	session := newProjectSessionController(nil)
	called := false
	if ok := session.withRuntime(func(desktopui.RuntimeClient) {
		called = true
	}); ok {
		t.Fatal("withRuntime() = true, want false without active runtime")
	}
	if called {
		t.Fatal("runtime callback must not run without an active runtime")
	}
}

func TestProjectSessionLeasesActiveRuntime(t *testing.T) {
	runtime := newFakeGatewayRuntime()
	session := newProjectSessionController(runtime)

	var got desktopui.RuntimeClient
	if ok := session.withRuntime(func(current desktopui.RuntimeClient) {
		got = current
	}); !ok {
		t.Fatal("withRuntime() = false, want true with active runtime")
	}
	if got != runtime {
		t.Fatalf("leased runtime = %T %p, want %T %p", got, got, runtime, runtime)
	}
}

func TestProjectSessionExclusiveMutationWaitsForInFlightReader(t *testing.T) {
	first := newFakeGatewayRuntime()
	second := newFakeGatewayRuntime()
	session := newProjectSessionController(first)

	readerEntered := make(chan struct{})
	releaseReader := make(chan struct{})
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		if ok := session.withRuntime(func(current desktopui.RuntimeClient) {
			if current != first {
				t.Errorf("reader runtime = %p, want first %p", current, first)
			}
			close(readerEntered)
			<-releaseReader
		}); !ok {
			t.Error("reader did not acquire active runtime")
		}
	}()

	select {
	case <-readerEntered:
	case <-time.After(time.Second):
		t.Fatal("reader did not enter runtime lease")
	}

	mutationEntered := make(chan struct{})
	mutationDone := make(chan struct{})
	go func() {
		defer close(mutationDone)
		session.mutate(func(current desktopui.RuntimeClient) desktopui.RuntimeClient {
			close(mutationEntered)
			if current != first {
				t.Errorf("mutation current = %p, want first %p", current, first)
			}
			return second
		})
	}()

	select {
	case <-mutationEntered:
		t.Fatal("exclusive mutation entered while reader lease was still active")
	case <-time.After(50 * time.Millisecond):
	}

	close(releaseReader)
	select {
	case <-readerDone:
	case <-time.After(time.Second):
		t.Fatal("reader did not finish")
	}
	select {
	case <-mutationDone:
	case <-time.After(time.Second):
		t.Fatal("exclusive mutation did not finish after reader released")
	}

	var got desktopui.RuntimeClient
	if ok := session.withRuntime(func(current desktopui.RuntimeClient) {
		got = current
	}); !ok {
		t.Fatal("replacement runtime was not active")
	}
	if got != second {
		t.Fatalf("active runtime = %p, want replacement %p", got, second)
	}
}

func TestProjectSessionMutationCanClearRuntime(t *testing.T) {
	runtime := newFakeGatewayRuntime()
	session := newProjectSessionController(runtime)
	session.mutate(func(current desktopui.RuntimeClient) desktopui.RuntimeClient {
		if current != runtime {
			t.Fatalf("mutation current = %p, want %p", current, runtime)
		}
		return nil
	})

	if ok := session.withRuntime(func(desktopui.RuntimeClient) {
		t.Fatal("callback must not run after runtime is cleared")
	}); ok {
		t.Fatal("withRuntime() = true after runtime clear")
	}
}
