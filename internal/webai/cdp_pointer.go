package webai

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/voocel/ainovel-cli/internal/webai/sites"
)

var _ sites.PointerEvaluator = (*cdpEvaluator)(nil)

// Click emits one trusted left-button click at CSS viewport coordinates through
// the already-connected loopback Chrome DevTools session. It does not retry a
// pressed/released sequence: any ambiguous CDP failure is returned to the
// transport, which performs only read-only SEND ACK observation afterwards.
func (e *cdpEvaluator) Click(ctx context.Context, x, y float64) error {
	if e == nil || e.conn == nil {
		return errors.New("CDP evaluator is closed")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if math.IsNaN(x) || math.IsNaN(y) || math.IsInf(x, 0) || math.IsInf(y, 0) || x < 0 || y < 0 {
		return fmt.Errorf("invalid pointer coordinates %.2f,%.2f", x, y)
	}

	common := map[string]any{
		"x":           x,
		"y":           y,
		"button":      "left",
		"clickCount":  1,
		"pointerType": "mouse",
	}
	pressed := clonePointerParams(common)
	pressed["type"] = "mousePressed"
	pressed["buttons"] = 1
	if err := e.callCDPNoResult(ctx, "Input.dispatchMouseEvent", pressed); err != nil {
		return fmt.Errorf("CDP pointer press: %w", err)
	}
	released := clonePointerParams(common)
	released["type"] = "mouseReleased"
	released["buttons"] = 0
	if err := e.callCDPNoResult(ctx, "Input.dispatchMouseEvent", released); err != nil {
		return fmt.Errorf("CDP pointer release: %w", err)
	}
	return nil
}

func clonePointerParams(src map[string]any) map[string]any {
	out := make(map[string]any, len(src)+2)
	for key, value := range src {
		out[key] = value
	}
	return out
}

func (e *cdpEvaluator) callCDPNoResult(ctx context.Context, method string, params map[string]any) error {
	deadline := time.Now().Add(cdpEvaluationTimeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err := e.conn.SetWriteDeadline(deadline); err != nil {
		return err
	}
	if err := e.conn.SetReadDeadline(deadline); err != nil {
		return err
	}

	id := e.nextID.Add(1)
	if err := e.conn.WriteJSON(map[string]any{
		"id":     id,
		"method": method,
		"params": params,
	}); err != nil {
		return err
	}

	for {
		var response struct {
			ID    int64 `json:"id"`
			Error *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := e.conn.ReadJSON(&response); err != nil {
			return err
		}
		if response.ID != id {
			continue
		}
		if response.Error != nil {
			return fmt.Errorf("CDP error %d: %s", response.Error.Code, response.Error.Message)
		}
		return nil
	}
}
