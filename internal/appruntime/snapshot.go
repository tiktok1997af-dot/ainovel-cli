package appruntime

import (
	"time"

	"github.com/voocel/ainovel-cli/internal/host"
	"github.com/voocel/ainovel-cli/internal/webai"
)

const (
	productName      = "AINOVEL"
	coreBaseline     = "ainovel-cli v0.1.3"
	executionModeWeb = "web-only"
)

func projectDesktopSnapshot(src host.UISnapshot, browser webai.SessionSnapshot, outputDir string, revision uint64, generatedAt time.Time) DesktopSnapshot {
	outline := make([]OutlineItemSnapshot, 0, len(src.Outline))
	for _, item := range src.Outline {
		outline = append(outline, OutlineItemSnapshot{
			Chapter:   item.Chapter,
			Title:     item.Title,
			CoreEvent: item.CoreEvent,
		})
	}

	agents := make([]AgentViewSnapshot, 0, len(src.Agents))
	for _, agent := range src.Agents {
		agents = append(agents, AgentViewSnapshot{
			Name:     agent.Name,
			State:    agent.State,
			TaskID:   agent.TaskID,
			TaskKind: agent.TaskKind,
			Summary:  agent.Summary,
			Tool:     agent.Tool,
			Turn:     agent.Turn,
			Context: AgentContextViewSnapshot{
				Tokens:          agent.Context.Tokens,
				ContextWindow:   agent.Context.ContextWindow,
				Percent:         agent.Context.Percent,
				Scope:           agent.Context.Scope,
				Strategy:        agent.Context.Strategy,
				ActiveMessages:  agent.Context.ActiveMessages,
				SummaryMessages: agent.Context.SummaryMessages,
				CompactedCount:  agent.Context.CompactedCount,
				KeptCount:       agent.Context.KeptCount,
			},
			UpdatedAt: utcTime(agent.UpdatedAt),
		})
	}

	return DesktopSnapshot{
		Revision:    revision,
		GeneratedAt: utcTime(generatedAt),
		Product: ProductViewSnapshot{
			Name:          productName,
			CoreBaseline:  coreBaseline,
			ExecutionMode: executionModeWeb,
		},
		Project: ProjectViewSnapshot{
			Title:            src.BookTitle,
			OutputDir:        outputDir,
			Synopsis:         src.Synopsis,
			Premise:          src.Premise,
			Style:            src.Style,
			Layered:          src.Layered,
			Outline:          outline,
			Characters:       append([]string(nil), src.Characters...),
			SupportingCount:  src.SupportingCount,
			RecentSupporting: append([]string(nil), src.RecentSupporting...),
			CurrentVolumeArc: src.CurrentVolumeArc,
			NextVolumeTitle:  src.NextVolumeTitle,
			CompassDirection: src.CompassDirection,
			CompassScale:     src.CompassScale,
		},
		Runtime: RuntimeViewSnapshot{
			State:                src.RuntimeState,
			Status:               src.StatusLabel,
			Phase:                src.Phase,
			Flow:                 src.Flow,
			IsRunning:            src.IsRunning,
			Provider:             src.Provider,
			Model:                src.ModelName,
			ModelContextWindow:   src.ModelContextWindow,
			ThinkingLevel:        src.ThinkingLevel,
			PendingSteer:         src.PendingSteer,
			AdvanceMode:          src.AdvanceMode,
			AdvancePermitChapter: src.AdvancePermitChapter,
			HasAdvanceHold:       src.HasAdvanceHold,
			AdvanceHoldReason:    src.AdvanceHoldReason,
			AITelemetryStatus:    src.AITelemetryStatus,
		},
		CurrentChapter: ChapterViewSnapshot{
			Current:        src.CurrentChapter,
			InProgress:     src.InProgressChapter,
			Total:          src.TotalChapters,
			Completed:      src.CompletedCount,
			TotalWordCount: src.TotalWordCount,
		},
		Agents: agents,
		Browser: BrowserViewSnapshot{
			State:       string(browser.State),
			Site:        browser.Site,
			BrowserPath: browser.BrowserPath,
			ProfileDir:  browser.ProfileDir,
			PID:         browser.PID,
			StartedAt:   utcTime(browser.StartedAt),
			ChangedAt:   utcTime(browser.ChangedAt),
			Reason:      browser.Reason,
		},
		Recovery: RecoveryViewSnapshot{
			Label:          src.RecoveryLabel,
			CanResume:      src.RecoveryLabel != "",
			LastCheckpoint: src.LastCheckpointName,
		},
		QualitySummary: QualitySummaryViewSnapshot{
			LastCommitSummary: src.LastCommitSummary,
			LastReviewSummary: src.LastReviewSummary,
			PendingRewrites:   append([]int(nil), src.PendingRewrites...),
			RewriteReason:     src.RewriteReason,
			RecentSummaries:   append([]string(nil), src.RecentSummaries...),
		},
	}
}

func utcTime(t time.Time) time.Time {
	if t.IsZero() {
		return time.Time{}
	}
	return t.UTC()
}
