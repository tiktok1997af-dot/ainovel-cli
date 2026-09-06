package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/voocel/agentcore"
	"github.com/voocel/ainovel-cli/internal/host"
	"github.com/voocel/ainovel-cli/internal/webai"
)

type fakeModelRuntime struct {
	providers   []string
	models      map[string][]host.ConfiguredModel
	curProvider string
	curModel    string
	thinking    map[string]string
	available   []agentcore.ThinkingLevel
	setCalls    []struct{ role, level string }
	switchCalls int
}

func (f *fakeModelRuntime) ConfiguredProviders() []string { return f.providers }
func (f *fakeModelRuntime) ConfiguredModelOptions(provider string) []host.ConfiguredModel {
	return f.models[provider]
}
func (f *fakeModelRuntime) CurrentModelSelection(role string) (string, string, bool) {
	return f.curProvider, f.curModel, true
}
func (f *fakeModelRuntime) AvailableThinking(role string) []agentcore.ThinkingLevel {
	return f.available
}
func (f *fakeModelRuntime) CurrentThinking(role string) string { return f.thinking[role] }
func (f *fakeModelRuntime) SwitchModel(role, provider, model string) error {
	f.switchCalls++
	f.curProvider, f.curModel = provider, model
	return nil
}
func (f *fakeModelRuntime) SetRoleThinking(role, level string) error {
	f.setCalls = append(f.setCalls, struct{ role, level string }{role, level})
	if f.thinking == nil {
		f.thinking = map[string]string{}
	}
	f.thinking[role] = level
	return nil
}

type fakeWebModelRuntime struct {
	*fakeModelRuntime
	web host.WebConfigurationSnapshot
}

func (f *fakeWebModelRuntime) WebConfiguration() host.WebConfigurationSnapshot { return f.web }

func TestModelSwitchKeepsUnrepresentableThinkingIntent(t *testing.T) {
	rt := &fakeModelRuntime{
		providers:   []string{"proxy"},
		models:      map[string][]host.ConfiguredModel{"proxy": {{Name: "chat-only"}}},
		curProvider: "proxy", curModel: "chat-only",
		thinking:  map[string]string{"writer": "high"},
		available: nil,
	}
	st := newModelSwitchState(rt, "writer")
	if st.thinkingKey() != "" {
		t.Fatalf("high 无法呈现时面板应落在继承档，得到 %q", st.thinkingKey())
	}
	if err := st.apply(rt); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(rt.setCalls) != 0 {
		t.Fatalf("未改动强度不应回写：%+v", rt.setCalls)
	}
	if rt.thinking["writer"] != "high" {
		t.Fatalf("意图被抹成 %q，应保留 high", rt.thinking["writer"])
	}
}

func TestModelSwitchAppliesExplicitThinkingChange(t *testing.T) {
	rt := &fakeModelRuntime{
		providers:   []string{"proxy"},
		models:      map[string][]host.ConfiguredModel{"proxy": {{Name: "m"}}},
		curProvider: "proxy", curModel: "m",
		thinking:  map[string]string{"writer": ""},
		available: []agentcore.ThinkingLevel{"low", "high"},
	}
	st := newModelSwitchState(rt, "writer")
	st.focus = modelFocusThinking
	st.cycle(1, rt)
	want := st.thinkingKey()
	if want == "" {
		t.Fatal("测试前置：应已移动到某个非空强度档")
	}
	if err := st.apply(rt); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(rt.setCalls) != 1 || rt.setCalls[0].level != want {
		t.Fatalf("显式改动应回写 %q，得到 %+v", want, rt.setCalls)
	}
}

func TestWebModelStatusIsReadOnlyAndCannotSwitchProvider(t *testing.T) {
	legacy := &fakeModelRuntime{
		providers:   []string{"openai"},
		models:      map[string][]host.ConfiguredModel{"openai": {{Name: "gpt"}}},
		curProvider: "openai",
		curModel:    "gpt",
	}
	rt := &fakeWebModelRuntime{
		fakeModelRuntime: legacy,
		web: host.WebConfigurationSnapshot{
			Enabled:     true,
			Site:        "gemini-web",
			Model:       "gemini-web",
			ProfileName: "default",
			HasSession:  true,
			Session: webai.SessionSnapshot{
				State: webai.SessionReady,
				PID:   4242,
			},
		},
	}
	st := newModelSwitchState(rt, "writer")
	if !st.webOnly {
		t.Fatal("WEB-only runtime must open read-only browser status")
	}
	if err := st.apply(rt); err == nil {
		t.Fatal("WEB-only /model must reject provider/model switching")
	}
	if legacy.switchCalls != 0 {
		t.Fatalf("WEB-only /model reached legacy SwitchModel %d times", legacy.switchCalls)
	}
	plain := ansi.Strip(renderModelSwitchBar(120, st))
	for _, want := range []string{"Gemini Web", "WEB-only", "READY", "default", "4242"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("WEB-only /model missing %q:\n%s", want, plain)
		}
	}
	for _, forbidden := range []string{"[openai]", "[gpt]", "Vai trò:", "Suy luận:"} {
		if strings.Contains(plain, forbidden) {
			t.Fatalf("WEB-only /model exposed legacy switching row %q:\n%s", forbidden, plain)
		}
	}
}

func TestWebModelAuthRequiredShowsManualLoginGuidance(t *testing.T) {
	st := &modelSwitchState{
		webOnly: true,
		web: host.WebConfigurationSnapshot{
			Enabled:     true,
			Site:        "gemini-web",
			Model:       "gemini-web",
			ProfileName: "default",
			HasSession:  true,
			Session:     webai.SessionSnapshot{State: webai.SessionAuthRequired},
		},
	}
	plain := ansi.Strip(renderModelSwitchBar(120, st))
	if !strings.Contains(plain, "AUTH_REQUIRED") || !strings.Contains(plain, "đăng nhập Gemini") {
		t.Fatalf("manual login guidance missing:\n%s", plain)
	}
}
