package main

import (
	"context"
	"testing"
	"time"

	"github.com/voocel/ainovel-cli/internal/appruntime"
	"github.com/voocel/ainovel-cli/internal/desktopui"
)

type blockingGatewayRuntime struct {
	*fakeGatewayRuntime
	queryEntered chan struct{}
	queryRelease chan struct{}
}

func (b *blockingGatewayRuntime) Query(ctx context.Context, req appruntime.QueryRequest) (appruntime.QueryResult, error) {
	select {
	case <-b.queryEntered:
	default:
		close(b.queryEntered)
	}
	select {
	case <-b.queryRelease:
	case <-ctx.Done():
		return appruntime.QueryResult{}, ctx.Err()
	}
	return b.fakeGatewayRuntime.Query(ctx, req)
}

// replaceRuntimeForRed models the current pre-GREEN gateway ownership semantics
// using only existing production fields: the runtime pointer is swapped under
// Gateway.mu, but the lock is released before Close. This keeps the RED test
// compiling while proving the missing behavior is a full-call session lease,
// not merely a missing helper method.
func replaceRuntimeForRed(g *Gateway, ctx context.Context, replacement desktopui.RuntimeClient) error {
	g.mu.Lock()
	old := g.runtime
	g.runtime = replacement
	g.mu.Unlock()
	if old != nil {
		return old.Close(ctx)
	}
	return nil
}

func TestGatewayRuntimeReplacementWaitsForInFlightQuery(t *testing.T) {
	oldRuntime := &blockingGatewayRuntime{
		fakeGatewayRuntime: newFakeGatewayRuntime(),
		queryEntered:       make(chan struct{}),
		queryRelease:       make(chan struct{}),
	}
	oldRuntime.queryResult = appruntime.QueryResult{ContractVersion: appruntime.ContractVersion}
	newRuntime := newFakeGatewayRuntime()
	newRuntime.queryResult = appruntime.QueryResult{ContractVersion: appruntime.ContractVersion}
	gateway := newGateway(oldRuntime, nil)

	queryDone := make(chan appruntime.QueryResult, 1)
	go func() {
		queryDone <- gateway.Query(appruntime.QueryRequest{Kind: appruntime.QueryProjectOverview})
	}()
	select {
	case <-oldRuntime.queryEntered:
	case <-time.After(time.Second):
		t.Fatal("query did not enter old runtime")
	}

	replaceDone := make(chan error, 1)
	go func() {
		replaceDone <- replaceRuntimeForRed(gateway, context.Background(), newRuntime)
	}()
	select {
	case err := <-replaceDone:
		t.Fatalf("runtime replacement returned while Query still held runtime lease: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	oldRuntime.mu.Lock()
	closed := oldRuntime.closeCalls
	oldRuntime.mu.Unlock()
	if closed != 0 {
		t.Fatalf("old runtime close calls during in-flight Query = %d, want 0", closed)
	}

	close(oldRuntime.queryRelease)
	select {
	case result := <-queryDone:
		if result.Error != nil {
			t.Fatalf("old runtime query failed: %#v", result.Error)
		}
	case <-time.After(time.Second):
		t.Fatal("query did not complete")
	}
	select {
	case err := <-replaceDone:
		if err != nil {
			t.Fatalf("runtime replacement: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("runtime replacement did not complete after Query released lease")
	}

	oldRuntime.mu.Lock()
	closed = oldRuntime.closeCalls
	oldRuntime.mu.Unlock()
	if closed != 1 {
		t.Fatalf("old runtime close calls after replacement = %d, want 1", closed)
	}

	result := gateway.Query(appruntime.QueryRequest{Kind: appruntime.QueryProjectOverview})
	if result.Error != nil {
		t.Fatalf("query on replacement runtime failed: %#v", result.Error)
	}
	newRuntime.mu.Lock()
	newCalls := newRuntime.queryCalls
	newRuntime.mu.Unlock()
	if newCalls != 1 {
		t.Fatalf("replacement runtime query calls = %d, want 1", newCalls)
	}
}
