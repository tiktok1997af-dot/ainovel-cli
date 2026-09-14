package appruntime

import (
	"fmt"
	"strings"
)

const D07MaxAutoRepairAttempts = 1

type D07PipelineRole string

const (
	D07RoleWriter       D07PipelineRole = "writer"
	D07RoleCoCreate     D07PipelineRole = "cocreate"
	D07RoleArchitect    D07PipelineRole = "architect"
	D07RoleReview       D07PipelineRole = "review"
	D07RoleContinuity   D07PipelineRole = "continuity"
	D07RoleRepair       D07PipelineRole = "repair"
	D07RoleQA           D07PipelineRole = "qa"
)

// D07PipelinePolicy resolves only the AUTHOR-frozen execution policy. It does
// not schedule work: Run Center remains the sole scheduler and D04 resource
// authority remains the sole conflicting-write lock owner.
type D07PipelinePolicy struct {
	Config DualWebParallelConfigDTO `json:"config"`
}

func NewD07PipelinePolicy(mode string, checkpointChapters int) (D07PipelinePolicy, error) {
	cfg := FrozenDualWebParallelDefaults()
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		mode = "balanced"
	}
	if checkpointChapters == 0 {
		checkpointChapters = DualWebCheckpointDefault
	}
	cfg.Mode = mode
	cfg.CheckpointChapters = checkpointChapters
	if err := cfg.Validate(); err != nil {
		return D07PipelinePolicy{}, err
	}
	return D07PipelinePolicy{Config: cfg}, nil
}

func (p D07PipelinePolicy) ProviderForRole(role D07PipelineRole) (WebAIProvider, error) {
	switch role {
	case D07RoleWriter, D07RoleCoCreate:
		return ProviderGeminiWeb, nil
	case D07RoleArchitect, D07RoleReview, D07RoleContinuity, D07RoleRepair, D07RoleQA:
		return ProviderChatGPTWeb, nil
	default:
		return "", fmt.Errorf("unsupported D07 pipeline role: %q", role)
	}
}

// Turbo may overlap independent draft/review work, but this permission never
// bypasses resource claims, provider model verification, QA, repair bounds or
// Official promotion. Balanced keeps the conservative serial preference.
func (p D07PipelinePolicy) AllowsIndependentDraftReviewOverlap() bool {
	return p.Config.Mode == "turbo"
}

func (p D07PipelinePolicy) CheckpointDue(completedChapters int) bool {
	return completedChapters > 0 && completedChapters%p.Config.CheckpointChapters == 0
}

func (p D07PipelinePolicy) AutoRepairAllowed(attempt int) bool {
	return p.Config.AutoRepair == "bounded" && attempt >= 0 && attempt < D07MaxAutoRepairAttempts
}
