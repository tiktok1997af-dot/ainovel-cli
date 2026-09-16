package main

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/voocel/ainovel-cli/internal/appruntime"
)

type fakeGatewayRuntime struct {
	mu            sync.Mutex
	snapshot      appruntime.DesktopSnapshot
	snapshotErr   error
	queryResult   appruntime.QueryResult
	queryErr      error
	queryRequest  appruntime.QueryRequest
	queryCalls    int
	commandResult appruntime.CommandResult
	commandErr    error
	command       appruntime.CommandRequest
	commandCalls  int
	sub           *fakeGatewaySubscription
	subscribeErr  error
	closeCalls    int
}

func newFakeGatewayRuntime() *fakeGatewayRuntime {
	return &fakeGatewayRuntime{sub: newFakeGatewaySubscription()}
}

func (f *fakeGatewayRuntime) Snapshot(context.Context) (appruntime.DesktopSnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.snapshot, f.snapshotErr
}

func (f *fakeGatewayRuntime) Query(_ context.Context, req appruntime.QueryRequest) (appruntime.QueryResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queryCalls++
	f.queryRequest = req
	result := f.queryResult
	if result.Kind == "" {
		result.Kind = req.Kind
	}
	return result, f.queryErr
}

func (f *fakeGatewayRuntime) Dispatch(_ context.Context, cmd appruntime.CommandRequest) (appruntime.CommandResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.commandCalls++
	f.command = cmd
	result := f.commandResult
	if result.CommandID == "" {
		result.CommandID = cmd.ID
	}
	return result, f.commandErr
}

func (f *fakeGatewayRuntime) Subscribe(context.Context, appruntime.EventCursor) (appruntime.EventSubscription, error) {
	if f.subscribeErr != nil {
		return nil, f.subscribeErr
	}
	return f.sub, nil
}

func (f *fakeGatewayRuntime) Close(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closeCalls++
	return nil
}

type fakeGatewaySubscription struct {
	mu     sync.Mutex
	events chan appruntime.DesktopEvent
	closed bool
}

func newFakeGatewaySubscription() *fakeGatewaySubscription {
	return &fakeGatewaySubscription{events: make(chan appruntime.DesktopEvent, 8)}
}

func (s *fakeGatewaySubscription) Events() <-chan appruntime.DesktopEvent { return s.events }

func (s *fakeGatewaySubscription) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	close(s.events)
	return nil
}

func TestGatewaySnapshotDelegatesThroughTypedEnvelope(t *testing.T) {
	runtime := newFakeGatewayRuntime()
	runtime.snapshot = appruntime.DesktopSnapshot{
		Contract: appruntime.CurrentContract(),
		Revision: 42,
	}
	gateway := newGateway(runtime, nil)

	got := gateway.Snapshot()
	if got.Error != nil {
		t.Fatalf("Snapshot() error = %#v", got.Error)
	}
	if got.ContractVersion != appruntime.ContractVersion {
		t.Fatalf("contract version = %q, want %q", got.ContractVersion, appruntime.ContractVersion)
	}
	if got.Data == nil || got.Data.Revision != 42 {
		t.Fatalf("snapshot = %#v, want revision 42", got.Data)
	}
}

func TestGatewayQueryInjectsContractAndDelegates(t *testing.T) {
	runtime := newFakeGatewayRuntime()
	runtime.queryResult = appruntime.QueryResult{
		Data: json.RawMessage(`{"title":"Hua An 2026"}`),
	}
	gateway := newGateway(runtime, nil)

	got := gateway.Query(appruntime.QueryRequest{Kind: appruntime.QueryProjectOverview})
	if got.Error != nil {
		t.Fatalf("Query() error = %#v", got.Error)
	}
	if got.ContractVersion != appruntime.ContractVersion {
		t.Fatalf("result contract = %q, want %q", got.ContractVersion, appruntime.ContractVersion)
	}
	if runtime.queryRequest.ContractVersion != appruntime.ContractVersion {
		t.Fatalf("request contract = %q, want injected current contract", runtime.queryRequest.ContractVersion)
	}
	if runtime.queryCalls != 1 {
		t.Fatalf("query calls = %d, want 1", runtime.queryCalls)
	}
}

