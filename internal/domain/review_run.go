package domain

import (
	"fmt"
	"strings"
)

const (
	ReviewWorkRun    = "review.run"
	ReviewWorkRepair = "review.repair"
	ReviewWorkRerun  = "review.rerun"
)

// ReviewWorkTarget is the serialization-safe durable identity of one Review
// target. It intentionally mirrors only canonical semantic coordinates and
// never carries paths or browser/provider state.
type ReviewWorkTarget struct {
	Scope          string `json:"scope"`
	Chapter        int    `json:"chapter,omitempty"`
	Volume         int    `json:"volume,omitempty"`
	Arc            int    `json:"arc,omitempty"`
	ThroughChapter int    `json:"through_chapter,omitempty"`
}

func (t ReviewWorkTarget) Validate() error {
	switch t.Scope {
	case "chapter":
		if t.Chapter <= 0 || t.Volume != 0 || t.Arc != 0 || t.ThroughChapter != 0 {
			return fmt.Errorf("invalid chapter review target")
		}
	case "arc":
		if t.Chapter != 0 || t.Volume <= 0 || t.Arc <= 0 || t.ThroughChapter <= 0 {
			return fmt.Errorf("invalid arc review target")
		}
	case "global":
		if t.Chapter != 0 || t.Volume != 0 || t.Arc != 0 || t.ThroughChapter <= 0 {
			return fmt.Errorf("invalid global review target")
		}
	default:
		return fmt.Errorf("unsupported review scope %q", t.Scope)
	}
	return nil
}

// ReviewRunWork is the minimal durable payload needed for the G06.4 scheduler
// to restart one bounded review action after a process crash. It is stored on
// the existing RunRegistryRecord so the G05 scheduler/resource/lane/recovery
// authority remains the only execution authority.
type ReviewRunWork struct {
	Action              string           `json:"action"`
	Target              ReviewWorkTarget `json:"target"`
	ExpectedFingerprint string           `json:"expected_fingerprint,omitempty"`
	GateIDs             []string         `json:"gate_ids,omitempty"`
	Chapters            []int            `json:"chapters,omitempty"`
	RepairMode          string           `json:"repair_mode,omitempty"` // rewrite / polish
}

func (w ReviewRunWork) Validate() error {
	if err := w.Target.Validate(); err != nil {
		return err
	}
	if w.ExpectedFingerprint != "" {
		if err := validateRunRecordText("expected_fingerprint", w.ExpectedFingerprint, 256, true); err != nil {
			return err
		}
	}

	switch w.Action {
	case ReviewWorkRun:
		if len(w.GateIDs) != 0 || len(w.Chapters) != 0 || w.RepairMode != "" {
			return fmt.Errorf("review.run must not carry repair fields")
		}
	case ReviewWorkRerun:
		if w.ExpectedFingerprint == "" {
			return fmt.Errorf("review.rerun requires expected_fingerprint")
		}
		if len(w.GateIDs) != 0 || len(w.Chapters) != 0 || w.RepairMode != "" {
			return fmt.Errorf("review.rerun must not carry repair fields")
		}
	case ReviewWorkRepair:
		if w.ExpectedFingerprint == "" {
			return fmt.Errorf("review.repair requires expected_fingerprint")
		}
		if len(w.Chapters) == 0 {
			return fmt.Errorf("review.repair requires at least one chapter")
		}
		if w.RepairMode != "rewrite" && w.RepairMode != "polish" {
			return fmt.Errorf("review.repair requires repair_mode rewrite or polish")
		}
		seenChapters := make(map[int]struct{}, len(w.Chapters))
		for _, chapter := range w.Chapters {
			if chapter <= 0 {
				return fmt.Errorf("review.repair chapter must be > 0")
			}
			if _, exists := seenChapters[chapter]; exists {
				return fmt.Errorf("duplicate review.repair chapter %d", chapter)
			}
			seenChapters[chapter] = struct{}{}
		}
		seenGates := make(map[string]struct{}, len(w.GateIDs))
		for _, gateID := range w.GateIDs {
			gateID = strings.TrimSpace(gateID)
			if gateID == "" {
				return fmt.Errorf("review.repair gate id must not be empty")
			}
			if err := validateRunRecordText("review_gate_id", gateID, 128, true); err != nil {
				return err
			}
			if _, exists := seenGates[gateID]; exists {
				return fmt.Errorf("duplicate review.repair gate id %q", gateID)
			}
			seenGates[gateID] = struct{}{}
		}
	default:
		return fmt.Errorf("unsupported review work action %q", w.Action)
	}
	return nil
}
