package desktopui

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/voocel/ainovel-cli/internal/appruntime"
)

type settingsWorkspaceRuntime struct {
	current  appruntime.SettingsGetResultDTO
	queries  []appruntime.QueryRequest
	commands []appruntime.CommandRequest
	stale    bool
}

func (f *settingsWorkspaceRuntime) Snapshot(context.Context) (appruntime.DesktopSnapshot, error) {
	return testSnapshot(1, "Novel"), nil
}

func (f *settingsWorkspaceRuntime) Query(_ context.Context, req appruntime.QueryRequest) (appruntime.QueryResult, error) {
	f.queries = append(f.queries, req)
	raw, err := json.Marshal(f.current)
	if err != nil {
		return appruntime.QueryResult{}, err
	}
	return appruntime.QueryResult{ContractVersion: appruntime.ContractVersion, Kind: req.Kind, Data: raw}, nil
}

func (f *settingsWorkspaceRuntime) Dispatch(_ context.Context, req appruntime.CommandRequest) (appruntime.CommandResult, error) {
	f.commands = append(f.commands, req)
	if f.stale {
		appErr := &appruntime.AppError{
			Code:     appruntime.ErrorCodeStaleConflict,
			Category: appruntime.ErrorCategoryConflict,
			Message:  "The project data changed after the action was prepared. Refresh and try again.",
		}
		return appruntime.CommandResult{ContractVersion: appruntime.ContractVersion, CommandID: req.ID, Error: appErr}, appErr
	}
	var payload appruntime.SettingsUpdateCommandPayload
	if err := json.Unmarshal(req.Payload, &payload); err != nil {
		return appruntime.CommandResult{}, err
	}
	f.current.Values = cloneSettingsValues(payload.Values)
	f.current.Values.ProfileName = "canonical-" + payload.Values.ProfileName
	f.current.Fingerprint = "fp-next"
	f.current.RestartRequired = true
	return appruntime.CommandResult{
		ContractVersion: appruntime.ContractVersion,
		CommandID:       req.ID,
		Accepted:        true,
		Status:          "completed",
		Data:            json.RawMessage("{}"),
	}, nil
}

func (f *settingsWorkspaceRuntime) Subscribe(context.Context, appruntime.EventCursor) (appruntime.EventSubscription, error) {
	return &workspaceSubscription{events: make(chan appruntime.DesktopEvent, 1)}, nil
}
func (f *settingsWorkspaceRuntime) Close(context.Context) error { return nil }

func newSettingsWorkspaceRuntime() *settingsWorkspaceRuntime {
	return &settingsWorkspaceRuntime{current: appruntime.SettingsGetResultDTO{
		Identity: appruntime.SettingsIdentityDTO{WebEnabled: true, WebSite: "gemini-web"},
		Values: appruntime.SettingsValuesDTO{
			ProfileName:     "default",
			Language:        "vi",
			ReasoningEffort: "medium",
			Style:           "default",
			ContextWindow:   200000,
			NotifyEnabled:   true,
		},
		Runtime:         appruntime.SettingsRuntimeDTO{Values: appruntime.SettingsValuesDTO{ProfileName: "default", NotifyEnabled: true}},
		ConfigScope:     "project",
		ConfigPathClass: "./.ainovel/config.json",
		Fingerprint:     "fp-current",
		ApplyMode:       "next_process_start",
	}}
}

func TestSettingsWorkspaceReadsSavesThenRequeriesCanonicalState(t *testing.T) {
	runtime := newSettingsWorkspaceRuntime()
	controller := NewController(runtime, 1440)
	ctx := context.Background()
	if err := controller.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	if err := controller.OpenSettings(ctx); err != nil {
		t.Fatal(err)
	}
	if controller.Shell().Route != RouteSettings || controller.Settings().Load != LoadReady {
		t.Fatalf("settings workspace not active: route=%q state=%+v", controller.Shell().Route, controller.Settings())
	}
	if len(runtime.queries) != 1 || runtime.queries[0].Kind != appruntime.QuerySettingsGet {
		t.Fatalf("settings queries=%+v", runtime.queries)
	}

	draft := controller.Settings().Form
	draft.ProfileName = "next"
	if err := controller.SetSettingsDraft(draft); err != nil {
		t.Fatal(err)
	}
	if err := controller.SaveSettings(ctx); err != nil {
		t.Fatal(err)
	}
	if len(runtime.commands) != 1 || runtime.commands[0].Kind != appruntime.CommandSettingsUpdate {
		t.Fatalf("settings commands=%+v", runtime.commands)
	}
	cmd := runtime.commands[0]
	if cmd.RunID != "" || cmd.TaskID != "" || cmd.Resource != "" {
		t.Fatalf("Settings UI claimed core-owned execution fields: %+v", cmd)
	}
	var payload appruntime.SettingsUpdateCommandPayload
	if err := json.Unmarshal(cmd.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.ExpectedFingerprint != "fp-current" || payload.Values.ProfileName != "next" {
		t.Fatalf("settings update payload=%+v", payload)
	}
	if len(runtime.queries) != 2 {
		t.Fatalf("successful save did not fresh-read canonical state: queries=%d", len(runtime.queries))
	}
	if controller.Settings().Canonical.Values.ProfileName != "canonical-next" || controller.Settings().Form.ProfileName != "canonical-next" || controller.Settings().Dirty {
		t.Fatalf("UI kept optimistic/local truth: %+v", controller.Settings())
	}
}

func TestSettingsWorkspaceStaleSaveFailsClosedUntilReload(t *testing.T) {
	runtime := newSettingsWorkspaceRuntime()
	controller := NewController(runtime, 1440)
	ctx := context.Background()
	if err := controller.OpenSettings(ctx); err != nil {
		t.Fatal(err)
	}
	draft := controller.Settings().Form
	draft.ProfileName = "stale-form"
	if err := controller.SetSettingsDraft(draft); err != nil {
		t.Fatal(err)
	}
	runtime.stale = true
	if err := controller.SaveSettings(ctx); err == nil {
		t.Fatal("stale save unexpectedly succeeded")
	}
	if controller.Settings().Load != LoadConflict || controller.Settings().Error == nil {
		t.Fatalf("stale conflict not projected: %+v", controller.Settings())
	}
	runtime.stale = false
	runtime.current.Values.ProfileName = "external-change"
	runtime.current.Fingerprint = "fp-external"
	if err := controller.ReloadSettings(ctx); err != nil {
		t.Fatal(err)
	}
	if controller.Settings().Canonical.Fingerprint != "fp-external" || controller.Settings().Form.ProfileName != "external-change" || controller.Settings().Dirty {
		t.Fatalf("reload did not restore runtime authority: %+v", controller.Settings())
	}
}
