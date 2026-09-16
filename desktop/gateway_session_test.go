package main

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/voocel/ainovel-cli/internal/appruntime"
)

type blockingGatewayRuntime struct {
	queryEntered chan struct{}
	releaseQuery chan struct{}
	closed       chan struct{}
	queryOnce    sync.Once
	closeOnce    sync.Once
}

func newBlockingGatewayRuntime() *blockingGatewayRuntime {
	return &blockingGatewayRuntime{
		queryEntered: make(chan struct{}),
		releaseQuery: make(chan struct{}),
		closed:       make(chan struct{}),
	}
}

func (r *blockingGatewayRuntime) Snapshot(context.Context) (appruntime.DesktopSnapshot, error) {
	return appruntime.DesktopSnapshot{Contract: appruntime.CurrentContract()}, nil
}

func (r *blockingGatewayRuntime) Query(_ context.Context, req appruntime.QueryRequest) (appruntime.QueryResult, error) {
	r.queryOnce.Do(func() { close(r.queryEntered) })
	<-r.releaseQuery
	return appruntime.QueryResult{
		ContractVersion: appruntime.ContractVersion,
		Kind:            req.Kind,
	}, nil
}

func (r *blockingGatewayRuntime) Dispatch(_ context.Context, cmd appruntime.CommandRequest) (appruntime.CommandResult, error) {
	return appruntime.CommandResult{
		ContractVersion: appruntime.ContractVersion,
		CommandID:       cmd.ID,
	}, nil
}

func (r *blockingGatewayRuntime) Subscribe(context.Context, appruntime.EventCursor) (appruntime.EventSubscription, error) {
	return newFakeGatewaySubscription(), nil
}

func (r *blockingGatewayRuntime) Close(context.Context) error {
	r.closeOnce.Do(func() { close(r.closed) })
	return nil
}

func TestGatewayShutdownWaitsForInFlightRuntimeCall(t *testing.T) {
	runtime := newBlockingGatewayRuntime()
	gateway := newGateway(runtime, nil)

	queryDone := make(chan struct{})
	go func() {
		defer close(queryDone)
		result := gateway.Query(appruntime.QueryRequest{Kind: appruntime.QueryProjectOverview})
		if result.Error != nil {
			t.Errorf("Query() error = %#v", result.Error)
		}
	}()

	select {
	case <-runtime.queryEntered:
	case <-time.After(time.Second):
		t.Fatal("query did not enter runtime")
	}

	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		gateway.shutdown(context.Background())
	}()

	select {
	case <-runtime.closed:
		t.Fatal("runtime closed while Query still held an in-flight call")
	case <-time.After(75 * time.Millisecond):
	}

	close(runtime.releaseQuery)
	select {
	case <-queryDone:
	case <-time.After(time.Second):
		t.Fatal("query did not finish after release")
	}
	select {
	case <-runtime.closed:
	case <-time.After(time.Second):
		t.Fatal("runtime was not closed after in-flight query completed")
	}
	select {
	case <-shutdownDone:
	case <-time.After(time.Second):
		t.Fatal("gateway shutdown did not complete")
	}
}
