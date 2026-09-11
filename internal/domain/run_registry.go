package domain

import (
	"fmt"
	"strings"
	"time"
	"unicode"
)

const CurrentRunRegistryRecordVersion = 1

// LegacySingleRunID is a reserved synthetic identity used only to project
// pre-G05 single-run projects without rewriting their legacy meta/run.json.
const LegacySingleRunID RunID = "legacy-single-run"

type RunRecordSource string

const (
	RunRecordSourceRegistry        RunRecordSource = "registry"
	RunRecordSourceLegacySingleRun RunRecordSource = "legacy_single_run"
)

// RunRegistryRecord is the additive durable per-run fact stored by G05.3.
// State and Status are intentionally opaque: G05.3 persists facts but does not
// define scheduler ordering, priority policy, recovery transitions, locks or lanes.
type RunRegistryRecord struct {
	Version     int             `json:"version"`
	RunID       RunID           `json:"run_id"`
	Source      RunRecordSource `json:"source"`
	State       string          `json:"state,omitempty"`
	Status      string          `json:"status,omitempty"`
	TaskID      TaskID          `json:"task_id,omitempty"`
	LegacyPhase Phase           `json:"legacy_phase,omitempty"`
	CreatedAt   time.Time       `json:"created_at,omitempty"`
	UpdatedAt   time.Time       `json:"updated_at,omitempty"`
	StartedAt   time.Time       `json:"started_at,omitempty"`
	FinishedAt  time.Time       `json:"finished_at,omitempty"`
}

func (r RunRegistryRecord) Validate() error {
	if r.Version != CurrentRunRegistryRecordVersion {
		return fmt.Errorf("unsupported run registry record version %d", r.Version)
	}
	if err := (RunIdentity{RunID: r.RunID, TaskID: r.TaskID}).Validate(); err != nil {
		return err
	}
	switch r.Source {
	case RunRecordSourceRegistry:
		if r.RunID == LegacySingleRunID {
			return fmt.Errorf("run_id %q is reserved for legacy projection", r.RunID)
		}
		if r.LegacyPhase != "" {
			return fmt.Errorf("registry run must not carry legacy_phase")
		}
	case RunRecordSourceLegacySingleRun:
		if r.RunID != LegacySingleRunID {
			return fmt.Errorf("legacy projection must use run_id %q", LegacySingleRunID)
		}
	default:
		return fmt.Errorf("unsupported run record source %q", r.Source)
	}
	if err := validateRunRecordText("state", r.State, 128, true); err != nil {
		return err
	}
	if err := validateRunRecordText("status", r.Status, 512, false); err != nil {
		return err
	}
	if !r.CreatedAt.IsZero() && !r.UpdatedAt.IsZero() && r.UpdatedAt.Before(r.CreatedAt) {
		return fmt.Errorf("updated_at precedes created_at")
	}
	if !r.StartedAt.IsZero() && !r.FinishedAt.IsZero() && r.FinishedAt.Before(r.StartedAt) {
		return fmt.Errorf("finished_at precedes started_at")
	}
	return nil
}

// RunHistoryRecord is an append-only, serialization-safe per-run activity fact.
// It deliberately omits arbitrary payload bodies and browser/provider secrets.
type RunHistoryRecord struct {
	Seq      int64     `json:"seq"`
	Time     time.Time `json:"time"`
	RunID    RunID     `json:"run_id"`
	TaskID   TaskID    `json:"task_id,omitempty"`
	Category string    `json:"category"`
	Type     string    `json:"type"`
	Level    string    `json:"level,omitempty"`
	Summary  string    `json:"summary,omitempty"`
}

func (r RunHistoryRecord) Validate() error {
	if r.Seq <= 0 {
		return fmt.Errorf("history seq must be > 0")
	}
	if r.Time.IsZero() {
		return fmt.Errorf("history time is required")
	}
	if err := (RunIdentity{RunID: r.RunID, TaskID: r.TaskID}).Validate(); err != nil {
		return err
	}
	if err := validateRunRecordText("category", r.Category, 64, true); err != nil {
		return err
	}
	if err := validateRunRecordText("type", r.Type, 128, true); err != nil {
		return err
	}
	if err := validateRunRecordText("level", r.Level, 32, true); err != nil {
		return err
	}
	if err := validateRunRecordText("summary", r.Summary, 2048, false); err != nil {
		return err
	}
	return nil
}

func validateRunRecordText(field, value string, max int, token bool) error {
	if value == "" {
		return nil
	}
	if len(value) > max {
		return fmt.Errorf("%s exceeds %d bytes", field, max)
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("%s must not contain surrounding whitespace", field)
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return fmt.Errorf("%s must not contain control characters", field)
		}
		if token && unicode.IsSpace(r) {
			return fmt.Errorf("%s must not contain whitespace", field)
		}
	}
	return nil
}
