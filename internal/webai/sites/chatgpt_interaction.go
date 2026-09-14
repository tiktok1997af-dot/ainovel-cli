package sites

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	chatGPTSendWait        = 6 * time.Second
	chatGPTSendPoll        = 100 * time.Millisecond
	chatGPTInputSettleWait = 2 * time.Second
	chatGPTInputSettlePoll = 75 * time.Millisecond
)

type chatGPTPromptReadback struct {
	OK             bool   `json:"ok"`
	Reason         string `json:"reason"`
	ComposerLength int    `json:"composer_length"`
	ExpectedLength int    `json:"expected_length"`
	Focused        bool   `json:"focused"`
}

func (ChatGPT) Conversation(ctx context.Context, evaluator Evaluator) (ConversationSnapshot, error) {
	raw, err := evaluator.Eval(ctx, chatGPTConversationExpression)
	if err != nil {
		return ConversationSnapshot{}, err
	}
	var snapshot ConversationSnapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return ConversationSnapshot{}, fmt.Errorf("chatgpt conversation snapshot: %w", err)
	}
	if snapshot.ResponseCount < 0 || snapshot.UserMessageCount < 0 || snapshot.ComposerLength < 0 {
		return ConversationSnapshot{}, fmt.Errorf("chatgpt conversation snapshot contains a negative count")
	}
	if !snapshot.ComposerPresent {
		snapshot.ComposerEmpty = false
		snapshot.ComposerLength = 0
	}
	snapshot.SubmitAction = strings.TrimSpace(snapshot.SubmitAction)
	snapshot.LastResponse = strings.TrimSpace(snapshot.LastResponse)
	return snapshot, nil
}

