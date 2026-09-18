package main

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/voocel/ainovel-cli/internal/appruntime"
	"github.com/voocel/ainovel-cli/internal/desktopui"
)

func TestGatewayExposesOnlyTypedProjectLifecycleBindings(t *testing.T) {
	type lifecycleGateway interface {
		CreateProject(ProjectLifecycleRequest) ProjectLifecycleResult
		OpenProject(ProjectLifecycleRequest) ProjectLifecycleResult
		SwitchProject(ProjectLifecycleRequest) ProjectLifecycleResult
		CloseProject() ProjectLifecycleResult
	}
	var _ lifecycleGateway = (*Gateway)(nil)

	if _, ok := reflect.TypeOf(&Gateway{}).MethodByName("Invoke"); ok {
		t.Fatal("Gateway must not expose a generic Invoke binding")
	}
}

func TestGatewayCreateProjectUsesCreateMode(t *testing.T) {
	root := filepath.Join(t.TempDir(), "new-project")
	runtime := newFakeGatewayRuntime()
	runtime.snapshot = appruntime.DesktopSnapshot{Contract: appruntime.CurrentContract(), Revision: 7}

	var gotRoot string
	var gotCreate bool
	gateway := newGatewayWithRuntimeFactory(nil, nil, func(_ context.Context, projectRoot string, create bool) (desktopui.RuntimeClient, error) {
		gotRoot = projectRoot
		gotCreate = create
		return runtime, nil
	})

	result := gateway.CreateProject(ProjectLifecycleRequest{ProjectRoot: root})
	if result.Error != nil {
		t.Fatalf("CreateProject() error = %#v", result.Error)
	}
	wantRoot, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	if gotRoot != wantRoot {
		t.Fatalf("factory root = %q, want %q", gotRoot, wantRoot)
	}
	if !gotCreate {
		t.Fatal("CreateProject passed create=false")
	}
}

func TestGatewayOpenProjectBuildsAndActivatesPathExplicitRuntime(t *testing.T) {
	root := t.TempDir()
	runtime := newFakeGatewayRuntime()
	runtime.snapshot = appruntime.DesktopSnapshot{Contract: appruntime.CurrentContract(), Revision: 11}

	var gotRoot string
	var gotCreate bool
	gateway := newGatewayWithRuntimeFactory(nil, nil, func(_ context.Context, projectRoot string, create bool) (desktopui.RuntimeClient, error) {
		gotRoot = projectRoot
		gotCreate = create
		return runtime, nil
	})

	result := gateway.OpenProject(ProjectLifecycleRequest{ProjectRoot: root})
	if result.Error != nil {
		t.Fatalf("OpenProject() error = %#v", result.Error)
	}
	wantRoot, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	if gotRoot != wantRoot {
		t.Fatalf("factory root = %q, want %q", gotRoot, wantRoot)
	}
	if gotCreate {
		t.Fatal("OpenProject passed create=true")
	}
	if result.ProjectRoot != wantRoot {
		t.Fatalf("result project root = %q, want %q", result.ProjectRoot, wantRoot)
	}
	if result.Data == nil || result.Data.Revision != 11 {
		t.Fatalf("result snapshot = %#v, want revision 11", result.Data)
	}
	if got := gateway.Snapshot(); got.Error != nil || got.Data == nil || got.Data.Revision != 11 {
		t.Fatalf("active Snapshot() = %#v", got)
	}
}

