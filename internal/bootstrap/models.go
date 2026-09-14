package bootstrap

import (
	"fmt"
	"strings"

	"github.com/voocel/agentcore"
	"github.com/voocel/agentcore/llm"
	"github.com/voocel/ainovel-cli/internal/errs"
	"github.com/voocel/ainovel-cli/internal/llmcontract"
	"github.com/voocel/ainovel-cli/internal/webai"
)

type SwappableModel struct {
	*agentcore.SwappableModel
	provider string
	name     string
}

func NewSwappableModel(provider, name string, model agentcore.ChatModel, _ *bool) *SwappableModel {
	return &SwappableModel{SwappableModel: agentcore.NewSwappableModel(model), provider: provider, name: name}
}
func (m *SwappableModel) ProviderName() string { return m.provider }
func (m *SwappableModel) Info() llm.ModelInfo  { return m.StructuredOutputFacts().Info }
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
		// The role wrapper is the runtime provider authority. Browser Model.Info
		// intentionally reports generic "web", so preserve the concrete lane here.
		modelInfo.Provider = m.provider
		facts.Info = modelInfo
	}
	return facts
}
func (m *SwappableModel) Capabilities() llm.Capabilities {
	return m.StructuredOutputFacts().Capabilities
}
func (m *SwappableModel) JSONSchemaOverride() *bool { return nil }
func (m *SwappableModel) Current() (provider, name string) {
	if m == nil {
		return "", ""
	}
	return m.provider, m.name
}

// ModelSet is the D07 strict-role WEB-only graph. Gemini owns primary writing
// and ordinary CoCreate; ChatGPT owns architect/review/QA/repair specialist work.
// There is no provider fallback.
type ModelSet struct {
	Default    *SwappableModel
	Specialist *SwappableModel
	chatGPT    *webai.ChatGPTLane
	config     Config
}

func specialistRole(role string) bool {
	role = strings.ToLower(strings.TrimSpace(role))
	switch role {
	case "architect", "editor", "repair":
		return true
	default:
		return false
	}
}

func (ms *ModelSet) ForRole(role string) agentcore.ChatModel {
	if ms == nil {
		return nil
	}
	if specialistRole(role) && ms.Specialist != nil {
		return ms.Specialist
	}
	return ms.Default
}

func (ms *ModelSet) Summary() string {
	if ms == nil || ms.Default == nil {
		return "default=unavailable"
	}
	p, n := ms.Default.Current()
	summary := fmt.Sprintf("default=%s/%s", p, n)
	if ms.Specialist != nil {
		sp, sn := ms.Specialist.Current()
		summary += fmt.Sprintf(" specialist=%s/%s", sp, sn)
	}
	return summary
}
func (ms *ModelSet) NormalizedLanguage() string {
	if ms == nil {
		return "vi"
	}
	return ms.config.NormalizedLanguage()
}
func (ms *ModelSet) CurrentSelection(role string) (provider, model string, explicit bool) {
	if ms == nil {
		return "", "", false
	}
	selected := ms.ForRole(role)
	if selected == nil {
		return "", "", false
	}
	if specialistRole(role) && ms.Specialist != nil {
		provider, model = ms.Specialist.Current()
	} else if ms.Default != nil {
		provider, model = ms.Default.Current()
	}
	return provider, model, true
}
func (ms *ModelSet) ResolveContextWindow(provider, model string) (int, ContextWindowSource) {
	if ms == nil {
		return DefaultContextWindow, CtxWindowDefault
	}
	return ms.config.ResolveContextWindow(provider, model)
}
func (ms *ModelSet) ChatGPTLane() *webai.ChatGPTLane {
	if ms == nil {
		return nil
	}
	return ms.chatGPT
}
func (ms *ModelSet) CloseSpecialist() error {
	if ms == nil || ms.chatGPT == nil {
		return nil
	}
	return ms.chatGPT.Stop()
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

func NewWebModelSet(cfg Config, session *webai.SessionManager) (*ModelSet, error) {
	if !cfg.Web.Enabled {
		return nil, fmt.Errorf("WEB-only runtime is not enabled; enable web.enabled=true: %w", errs.ErrConfig)
	}
	if session == nil {
		return nil, fmt.Errorf("WEB-only model set requires a browser session: %w", errs.ErrConfig)
	}
	baseTransport, err := webai.NewGeminiWebTransport(webai.GeminiWebTransportConfig{Session: session})
	if err != nil {
		return nil, fmt.Errorf("create Gemini Web transport: %w", err)
	}
	transport, err := webai.NewAutoRecoveryTransport(webai.AutoRecoveryConfig{Inner: baseTransport, Session: session})
	if err != nil {
		return nil, fmt.Errorf("create Gemini Web auto-recovery watchdog: %w", err)
	}
	geminiModel, err := webai.NewModel(webai.ModelConfig{Site: WebModelName, Model: WebModelName, Transport: transport})
	if err != nil {
		return nil, fmt.Errorf("create Gemini WEB ChatModel: %w", err)
	}

	chatGPTLane := webai.NewChatGPTLane(webai.ChatGPTLaneConfig{BrowserPath: cfg.Web.BrowserPath, ProfileName: webai.DefaultChatGPTProfileName})
	chatGPTModel, err := webai.NewModel(webai.ModelConfig{Site: webai.ChatGPTWebSite, Model: webai.ChatGPTWebSite, Transport: chatGPTLane})
	if err != nil {
		return nil, fmt.Errorf("create ChatGPT WEB ChatModel: %w", err)
	}

	return &ModelSet{
		Default:    NewSwappableModel(WebProviderName, WebModelName, geminiModel, nil),
		Specialist: NewSwappableModel(webai.ChatGPTWebSite, webai.ChatGPTWebSite, chatGPTModel, nil),
		chatGPT:    chatGPTLane,
		config:     cfg,
	}, nil
}