func (ChatGPT) Submit(ctx context.Context, evaluator Evaluator, prompt string) error {
	if strings.TrimSpace(prompt) == "" {
		return fmt.Errorf("chatgpt submit: prompt is empty")
	}
	input, ok := evaluator.(TextInputEvaluator)
	if !ok {
		return fmt.Errorf("chatgpt submit: evaluator does not support trusted text input")
	}
	encoded, err := json.Marshal(prompt)
	if err != nil {
		return fmt.Errorf("chatgpt submit: encode prompt: %w", err)
	}
	raw, err := evaluator.Eval(ctx, chatGPTResolveComposerExpression)
	if err != nil {
		return err
	}
	var composer struct {
		Found bool    `json:"found"`
		X     float64 `json:"x"`
		Y     float64 `json:"y"`
	}
	if err := json.Unmarshal(raw, &composer); err != nil {
		return fmt.Errorf("chatgpt resolve composer result: %w", err)
	}
	if !composer.Found || composer.X < 0 || composer.Y < 0 {
		return fmt.Errorf("chatgpt submit: prompt composer is unavailable")
	}
	if err := input.ReplaceText(ctx, composer.X, composer.Y, prompt); err != nil {
		return fmt.Errorf("chatgpt trusted composer input: %w", err)
	}
	verifyExpression := fmt.Sprintf(chatGPTVerifyPromptExpressionTemplate, string(encoded))
	prepared, err := waitForChatGPTPromptReadback(ctx, evaluator, verifyExpression)
	if err != nil {
		return err
	}
	if !prepared.OK || prepared.ComposerLength <= 0 {
		reason := strings.TrimSpace(prepared.Reason)
		if reason == "" {
			reason = "prompt composer did not retain trusted input"
		}
		return fmt.Errorf("chatgpt submit: %s after bounded settle (expected_length=%d actual_length=%d focused=%t)", reason, prepared.ExpectedLength, prepared.ComposerLength, prepared.Focused)
	}

	deadline := time.Now().Add(chatGPTSendWait)
	lastReason := "send control is not ready"
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		raw, err := evaluator.Eval(ctx, chatGPTResolveSendExpression)
		if err != nil {
			return err
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
			return fmt.Errorf("chatgpt resolve send result: %w", err)
		}
		if result.OK {
			if result.X < 0 || result.Y < 0 {
				return fmt.Errorf("chatgpt submit: resolved send coordinates are invalid")
			}
			if err := input.Click(ctx, result.X, result.Y); err != nil {
				return fmt.Errorf("chatgpt trusted send click (%s): %w", strings.TrimSpace(result.Action), err)
			}
			return nil
		}
		if reason := strings.TrimSpace(result.Reason); reason != "" {
			lastReason = reason
		}
		if !result.Retry || time.Now().After(deadline) {
			return fmt.Errorf("chatgpt submit: %s", lastReason)
		}
		timer := time.NewTimer(chatGPTSendPoll)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func waitForChatGPTPromptReadback(ctx context.Context, evaluator Evaluator, expression string) (chatGPTPromptReadback, error) {
	deadline := time.Now().Add(chatGPTInputSettleWait)
	var last chatGPTPromptReadback
	for {
		if err := ctx.Err(); err != nil {
			return last, err
		}
		raw, err := evaluator.Eval(ctx, expression)
		if err != nil {
			return last, err
		}
		var current chatGPTPromptReadback
		if err := json.Unmarshal(raw, &current); err != nil {
			return last, fmt.Errorf("chatgpt verify prompt result: %w", err)
		}
		last = current
		if current.OK || time.Now().After(deadline) {
			return current, nil
		}
		timer := time.NewTimer(chatGPTInputSettlePoll)
		select {
		case <-ctx.Done():
			timer.Stop()
			return last, ctx.Err()
		case <-timer.C:
		}
	}
}

func (ChatGPT) Cancel(ctx context.Context, evaluator Evaluator) (bool, error) {
	pointer, ok := evaluator.(PointerEvaluator)
	if !ok {
		return false, fmt.Errorf("chatgpt cancel: evaluator does not support trusted pointer input")
	}
	raw, err := evaluator.Eval(ctx, chatGPTResolveCancelExpression)
	if err != nil {
		return false, err
	}
	var result struct {
		Found bool    `json:"found"`
		X     float64 `json:"x"`
		Y     float64 `json:"y"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return false, fmt.Errorf("chatgpt cancel result: %w", err)
	}
	if !result.Found {
		return false, nil
	}
	if result.X < 0 || result.Y < 0 {
		return false, fmt.Errorf("chatgpt cancel: resolved stop coordinates are invalid")
	}
	if err := pointer.Click(ctx, result.X, result.Y); err != nil {
		return false, fmt.Errorf("chatgpt trusted stop click: %w", err)
	}
	return true, nil
}

const chatGPTConversationExpression = `(() => {
  const visible = (el) => {
    if (!el) return false;
    const style = window.getComputedStyle(el);
    if (style.display === 'none' || style.visibility === 'hidden') return false;
    const rect = el.getBoundingClientRect();
    return rect.width > 0 && rect.height > 0;
  };
  const all = (selectors) => selectors.flatMap((s) => Array.from(document.querySelectorAll(s))).filter(visible);
  const composer = all(['#prompt-textarea','textarea[data-testid*="prompt" i]','[contenteditable="true"][role="textbox"]'])[0] || null;
  const assistants = all(['[data-message-author-role="assistant"]','article[data-testid*="conversation-turn" i] [data-message-author-role="assistant"]']);
  const users = all(['[data-message-author-role="user"]','article[data-testid*="conversation-turn" i] [data-message-author-role="user"]']);
  const stop = all(['button[data-testid="stop-button"]','button[aria-label*="stop" i]'])[0] || null;
  const send = all(['button[data-testid="send-button"]','button[aria-label*="send" i]'])[0] || null;
  const last = assistants.length ? assistants[assistants.length - 1] : null;
  const text = last ? String(last.innerText || last.textContent || '').trim() : '';
  const value = composer ? String(('value' in composer ? composer.value : composer.innerText) || '') : '';
  return {
    busy: Boolean(stop),
    response_count: assistants.length,
    user_message_count: users.length,
    composer_present: Boolean(composer),
    composer_empty: Boolean(composer) && value.length === 0,
    composer_length: value.length,
    submit_action: send ? 'button' : '',
    last_response: text.slice(0, 2000000),
    truncated: text.length > 2000000
  };
})()`

const chatGPTResolveComposerExpression = `(() => {
  const visible = (el) => { if (!el) return false; const s = getComputedStyle(el); const r = el.getBoundingClientRect(); return s.display !== 'none' && s.visibility !== 'hidden' && r.width > 0 && r.height > 0; };
  const selectors = ['#prompt-textarea','textarea[data-testid*="prompt" i]','[contenteditable="true"][role="textbox"]'];
  for (const selector of selectors) for (const el of document.querySelectorAll(selector)) if (visible(el)) { const r = el.getBoundingClientRect(); return {found:true,x:r.left+r.width/2,y:r.top+r.height/2}; }
  return {found:false,x:0,y:0};
})()`

const chatGPTVerifyPromptExpressionTemplate = `(() => {
  const expected = %s;
  const visible = (el) => { if (!el) return false; const s = getComputedStyle(el); const r = el.getBoundingClientRect(); return s.display !== 'none' && s.visibility !== 'hidden' && r.width > 0 && r.height > 0; };
  const selectors = ['#prompt-textarea','textarea[data-testid*="prompt" i]','[contenteditable="true"][role="textbox"]'];
  let composer = null;
  for (const selector of selectors) { composer = Array.from(document.querySelectorAll(selector)).find(visible); if (composer) break; }
  if (!composer) return {ok:false,reason:'composer not found',composer_length:0,expected_length:expected.length,focused:false};
  const actual = String(('value' in composer ? composer.value : composer.innerText) || '');
  return {ok:actual===expected,reason:actual===expected?'':'composer text mismatch',composer_length:actual.length,expected_length:expected.length,focused:document.activeElement===composer};
})()`

const chatGPTResolveSendExpression = `(() => {
  const visible = (el) => { if (!el) return false; const s = getComputedStyle(el); const r = el.getBoundingClientRect(); return s.display !== 'none' && s.visibility !== 'hidden' && r.width > 0 && r.height > 0; };
  const selectors = ['button[data-testid="send-button"]','button[aria-label*="send" i]'];
  for (const selector of selectors) for (const el of document.querySelectorAll(selector)) if (visible(el)) { const disabled = el.disabled || el.getAttribute('aria-disabled') === 'true'; const r = el.getBoundingClientRect(); return {ok:!disabled,retry:disabled,reason:disabled?'send button disabled':'',x:r.left+r.width/2,y:r.top+r.height/2,action:'button'}; }
  return {ok:false,retry:true,reason:'send button not found',x:0,y:0,action:''};
})()`

const chatGPTResolveCancelExpression = `(() => {
  const visible = (el) => { if (!el) return false; const s = getComputedStyle(el); const r = el.getBoundingClientRect(); return s.display !== 'none' && s.visibility !== 'hidden' && r.width > 0 && r.height > 0; };
  const selectors = ['button[data-testid="stop-button"]','button[aria-label*="stop" i]'];
  for (const selector of selectors) for (const el of document.querySelectorAll(selector)) if (visible(el)) { const r = el.getBoundingClientRect(); return {found:true,x:r.left+r.width/2,y:r.top+r.height/2}; }
  return {found:false,x:0,y:0};
})()`
