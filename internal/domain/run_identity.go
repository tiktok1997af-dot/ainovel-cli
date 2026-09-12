package domain

import (
	"fmt"
	"strings"
	"unicode"
)

const (
	MaxRunIDLength     = 128
	MaxTaskIDLength    = 128
	MaxResourceKeySize = 256
	MaxBrowserLaneID   = 128
)

// RunID is an opaque stable identifier for one runtime run. It is deliberately
// not a filesystem path, project path, browser profile path, or story authority.
type RunID string

// TaskID is an opaque stable identifier for work owned by a run.
type TaskID string

// ResourceKey is an opaque logical orchestration resource. G05.5 owns canonical
// resource derivation and locking; callers must never treat this value as a path.
type ResourceKey string

// BrowserLaneID identifies one bounded browser execution lane. It never carries
// a Chrome profile path, credentials, cookies, or other browser secrets.
type BrowserLaneID string

// RunIdentity is the serialization-safe identity envelope shared by later G05
// scheduler, persistence, lane, event and Run Center contracts.
type RunIdentity struct {
	RunID    RunID         `json:"run_id"`
	TaskID   TaskID        `json:"task_id,omitempty"`
	Resource ResourceKey   `json:"resource,omitempty"`
	LaneID   BrowserLaneID `json:"lane_id,omitempty"`
}

func (id RunIdentity) Validate() error {
	if err := validateRuntimeIdentity("run_id", string(id.RunID), MaxRunIDLength, true); err != nil {
		return err
	}
	if err := validateRuntimeIdentity("task_id", string(id.TaskID), MaxTaskIDLength, false); err != nil {
		return err
	}
	if err := validateRuntimeIdentity("resource", string(id.Resource), MaxResourceKeySize, false); err != nil {
		return err
	}
	if err := validateRuntimeIdentity("lane_id", string(id.LaneID), MaxBrowserLaneID, false); err != nil {
		return err
	}
	return nil
}

func validateRuntimeIdentity(field, value string, max int, required bool) error {
	if value == "" {
		if required {
			return fmt.Errorf("%s is required", field)
		}
		return nil
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("%s must not contain surrounding whitespace", field)
	}
	if len(value) > max {
		return fmt.Errorf("%s exceeds %d bytes", field, max)
	}
	if strings.ContainsAny(value, `/\\`) {
		return fmt.Errorf("%s must be opaque and not path-like", field)
	}
	for _, r := range value {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return fmt.Errorf("%s must not contain whitespace or control characters", field)
		}
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("._:-", r)) {
			return fmt.Errorf("%s contains unsupported character %q", field, r)
		}
	}
	return nil
}
