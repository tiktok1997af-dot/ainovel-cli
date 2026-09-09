package webai

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/voocel/ainovel-cli/internal/webai/sites"
)

const pendingComposerAckDiagnostic = "Gemini SEND ACK missing: prompt remained in the composer"

type pendingSubmitRetrier interface {
	RetryPendingSubmit(ctx context.Context, evaluator sites.Evaluator, prompt string) (bool, error)
}

// pendingComposerSubmitFailure is intentionally strict. Recovery is allowed
// only for the exact fail-closed case where the original Submit returned nil,
// read-only ACK confirmation completed without another joined transport error,
// and the prompt remained in the composer. Ambiguous submit/renderer failures
// must never enter this path because a second Send could duplicate a request.
func pendingComposerSubmitFailure(err error) bool {
	var webErr *Error
	if !errors.As(err, &webErr) || webErr == nil {
		return false
	}
	if webErr.Kind != ErrorTransport || webErr.Op != "confirm Gemini web prompt submit" || webErr.Cause == nil {
		return false
	}
	return strings.TrimSpace(webErr.Cause.Error()) == pendingComposerAckDiagnostic
}

func pendingComposerSnapshotStable(a, b sites.ConversationSnapshot) bool {
	if a.Truncated || b.Truncated || a.Busy || b.Busy {
		return false
	}
	if !a.ComposerPresent || a.ComposerEmpty || !b.ComposerPresent || b.ComposerEmpty {
		return false
	}
	return a.ResponseCount == b.ResponseCount &&
		a.UserMessageCount == b.UserMessageCount &&
		strings.TrimSpace(a.LastResponse) == strings.TrimSpace(b.LastResponse)
}

// recoverPendingComposerSubmit performs at most one trusted recovery Send click
// without retyping/replaying the prompt. It is called only after the exact
// pending-composer SEND ACK failure above. Two read-only snapshots must remain
// stable before the site adapter is allowed to verify the exact prompt and
// click Send once more.
func (t *GeminiWebTransport) recoverPendingComposerSubmit(ctx context.Context, prompt string, idleTimeout time.Duration) (string, bool, error) {
	retrier, ok := t.adapter.(pendingSubmitRetrier)
	if !ok {
		return "", false, nil
	}

	snap, err := t.ensureReady(ctx)
	if err != nil {
		return "", false, err
	}
	evaluator, err := t.openWithRetry(ctx, snap)
	if err != nil {
		return "", false, err
	}
	defer func() { _ = evaluator.Close() }()

	first, err := t.adapter.Conversation(ctx, evaluator)
	if err != nil {
		return "", false, readinessTransportError("inspect pending Gemini submit", err)
	}
	guardDelay := 2 * t.submitConfirmPollInterval
	if guardDelay <= 0 {
		guardDelay = 300 * time.Millisecond
	}
	if err := waitContext(ctx, guardDelay); err != nil {
		return "", false, err
	}
	second, err := t.adapter.Conversation(ctx, evaluator)
	if err != nil {
		return "", false, readinessTransportError("recheck pending Gemini submit", err)
	}
	if !pendingComposerSnapshotStable(first, second) {
		return "", false, nil
	}

	clicked, err := retrier.RetryPendingSubmit(ctx, evaluator, prompt)
	if err != nil {
		return "", true, &Error{Kind: ErrorTransport, Op: "recover pending Gemini prompt submit", Cause: err, Retry: false}
	}
	if !clicked {
		return "", false, nil
	}

	confirmed, confirmErr := t.confirmSubmit(ctx, &evaluator, second, false)
	if !confirmed {
		t.session.finishBusy(SessionDegraded, "Gemini recovery Send click was not acknowledged")
		if confirmErr == nil {
			confirmErr = fmt.Errorf("Gemini recovery Send click did not produce a SEND ACK")
		}
		return "", true, &Error{Kind: ErrorTransport, Op: "confirm recovered Gemini prompt submit", Cause: confirmErr, Retry: false}
	}
	if err := t.session.beginBusy("Gemini recovery SEND ACK confirmed; response capture started"); err != nil {
		return "", true, err
	}

	opCtx, cancel := context.WithTimeout(ctx, t.responseTimeout)
	defer cancel()
	var final string
	if idleTimeout > 0 {
		final, err = t.captureFinalWithIdleWatchdog(opCtx, ctx, &evaluator, second, idleTimeout)
	} else {
		final, err = t.captureFinal(opCtx, ctx, &evaluator, second)
	}
	if err != nil {
		return "", true, err
	}
	t.session.finishBusy(SessionReady, "Gemini web final response captured after pending-submit recovery")
	return final, true, nil
}
