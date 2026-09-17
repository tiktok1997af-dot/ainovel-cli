package main

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/voocel/ainovel-cli/internal/appruntime"
)

func TestGatewayExposesTypedProjectLifecycleBindings(t *testing.T) {
	typeOfGateway := reflect.TypeOf(&Gateway{})
	for _, name := range []string{"CreateProject", "OpenProject", "SwitchProject", "CloseProject"} {
		if _, ok := typeOfGateway.MethodByName(name); !ok {
			t.Errorf("Gateway is missing typed lifecycle binding %s", name)
		}
	}
	if _, ok := typeOfGateway.MethodByName("Invoke"); ok {
		t.Fatal("Gateway must not expose generic Invoke")
	}
}

func TestGatewayLifecycleRejectsBlankProjectRoot(t *testing.T) {
	gateway := newGateway(nil, nil)
	for name, result := range map[string]ProjectLifecycleResult{
		"create": gateway.CreateProject(ProjectLifecycleRequest{}),
		"open":   gateway.OpenProject(ProjectLifecycleRequest{}),
		"switch": gateway.SwitchProject(ProjectLifecycleRequest{}),
	} {
		if result.Error == nil || result.Error.Code != appruntime.ErrorCodeInvalidArgument {
			t.Errorf("%s blank root error = %#v, want invalid_argument", name, result.Error)
		}
	}
}

func TestGatewayOpenMissingProjectIsReadOnly(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing-project")
	gateway := newGateway(nil, nil)

	result := gateway.OpenProject(ProjectLifecycleRequest{ProjectRoot: root})
	if result.Error == nil || result.Error.Code != appruntime.ErrorCodeTargetNotFound {
		t.Fatalf("OpenProject() error = %#v, want target_not_found", result.Error)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("OpenProject() mutated missing target, stat err = %v", err)
	}
}

func TestGatewayCreateRefusesNonEmptyTargetWithoutMutation(t *testing.T) {
	root := t.TempDir()
	sentinel := filepath.Join(root, "keep.txt")
	const want = "do-not-touch"
	if err := os.WriteFile(sentinel, []byte(want), 0o600); err != nil {
		t.Fatal(err)
	}
	gateway := newGateway(nil, nil)

	result := gateway.CreateProject(ProjectLifecycleRequest{ProjectRoot: root})
	if result.Error == nil || result.Error.Code != appruntime.ErrorCodePreconditionConflict {
		t.Fatalf("CreateProject() error = %#v, want precondition_conflict", result.Error)
	}
	got, err := os.ReadFile(sentinel)
	if err != nil {
		t.Fatalf("read sentinel after rejected create: %v", err)
	}
	if string(got) != want {
		t.Fatalf("sentinel after rejected create = %q, want %q", got, want)
	}
}

func TestGatewayCloseProjectWaitsForInFlightQueryAndFailsClosed(t *testing.T) {
	runtime := &blockingGatewayRuntime{
		fakeGatewayRuntime: newFakeGatewayRuntime(),
		queryEntered:       make(chan struct{}),
		queryRelease:       make(chan struct{}),
	}
	runtime.queryResult = appruntime.QueryResult{ContractVersion: appruntime.ContractVersion}
	gateway := newGateway(runtime, nil)

	queryDone := make(chan appruntime.QueryResult, 1)
	go func() {
		queryDone <- gateway.Query(appruntime.QueryRequest{Kind: appruntime.QueryProjectOverview})
	}()
	select {
	case <-runtime.queryEntered:
	case <-time.After(time.Second):
		t.Fatal("query did not enter runtime")
	}

	closeDone := make(chan ProjectLifecycleResult, 1)
	go func() {
		closeDone <- gateway.CloseProject()
	}()
	select {
	case result := <-closeDone:
		t.Fatalf("CloseProject returned while Query held runtime lease: %#v", result)
	case <-time.After(100 * time.Millisecond):
	}

	runtime.mu.Lock()
	closed := runtime.closeCalls
	runtime.mu.Unlock()
	if closed != 0 {
		t.Fatalf("runtime close calls during in-flight Query = %d, want 0", closed)
	}

	close(runtime.queryRelease)
	select {
	case result := <-queryDone:
		if result.Error != nil {
			t.Fatalf("in-flight query failed: %#v", result.Error)
		}
	case <-time.After(time.Second):
		t.Fatal("query did not complete")
	}
	select {
	case result := <-closeDone:
		if result.Error != nil {
			t.Fatalf("CloseProject() error = %#v", result.Error)
		}
		if result.Active {
			t.Fatal("CloseProject() returned active=true")
		}
	case <-time.After(time.Second):
		t.Fatal("CloseProject did not complete after Query released lease")
	}

	runtime.mu.Lock()
	closed = runtime.closeCalls
	runtime.mu.Unlock()
	if closed != 1 {
		t.Fatalf("runtime close calls after CloseProject = %d, want 1", closed)
	}

	after := gateway.Query(appruntime.QueryRequest{Kind: appruntime.QueryProjectOverview})
	if after.Error == nil || after.Error.Code != appruntime.ErrorCodeRuntimeUnavailable {
		t.Fatalf("Query after CloseProject error = %#v, want runtime_unavailable", after.Error)
	}

	_ = context.Background()
}
