package sites

import (
	"context"
	"encoding/json"
	"testing"
)

func TestD08ChatGPTGoogleOAuthTargetIsAuthBoundary(t *testing.T) {
	adapter := ChatGPT{}
	if got := adapter.TargetScore("https://accounts.google.com/v3/signin/rejected?app_domain=https%3A%2F%2Fauth.openai.com"); got != 70 {
		t.Fatalf("Google OAuth rejected target score = %d, want 70", got)
	}
	if got := adapter.TargetScore("https://accounts.google.com/ServiceLogin"); got != 0 {
		t.Fatalf("unscoped Google account page score = %d, want 0", got)
	}

	got, err := adapter.Probe(context.Background(), chatGPTFakeEvaluator{raw: json.RawMessage(`{"host":"accounts.google.com","path":"/v3/signin/rejected","has_composer":false,"has_sign_in":false,"has_account_control":false,"security_challenge":false}`)})
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if got.State != ReadinessAuthRequired {
		t.Fatalf("Google OAuth probe state = %q, want %q", got.State, ReadinessAuthRequired)
	}
}
