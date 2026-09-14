package appruntime

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/voocel/ainovel-cli/internal/domain"
)

func d06OpenCircuitForTest(t *testing.T, layer *providerResilienceLayer, provider WebAIProvider) {
	t.Helper()
	for i := 0; i < d06ProviderFailureThreshold; i++ {
		if err := layer.beginAdmission(provider); err != nil {
			t.Fatalf("begin failure %d: %v", i+1, err)
		}
		layer.recordFailure(provider)
	}
}

func TestD06NeutralHalfOpenProbeReturnsToOpenWithoutFailureIncrement(t *testing.T) {
	layer := newProviderResilienceLayer()
	now := time.Date(2026, 9, 14, 11, 0, 0, 0, time.UTC)
	layer.now = func() time.Time { return now }
	d06OpenCircuitForTest(t, layer, ProviderGeminiWeb)

	now = now.Add(d06ProviderOpenCooldown + time.Millisecond)
	if err := layer.beginAdmission(ProviderGeminiWeb); err != nil {
		t.Fatalf("half-open probe: %v", err)
	}
	layer.recordNeutral(ProviderGeminiWeb)

	got := layer.snapshot(ProviderGeminiWeb)
	if got.State != providerCircuitOpen || got.ProbeInFlight {
		t.Fatalf("neutral half-open outcome = %+v", got)
	}
	if got.ConsecutiveFailures != d06ProviderFailureThreshold {
		t.Fatalf("neutral outcome changed failure count: %+v", got)
	}
	if !got.OpenedAt.Equal(now) {
		t.Fatalf("neutral outcome did not restart bounded cooldown: opened=%v now=%v", got.OpenedAt, now)
	}
}

func TestD06PreCancelledAdmissionCannotReleaseAnotherHalfOpenProbe(t *testing.T) {
	layer := newProviderResilienceLayer()
	now := time.Date(2026, 9, 14, 11, 0, 0, 0, time.UTC)
	layer.now = func() time.Time { return now }
	d06OpenCircuitForTest(t, layer, ProviderGeminiWeb)
	now = now.Add(d06ProviderOpenCooldown + time.Millisecond)
	if err := layer.beginAdmission(ProviderGeminiWeb); err != nil {
		t.Fatal(err)
	}
	before := layer.snapshot(ProviderGeminiWeb)
	if before.State != providerCircuitHalfOpen || !before.ProbeInFlight {
		t.Fatalf("expected owned half-open probe: %+v", before)
	}

	lanes := &d06FakeLanePool{}
	authority := newResilientDualWebLaneAuthority(&Runtime{run: &runCoordinator{lanes: lanes}})
	authority.health = layer
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := authority.Acquire(ctx, ProviderGeminiWeb, domain.RunID("run-d06-cancel-other")); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled admission err=%v", err)
	}

	after := layer.snapshot(ProviderGeminiWeb)
	if after.State != providerCircuitHalfOpen || !after.ProbeInFlight {
		t.Fatalf("cancelled non-owner mutated probe: before=%+v after=%+v", before, after)
	}
	if lanes.allocateCalls != 0 {
		t.Fatalf("pre-cancelled admission touched browser lane: %d", lanes.allocateCalls)
	}
}
