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

// ReviewWorkTarget is the serialization-safe durable identity of one Review target.
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

type ReviewWorkRevision struct {
	Chapter       int    `json:"chapter"`
	Revision      int    `json:"revision"`
	ContentSHA256 string `json:"content_sha256"`
}

func (r ReviewWorkRevision) Validate() error {
	if r.Chapter <= 0 || r.Revision <= 0 {
		return fmt.Errorf("review work revision requires positive chapter and revision")
	}
	if strings.TrimSpace(r.ContentSHA256) == "" {
		return fmt.Errorf("review work revision requires content_sha256")
	}
	if err := validateRunRecordText("content_sha256", r.ContentSHA256, 128, true); err != nil {
		return err
	}
	return nil
}

// ReviewRunWork is the minimal durable payload needed for the G06.4 scheduler
// to restart one bounded Review action after a process crash. Prepared,
// CompletedChapters and Executed are orchestration progress facts, not story facts.
// ExpectedRevisions and BaselineReviewSeq close the cross-file crash window:
// recovery can prove that a chapter/review already committed before the process
// died even when the final run-metadata update did not happen.
type ReviewRunWork struct {
	Action              string               `json:"action"`
	Target              ReviewWorkTarget     `json:"target"`
	ExpectedFingerprint string               `json:"expected_fingerprint,omitempty"`
	ExpectedRevisions   []ReviewWorkRevision `json:"expected_revisions,omitempty"`
	BaselineReviewSeq   int64                `json:"baseline_review_seq,omitempty"`
	GateIDs             []string             `json:"gate_ids,omitempty"`
	Chapters            []int                `json:"chapters,omitempty"`
	RepairMode          string               `json:"repair_mode,omitempty"` // rewrite / polish
	Prepared            bool                 `json:"prepared,omitempty"`
	CompletedChapters   []int                `json:"completed_chapters,omitempty"`
	Executed            bool                 `json:"executed,omitempty"`
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
	if w.BaselineReviewSeq < 0 {
		return fmt.Errorf("baseline_review_seq must be >= 0")
	}
	seenRevisions := make(map[int]struct{}, len(w.ExpectedRevisions))
	for _, revision := range w.ExpectedRevisions {
		if err := revision.Validate(); err != nil {
			return err
		}
		if _, exists := seenRevisions[revision.Chapter]; exists {
			return fmt.Errorf("duplicate expected revision chapter %d", revision.Chapter)
		}
		seenRevisions[revision.Chapter] = struct{}{}
	}

	switch w.Action {
	case ReviewWorkRun:
		if len(w.GateIDs) != 0 || len(w.Chapters) != 0 || w.RepairMode != "" || len(w.CompletedChapters) != 0 {
			return fmt.Errorf("review.run must not carry repair fields")
		}
	case ReviewWorkRerun:
		if w.ExpectedFingerprint == "" {
			return fmt.Errorf("review.rerun requires expected_fingerprint")
		}
		if len(w.GateIDs) != 0 || len(w.Chapters) != 0 || w.RepairMode != "" || len(w.CompletedChapters) != 0 {
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
			if _, exists := seenRevisions[chapter]; !exists {
				return fmt.Errorf("review.repair chapter %d has no expected revision", chapter)
			}
		}
		seenCompleted := make(map[int]struct{}, len(w.CompletedChapters))
		for _, chapter := range w.CompletedChapters {
			if _, allowed := seenChapters[chapter]; !allowed {
				return fmt.Errorf("completed repair chapter %d is outside bounded repair set", chapter)
			}
			if _, exists := seenCompleted[chapter]; exists {
				return fmt.Errorf("duplicate completed repair chapter %d", chapter)
			}
			seenCompleted[chapter] = struct{}{}
		}
		if w.Executed && len(seenCompleted) != len(seenChapters) {
			return fmt.Errorf("executed review.repair must complete every bounded chapter")
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
