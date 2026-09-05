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

// Evaluator evaluates a read-only JavaScript expression in the selected page
// and returns the JSON value produced by Runtime.evaluate.
type Evaluator interface {
	Eval(ctx context.Context, expression string) (json.RawMessage, error)
}

// Adapter contains site-specific target selection and readiness detection.
// Prompt submission/capture is deliberately outside this W2 contract.
type Adapter interface {
	Name() string
	TargetScore(rawURL string) int
	Probe(ctx context.Context, evaluator Evaluator) (Result, error)
}
