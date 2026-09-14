package appruntime

import (
	"context"
	"errors"
	"testing"

	"github.com/voocel/ainovel-cli/internal/webai"
	"github.com/voocel/ainovel-cli/internal/webai/sites"
)

type fakeAppChatGPTLane struct {
	snapshot        webai.ChatGPTLaneSnapshot
	startSnapshot   webai.ChatGPTLaneSnapshot
	refreshSnapshot webai.ChatGPTLaneSnapshot
	startErr        error
	refreshErr      error
	stopErr         error
	startCalls      int
	refreshCalls    int
	stopCalls       int
}

func (f *fakeAppChatGPTLane) Start(context.Context) (webai.ChatGPTLaneSnapshot, error) {
	f.startCalls++
	f.snapshot = f.startSnapshot
	return f.snapshot, f.startErr
}

func (f *fakeAppChatGPTLane) Refresh(context.Context) (webai.ChatGPTLaneSnapshot, error) {
	f.refreshCalls++
	f.snapshot = f.refreshSnapshot
	return f.snapshot, f.refreshErr
}

func (f *fakeAppChatGPTLane) Stop() error {
	f.stopCalls++
	f.snapshot = webai.ChatGPTLaneSnapshot{
		LaneID: webai.ChatGPTWebLaneID,
		State:  webai.SessionStopped,
	}
	return f.stopErr
}

func (f *fakeAppChatGPTLane) Snapshot() webai.ChatGPTLaneSnapshot { return f.snapshot }

func d03ChatGPTReadySnapshot(modelID, revision string) webai.ChatGPTLaneSnapshot {
	return webai.ChatGPTLaneSnapshot{
		LaneID:        webai.ChatGPTWebLaneID,
		State:         webai.SessionReady,
		Authenticated: true,
		Ready:         true,
		Catalog: sites.ModelCatalogSnapshot{
			ActiveModelID: modelID,
			Revision:      revision,
			Models: []sites.ModelOption{
				{ID: modelID, Label: "ChatGPT observed model", Available: true},
			},
		},
	}
}

func TestD03AppRuntimeProjectsChatGPTLaneIntoD02Preflight(t *testing.T) {
	lane := &fakeAppChatGPTLane{snapshot: d03ChatGPTReadySnapshot("chatgpt-observed", "rev-1")}
	rt := &Runtime{dualWeb: newDualWebProviderRuntime(), chatGPTLane: lane}

	if err := rt.syncChatGPTProviderSnapshot(); err != nil {
		t.Fatal(err)
	}
	providers := rt.dualWeb.providersResult()
	var chatgpt *WebAIProviderRuntimeDTO
	for i := range providers.Providers {
		if providers.Providers[i].Provider == ProviderChatGPTWeb {
			chatgpt = &providers.Providers[i]
		}
	}
	if chatgpt == nil || !chatgpt.Authenticated || !chatgpt.Ready || chatgpt.ActiveModelID != "chatgpt-observed" {
		t.Fatalf("unexpected ChatGPT provider projection: %+v", chatgpt)
	}
	if len(chatgpt.Catalog.Models) != 1 || chatgpt.Catalog.Revision != "rev-1" {
		t.Fatalf("unexpected ChatGPT catalog projection: %+v", chatgpt.Catalog)
	}

	selection, err := rt.dualWeb.selectModel(ProviderChatGPTWeb, "chatgpt-observed")
	if err != nil || !selection.Verified {
		t.Fatalf("selection = %+v err=%v", selection, err)
	}
	preflight, err := rt.dualWeb.preflight([]WebAIProvider{ProviderChatGPTWeb})
	if err != nil || !preflight.StartAllowed {
		t.Fatalf("ready preflight = %+v err=%v", preflight, err)
	}
}

