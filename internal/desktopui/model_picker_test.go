package desktopui

import (
	"testing"

	"github.com/voocel/ainovel-cli/internal/appruntime"
)

func TestD05ModelPickerRequiresExplicitProviderSelection(t *testing.T) {
	controller := NewController(nil, 1200)
	state := controller.ModelSelection()
	if state.Selected != "" {
		t.Fatalf("default provider = %q, want explicit empty selection", state.Selected)
	}
	if len(state.Options) != 2 {
		t.Fatalf("picker options = %d, want Gemini + ChatGPT", len(state.Options))
	}
	if state.Options[0].Provider != appruntime.ProviderGeminiWeb || state.Options[0].Label != "Gemini" {
		t.Fatalf("first picker option = %+v", state.Options[0])
	}
	if state.Options[1].Provider != appruntime.ProviderChatGPTWeb || state.Options[1].Label != "ChatGPT" {
		t.Fatalf("second picker option = %+v", state.Options[1])
	}
	for _, option := range state.Options {
		if option.Selected {
			t.Fatalf("picker must fail closed before user selection: %+v", state.Options)
		}
	}
}

func TestD05ModelPickerSelectsExactlyOneProvider(t *testing.T) {
	controller := NewController(nil, 1200)
	if err := controller.SelectModelProvider(appruntime.ProviderChatGPTWeb); err != nil {
		t.Fatal(err)
	}
	state := controller.ModelSelection()
	if state.Selected != appruntime.ProviderChatGPTWeb {
		t.Fatalf("selected provider = %q", state.Selected)
	}
	selected := 0
	for _, option := range state.Options {
		if option.Selected {
			selected++
			if option.Provider != appruntime.ProviderChatGPTWeb {
				t.Fatalf("wrong selected option: %+v", option)
			}
		}
	}
	if selected != 1 {
		t.Fatalf("selected option count = %d, want 1", selected)
	}

	if err := controller.SelectModelProvider(appruntime.WebAIProvider("other")); err == nil {
		t.Fatal("unsupported provider must fail local picker validation")
	}
}
