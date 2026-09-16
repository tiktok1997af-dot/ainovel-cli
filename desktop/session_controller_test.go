package main

import (
	"context"
	"testing"
	"time"

	"github.com/voocel/ainovel-cli/internal/desktopui"
)

func TestProjectSessionControllerReplaceWaitsForInFlightRuntimeLease(t *testing.T) {
	oldRuntime := newFakeGatewayRuntime()
	newRuntime := newFakeGatewayRuntime()
	controller := newProjectSessionController(oldRuntime)

	leased, release, ok := controller.acquireRuntime()
	if !ok || leased != desktopui.RuntimeClient(oldRuntime) {
		t.Fatalf("acquireRuntime() = %#v, %v; want old runtime", leased, ok)
	}

	done := make(chan error, 1)
	go func() {
		done <- controller.replaceRuntime(context.Background(), newRuntime)
	}()

	select {
	case err := <-done:
		t.Fatalf("replaceRuntime returned before in-flight lease released: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	oldRuntime.mu.Lock()
	closedBeforeRelease := oldRuntime.closeCalls
	oldRuntime.mu.Unlock()
	if closedBeforeRelease != 0 {
		t.Fatalf("old runtime close calls before release = %d, want 0", closedBeforeRelease)
	}

	release()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("replaceRuntime: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("replaceRuntime did not finish after lease release")
	}

	oldRuntime.mu.Lock()
	closedAfterRelease := oldRuntime.closeCalls
	oldRuntime.mu.Unlock()
	if closedAfterRelease != 1 {
		t.Fatalf("old runtime close calls = %d, want 1", closedAfterRelease)
	}

	leased, release, ok = controller.acquireRuntime()
	if !ok || leased != desktopui.RuntimeClient(newRuntime) {
		t.Fatalf("runtime after replace = %#v, %v; want new runtime", leased, ok)
	}
	release()
}

func TestProjectSessionControllerFailsClosedWithoutActiveRuntime(t *testing.T) {
	controller := newProjectSessionController(nil)
	if runtime, release, ok := controller.acquireRuntime(); ok || runtime != nil || release != nil {
		t.Fatalf("acquireRuntime() = %#v, release_non_nil=%v, ok=%v; want no active runtime", runtime, release != nil, ok)
	}
}
