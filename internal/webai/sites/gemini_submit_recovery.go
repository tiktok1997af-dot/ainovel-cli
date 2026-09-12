package sites

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type trustedSubmitKeyEvaluator interface {
	PressEnter(ctx context.Context) error
}

// RetryPendingSubmit performs one bounded recovery submit for a prompt that is
// still present verbatim in Gemini's composer after an earlier Send click was
// not acknowledged. It deliberately never rewrites/reinserts the prompt. The
// transport must independently prove that no user turn/BUSY/response progress
// occurred before calling this method.
func (Gemini) RetryPendingSubmit(ctx context.Context, evaluator Evaluator, prompt string) (bool, error) {
	if strings.TrimSpace(prompt) == "" {
		return false, fmt.Errorf("gemini pending submit recovery: prompt is empty")
	}
	input, ok := evaluator.(TextInputEvaluator)
	if !ok {
		return false, fmt.Errorf("gemini pending submit recovery: evaluator does not support trusted pointer input")
	}
	submitKey, ok := evaluator.(trustedSubmitKeyEvaluator)
	if !ok {
		return false, fmt.Errorf("gemini pending submit recovery: evaluator does not support trusted Enter submit")
	}

	encoded, err := json.Marshal(prompt)
	if err != nil {
		return false, fmt.Errorf("gemini pending submit recovery: encode prompt: %w", err)
	}
	verifyExpression := fmt.Sprintf(geminiVerifyPromptExpressionTemplate, string(encoded))

	// Fail closed unless the composer still contains the exact original prompt.
	prepared, err := waitForGeminiPromptReadback(ctx, evaluator, verifyExpression)
	if err != nil {
		return false, err
	}
	if !prepared.OK || prepared.ComposerLength <= 0 {
		return false, nil
	}

	// Restore focus without replacing any text. The original pointer Send action
	// has already failed its bounded ACK check; this benign focus action prepares
	// an independent trusted-key recovery path without replaying prompt content.
	raw, err := evaluator.Eval(ctx, geminiResolveComposerExpression)
	if err != nil {
		return false, err
	}
	var composer struct {
		Found bool    `json:"found"`
		X     float64 `json:"x"`
		Y     float64 `json:"y"`
	}
	if err := json.Unmarshal(raw, &composer); err != nil {
		return false, fmt.Errorf("gemini pending submit recovery composer: %w", err)
	}
	if !composer.Found || composer.X < 0 || composer.Y < 0 {
		return false, nil
	}
	if err := input.Click(ctx, composer.X, composer.Y); err != nil {
		return false, fmt.Errorf("gemini pending submit recovery composer focus: %w", err)
	}

	// Re-check exact prompt retention after refocus, immediately before the only
	// recovery submit side effect. The recovery uses one trusted Enter key event
	// instead of repeating the same Send-button click that just failed ACK.
	prepared, err = waitForGeminiPromptReadback(ctx, evaluator, verifyExpression)
	if err != nil {
		return false, err
	}
	if !prepared.OK || prepared.ComposerLength <= 0 {
		return false, nil
	}
	if err := submitKey.PressEnter(ctx); err != nil {
		return false, fmt.Errorf("gemini pending submit recovery trusted Enter submit: %w", err)
	}
	return true, nil
}
