package appruntime

import (
	"fmt"
	"strings"
)

// D01 freezes the finite provider/model-selection vocabulary only. Live model
// discovery, browser activation, scheduler admission and desktop wiring belong
// to later Dual-Web Parallel v1 steps.
type WebAIProvider string

const (
	ProviderGeminiWeb  WebAIProvider = "gemini-web"
	ProviderChatGPTWeb WebAIProvider = "chatgpt-web"
)

const (
	MaxWebAIModelIDBytes     = 128
	MaxWebAIModelLabelBytes  = 256
	DualWebAILaneCapacity    = 1
	DualWebAIConcurrency     = 2
	DualWebLocalWorkers      = 2
	DualWebMaxActiveJobs     = 4
	DualWebCheckpointMin     = 5
	DualWebCheckpointMax     = 10
	DualWebCheckpointDefault = 10
)

type WebAIModelOptionDTO struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Available bool   `json:"available"`
}

type WebAIModelCatalogDTO struct {
	Provider WebAIProvider         `json:"provider"`
	Models   []WebAIModelOptionDTO `json:"models"`
	Revision string                `json:"revision,omitempty"`
}

type WebAIModelSelectionDTO struct {
	Provider         WebAIProvider `json:"provider"`
	RequestedModelID string        `json:"requested_model_id"`
	RequestedLabel   string        `json:"requested_label,omitempty"`
	ObservedModelID  string        `json:"observed_model_id,omitempty"`
	CatalogRevision  string        `json:"catalog_revision,omitempty"`
	Authenticated    bool          `json:"authenticated"`
	ProviderReady    bool          `json:"provider_ready"`
	ModelAvailable   bool          `json:"model_available"`
	Verified         bool          `json:"verified"`
}

type WebAIModelPreflightDTO struct {
	RequiredProviders []WebAIProvider          `json:"required_providers"`
	Selections        []WebAIModelSelectionDTO `json:"selections"`
	StartAllowed      bool                     `json:"start_allowed"`
}

type DualWebParallelConfigDTO struct {
	Mode                string `json:"mode"`
	StrictRole          bool   `json:"strict_role"`
	LazyBrowserStart    bool   `json:"lazy_browser_start"`
	PipelineEnabled     bool   `json:"pipeline_enabled"`
	AutoReview          bool   `json:"auto_review"`
	AutoQA              bool   `json:"auto_qa"`
	AutoRepair          string `json:"auto_repair"`
	CheckpointChapters  int    `json:"checkpoint_chapters"`
	GeminiLaneCapacity  int    `json:"gemini_lane_capacity"`
	ChatGPTLaneCapacity int    `json:"chatgpt_lane_capacity"`
	AIConcurrency       int    `json:"ai_concurrency"`
	LocalWorkers        int    `json:"local_workers"`
	MaxActiveJobs       int    `json:"max_active_jobs"`
}

func FrozenDualWebParallelDefaults() DualWebParallelConfigDTO {
	return DualWebParallelConfigDTO{
		Mode:                "balanced",
		StrictRole:          true,
		LazyBrowserStart:    true,
		PipelineEnabled:     true,
		AutoReview:          true,
		AutoQA:              true,
		AutoRepair:          "bounded",
		CheckpointChapters:  DualWebCheckpointDefault,
		GeminiLaneCapacity:  DualWebAILaneCapacity,
		ChatGPTLaneCapacity: DualWebAILaneCapacity,
		AIConcurrency:       DualWebAIConcurrency,
		LocalWorkers:        DualWebLocalWorkers,
		MaxActiveJobs:       DualWebMaxActiveJobs,
	}
}

func (p WebAIProvider) Validate() error {
	switch p {
	case ProviderGeminiWeb, ProviderChatGPTWeb:
		return nil
	default:
		return fmt.Errorf("unsupported web AI provider: %q", p)
	}
}

func (m WebAIModelOptionDTO) Validate() error {
	id := strings.TrimSpace(m.ID)
	label := strings.TrimSpace(m.Label)
	if id == "" || len(id) > MaxWebAIModelIDBytes {
		return fmt.Errorf("invalid web AI model id")
	}
	if label == "" || len(label) > MaxWebAIModelLabelBytes {
		return fmt.Errorf("invalid web AI model label")
	}
	return nil
}

