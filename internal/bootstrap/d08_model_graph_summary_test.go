package bootstrap

import (
	"testing"

	"github.com/voocel/ainovel-cli/internal/webai"
)

func TestD08ModelSetSummaryProjectsExactStrictRoleGraph(t *testing.T) {
	t.Parallel()

	models := &ModelSet{
		Default:    &SwappableModel{provider: WebProviderName, name: WebModelName},
		Specialist: &SwappableModel{provider: webai.ChatGPTWebSite, name: webai.ChatGPTWebSite},
	}
	const want = "default=web/gemini-web specialist=chatgpt-web/chatgpt-web"
	if got := models.Summary(); got != want {
		t.Fatalf("model graph summary = %q, want %q", got, want)
	}
}

func TestD08ModelSetSummaryCannotMasqueradeWrongOrMissingSpecialist(t *testing.T) {
	t.Parallel()

	const authority = "default=web/gemini-web specialist=chatgpt-web/chatgpt-web"
	cases := []*ModelSet{
		{Default: &SwappableModel{provider: WebProviderName, name: WebModelName}},
		{
			Default:    &SwappableModel{provider: WebProviderName, name: WebModelName},
			Specialist: &SwappableModel{provider: WebProviderName, name: WebModelName},
		},
	}
	for _, models := range cases {
		if got := models.Summary(); got == authority {
			t.Fatalf("invalid graph unexpectedly matched D08 authority: %q", got)
		}
	}
}