func TestGatewaySwitchProjectWaitsForInFlightLeaseAndClosesOldRuntime(t *testing.T) {
	oldRuntime := &blockingGatewayRuntime{
		fakeGatewayRuntime: newFakeGatewayRuntime(),
		queryEntered:       make(chan struct{}),
		queryRelease:       make(chan struct{}),
	}
	oldRuntime.queryResult = appruntime.QueryResult{ContractVersion: appruntime.ContractVersion}
	newRuntime := newFakeGatewayRuntime()
	newRuntime.snapshot = appruntime.DesktopSnapshot{Contract: appruntime.CurrentContract(), Revision: 22}
	root := t.TempDir()

	gateway := newGatewayWithRuntimeFactory(oldRuntime, nil, func(context.Context, string, bool) (desktopui.RuntimeClient, error) {
		return newRuntime, nil
	})

	queryDone := make(chan appruntime.QueryResult, 1)
	go func() {
		queryDone <- gateway.Query(appruntime.QueryRequest{Kind: appruntime.QueryProjectOverview})
	}()
	select {
	case <-oldRuntime.queryEntered:
	case <-time.After(time.Second):
		t.Fatal("query did not enter old runtime")
	}

	switchDone := make(chan ProjectLifecycleResult, 1)
	go func() {
		switchDone <- gateway.SwitchProject(ProjectLifecycleRequest{ProjectRoot: root})
	}()
	select {
	case result := <-switchDone:
		t.Fatalf("SwitchProject returned while Query held runtime lease: %#v", result)
	case <-time.After(100 * time.Millisecond):
	}

	oldRuntime.mu.Lock()
	closed := oldRuntime.closeCalls
	oldRuntime.mu.Unlock()
	if closed != 0 {
		t.Fatalf("old runtime closed during in-flight Query: %d", closed)
	}

	close(oldRuntime.queryRelease)
	select {
	case result := <-queryDone:
		if result.Error != nil {
			t.Fatalf("query error = %#v", result.Error)
		}
	case <-time.After(time.Second):
		t.Fatal("query did not finish")
	}

	select {
	case result := <-switchDone:
		if result.Error != nil {
			t.Fatalf("SwitchProject error = %#v", result.Error)
		}
	case <-time.After(time.Second):
		t.Fatal("SwitchProject did not finish after lease release")
	}
	oldRuntime.mu.Lock()
	closed = oldRuntime.closeCalls
	oldRuntime.mu.Unlock()
	if closed != 1 {
		t.Fatalf("old runtime close calls = %d, want 1", closed)
	}
}

type eventOrderRuntime struct {
	*fakeGatewayRuntime
	closeSawSubscriptionClosed bool
}

func (r *eventOrderRuntime) Close(ctx context.Context) error {
	r.sub.mu.Lock()
	r.closeSawSubscriptionClosed = r.sub.closed
	r.sub.mu.Unlock()
	return r.fakeGatewayRuntime.Close(ctx)
}

