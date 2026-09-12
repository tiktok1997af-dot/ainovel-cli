package domain

import (
	"strings"
	"testing"
)

func TestRunIdentityValidateAcceptsOpaqueLogicalIdentifiers(t *testing.T) {
	id := RunIdentity{
		RunID:    "run-20260911-001",
		TaskID:   "task_01",
		Resource: "project:alpha",
		LaneID:   "lane-1",
	}
	if err := id.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestRunIdentityValidateAllowsOptionalTaskResourceAndLane(t *testing.T) {
	if err := (RunIdentity{RunID: "run-1"}).Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestRunIdentityValidateRejectsInvalidIdentities(t *testing.T) {
	tests := []struct {
		name string
		id   RunIdentity
	}{
		{name: "missing run", id: RunIdentity{}},
		{name: "surrounding whitespace", id: RunIdentity{RunID: " run-1"}},
		{name: "embedded whitespace", id: RunIdentity{RunID: "run 1"}},
		{name: "path slash", id: RunIdentity{RunID: "run/1"}},
		{name: "path backslash", id: RunIdentity{RunID: `run\1`}},
		{name: "control", id: RunIdentity{RunID: "run\n1"}},
		{name: "unsupported punctuation", id: RunIdentity{RunID: "run@1"}},
		{name: "task path", id: RunIdentity{RunID: "run-1", TaskID: "task/1"}},
		{name: "resource path", id: RunIdentity{RunID: "run-1", Resource: `C:\\book`}},
		{name: "lane path", id: RunIdentity{RunID: "run-1", LaneID: "profile/lane"}},
		{name: "run too long", id: RunIdentity{RunID: RunID(strings.Repeat("a", MaxRunIDLength+1))}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.id.Validate(); err == nil {
				t.Fatal("Validate() error = nil, want rejection")
			}
		})
	}
}
