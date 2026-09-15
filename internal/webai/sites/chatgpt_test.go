package sites

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type chatGPTFakeEvaluator struct {
	raw json.RawMessage
	err error
}

func (f chatGPTFakeEvaluator) Eval(context.Context, string) (json.RawMessage, error) {
	return f.raw, f.err
}

func TestChatGPTTargetScore(t *testing.T) {
	adapter := ChatGPT{}
	cases := []struct {
		url  string
		want int
	}{
		{"https://chatgpt.com/", 100},
		{"https://www.chatgpt.com/c/abc", 100},
		{"https://auth.openai.com/authorize", 80},
		{"https://example.com/", 0},
	}
	for _, tc := range cases {
		if got := adapter.TargetScore(tc.url); got != tc.want {
			t.Fatalf("TargetScore(%q) = %d, want %d", tc.url, got, tc.want)
		}
	}
}

func TestChatGPTProbeFailClosedAndReady(t *testing.T) {
	adapter := ChatGPT{}
	cases := []struct {
		name string
		raw  string
		want Readiness
	}{
		{"auth host", `{"host":"auth.openai.com","path":"/authorize","has_composer":false,"has_sign_in":false,"has_account_control":false,"security_challenge":false}`, ReadinessAuthRequired},
		{"visible login", `{"host":"chatgpt.com","path":"/","has_composer":true,"has_sign_in":true,"has_account_control":false,"security_challenge":false}`, ReadinessAuthRequired},
		{"composer without account", `{"host":"chatgpt.com","path":"/","has_composer":true,"has_sign_in":false,"has_account_control":false,"security_challenge":false}`, ReadinessAuthRequired},
		{"authenticated ready", `{"host":"chatgpt.com","path":"/","has_composer":true,"has_sign_in":false,"has_account_control":true,"security_challenge":false}`, ReadinessReady},
		{"authenticated shell no composer", `{"host":"chatgpt.com","path":"/","has_composer":false,"has_sign_in":false,"has_account_control":true,"security_challenge":false}`, ReadinessDegraded},
		{"wrong target", `{"host":"openai.com","path":"/","has_composer":false,"has_sign_in":false,"has_account_control":false,"security_challenge":false}`, ReadinessDegraded},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := adapter.Probe(context.Background(), chatGPTFakeEvaluator{raw: json.RawMessage(tc.raw)})
			if err != nil {
				t.Fatalf("Probe() error = %v", err)
			}
			if got.State != tc.want {
				t.Fatalf("Probe() state = %q, want %q", got.State, tc.want)
			}
		})
	}
}

func TestD08ChatGPTReadinessExpressionCoversCurrentSidebarAccountControls(t *testing.T) {
	for _, want := range []string{
		`[data-testid*="profile" i]`,
		`[data-testid*="account" i]`,
		`[data-testid*="user-menu" i]`,
	} {
		if !strings.Contains(chatGPTReadinessExpression, want) {
			t.Fatalf("ChatGPT readiness selector coverage missing %q", want)
		}
	}
}

func TestChatGPTObserveModelsDynamicAndDeterministic(t *testing.T) {
	adapter := ChatGPT{}
	raw := json.RawMessage(`{"active_label":" Model Alpha ","models":[{"label":"Model Beta","available":true},{"label":"Model Alpha","available":true},{"label":"Model Beta","available":true}]}`)
	first, err := adapter.ObserveModels(context.Background(), chatGPTFakeEvaluator{raw: raw})
	if err != nil {
		t.Fatalf("ObserveModels() error = %v", err)
	}
	second, err := adapter.ObserveModels(context.Background(), chatGPTFakeEvaluator{raw: raw})
	if err != nil {
		t.Fatalf("ObserveModels() second error = %v", err)
	}
	if first.ActiveModelID == "" || !strings.HasPrefix(first.ActiveModelID, "observed-") {
		t.Fatalf("active model id = %q", first.ActiveModelID)
	}
	if first.Revision == "" || first.Revision != second.Revision {
		t.Fatalf("revision first=%q second=%q", first.Revision, second.Revision)
	}
	if len(first.Models) != 2 {
		t.Fatalf("models len = %d, want 2", len(first.Models))
	}
	foundActive := false
	for _, model := range first.Models {
		if model.ID == first.ActiveModelID {
			foundActive = model.Available && model.Label == "Model Alpha"
		}
	}
	if !foundActive {
		t.Fatalf("active model not present/available in catalog: %+v", first.Models)
	}
}

func TestChatGPTObserveModelsFailsClosedWithoutActiveModel(t *testing.T) {
	adapter := ChatGPT{}
	_, err := adapter.ObserveModels(context.Background(), chatGPTFakeEvaluator{raw: json.RawMessage(`{"active_label":"","models":[]}`)})
	if err == nil {
		t.Fatal("ObserveModels() error = nil, want fail-closed error")
	}
}
