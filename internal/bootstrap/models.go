package bootstrap

import (
	"fmt"
	"sync"

	"github.com/voocel/agentcore"
	"github.com/voocel/agentcore/llm"
	"github.com/voocel/ainovel-cli/internal/errs"
	"github.com/voocel/ainovel-cli/internal/llmcontract"
	"github.com/voocel/ainovel-cli/internal/webai"
)

// FailoverEvent/FailoverReporter are retained temporarily as compile-time
// compatibility shims while W5C removes the old callers. The WEB-only runtime
// never constructs fallback targets and never resubmits a request to another
// provider.
type FailoverEvent struct {
	Role         string
	Reason       string
	FromProvider string
	FromModel    string
	ToProvider   string
	ToModel      string
	Err          error
}

type FailoverReporter func(FailoverEvent)

// SwappableModel is retained temporarily because existing Engine/Host code
// expects its identity/capability projection. W5C has removed every production
// path that builds an API model for Swap. ModelSet.Swap is fail-closed.
type SwappableModel struct {
	*agentcore.SwappableModel
	mu       sync.RWMutex
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

func (m *SwappableModel) ProviderName() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.provider
}

func (m *SwappableModel) Info() llm.ModelInfo {
	return m.StructuredOutputFacts().Info
}

func (m *SwappableModel) StructuredOutputFacts() llmcontract.ModelFacts {
	m.mu.RLock()
	defer m.mu.RUnlock()
	current := m.SwappableModel.Current()
	facts := llmcontract.ModelFacts{
		Info: llm.ModelInfo{Name: m.name, Provider: m.provider},
	}
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

// JSONSchemaOverride intentionally returns nil for the browser model. Native
// provider JSON-schema routing is not part of the WEB-only transport; existing
// llmcontract logic therefore uses the prompt contract proved in W1.
func (m *SwappableModel) JSONSchemaOverride() *bool { return nil }

func (m *SwappableModel) Current() (provider, name string) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.provider, m.name
}

// ModelSet is now a fixed WEB-only model graph. All roles share Default.
type ModelSet struct {
	Default *SwappableModel
	config  Config
	webOnly bool
}

func (ms *ModelSet) ForRole(_ string) agentcore.ChatModel {
	if ms == nil {
		return nil
	}
	return ms.Default
}

// ForRoleWithFailover remains only until W5C removes the old caller name. It
// deliberately performs no failover and returns the same browser model.
func (ms *ModelSet) ForRoleWithFailover(_ string, _ FailoverReporter) agentcore.ChatModel {
	return ms.ForRole("")
}

func (ms *ModelSet) Summary() string {
	if ms == nil || ms.Default == nil {
		return "default=unavailable"
	}
	provider, name := ms.Default.Current()
	return fmt.Sprintf("default=%s/%s", provider, name)
}

// CurrentSelection always resolves to the single logged-in browser model.
func (ms *ModelSet) CurrentSelection(_ string) (provider, model string, explicit bool) {
	if ms == nil || ms.Default == nil {
		return "", "", false
	}
	provider, model = ms.Default.Current()
	return provider, model, true
}

// Swap is a fail-closed compatibility surface. Provider/model hot switching is
// not supported by the WEB-only product and can never create another model.
func (ms *ModelSet) Swap(_, _, _ string) error {
	return fmt.Errorf("provider/API model switching was removed; WEB-only runtime uses the logged-in Gemini Web session: %w", errs.ErrConfig)
}

func (ms *ModelSet) ResolveContextWindow(provider, model string) (int, ContextWindowSource) {
	if ms == nil {
		return DefaultContextWindow, CtxWindowDefault
	}
	return ms.config.ResolveContextWindow(provider, model)
}

// ApplyPrepared is retained only so W5C can remove legacy Host provider
// management in a separate compile-safe step. No replacement model is applied.
func (ms *ModelSet) ApplyPrepared(_ *ModelSet) {}

// ModelName extracts the current model label from a ChatModel.
func ModelName(m agentcore.ChatModel) string {
	if info, ok := m.(interface{ Info() llm.ModelInfo }); ok {
		return info.Info().Name
	}
	return ""
}

// ModelProvider extracts the current provider/transport label from a ChatModel.
func ModelProvider(m agentcore.ChatModel) string {
	if info, ok := m.(interface{ Info() llm.ModelInfo }); ok {
		return info.Info().Provider
	}
	if provider, ok := m.(interface{ ProviderName() string }); ok {
		return provider.ProviderName()
	}
	return ""
}

// NewModelSet is deliberately fail-closed. The API/provider constructor was
// removed in W5C; callers must migrate to NewWebModelSet via the owned browser
// session. Keeping this small rejection shim during C1 gives legacy config/tests
// a deterministic migration error without retaining any API execution path.
func NewModelSet(_ Config) (*ModelSet, error) {
	return nil, fmt.Errorf("legacy AI provider/API runtime has been removed; enable web.enabled=true and use web.site=gemini-web: %w", errs.ErrConfig)
}

// NewWebModelSet builds the only production AI model graph over one owned
// browser session. Architect/Writer/Editor/Arbiter all share this transport.
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
	model, err := webai.NewModel(webai.ModelConfig{Site: "gemini-web", Model: "gemini-web", Transport: transport})
	if err != nil {
		return nil, fmt.Errorf("create WEB ChatModel: %w", err)
	}
	return &ModelSet{
		Default: NewSwappableModel("web", "gemini-web", model, nil),
		config:  cfg,
		webOnly: true,
	}, nil
}
