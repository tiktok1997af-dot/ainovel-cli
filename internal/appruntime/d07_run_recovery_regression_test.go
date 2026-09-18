package appruntime

import (
	"context"
	"testing"
)

func TestD07AdmissionBackendPreservesRestartRecoveryCapability(t *testing.T) {
	base := newFakeRunBackend()
	backend := newD07AdmissionRunBackend(base)
	coord := newRunCoordinator(backend, &fakeRunLanePool{})

	if err := coord.recoverRestart(context.Background()); err != nil {
		t.Fatalf("D07 admission backend must preserve restart recovery capability: %v", err)
	}
}
