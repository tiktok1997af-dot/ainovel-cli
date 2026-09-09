package sites

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// RetryPendingSubmit performs one bounded recovery click for a prompt that is
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

	// Restore focus without replacing any text. This is a benign pointer action
	// and helps when Gemini enabled Send but the first click was dropped while the
	// controlled editor was still settling.
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
	// recovery Send click.
	prepared, err = waitForGeminiPromptReadback(ctx, evaluator, verifyExpression)
	if err != nil {
		return false, err
	}
	if !prepared.OK || prepared.ComposerLength <= 0 {
		return false, nil
	}

	raw, err = evaluator.Eval(ctx, geminiResolveSendExpression)
	if err != nil {
		return false, err
	}
	var result struct {
		OK     bool    `json:"ok"`
		Retry  bool    `json:"retry"`
		Reason string  `json:"reason"`
		X      float64 `json:"x"`
		Y      float64 `json:"y"`
		Action string  `json:"action"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return false, fmt.Errorf("gemini pending submit recovery resolve send: %w", err)
	}
	if !result.OK {
		if result.Retry {
			return false, nil
		}
		reason := strings.TrimSpace(result.Reason)
		if reason == "" {
			reason = "send action is unavailable"
		}
		return false, fmt.Errorf("gemini pending submit recovery: %s", reason)
	}
	if result.X < 0 || result.Y < 0 {
		return false, fmt.Errorf("gemini pending submit recovery: resolved send coordinates are invalid")
	}
	if err := input.Click(ctx, result.X, result.Y); err != nil {
		return false, fmt.Errorf("gemini pending submit recovery trusted send click (%s): %w", strings.TrimSpace(result.Action), err)
	}
	return true, nil
}
