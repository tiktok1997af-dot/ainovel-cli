package webai

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/voocel/ainovel-cli/internal/webai/sites"
)

type fixedChatGPTEvaluator struct {
	payload json.RawMessage
	closed  bool
}

func (e *fixedChatGPTEvaluator) Eval(context.Context, string) (json.RawMessage, error) {
	return append(json.RawMessage(nil), e.payload...), nil
}

func (e *fixedChatGPTEvaluator) Close() error {
	e.closed = true
	return nil
}

func chatGPTModelEvaluatorFactory(payload string) func(context.Context, SessionSnapshot, sites.Adapter) (interactionEvaluator, error) {
	return func(context.Context, SessionSnapshot, sites.Adapter) (interactionEvaluator, error) {
		return &fixedChatGPTEvaluator{payload: json.RawMessage(payload)}, nil
	}
}

type sequenceReadinessProbe struct {
	results []ReadinessResult
	calls   int
}

func (p *sequenceReadinessProbe) Probe(context.Context, SessionSnapshot) (ReadinessResult, error) {
	if len(p.results) == 0 {
		return ReadinessResult{State: SessionFailed, Reason: "empty readiness sequence"}, nil
	}
	index := p.calls
	if index >= len(p.results) {
		index = len(p.results) - 1
	}
	p.calls++
	return p.results[index], nil
}

func TestD03ChatGPTLaneIsLazyIsolatedAndSingleTurn(t *testing.T) {
	launcher := &fakeBrowserLauncher{}
	profileDir := filepath.Join(t.TempDir(), "chatgpt-profile")
	lane := NewChatGPTLane(ChatGPTLaneConfig{
		BrowserPath: fakeBrowserExecutable(t),
		ProfileDir:  profileDir,
		Launcher:    launcher,
		Probe: fixedReadinessProbe{result: ReadinessResult{
			State:  SessionReady,
			Reason: "authenticated ChatGPT test session",
		}},
		evaluatorFactory: chatGPTModelEvaluatorFactory(`{
			"active_label":"GPT Test",
			"models":[{"label":"GPT Test","available":true},{"label":"GPT Alt","available":true}]
		}`),
	})

	if got := lane.Snapshot(); got.State != SessionStopped || got.Ready || got.Authenticated {
		t.Fatalf("lazy lane initial snapshot = %+v", got)
	}
	if len(launcher.configs) != 0 {
		t.Fatalf("lazy lane launched browser during construction: %d launches", len(launcher.configs))
	}

	snap, err := lane.Start(context.Background())
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer lane.Stop()
	if !snap.Authenticated || !snap.Ready || snap.Catalog.ActiveModelID == "" || snap.Catalog.Revision == "" {
		t.Fatalf("ready lane snapshot = %+v", snap)
	}
	if snap.LaneID != ChatGPTWebLaneID {
		t.Fatalf("lane id = %q, want %q", snap.LaneID, ChatGPTWebLaneID)
	}
	if len(launcher.configs) != 1 || launcher.lastConfig().ProfileDir != profileDir {
		t.Fatalf("isolated launch config = %+v", launcher.configs)
	}

	if err := lane.BeginTurn(); err != nil {
		t.Fatalf("BeginTurn: %v", err)
	}
	if err := lane.BeginTurn(); err == nil {
		t.Fatal("second concurrent ChatGPT turn was accepted")
	}
	if got := lane.Snapshot(); got.Ready || !got.TurnActive {
		t.Fatalf("active-turn snapshot = %+v", got)
	}
	lane.EndTurn()
	if got := lane.Snapshot(); !got.Ready || got.TurnActive {
		t.Fatalf("post-turn snapshot = %+v", got)
	}

	if err := lane.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if got := lane.Snapshot(); got.State != SessionStopped || got.Ready || got.Authenticated || got.Catalog.ActiveModelID != "" || len(got.Catalog.Models) != 0 {
		t.Fatalf("stopped lane leaked readiness/catalog state: %+v", got)
	}
}

func TestD03ChatGPTLaneFailsClosedWhenActiveModelCannotBeObserved(t *testing.T) {
	lane := NewChatGPTLane(ChatGPTLaneConfig{
		BrowserPath:          fakeBrowserExecutable(t),
		ProfileDir:           filepath.Join(t.TempDir(), "chatgpt-profile"),
		Launcher:             &fakeBrowserLauncher{},
		RequireObservedModel: true,
		Probe: fixedReadinessProbe{result: ReadinessResult{
			State: SessionReady,
		}},
		evaluatorFactory: chatGPTModelEvaluatorFactory(`{"active_label":"","models":[]}`),
	})

	snap, err := lane.Start(context.Background())
	defer lane.Stop()
	if err == nil {
		t.Fatal("missing active model observation unexpectedly succeeded")
	}
	if !snap.Authenticated || snap.Ready || snap.Catalog.ActiveModelID != "" || snap.Catalog.Revision != "" {
		t.Fatalf("model-observation failure did not fail closed: %+v", snap)
	}
	if err := lane.BeginTurn(); err == nil {
		t.Fatal("turn admitted without verified ChatGPT model observation")
	}
}

