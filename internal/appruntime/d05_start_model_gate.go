package appruntime

import (
	"context"
	"fmt"
	"strings"

	"github.com/voocel/ainovel-cli/internal/webai"
)

// enforceDesktopStartModelGate is the D05 authority boundary for the global
// desktop START command. The selected Web provider is carried in
// CommandRequest.Resource so the existing v1 START payload remains unchanged.
// Missing, unknown, unauthenticated, unready, or unverified providers fail
// closed before the legacy engine can be resumed.
func (r *Runtime) enforceDesktopStartModelGate(ctx context.Context, cmd CommandRequest) error {
	if cmd.Kind != CommandStart {
		return nil
	}
	provider := WebAIProvider(strings.TrimSpace(cmd.Resource))
	if err := provider.Validate(); err != nil {
		return fmt.Errorf("%w: START requires an explicit Gemini or ChatGPT selection", ErrMutationPrecondition)
	}

	switch provider {
	case ProviderGeminiWeb:
		snapshot := r.core.WebSessionSnapshot()
		if snapshot.State != webai.SessionReady && snapshot.State != webai.SessionBusy {
			return fmt.Errorf("%w: Gemini Web is not authenticated and READY", ErrMutationPrecondition)
		}
		return nil

	case ProviderChatGPTWeb:
		if r.chatGPTLane == nil || r.dualWeb == nil {
			return fmt.Errorf("%w: ChatGPT Web lane is unavailable", ErrMutationPrecondition)
		}
		snapshot := r.chatGPTLane.Snapshot()
		var err error
		if snapshot.State == webai.SessionStopped {
			snapshot, err = r.startChatGPTLane(ctx)
		} else {
			snapshot, err = r.refreshChatGPTLane(ctx)
		}
		if err != nil && !snapshot.Ready {
			return fmt.Errorf("%w: ChatGPT Web readiness refresh failed: %v", ErrMutationPrecondition, err)
		}
		if !snapshot.Authenticated || !snapshot.Ready ||
			strings.TrimSpace(snapshot.Catalog.ActiveModelID) == "" ||
			strings.TrimSpace(snapshot.Catalog.Revision) == "" {
			return fmt.Errorf("%w: ChatGPT Web model observation is not verified", ErrMutationPrecondition)
		}

		// Pin the active observed model through the frozen D02 selection contract.
		// D05 does not fake DOM model switching: if the observed model cannot be
		// selected and verified, START is rejected.
		if _, err := r.dualWeb.selectModel(provider, snapshot.Catalog.ActiveModelID); err != nil {
			return fmt.Errorf("%w: ChatGPT Web active model cannot be pinned", ErrMutationPrecondition)
		}
		preflight, err := r.dualWeb.preflight([]WebAIProvider{provider})
		if err != nil || !preflight.StartAllowed {
			return fmt.Errorf("%w: ChatGPT Web model preflight rejected START", ErrMutationPrecondition)
		}
		return nil
	default:
		return fmt.Errorf("%w: unsupported desktop model provider", ErrMutationPrecondition)
	}
}