func TestGatewaySwitchStopsOldEventsBeforeClosingOldRuntime(t *testing.T) {
	oldRuntime := &eventOrderRuntime{fakeGatewayRuntime: newFakeGatewayRuntime()}
	newRuntime := newFakeGatewayRuntime()
	newRuntime.snapshot = appruntime.DesktopSnapshot{Contract: appruntime.CurrentContract(), Revision: 44}
	gateway := newGatewayWithRuntimeFactory(oldRuntime, nil, func(context.Context, string, bool) (desktopui.RuntimeClient, error) {
		return newRuntime, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	gateway.startup(ctx)

	result := gateway.SwitchProject(ProjectLifecycleRequest{ProjectRoot: t.TempDir()})
	if result.Error != nil {
		t.Fatalf("SwitchProject() error = %#v", result.Error)
	}
	if !oldRuntime.closeSawSubscriptionClosed {
		t.Fatal("old runtime was closed before its event subscription stopped")
	}
}

type blockingSubscribeRuntime struct {
	*fakeGatewayRuntime
	subscribeEntered chan struct{}
	subscribeRelease chan struct{}
}

func (r *blockingSubscribeRuntime) Subscribe(ctx context.Context, cursor appruntime.EventCursor) (appruntime.EventSubscription, error) {
	close(r.subscribeEntered)
	select {
	case <-r.subscribeRelease:
		return r.fakeGatewayRuntime.Subscribe(ctx, cursor)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestGatewaySwitchKeepsReplacementHiddenUntilEventBridgeReady(t *testing.T) {
	oldRuntime := newFakeGatewayRuntime()
	newRuntime := &blockingSubscribeRuntime{
		fakeGatewayRuntime: newFakeGatewayRuntime(),
		subscribeEntered:   make(chan struct{}),
		subscribeRelease:   make(chan struct{}),
	}
	newRuntime.snapshot = appruntime.DesktopSnapshot{Contract: appruntime.CurrentContract(), Revision: 55}
	gateway := newGatewayWithRuntimeFactory(oldRuntime, nil, func(context.Context, string, bool) (desktopui.RuntimeClient, error) {
		return newRuntime, nil
	})

	switchDone := make(chan ProjectLifecycleResult, 1)
	go func() {
		switchDone <- gateway.SwitchProject(ProjectLifecycleRequest{ProjectRoot: t.TempDir()})
	}()
	select {
	case <-newRuntime.subscribeEntered:
	case <-time.After(time.Second):
		t.Fatal("replacement subscription did not start")
	}

	snapshotDone := make(chan GatewaySnapshotResult, 1)
	go func() {
		snapshotDone <- gateway.Snapshot()
	}()
	select {
	case result := <-snapshotDone:
		t.Fatalf("Snapshot observed replacement before event bridge was ready: %#v", result)
	case <-time.After(100 * time.Millisecond):
	}

	close(newRuntime.subscribeRelease)
	select {
	case result := <-switchDone:
		if result.Error != nil {
			t.Fatalf("SwitchProject() error = %#v", result.Error)
		}
	case <-time.After(time.Second):
		t.Fatal("SwitchProject did not finish after subscription became ready")
	}
	select {
	case result := <-snapshotDone:
		if result.Error != nil || result.Data == nil || result.Data.Revision != 55 {
			t.Fatalf("Snapshot after activation = %#v, want revision 55", result)
		}
	case <-time.After(time.Second):
		t.Fatal("Snapshot did not resume after replacement activation")
	}
}

func TestGatewayCloseProjectStopsSessionAndFailsClosed(t *testing.T) {
	runtime := newFakeGatewayRuntime()
	runtime.snapshot = appruntime.DesktopSnapshot{Contract: appruntime.CurrentContract(), Revision: 33}
	gateway := newGateway(runtime, nil)

	result := gateway.CloseProject()
	if result.Error != nil {
		t.Fatalf("CloseProject error = %#v", result.Error)
	}
	runtime.mu.Lock()
	closed := runtime.closeCalls
	runtime.mu.Unlock()
	if closed != 1 {
		t.Fatalf("runtime close calls = %d, want 1", closed)
	}
	snapshot := gateway.Snapshot()
	if snapshot.Error == nil || snapshot.Error.Code != appruntime.ErrorCodeRuntimeUnavailable {
		t.Fatalf("Snapshot after close error = %#v, want runtime_unavailable", snapshot.Error)
	}
}

type handoffGatewayRuntime struct {
	*fakeGatewayRuntime
	suspendCalls int
	resumeCalls  int
	suspendErr   error
	resumeErr    error
}

func newHandoffGatewayRuntime() *handoffGatewayRuntime {
	return &handoffGatewayRuntime{fakeGatewayRuntime: newFakeGatewayRuntime()}
}

func (r *handoffGatewayRuntime) SuspendProjectHandoff(context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.suspendCalls++
	return r.suspendErr
}

func (r *handoffGatewayRuntime) ResumeProjectHandoff(context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.resumeCalls++
	return r.resumeErr
}

func TestGatewaySwitchSuspendsOldRuntimeBeforeBuildingReplacement(t *testing.T) {
	oldRuntime := newHandoffGatewayRuntime()
	oldRuntime.snapshot = appruntime.DesktopSnapshot{Contract: appruntime.CurrentContract(), Revision: 61}
	newRuntime := newFakeGatewayRuntime()
	newRuntime.snapshot = appruntime.DesktopSnapshot{Contract: appruntime.CurrentContract(), Revision: 62}

	rootA := t.TempDir()
	rootB := t.TempDir()
	factorySawSuspended := false

	gateway := newGatewayWithRuntimeFactory(oldRuntime, nil, func(context.Context, string, bool) (desktopui.RuntimeClient, error) {
		oldRuntime.mu.Lock()
		factorySawSuspended = oldRuntime.suspendCalls == 1
		oldRuntime.mu.Unlock()
		return newRuntime, nil
	})
	gateway.projectRoot = rootA

	result := gateway.SwitchProject(ProjectLifecycleRequest{ProjectRoot: rootB})
	if result.Error != nil {
		t.Fatalf("SwitchProject() error = %#v", result.Error)
	}
	if !factorySawSuspended {
		t.Fatal("replacement factory ran before old runtime released WEB/profile ownership")
	}
	oldRuntime.mu.Lock()
	suspendCalls := oldRuntime.suspendCalls
	resumeCalls := oldRuntime.resumeCalls
	oldRuntime.mu.Unlock()
	if suspendCalls != 1 {
		t.Fatalf("old runtime suspend calls = %d, want 1", suspendCalls)
	}
	if resumeCalls != 0 {
		t.Fatalf("old runtime resume calls = %d, want 0 after successful switch", resumeCalls)
	}
}

func TestGatewaySwitchFactoryFailureResumesAndRestoresOldRuntime(t *testing.T) {
	oldRuntime := newHandoffGatewayRuntime()
	oldRuntime.snapshot = appruntime.DesktopSnapshot{Contract: appruntime.CurrentContract(), Revision: 71}
	rootA := t.TempDir()
	rootB := t.TempDir()

	gateway := newGatewayWithRuntimeFactory(oldRuntime, nil, func(context.Context, string, bool) (desktopui.RuntimeClient, error) {
		return nil, errors.New("replacement construction failed")
	})
	gateway.projectRoot = rootA

	result := gateway.SwitchProject(ProjectLifecycleRequest{ProjectRoot: rootB})
	if result.Error == nil {
		t.Fatal("SwitchProject() unexpectedly succeeded")
	}
	oldRuntime.mu.Lock()
	suspendCalls := oldRuntime.suspendCalls
	resumeCalls := oldRuntime.resumeCalls
	closeCalls := oldRuntime.closeCalls
	oldRuntime.mu.Unlock()
	if suspendCalls != 1 || resumeCalls != 1 {
		t.Fatalf("handoff calls suspend=%d resume=%d, want 1/1", suspendCalls, resumeCalls)
	}
	if closeCalls != 0 {
		t.Fatalf("old runtime close calls = %d, want 0 after replacement factory failure", closeCalls)
	}
	if gateway.projectRoot != rootA {
		t.Fatalf("project root after failed switch = %q, want old root %q", gateway.projectRoot, rootA)
	}
	snapshot := gateway.Snapshot()
	if snapshot.Error != nil || snapshot.Data == nil || snapshot.Data.Revision != 71 {
		t.Fatalf("old runtime was not restored after failed switch: %#v", snapshot)
	}
}

func TestGatewaySwitchSuspendFailureDoesNotRunReplacementFactory(t *testing.T) {
	oldRuntime := newHandoffGatewayRuntime()
	oldRuntime.snapshot = appruntime.DesktopSnapshot{Contract: appruntime.CurrentContract(), Revision: 81}
	oldRuntime.suspendErr = errors.New("suspend failed")
	rootA := t.TempDir()
	rootB := t.TempDir()
	factoryCalls := 0

	gateway := newGatewayWithRuntimeFactory(oldRuntime, nil, func(context.Context, string, bool) (desktopui.RuntimeClient, error) {
		factoryCalls++
		return newFakeGatewayRuntime(), nil
	})
	gateway.projectRoot = rootA

	result := gateway.SwitchProject(ProjectLifecycleRequest{ProjectRoot: rootB})
	if result.Error == nil {
		t.Fatal("SwitchProject() unexpectedly succeeded")
	}
	if factoryCalls != 0 {
		t.Fatalf("replacement factory calls = %d, want 0 after suspend failure", factoryCalls)
	}
	if gateway.projectRoot != rootA {
		t.Fatalf("project root after suspend failure = %q, want %q", gateway.projectRoot, rootA)
	}
	snapshot := gateway.Snapshot()
	if snapshot.Error != nil || snapshot.Data == nil || snapshot.Data.Revision != 81 {
		t.Fatalf("old runtime unavailable after suspend failure: %#v", snapshot)
	}
}
