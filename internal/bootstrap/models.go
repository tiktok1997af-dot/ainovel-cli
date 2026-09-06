package bootstrap

import (
	"fmt"

	"github.com/voocel/agentcore"
	"github.com/voocel/agentcore/llm"
	"github.com/voocel/ainovel-cli/internal/errs"
	"github.com/voocel/ainovel-cli/internal/llmcontract"
	"github.com/voocel/ainovel-cli/internal/webai"
)

// SwappableModel keeps the historical wrapper name because Engine/Host code
// consumes its identity/capability projection. C4 removed every mutation
// method: the wrapped browser model is fixed for the lifetime of the Host.
type SwappableModel struct {
	*agentcore.SwappableModel
	provider string
	name     string
}

func NewSwappableModel(provider, name string, model agentcore.ChatModel, _ *bool) *SwappableModel {
	return &SwappableModel{
		SwappableModel: agentcore.NewSwappableModel(model),
		provider:       provider,
		name:           name,
	}
}

func (m *SwappableModel) ProviderName() string { return m.provider }

func (m *SwappableModel) Info() llm.ModelInfo {
	return m.StructuredOutputFacts().Info
}

func (m *SwappableModel) StructuredOutputFacts() llmcontract.ModelFacts {
	if m == nil || m.SwappableModel == nil {
		return llmcontract.ModelFacts{}
	}
	current := m.SwappableModel.Current()
	facts := llmcontract.ModelFacts{Info: llm.ModelInfo{Name: m.name, Provider: m.provider}}
	if cp, ok := current.(llm.CapabilityProvider); ok {
		facts.Capabilities = cp.Capabilities()
	}
	if info, ok := current.(interface{ Info() llm.ModelInfo }); ok {
		modelInfo := info.Info()
		if modelInfo.Name == "" {
			modelInfo.Name = m.name
		}
		if modelInfo.Provider == "" {
			modelInfo.Provider = m.provider
		}
		facts.Info = modelInfo
	}
	return facts
}

func (m *SwappableModel) Capabilities() llm.Capabilities {
	return m.StructuredOutputFacts().Capabilities
}

// Browser transport uses the prompt contract instead of provider-native JSON
// schema routing.
func (m *SwappableModel) JSONSchemaOverride() *bool { return nil }

func (m *SwappableModel) Current() (provider, name string) {
	if m == nil {
		return "", ""
	}
	return m.provider, m.name
}

// ModelSet is a fixed WEB-only graph. Every role shares the same browser model.
type ModelSet struct {
	Default *SwappableModel
	config  Config
}

func (ms *ModelSet) ForRole(_ string) agentcore.ChatModel {
	if ms == nil {
		return nil
	}
	return ms.Default
}

func (ms *ModelSet) Summary() string {
	if ms == nil || ms.Default == nil {
		return "default=unavailable"
	}
	provider, name := ms.Default.Current()
	return fmt.Sprintf("default=%s/%s", provider, name)
}

func (ms *ModelSet) CurrentSelection(_ string) (provider, model string, explicit bool) {
	if ms == nil || ms.Default == nil {
		return "", "", false
	}
	provider, model = ms.Default.Current()
	return provider, model, true
}

func (ms *ModelSet) ResolveContextWindow(provider, model string) (int, ContextWindowSource) {
	if ms == nil {
		return DefaultContextWindow, CtxWindowDefault
	}
	return ms.config.ResolveContextWindow(provider, model)
}

func ModelName(m agentcore.ChatModel) string {
	if info, ok := m.(interface{ Info() llm.ModelInfo }); ok {
		return info.Info().Name
	}
	return ""
}

func ModelProvider(m agentcore.ChatModel) string {
	if info, ok := m.(interface{ Info() llm.ModelInfo }); ok {
		return info.Info().Provider
	}
	if provider, ok := m.(interface{ ProviderName() string }); ok {
		return provider.ProviderName()
	}
	return ""
}

// NewWebModelSet builds the only AI model graph over one owned browser session.
func NewWebModelSet(cfg Config, session *webai.SessionManager) (*ModelSet, error) {
	if !cfg.Web.Enabled {
		return nil, fmt.Errorf("WEB-only runtime is not enabled; enable web.enabled=true: %w", errs.ErrConfig)
	}
	if session == nil {
		return nil, fmt.Errorf("WEB-only model set requires a browser session: %w", errs.ErrConfig)
	}
	transport, err := webai.NewGeminiWebTransport(webai.GeminiWebTransportConfig{Session: session})
	if err != nil {
		return nil, fmt.Errorf("create Gemini Web transport: %w", err)
	}
	model, err := webai.NewModel(webai.ModelConfig{Site: WebModelName, Model: WebModelName, Transport: transport})
	if err != nil {
		return nil, fmt.Errorf("create WEB ChatModel: %w", err)
	}
	return &ModelSet{
		Default: NewSwappableModel(WebProviderName, WebModelName, model, nil),
		config:  cfg,
	}, nil
}
