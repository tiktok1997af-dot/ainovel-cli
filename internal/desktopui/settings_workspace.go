package desktopui

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/voocel/ainovel-cli/internal/appruntime"
)

type SettingsWorkspaceState struct {
	Load      LoadState
	Canonical appruntime.SettingsGetResultDTO
	Form      appruntime.SettingsValuesDTO
	Dirty     bool
	Error     *ErrorView
}

func NewSettingsWorkspaceState() SettingsWorkspaceState {
	return SettingsWorkspaceState{Load: LoadInitial}
}

func (c *Controller) Settings() *SettingsWorkspaceState {
	if c == nil {
		return nil
	}
	return &c.settings
}

func (c *Controller) OpenSettings(ctx context.Context) error {
	if c == nil || c.shell == nil {
		return fmt.Errorf("desktopui: Settings route is unavailable")
	}
	if !c.shell.SelectRoute(RouteSettings) {
		return fmt.Errorf("desktopui: Settings route is unavailable")
	}
	return c.RefreshSettings(ctx)
}

func (c *Controller) RefreshSettings(ctx context.Context) error {
	if c == nil || c.runtime == nil {
		return ErrNilRuntime
	}
	state := &c.settings
	if state.Load == LoadInitial {
		state.Load = LoadLoading
	} else {
		state.Load = LoadRefreshing
	}
	state.Error = nil

	result, err := c.runtime.Query(ctx, appruntime.QueryRequest{
		ContractVersion: appruntime.ContractVersion,
		Kind:            appruntime.QuerySettingsGet,
	})
	if err != nil {
		return c.failSettings(err)
	}
	if result.Error != nil {
		return c.failSettings(result.Error)
	}
	if result.ContractVersion != appruntime.ContractVersion {
		return c.failSettings(fmt.Errorf("desktopui: Settings query contract mismatch"))
	}
	var canonical appruntime.SettingsGetResultDTO
	if err := json.Unmarshal(result.Data, &canonical); err != nil {
		return c.failSettings(err)
	}
	state.Canonical = cloneSettingsResult(canonical)
	state.Form = cloneSettingsValues(canonical.Values)
	state.Dirty = false
	state.Load = LoadReady
	state.Error = nil
	return nil
}

func (c *Controller) ReloadSettings(ctx context.Context) error {
	return c.RefreshSettings(ctx)
}

func (c *Controller) SetSettingsDraft(values appruntime.SettingsValuesDTO) error {
	if c == nil || c.settings.Load == LoadInitial {
		return fmt.Errorf("desktopui: Settings workspace is not loaded")
	}
	c.settings.Form = cloneSettingsValues(values)
	c.settings.Dirty = true
	c.settings.Error = nil
	return nil
}

func (c *Controller) SaveSettings(ctx context.Context) error {
	if c == nil || c.runtime == nil {
		return ErrNilRuntime
	}
	state := &c.settings
	if state.Load == LoadInitial || state.Canonical.Fingerprint == "" {
		return fmt.Errorf("desktopui: Settings workspace is not loaded")
	}
	payload, err := json.Marshal(appruntime.SettingsUpdateCommandPayload{
		ExpectedFingerprint: state.Canonical.Fingerprint,
		Values:              cloneSettingsValues(state.Form),
	})
	if err != nil {
		return c.failSettings(err)
	}
	state.Load = LoadSaving
	state.Error = nil

	result, err := c.runtime.Dispatch(ctx, appruntime.CommandRequest{
		ContractVersion: appruntime.ContractVersion,
		ID:              c.nextRequestID(),
		Kind:            appruntime.CommandSettingsUpdate,
		Payload:         payload,
	})
	if err != nil {
		return c.failSettings(err)
	}
	if result.Error != nil {
		return c.failSettings(result.Error)
	}
	if result.ContractVersion != appruntime.ContractVersion {
		return c.failSettings(fmt.Errorf("desktopui: Settings command contract mismatch"))
	}
	if !result.Accepted {
		return c.failSettings(fmt.Errorf("desktopui: Settings update was not accepted"))
	}
	return c.RefreshSettings(ctx)
}

func (c *Controller) failSettings(err error) error {
	c.settings.Error = viewErrorFromError(err)
	c.settings.Load = loadStateForWorkspaceError(c.settings.Error)
	return err
}

func cloneSettingsResult(in appruntime.SettingsGetResultDTO) appruntime.SettingsGetResultDTO {
	out := in
	out.Values = cloneSettingsValues(in.Values)
	out.Runtime.Values = cloneSettingsValues(in.Runtime.Values)
	return out
}

func cloneSettingsValues(in appruntime.SettingsValuesDTO) appruntime.SettingsValuesDTO {
	out := in
	out.NotifyEvents = append([]string(nil), in.NotifyEvents...)
	return out
}