func TestD03AppRuntimeLaneLossInvalidatesVerifiedChatGPTSelection(t *testing.T) {
	lane := &fakeAppChatGPTLane{snapshot: d03ChatGPTReadySnapshot("chatgpt-observed", "rev-1")}
	rt := &Runtime{dualWeb: newDualWebProviderRuntime(), chatGPTLane: lane}
	if err := rt.syncChatGPTProviderSnapshot(); err != nil {
		t.Fatal(err)
	}
	if selection, err := rt.dualWeb.selectModel(ProviderChatGPTWeb, "chatgpt-observed"); err != nil || !selection.Verified {
		t.Fatalf("initial verified selection = %+v err=%v", selection, err)
	}

	lane.snapshot = webai.ChatGPTLaneSnapshot{
		LaneID: webai.ChatGPTWebLaneID,
		State:  webai.SessionAuthRequired,
	}
	if err := rt.syncChatGPTProviderSnapshot(); err != nil {
		t.Fatal(err)
	}
	preflight, err := rt.dualWeb.preflight([]WebAIProvider{ProviderChatGPTWeb})
	if err != nil {
		t.Fatal(err)
	}
	if preflight.StartAllowed || len(preflight.Selections) != 1 || preflight.Selections[0].Verified {
		t.Fatalf("lane loss failed to invalidate selection: %+v", preflight)
	}
}

func TestD03AppRuntimeStartRefreshAndStopAlwaysRepublishLaneState(t *testing.T) {
	lane := &fakeAppChatGPTLane{
		startSnapshot: d03ChatGPTReadySnapshot("chatgpt-a", "rev-a"),
		refreshSnapshot: webai.ChatGPTLaneSnapshot{
			LaneID: webai.ChatGPTWebLaneID,
			State:  webai.SessionDegraded,
		},
		refreshErr: errors.New("temporary readiness failure"),
	}
	rt := &Runtime{dualWeb: newDualWebProviderRuntime(), chatGPTLane: lane}

	started, err := rt.startChatGPTLane(context.Background())
	if err != nil || !started.Ready || lane.startCalls != 1 {
		t.Fatalf("start snapshot=%+v calls=%d err=%v", started, lane.startCalls, err)
	}
	if provider, _ := rt.dualWeb.models(ProviderChatGPTWeb); provider.Revision != "rev-a" {
		t.Fatalf("start catalog not published: %+v", provider)
	}

	refreshed, err := rt.refreshChatGPTLane(context.Background())
	if err == nil || refreshed.State != webai.SessionDegraded || lane.refreshCalls != 1 {
		t.Fatalf("refresh snapshot=%+v calls=%d err=%v", refreshed, lane.refreshCalls, err)
	}
	providers := rt.dualWeb.providersResult()
	for _, provider := range providers.Providers {
		if provider.Provider == ProviderChatGPTWeb && (provider.Ready || provider.Authenticated || provider.ActiveModelID != "" || len(provider.Catalog.Models) != 0) {
			t.Fatalf("degraded refresh was not projected fail-closed: %+v", provider)
		}
	}

	if err := rt.stopChatGPTLane(); err != nil {
		t.Fatal(err)
	}
	if lane.stopCalls != 1 {
		t.Fatalf("stop calls = %d, want 1", lane.stopCalls)
	}
	providers = rt.dualWeb.providersResult()
	for _, provider := range providers.Providers {
		if provider.Provider == ProviderChatGPTWeb && (provider.Ready || provider.Authenticated || provider.ActiveModelID != "" || provider.Catalog.Revision != "") {
			t.Fatalf("stopped lane leaked provider readiness: %+v", provider)
		}
	}
}

func TestD03ProjectedChatGPTSnapshotRejectsWrongLaneIdentity(t *testing.T) {
	projected := projectChatGPTProviderSnapshot(d03ChatGPTReadySnapshot("chatgpt-a", "rev-a"))
	if err := validateProjectedChatGPTSnapshot(projected); err != nil {
		t.Fatalf("valid projection rejected: %v", err)
	}
	projected.LaneID = "shared-gemini-lane"
	if err := validateProjectedChatGPTSnapshot(projected); err == nil {
		t.Fatal("non-isolated ChatGPT lane identity was accepted")
	}
}
