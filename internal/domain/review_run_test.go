package domain

import "testing"

func validRepairWork() ReviewRunWork {
	return ReviewRunWork{
		Action:              ReviewWorkRepair,
		Target:              ReviewWorkTarget{Scope: "chapter", Chapter: 7},
		ExpectedFingerprint: "sha256:abc",
		ExpectedRevisions: []ReviewWorkRevision{{
			Chapter:       7,
			Revision:      3,
			ContentSHA256: "abc",
		}},
		GateIDs:    []string{"consistency"},
		Chapters:   []int{7},
		RepairMode: "rewrite",
	}
}

func TestReviewRunWorkValidRepair(t *testing.T) {
	work := validRepairWork()
	if err := work.Validate(); err != nil {
		t.Fatalf("valid repair rejected: %v", err)
	}
}

func TestReviewRunWorkRepairRequiresExpectedRevisionForEveryChapter(t *testing.T) {
	work := validRepairWork()
	work.Chapters = append(work.Chapters, 8)
	if err := work.Validate(); err == nil {
		t.Fatal("repair without expected revision for chapter 8 must fail")
	}
}

func TestReviewRunWorkCompletedChapterMustRemainBounded(t *testing.T) {
	work := validRepairWork()
	work.Prepared = true
	work.CompletedChapters = []int{8}
	if err := work.Validate(); err == nil {
		t.Fatal("completed chapter outside bounded repair set must fail")
	}
}

func TestReviewRunWorkExecutedRepairRequiresEveryChapterComplete(t *testing.T) {
	work := validRepairWork()
	work.Prepared = true
	work.Executed = true
	if err := work.Validate(); err == nil {
		t.Fatal("executed repair with incomplete bounded chapter set must fail")
	}
}

func TestReviewRunWorkRejectsPromoteOfficialInG064(t *testing.T) {
	work := ReviewRunWork{
		Action:              "review.promote_official",
		Target:              ReviewWorkTarget{Scope: "chapter", Chapter: 7},
		ExpectedFingerprint: "sha256:abc",
	}
	if err := work.Validate(); err == nil {
		t.Fatal("review.promote_official must remain unsupported durable work in G06.4")
	}
}
