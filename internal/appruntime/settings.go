package appruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/voocel/ainovel-cli/internal/host"
)

func (r *Runtime) querySettingsGet(req QueryRequest) (json.RawMessage, error) {
	var payload SettingsGetQuery
	if err := decodeQueryPayload(req.Payload, &payload); err != nil {
		return nil, err
	}
	state, err := r.core.DesktopSettingsRead()
	if err != nil {
		return nil, err
	}
	return json.Marshal(settingsDTOFromHost(state))
}

func (r *Runtime) dispatchSettingsUpdate(ctx context.Context, cmd CommandRequest) (CommandResult, error) {
	r.commandMu.Lock()
	defer r.commandMu.Unlock()

	cmd.ID = r.ensureCommandID(cmd.ID)
	result := CommandResult{CommandID: cmd.ID}
	if err := ctx.Err(); err != nil {
		return result, err
	}

	payload, err := decodeSettingsUpdatePayload(cmd.Payload)
	if err != nil {
		return result, err
	}
	state, err := r.core.DesktopSettingsSave(payload.ExpectedFingerprint, host.DesktopSettingsUpdate{
		BrowserPath:     payload.Values.BrowserPath,
		ProfileName:     payload.Values.ProfileName,
		StartURL:        payload.Values.StartURL,
		Language:        payload.Values.Language,
		ReasoningEffort: payload.Values.ReasoningEffort,
		Style:           payload.Values.Style,
		ContextWindow:   payload.Values.ContextWindow,
		NotifyEnabled:   payload.Values.NotifyEnabled,
		NotifyEvents:    append([]string(nil), payload.Values.NotifyEvents...),
	})
	if err != nil {
		if errors.Is(err, host.ErrDesktopSettingsStale) {
			return result, ErrMutationStale
		}
		return result, err
	}

	data, err := json.Marshal(SettingsUpdateResultDTO{Saved: true, Fingerprint: state.Fingerprint})
	if err != nil {
		return result, fmt.Errorf("marshal settings update result: %w", err)
	}
	result.Accepted = true
	result.Status = mutationStatusCompleted
	result.Data = data
	return result, nil
}

func decodeSettingsUpdatePayload(raw json.RawMessage) (SettingsUpdateCommandPayload, error) {
	if len(raw) == 0 || string(raw) == "null" || !json.Valid(raw) {
		return SettingsUpdateCommandPayload{}, ErrInvalidCommand
	}
	var payload SettingsUpdateCommandPayload
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return SettingsUpdateCommandPayload{}, ErrInvalidCommand
	}
	if strings.TrimSpace(payload.ExpectedFingerprint) == "" {
		return SettingsUpdateCommandPayload{}, ErrInvalidCommand
	}
	return payload, nil
}

func settingsDTOFromHost(state host.DesktopSettingsSnapshot) SettingsGetResultDTO {
	return SettingsGetResultDTO{
		Identity: SettingsIdentityDTO{WebEnabled: state.WebEnabled, WebSite: state.WebSite},
		Values: settingsValuesDTOFromHost(state.Values.BrowserPath, state.Values.ProfileName, state.Values.StartURL,
			state.Values.Language, state.Values.ReasoningEffort, state.Values.Style, state.Values.ContextWindow,
			state.Values.NotifyEnabled, state.Values.NotifyEvents),
		Runtime: SettingsRuntimeDTO{Values: settingsValuesDTOFromHost(state.Runtime.BrowserPath, state.Runtime.ProfileName,
			state.Runtime.StartURL, state.Runtime.Language, state.Runtime.ReasoningEffort, state.Runtime.Style,
			state.Runtime.ContextWindow, state.Runtime.NotifyEnabled, state.Runtime.NotifyEvents)},
		ConfigScope:     state.TargetScope,
		ConfigPathClass: state.TargetPathClass,
		Fingerprint:     state.Fingerprint,
		RestartRequired: state.RestartRequired,
		ApplyMode:       state.ApplyMode,
	}
}

func settingsValuesDTOFromHost(browserPath, profileName, startURL, language, reasoningEffort, style string, contextWindow int, notifyEnabled bool, notifyEvents []string) SettingsValuesDTO {
	return SettingsValuesDTO{
		BrowserPath:     browserPath,
		ProfileName:     profileName,
		StartURL:        startURL,
		Language:        language,
		ReasoningEffort: reasoningEffort,
		Style:           style,
		ContextWindow:   contextWindow,
		NotifyEnabled:   notifyEnabled,
		NotifyEvents:    append([]string(nil), notifyEvents...),
	}
}
