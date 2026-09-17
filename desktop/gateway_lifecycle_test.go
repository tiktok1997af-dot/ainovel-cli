package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

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