func (c WebAIModelCatalogDTO) Validate() error {
	if err := c.Provider.Validate(); err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(c.Models))
	for _, model := range c.Models {
		if err := model.Validate(); err != nil {
			return err
		}
		id := strings.TrimSpace(model.ID)
		if _, ok := seen[id]; ok {
			return fmt.Errorf("duplicate web AI model id: %q", id)
		}
		seen[id] = struct{}{}
	}
	return nil
}

func (s WebAIModelSelectionDTO) Validate() error {
	if err := s.Provider.Validate(); err != nil {
		return err
	}
	requested := strings.TrimSpace(s.RequestedModelID)
	if requested == "" || len(requested) > MaxWebAIModelIDBytes {
		return fmt.Errorf("invalid requested web AI model id")
	}
	if strings.TrimSpace(s.RequestedLabel) != "" && len(strings.TrimSpace(s.RequestedLabel)) > MaxWebAIModelLabelBytes {
		return fmt.Errorf("invalid requested web AI model label")
	}
	observed := strings.TrimSpace(s.ObservedModelID)
	if observed != "" && len(observed) > MaxWebAIModelIDBytes {
		return fmt.Errorf("invalid observed web AI model id")
	}
	if s.Verified {
		if !s.Authenticated || !s.ProviderReady || !s.ModelAvailable {
			return fmt.Errorf("verified model selection is not provider-ready")
		}
		if observed == "" || observed != requested {
			return fmt.Errorf("verified model selection does not match observed model")
		}
		if strings.TrimSpace(s.CatalogRevision) == "" {
			return fmt.Errorf("verified model selection requires catalog revision")
		}
	}
	return nil
}

func (p WebAIModelPreflightDTO) Validate() error {
	if len(p.RequiredProviders) == 0 || len(p.RequiredProviders) > DualWebAIConcurrency {
		return fmt.Errorf("invalid required provider count")
	}
	required := make(map[WebAIProvider]struct{}, len(p.RequiredProviders))
	for _, provider := range p.RequiredProviders {
		if err := provider.Validate(); err != nil {
			return err
		}
		if _, ok := required[provider]; ok {
			return fmt.Errorf("duplicate required provider: %q", provider)
		}
		required[provider] = struct{}{}
	}
	selected := make(map[WebAIProvider]WebAIModelSelectionDTO, len(p.Selections))
	for _, selection := range p.Selections {
		if err := selection.Validate(); err != nil {
			return err
		}
		if _, ok := selected[selection.Provider]; ok {
			return fmt.Errorf("duplicate provider selection: %q", selection.Provider)
		}
		selected[selection.Provider] = selection
	}
	ready := true
	for provider := range required {
		selection, ok := selected[provider]
		if !ok || !selection.Verified {
			ready = false
		}
	}
	if p.StartAllowed != ready {
		return fmt.Errorf("start_allowed does not match verified provider preflight")
	}
	return nil
}

func (c DualWebParallelConfigDTO) Validate() error {
	if c.Mode != "balanced" && c.Mode != "turbo" {
		return fmt.Errorf("unsupported dual-web mode: %q", c.Mode)
	}
	if !c.StrictRole || !c.PipelineEnabled || !c.AutoReview || !c.AutoQA || c.AutoRepair != "bounded" {
		return fmt.Errorf("dual-web safety defaults may not be weakened in D01")
	}
	if c.CheckpointChapters < DualWebCheckpointMin || c.CheckpointChapters > DualWebCheckpointMax {
		return fmt.Errorf("checkpoint interval must be within %d..%d", DualWebCheckpointMin, DualWebCheckpointMax)
	}
	if c.GeminiLaneCapacity != DualWebAILaneCapacity || c.ChatGPTLaneCapacity != DualWebAILaneCapacity {
		return fmt.Errorf("each web AI lane capacity must remain %d", DualWebAILaneCapacity)
	}
	if c.AIConcurrency != DualWebAIConcurrency || c.LocalWorkers != DualWebLocalWorkers || c.MaxActiveJobs != DualWebMaxActiveJobs {
		return fmt.Errorf("dual-web concurrency contract mismatch")
	}
	return nil
}
