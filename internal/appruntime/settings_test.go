package appruntime

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestSettingsUpdatePayloadRequiresCASAndRejectsUnknownFields(t *testing.T) {
	valid, err := json.Marshal(SettingsUpdateCommandPayload{
		ExpectedFingerprint: "abc",
		Values: SettingsValuesDTO{
			ProfileName:   "default",
			NotifyEnabled: true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeSettingsUpdatePayload(valid); err != nil {
		t.Fatalf("valid payload rejected: %v", err)
	}
	missingCAS, err := json.Marshal(map[string]any{
		"values": map[string]any{"notify_enabled": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeSettingsUpdatePayload(missingCAS); !errors.Is(err, ErrInvalidCommand) {
		t.Fatalf("missing fingerprint error=%v", err)
	}
	unknownField, err := json.Marshal(map[string]any{
		"expected_fingerprint": "abc",
		"values":               map[string]any{"notify_enabled": true},
		"provider":             "api",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeSettingsUpdatePayload(unknownField); !errors.Is(err, ErrInvalidCommand) {
		t.Fatalf("unknown provider field error=%v", err)
	}
}

func TestSettingsDTOHasNoClosedProviderCredentialOrCommandSurface(t *testing.T) {
	data, err := json.Marshal(SettingsGetResultDTO{
		Identity: SettingsIdentityDTO{WebEnabled: true, WebSite: "gemini-web"},
		Values:   SettingsValuesDTO{ProfileName: "default", NotifyEnabled: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, forbidden := range []string{"api_key", "base_url", "providers", "notify_command", "roles"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("Settings DTO leaked closed field %q: %s", forbidden, text)
		}
	}
}
