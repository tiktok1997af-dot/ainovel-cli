package appruntime

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/voocel/ainovel-cli/internal/host"
	"github.com/voocel/ainovel-cli/internal/webai"
)

func TestProjectDesktopSnapshotMapsAllTopLevelSections(t *testing.T) {
	generatedAt := time.Date(2026, 9, 9, 13, 0, 0, 0, time.FixedZone("UTC+7", 7*60*60))
	agentUpdated := generatedAt.Add(-time.Minute)
	browserChanged := generatedAt.Add(-2 * time.Minute)

	src := host.UISnapshot{
		Provider:             "gemini-web",
		BookTitle:            "Demo Novel",
		ModelName:            "gemini",
		ModelContextWindow:   100000,
		ThinkingLevel:        "high",
		Style:                "webnovel",
		RuntimeState:         "running",
		StatusLabel:          "Writing chapter 12",
		Phase:                "writing",
		Flow:                 "drafting",
		CurrentChapter:       12,
		TotalChapters:        1500,
		CompletedCount:       11,
		TotalWordCount:       42000,
		InProgressChapter:    12,
		PendingRewrites:      []int{9, 10},
		RewriteReason:        "review",
		PendingSteer:         "slow down pacing",
		AdvanceMode:          "review",
		AdvancePermitChapter: 12,
		HasAdvanceHold:       true,
		AdvanceHoldReason:    "await review",
		RecoveryLabel:        "resume chapter 12",
		IsRunning:            true,
		AITelemetryStatus:    "unavailable",
		Synopsis:             "synopsis",
		Premise:              "premise",
		Outline: []host.OutlineSnapshot{{
			Chapter: 12, Title: "Chapter 12", CoreEvent: "turning point",
		}},
		Characters:         []string{"A", "B"},
		SupportingCount:    2,
		RecentSupporting:   []string{"B"},
		Layered:            true,
		CurrentVolumeArc:   "V2 A3",
		NextVolumeTitle:    "Volume 3",
		CompassDirection:   "north",
		CompassScale:       "long",
		LastCommitSummary:  "commit 11",
		LastReviewSummary:  "review 11",
		LastCheckpointName: "checkpoint-11",
		RecentSummaries:    []string{"summary 10", "summary 11"},
		Agents: []host.AgentSnapshot{{
			Name: "writer", State: "working", TaskID: "task-1", TaskKind: "write",
			Summary: "drafting", Tool: "draft_chapter", Turn: 3, UpdatedAt: agentUpdated,
			Context: host.AgentContextSnapshot{
				Tokens: 1000, ContextWindow: 100000, Percent: 1,
				Scope: "chapter", Strategy: "compact", ActiveMessages: 4,
				SummaryMessages: 1, CompactedCount: 2, KeptCount: 3,
			},
		}},
	}
	browser := webai.SessionSnapshot{
		State: webai.SessionReady, Site: "gemini-web", BrowserPath: "chrome",
		ProfileDir: "profile", PID: 123, StartedAt: generatedAt.Add(-time.Hour),
		ChangedAt: browserChanged, Reason: "ready",
	}

	got := projectDesktopSnapshot(src, browser, "/project", 7, generatedAt)

	if got.Revision != 7 || got.GeneratedAt.Location() != time.UTC {
		t.Fatalf("revision/time projection mismatch: %+v", got)
	}
	if got.Product.Name != productName || got.Product.CoreBaseline != coreBaseline || got.Product.ExecutionMode != executionModeWeb {
		t.Fatalf("product projection mismatch: %+v", got.Product)
	}
	if got.Project.Title != src.BookTitle || got.Project.OutputDir != "/project" || len(got.Project.Outline) != 1 {
		t.Fatalf("project projection mismatch: %+v", got.Project)
	}
	if got.Runtime.State != "running" || got.Runtime.Provider != "gemini-web" || !got.Runtime.HasAdvanceHold {
		t.Fatalf("runtime projection mismatch: %+v", got.Runtime)
	}
	if got.CurrentChapter.Current != 12 || got.CurrentChapter.Completed != 11 || got.CurrentChapter.TotalWordCount != 42000 {
		t.Fatalf("chapter projection mismatch: %+v", got.CurrentChapter)
	}
	if len(got.Agents) != 1 || got.Agents[0].TaskID != "task-1" || got.Agents[0].UpdatedAt.Location() != time.UTC {
		t.Fatalf("agent projection mismatch: %+v", got.Agents)
	}
	if got.Browser.State != string(webai.SessionReady) || got.Browser.PID != 123 || got.Browser.ChangedAt.Location() != time.UTC {
		t.Fatalf("browser projection mismatch: %+v", got.Browser)
	}
	if !got.Recovery.CanResume || got.Recovery.LastCheckpoint != "checkpoint-11" {
		t.Fatalf("recovery projection mismatch: %+v", got.Recovery)
	}
	if len(got.QualitySummary.PendingRewrites) != 2 || got.QualitySummary.LastReviewSummary != "review 11" {
		t.Fatalf("quality projection mismatch: %+v", got.QualitySummary)
	}

	if _, err := json.Marshal(got); err != nil {
		t.Fatalf("DesktopSnapshot JSON marshal failed: %v", err)
	}
}

func TestProjectDesktopSnapshotDoesNotAliasHostSlices(t *testing.T) {
	src := host.UISnapshot{
		Characters:       []string{"A"},
		RecentSupporting: []string{"B"},
		PendingRewrites:  []int{3},
		RecentSummaries:  []string{"S"},
		Outline:          []host.OutlineSnapshot{{Chapter: 1, Title: "T"}},
	}
	got := projectDesktopSnapshot(src, webai.SessionSnapshot{}, "/project", 1, time.Now())

	src.Characters[0] = "changed"
	src.RecentSupporting[0] = "changed"
	src.PendingRewrites[0] = 99
	src.RecentSummaries[0] = "changed"
	src.Outline[0].Title = "changed"

	if got.Project.Characters[0] != "A" || got.Project.RecentSupporting[0] != "B" {
		t.Fatal("project DTO aliases host string slices")
	}
	if got.QualitySummary.PendingRewrites[0] != 3 || got.QualitySummary.RecentSummaries[0] != "S" {
		t.Fatal("quality DTO aliases host slices")
	}
	if got.Project.Outline[0].Title != "T" {
		t.Fatal("outline DTO aliases host outline slice")
	}
}

func TestRecoveryCanResumeRequiresRecoveryFact(t *testing.T) {
	got := projectDesktopSnapshot(host.UISnapshot{LastCheckpointName: "checkpoint-only"}, webai.SessionSnapshot{}, "", 1, time.Now())
	if got.Recovery.CanResume {
		t.Fatal("checkpoint presence alone must not invent resumability")
	}
}
