package appruntime

import (
	"context"
	"errors"
	"fmt"

	"github.com/voocel/ainovel-cli/internal/host"
	"github.com/voocel/ainovel-cli/internal/webai"
)

// chatGPTLaneRuntime is the narrow D03 ownership seam between AppRuntime and
// the isolated ChatGPT Web browser lane. It is intentionally not exposed as a
// desktop command yet: D04 owns scheduler admission and D05 owns UI/START
// wiring. D03 only makes the real lane lifecycle/model state available behind
// AppRuntime while preserving lazy browser startup.
type chatGPTLaneRuntime interface {
	Start(context.Context) (webai.ChatGPTLaneSnapshot, error)
	Refresh(context.Context) (webai.ChatGPTLaneSnapshot, error)
	Stop() error
	Snapshot() webai.ChatGPTLaneSnapshot
}

func newAppRuntimeChatGPTLane(core *host.Host) chatGPTLaneRuntime {
	if core == nil {
		return nil
	}
	webCfg := core.WebConfiguration()
	return webai.NewChatGPTLane(webai.ChatGPTLaneConfig{
		BrowserPath: webCfg.BrowserPath,
		ProfileName: webai.DefaultChatGPTProfileName,
	})
}

// syncChatGPTProviderSnapshot projects only sanitized lane/model state into the
// D02 registry. It never starts or refreshes the browser, so dual-web read
// queries remain side-effect free and Lazy Start stays authoritative.
func (r *Runtime) syncChatGPTProviderSnapshot() error {
	if r == nil || r.dualWeb == nil {
		return ErrRuntimeUnavailable
	}
	if r.chatGPTLane == nil {
		return r.dualWeb.updateProvider(webAIProviderSnapshot{
			Provider: ProviderChatGPTWeb,
			LaneID:   webai.ChatGPTWebLaneID,
			Catalog: WebAIModelCatalogDTO{
				Provider: ProviderChatGPTWeb,
			},
		})
	}
	return r.dualWeb.updateProvider(projectChatGPTProviderSnapshot(r.chatGPTLane.Snapshot()))
}

func projectChatGPTProviderSnapshot(snapshot webai.ChatGPTLaneSnapshot) webAIProviderSnapshot {
	catalog := WebAIModelCatalogDTO{
		Provider: ProviderChatGPTWeb,
		Revision: snapshot.Catalog.Revision,
		Models:   make([]WebAIModelOptionDTO, 0, len(snapshot.Catalog.Models)),
	}
	for _, model := range snapshot.Catalog.Models {
		catalog.Models = append(catalog.Models, WebAIModelOptionDTO{
			ID:        model.ID,
			Label:     model.Label,
			Available: model.Available,
		})
	}
	laneID := snapshot.LaneID
	if laneID == "" {
		laneID = webai.ChatGPTWebLaneID
	}
	return webAIProviderSnapshot{
		Provider:      ProviderChatGPTWeb,
		LaneID:        laneID,
		Authenticated: snapshot.Authenticated,
		Ready:         snapshot.Ready,
		ActiveModelID: snapshot.Catalog.ActiveModelID,
		Catalog:       catalog,
	}
}

// startChatGPTLane is an internal D03 lifecycle boundary. No current desktop
// command calls it; D04/D05 may adopt it later without bypassing AppRuntime.
func (r *Runtime) startChatGPTLane(ctx context.Context) (webai.ChatGPTLaneSnapshot, error) {
	if r == nil || r.chatGPTLane == nil || r.dualWeb == nil {
		return webai.ChatGPTLaneSnapshot{}, ErrRuntimeUnavailable
	}
	snapshot, laneErr := r.chatGPTLane.Start(ctx)
	syncErr := r.dualWeb.updateProvider(projectChatGPTProviderSnapshot(snapshot))
	if syncErr != nil {
		if laneErr != nil {
			return snapshot, errors.Join(laneErr, syncErr)
		}
		return snapshot, syncErr
	}
	return snapshot, laneErr
}

// refreshChatGPTLane re-checks readiness/model observation for an already
// running lane. It does not launch a stopped browser and always republishes the
// resulting fail-closed snapshot even when refresh fails.
func (r *Runtime) refreshChatGPTLane(ctx context.Context) (webai.ChatGPTLaneSnapshot, error) {
	if r == nil || r.chatGPTLane == nil || r.dualWeb == nil {
		return webai.ChatGPTLaneSnapshot{}, ErrRuntimeUnavailable
	}
	snapshot, laneErr := r.chatGPTLane.Refresh(ctx)
	syncErr := r.dualWeb.updateProvider(projectChatGPTProviderSnapshot(snapshot))
	if syncErr != nil {
		if laneErr != nil {
			return snapshot, errors.Join(laneErr, syncErr)
		}
		return snapshot, syncErr
	}
	return snapshot, laneErr
}

func (r *Runtime) stopChatGPTLane() error {
	if r == nil || r.chatGPTLane == nil {
		return nil
	}
	stopErr := r.chatGPTLane.Stop()
	if r.dualWeb == nil {
		return stopErr
	}
	syncErr := r.dualWeb.updateProvider(projectChatGPTProviderSnapshot(r.chatGPTLane.Snapshot()))
	if stopErr != nil || syncErr != nil {
		return errors.Join(stopErr, syncErr)
	}
	return nil
}

func validateProjectedChatGPTSnapshot(snapshot webAIProviderSnapshot) error {
	if snapshot.Provider != ProviderChatGPTWeb {
		return fmt.Errorf("ChatGPT provider projection mismatch")
	}
	if snapshot.LaneID != webai.ChatGPTWebLaneID {
		return fmt.Errorf("ChatGPT lane projection mismatch")
	}
	return snapshot.Catalog.Validate()
}