func TestD08ChatGPTLaneUsesProviderDefaultWhenAuthenticatedUIHidesModel(t *testing.T) {
	lane := NewChatGPTLane(ChatGPTLaneConfig{
		BrowserPath: fakeBrowserExecutable(t),
		ProfileDir:  filepath.Join(t.TempDir(), "chatgpt-profile"),
		Launcher:    &fakeBrowserLauncher{},
		Probe: fixedReadinessProbe{result: ReadinessResult{
			State:  SessionReady,
			Reason: "authenticated ChatGPT composer ready",
		}},
		evaluatorFactory: chatGPTModelEvaluatorFactory(`{"active_label":"","models":[]}`),
	})

	snap, err := lane.Start(context.Background())
	defer lane.Stop()
	if err != nil {
		t.Fatalf("provider-default Start: %v", err)
	}
	if !snap.Authenticated || !snap.Ready || snap.State != SessionReady {
		t.Fatalf("provider-default lane snapshot = %+v", snap)
	}
	if snap.Catalog.ActiveModelID != chatGPTProviderDefaultModelID || snap.Catalog.Revision != chatGPTProviderDefaultCatalogRev {
		t.Fatalf("provider-default catalog = %+v", snap.Catalog)
	}
	if len(snap.Catalog.Models) != 1 || !snap.Catalog.Models[0].Available || snap.Catalog.Models[0].Label != chatGPTProviderDefaultModelLabel {
		t.Fatalf("provider-default models = %+v", snap.Catalog.Models)
	}
	if err := lane.BeginTurn(); err != nil {
		t.Fatalf("provider-default BeginTurn: %v", err)
	}
	lane.EndTurn()
}

func TestD03ChatGPTLaneDefaultProfileIsProviderSpecific(t *testing.T) {
	lane := NewChatGPTLane(ChatGPTLaneConfig{})
	if lane.session == nil {
		t.Fatal("ChatGPT lane session is nil")
	}
	if lane.session.cfg.Site != ChatGPTWebSite {
		t.Fatalf("site = %q", lane.session.cfg.Site)
	}
	if lane.session.cfg.ProfileName != DefaultChatGPTProfileName {
		t.Fatalf("profile = %q, want %q", lane.session.cfg.ProfileName, DefaultChatGPTProfileName)
	}
	if lane.session.cfg.StartURL != "https://chatgpt.com/" {
		t.Fatalf("start URL = %q", lane.session.cfg.StartURL)
	}
}

func TestD08ChatGPTLaneBoundedRevalidatesFreshSessionBeforeDispatch(t *testing.T) {
	probe := &sequenceReadinessProbe{results: []ReadinessResult{
		{State: SessionAuthRequired, Reason: "renderer still settling"},
		{State: SessionReady, Reason: "persisted ChatGPT login ready"},
	}}
	lane := NewChatGPTLane(ChatGPTLaneConfig{
		BrowserPath: fakeBrowserExecutable(t),
		ProfileDir:  filepath.Join(t.TempDir(), "chatgpt-profile"),
		Launcher:    &fakeBrowserLauncher{},
		Probe:       probe,
		evaluatorFactory: chatGPTModelEvaluatorFactory(`{
			"active_label":"GPT Test",
			"models":[{"label":"GPT Test","available":true}]
		}`),
	})
	lane.transport.authRequiredGrace = 100 * time.Millisecond
	lane.transport.readinessPollInterval = time.Millisecond

	snap, err := lane.ensureVerifiedReady(context.Background())
	defer lane.Stop()
	if err != nil {
		t.Fatalf("ensureVerifiedReady: %v", err)
	}
	if !snap.Authenticated || !snap.Ready || snap.State != SessionReady {
		t.Fatalf("revalidated lane snapshot = %+v", snap)
	}
	if probe.calls < 2 {
		t.Fatalf("readiness probe calls = %d, want at least 2", probe.calls)
	}
	if snap.Catalog.ActiveModelID == "" || snap.Catalog.Revision == "" {
		t.Fatalf("revalidated model catalog = %+v", snap.Catalog)
	}
}

func TestD08ChatGPTLanePersistentAuthRequiredStillFailsClosed(t *testing.T) {
	lane := NewChatGPTLane(ChatGPTLaneConfig{
		BrowserPath: fakeBrowserExecutable(t),
		ProfileDir:  filepath.Join(t.TempDir(), "chatgpt-profile"),
		Launcher:    &fakeBrowserLauncher{},
		Probe: fixedReadinessProbe{result: ReadinessResult{
			State:  SessionAuthRequired,
			Reason: "manual login required",
		}},
		evaluatorFactory: chatGPTModelEvaluatorFactory(`{
			"active_label":"GPT Test",
			"models":[{"label":"GPT Test","available":true}]
		}`),
	})
	lane.transport.authRequiredGrace = 10 * time.Millisecond
	lane.transport.readinessPollInterval = time.Millisecond

	_, err := lane.ensureVerifiedReady(context.Background())
	defer lane.Stop()
	if err == nil {
		t.Fatal("persistent AUTH_REQUIRED unexpectedly became READY")
	}
	var webErr *Error
	if !errors.As(err, &webErr) || webErr.Kind != ErrorAuthRequired {
		t.Fatalf("error = %v, want ErrorAuthRequired", err)
	}
	if got := lane.Snapshot(); got.Ready || got.Authenticated || got.Catalog.ActiveModelID != "" || got.Catalog.Revision != "" {
		t.Fatalf("AUTH_REQUIRED lane did not fail closed: %+v", got)
	}
}
