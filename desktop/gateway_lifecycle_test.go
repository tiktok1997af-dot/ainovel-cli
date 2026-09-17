package main

import (
	"reflect"
	"testing"
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
