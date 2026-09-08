package webai

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/voocel/ainovel-cli/internal/webai/sites"
)

// roundTripWithIdleWatchdog mirrors the owned Gemini browser round trip while
// making the watchdog deadline progress-aware. The idle timer is refreshed by
// observable browser progress (BUSY transitions, rendered user/assistant turn
// counts, or assistant text changes), so long healthy generations are not cut
// merely because their total duration exceeds StallTimeout.
func (t *GeminiWebTransport) roundTripWithIdleWatchdog(ctx context.Context, prompt string, idleTimeout time.Duration) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if strings.TrimSpace(prompt) == "" {
		return "", protocolError("Gemini web round trip", fmt.Errorf("prompt is empty"))
	}

	snap, err := t.ensureReady(ctx)
	if err != nil {
		return "", err
	}
	evaluator, err := t.openWithRetry(ctx, snap)
	if err != nil {
		return "", err
	}
	defer func() {
		if evaluator != nil {
			_ = evaluator.Close()
		}
	}()

	baseline, err := t.adapter.Conversation(ctx, evaluator)
	if err != nil {
		return "", readinessTransportError("read Gemini baseline", err)
	}
	if baseline.Truncated {
		return "", protocolError("read Gemini baseline", fmt.Errorf("existing response exceeds capture limit"))
	}
	if baseline.Busy {
		return "", readinessTransportError("read Gemini baseline", fmt.Errorf("Gemini conversation is still busy"))
	}

	submitErr := t.adapter.Submit(ctx, evaluator, prompt)
	confirmed, confirmErr := t.confirmSubmit(ctx, &evaluator, baseline, submitErr != nil)
	if !confirmed {
		t.session.finishBusy(SessionDegraded, "Gemini SEND ACK missing; prompt delivery is unconfirmed")
		cause := confirmErr
		if cause == nil {
			cause = fmt.Errorf("Gemini UI did not acknowledge the submitted prompt")
		}
		if submitErr != nil {
			cause = errors.Join(submitErr, cause)
		}
		return "", &Error{Kind: ErrorTransport, Op: "confirm Gemini web prompt submit", Cause: cause, Retry: false}
	}
	if err := t.session.beginBusy("Gemini SEND ACK confirmed; response capture started"); err != nil {
		return "", err
	}

	opCtx, cancel := context.WithTimeout(ctx, t.responseTimeout)
	defer cancel()
	final, err := t.captureFinalWithIdleWatchdog(opCtx, ctx, &evaluator, baseline, idleTimeout)
	if err != nil {
		return "", err
	}
	t.session.finishBusy(SessionReady, "Gemini web final response captured")
	return final, nil
}

func (t *GeminiWebTransport) captureFinalWithIdleWatchdog(
	opCtx context.Context,
	parentCtx context.Context,
	evaluator *interactionEvaluator,
	baseline sites.ConversationSnapshot,
	idleTimeout time.Duration,
) (string, error) {
	ticker := time.NewTicker(t.pollInterval)
	defer ticker.Stop()

	var candidate string
	var stableSince time.Time
	reconnects := 0
	lastProgress := time.Now()
	lastBusy := baseline.Busy
	lastResponseCount := baseline.ResponseCount
	lastUserMessageCount := baseline.UserMessageCount
	lastText := strings.TrimSpace(baseline.LastResponse)

	for {
		select {
		case <-opCtx.Done():
			var current interactionEvaluator
			if evaluator != nil {
				current = *evaluator
			}
			clicked := t.bestEffortCancel(current)
			reason := "Gemini web request ended; manual readiness refresh required"
			if clicked {
				reason = "Gemini web Stop requested; readiness refresh required"
			}
			t.session.finishBusy(SessionDegraded, reason)
			if parentCtx.Err() != nil {
				return "", parentCtx.Err()
			}
			return "", &Error{Kind: ErrorTimeout, Op: "wait Gemini web response", Cause: opCtx.Err(), Retry: false}
		case <-ticker.C:
			if evaluator == nil || *evaluator == nil {
				t.session.finishBusy(SessionDegraded, "lost Gemini web response capture")
				return "", &Error{Kind: ErrorTransport, Op: "capture Gemini web response", Cause: fmt.Errorf("Gemini evaluator is unavailable"), Retry: false}
			}
			snapshot, err := t.adapter.Conversation(opCtx, *evaluator)
			if err != nil {
				if reconnects < t.captureReconnects {
					_ = (*evaluator).Close()
					*evaluator = nil
					reconnects++
					next, openErr := t.openWithRetry(opCtx, t.session.Snapshot())
					if openErr == nil {
						*evaluator = next
						lastProgress = time.Now()
						continue
					}
					err = errors.Join(err, openErr)
				}
				t.session.finishBusy(SessionDegraded, "lost Gemini web response capture")
				return "", &Error{Kind: ErrorTransport, Op: "capture Gemini web response", Cause: err, Retry: false}
			}
			if snapshot.Truncated {
				t.bestEffortCancel(*evaluator)
				t.session.finishBusy(SessionDegraded, "Gemini web response exceeded capture limit")
				return "", protocolError("capture Gemini web response", fmt.Errorf("final response exceeds capture limit"))
			}

			text := strings.TrimSpace(snapshot.LastResponse)
			if snapshot.Busy != lastBusy ||
				snapshot.ResponseCount != lastResponseCount ||
				snapshot.UserMessageCount != lastUserMessageCount ||
				text != lastText {
				lastProgress = time.Now()
				lastBusy = snapshot.Busy
				lastResponseCount = snapshot.ResponseCount
				lastUserMessageCount = snapshot.UserMessageCount
				lastText = text
			}

			if idleTimeout > 0 && time.Since(lastProgress) >= idleTimeout {
				t.bestEffortCancel(*evaluator)
				t.session.finishBusy(SessionDegraded, "Gemini web watchdog detected no browser progress")
				return "", &recoverySignal{
					reason: "Gemini browser made no observable progress",
					cause:  fmt.Errorf("no BUSY/turn/text progress for %s", idleTimeout),
				}
			}

			changed := snapshot.ResponseCount > baseline.ResponseCount || (text != "" && text != strings.TrimSpace(baseline.LastResponse))
			if !changed || text == "" || snapshot.Busy {
				candidate = ""
				stableSince = time.Time{}
				continue
			}
			if text != candidate {
				candidate = text
				stableSince = time.Now()
				continue
			}
			if !stableSince.IsZero() && time.Since(stableSince) >= t.stableWindow {
				return candidate, nil
			}
		}
	}
}
