package sites

import (
	"context"
	"encoding/json"
)

// Readiness is a site-local verdict that the parent webai package maps onto
// its browser session state machine.
type Readiness string

const (
	ReadinessAuthRequired Readiness = "auth_required"
	ReadinessReady        Readiness = "ready"
	ReadinessDegraded     Readiness = "degraded"
	ReadinessFailed       Readiness = "failed"
)

// Result contains only operational readiness facts. Site adapters must not
// return cookies, tokens, account identifiers, local-storage values or other
// authentication secrets.
type Result struct {
	State  Readiness
	Reason string
}

// Evaluator evaluates a JavaScript expression in the selected visible page and
// returns the JSON value produced by Runtime.evaluate. Readiness adapters use it
// read-only; interaction adapters may use narrowly scoped DOM preparation.
type Evaluator interface {
	Eval(ctx context.Context, expression string) (json.RawMessage, error)
}

// PointerEvaluator is an optional trusted-input capability exposed by the local
// Chrome DevTools evaluator. Coordinates are CSS viewport pixels previously
// resolved from the same visible page. Interaction adapters use it to perform
// one real browser pointer action instead of relying on synthetic element.click.
type PointerEvaluator interface {
	Evaluator
	Click(ctx context.Context, x, y float64) error
}

// Adapter contains site-specific target selection and readiness detection.
// Prompt submission/capture is deliberately outside this W2 contract.
type Adapter interface {
	Name() string
	TargetScore(rawURL string) int
	Probe(ctx context.Context, evaluator Evaluator) (Result, error)
}
