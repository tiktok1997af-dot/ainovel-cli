package desktopui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/voocel/ainovel-cli/internal/appruntime"
)

type RunCenterWorkspaceState struct {
	Load          LoadState
	Runs          []appruntime.RunSummaryDTO
	Lanes         []appruntime.BrowserLaneDTO
	SelectedRunID string
	Selected      *appruntime.RunsGetResultDTO
	Activity      []appruntime.RunActivityDTO
	Error         *ErrorView
}

func NewRunCenterWorkspaceState() RunCenterWorkspaceState {
	return RunCenterWorkspaceState{Load: LoadInitial}
}

func (c *Controller) RunCenter() *RunCenterWorkspaceState {
	if c == nil {
		return nil
	}
	return &c.runCenter
}

func (c *Controller) OpenRunCenter(ctx context.Context) error {
	if c == nil || c.shell == nil {
		return fmt.Errorf("desktopui: Run Center route is unavailable")
	}
	if !c.shell.SelectRoute(RouteRunCenter) {
		return fmt.Errorf("desktopui: Run Center route is unavailable")
	}
	return c.RefreshRunCenter(ctx)
}

func (c *Controller) RefreshRunCenter(ctx context.Context) error {
	if c == nil || c.runtime == nil {
		return ErrNilRuntime
	}
	state := &c.runCenter
	if len(state.Runs) == 0 {
		state.Load = LoadLoading
	} else {
		state.Load = LoadRefreshing
	}
	state.Error = nil

	payload, _ := json.Marshal(appruntime.RunsListQuery{
		PageQuery: appruntime.PageQuery{Limit: 100},
	})
	result, err := c.runtime.Query(ctx, appruntime.QueryRequest{
		ContractVersion: appruntime.ContractVersion,
		Kind:            appruntime.QueryRunsList,
		Payload:         payload,
	})
	if err != nil {
		state.Load = LoadRuntimeError
		state.Error = viewErrorFromError(err)
		return err
	}
	var list appruntime.RunsListResultDTO
	if err := json.Unmarshal(result.Data, &list); err != nil {
		state.Load = LoadRuntimeError
		state.Error = runtimeUnavailableViewError()
		return err
	}
	state.Runs = append([]appruntime.RunSummaryDTO(nil), list.Items...)
	state.Lanes = append([]appruntime.BrowserLaneDTO(nil), list.Lanes...)
	if len(state.Runs) == 0 {
		state.Load = LoadEmpty
	} else {
		state.Load = LoadReady
	}
	if state.SelectedRunID != "" {
		if err := c.SelectRun(ctx, state.SelectedRunID); err != nil {
			return err
		}
	}
	return nil
}

func (c *Controller) SelectRun(ctx context.Context, runID string) error {
	if c == nil || c.runtime == nil {
		return ErrNilRuntime
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		c.runCenter.SelectedRunID = ""
		c.runCenter.Selected = nil
		c.runCenter.Activity = nil
		return nil
	}

	getPayload, _ := json.Marshal(map[string]any{"run_id": runID})
	getResult, err := c.runtime.Query(ctx, appruntime.QueryRequest{
		ContractVersion: appruntime.ContractVersion,
		Kind:            appruntime.QueryRunsGet,
		Payload:         getPayload,
	})
	if err != nil {
		c.runCenter.Error = viewErrorFromError(err)
		return err
	}
	var selected appruntime.RunsGetResultDTO
	if err := json.Unmarshal(getResult.Data, &selected); err != nil {
		c.runCenter.Error = runtimeUnavailableViewError()
		return err
	}

	activityPayload, _ := json.Marshal(map[string]any{
		"run_id": runID,
		"limit":  100,
	})
	activityResult, err := c.runtime.Query(ctx, appruntime.QueryRequest{
		ContractVersion: appruntime.ContractVersion,
		Kind:            appruntime.QueryRunsActivity,
		Payload:         activityPayload,
	})
	if err != nil {
		c.runCenter.Error = viewErrorFromError(err)
		return err
	}
	var activity appruntime.RunsActivityResultDTO
	if err := json.Unmarshal(activityResult.Data, &activity); err != nil {
		c.runCenter.Error = runtimeUnavailableViewError()
		return err
	}
	c.runCenter.SelectedRunID = runID
	c.runCenter.Selected = &selected
	c.runCenter.Activity = append([]appruntime.RunActivityDTO(nil), activity.Items...)
	c.runCenter.Error = nil
	return nil
}

func (c *Controller) DispatchRunControl(ctx context.Context, kind appruntime.CommandKind, runID string) error {
	if c == nil || c.runtime == nil {
		return ErrNilRuntime
	}
	cmd, err := appruntime.NewDesktopRunControlCommand(c.nextRequestID(), kind, runID)
	if err != nil {
		c.runCenter.Error = runtimeUnavailableViewError()
		return err
	}
	c.runCenter.Load = LoadCommandPending
	result, err := c.runtime.Dispatch(ctx, cmd)
	if err != nil {
		c.runCenter.Load = LoadRuntimeError
		c.runCenter.Error = viewErrorFromError(err)
		return err
	}
	if !result.Accepted {
		c.runCenter.Load = LoadRuntimeError
		c.runCenter.Error = runtimeUnavailableViewError()
		return fmt.Errorf("desktopui: run command was not accepted")
	}
	return c.RefreshRunCenter(ctx)
}

func RunControlsForState(state string) []LifecycleControlView {
	state = strings.ToLower(strings.TrimSpace(state))
	eligible := func(action string) bool {
		switch action {
		case "start":
			return state == ""
		case "pause":
			return state == string(appruntime.RunStateRunning) || state == string(appruntime.RunStateStarting)
		case "resume":
			return state == string(appruntime.RunStatePaused)
		case "stop", "cancel":
			switch state {
			case string(appruntime.RunStateQueued), string(appruntime.RunStateBlockedResource),
				string(appruntime.RunStateBlockedLane), string(appruntime.RunStateStarting),
				string(appruntime.RunStateRunning), string(appruntime.RunStateRecovering),
				string(appruntime.RunStatePaused):
				return true
			}
		case "retry":
			return state == string(appruntime.RunStateFailed) || state == string(appruntime.RunStateCancelled) ||
				state == string(appruntime.RunStateStopped)
		}
		return false
	}
	return []LifecycleControlView{
		{ID: "run.start", Label: "Start", Eligible: eligible("start"), Enabled: eligible("start")},
		{ID: "run.pause", Label: "Pause", Eligible: eligible("pause"), Enabled: eligible("pause")},
		{ID: "run.resume", Label: "Resume", Eligible: eligible("resume"), Enabled: eligible("resume")},
		{ID: "run.stop", Label: "Stop", Eligible: eligible("stop"), Enabled: eligible("stop")},
		{ID: "run.cancel", Label: "Cancel", Eligible: eligible("cancel"), Enabled: eligible("cancel")},
		{ID: "run.retry", Label: "Retry", Eligible: eligible("retry"), Enabled: eligible("retry")},
	}
}