func TestGatewayDispatchInjectsContractAndPreservesCommandIdentity(t *testing.T) {
	runtime := newFakeGatewayRuntime()
	runtime.commandResult = appruntime.CommandResult{Accepted: true, Status: "running"}
	gateway := newGateway(runtime, nil)

	got := gateway.Dispatch(appruntime.CommandRequest{ID: "cmd-7", Kind: appruntime.CommandStart})
	if got.Error != nil {
		t.Fatalf("Dispatch() error = %#v", got.Error)
	}
	if got.ContractVersion != appruntime.ContractVersion {
		t.Fatalf("result contract = %q, want %q", got.ContractVersion, appruntime.ContractVersion)
	}
	if got.CommandID != "cmd-7" {
		t.Fatalf("command id = %q, want cmd-7", got.CommandID)
	}
	if runtime.command.ContractVersion != appruntime.ContractVersion {
		t.Fatalf("request contract = %q, want injected current contract", runtime.command.ContractVersion)
	}
	if runtime.commandCalls != 1 {
		t.Fatalf("command calls = %d, want 1", runtime.commandCalls)
	}
}

func TestGatewayRejectsMismatchedContractBeforeRuntime(t *testing.T) {
	runtime := newFakeGatewayRuntime()
	gateway := newGateway(runtime, nil)

	got := gateway.Query(appruntime.QueryRequest{
		ContractVersion: "ainovel.desktop.future",
		Kind:            appruntime.QueryProjectOverview,
	})
	if got.Error == nil || got.Error.Code != appruntime.ErrorCodeContractMismatch {
		t.Fatalf("Query() error = %#v, want contract_mismatch", got.Error)
	}
	if runtime.queryCalls != 0 {
		t.Fatalf("query calls = %d, want 0 for fail-closed mismatch", runtime.queryCalls)
	}
}

func TestGatewayFailsClosedWithoutRuntime(t *testing.T) {
	gateway := newGateway(nil, nil)

	snapshot := gateway.Snapshot()
	if snapshot.Error == nil || snapshot.Error.Code != appruntime.ErrorCodeRuntimeUnavailable {
		t.Fatalf("Snapshot() error = %#v, want runtime_unavailable", snapshot.Error)
	}
	query := gateway.Query(appruntime.QueryRequest{Kind: appruntime.QueryProjectOverview})
	if query.Error == nil || query.Error.Code != appruntime.ErrorCodeRuntimeUnavailable {
		t.Fatalf("Query() error = %#v, want runtime_unavailable", query.Error)
	}
	command := gateway.Dispatch(appruntime.CommandRequest{ID: "cmd-1", Kind: appruntime.CommandStart})
	if command.Error == nil || command.Error.Code != appruntime.ErrorCodeRuntimeUnavailable {
		t.Fatalf("Dispatch() error = %#v, want runtime_unavailable", command.Error)
	}
}

func TestGatewayEventBridgeForwardsOnlyCurrentContractEvents(t *testing.T) {
	runtime := newFakeGatewayRuntime()
	emitted := make(chan appruntime.DesktopEvent, 2)
	topics := make(chan string, 2)
	gateway := newGateway(runtime, func(_ context.Context, topic string, event appruntime.DesktopEvent) {
		topics <- topic
		emitted <- event
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	gateway.startup(ctx)
	defer gateway.shutdown(context.Background())

	runtime.sub.events <- appruntime.DesktopEvent{
		ContractVersion: appruntime.ContractVersion,
		Seq:             9,
		Category:        "RUN",
		Type:            "run_updated",
		Time:            time.Now().UTC(),
	}

	select {
	case topic := <-topics:
		if topic != desktopEventTopic {
			t.Fatalf("topic = %q, want %q", topic, desktopEventTopic)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event topic")
	}
	select {
	case event := <-emitted:
		if event.Seq != 9 {
			t.Fatalf("event seq = %d, want 9", event.Seq)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for desktop event")
	}

	runtime.sub.events <- appruntime.DesktopEvent{
		ContractVersion: "ainovel.desktop.future",
		Seq:             10,
		Category:        "RUN",
		Type:            "run_updated",
		Time:            time.Now().UTC(),
	}
	select {
	case event := <-emitted:
		t.Fatalf("unexpected mismatched event forwarded: %#v", event)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestGatewayConvertsUnexpectedRuntimeErrorsWithoutLeakingCause(t *testing.T) {
	runtime := newFakeGatewayRuntime()
	runtime.queryErr = errors.New("secret provider path C:/private/session")
	gateway := newGateway(runtime, nil)

	got := gateway.Query(appruntime.QueryRequest{Kind: appruntime.QueryProjectOverview})
	if got.Error == nil || got.Error.Code != appruntime.ErrorCodeInternal {
		t.Fatalf("Query() error = %#v, want internal", got.Error)
	}
	if got.Error.Message != "An internal runtime error occurred." {
		t.Fatalf("safe message = %q", got.Error.Message)
	}
}
