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
	OK                 bool   `json:"ok"`
	Reason             string `json:"reason"`
	ComposerLength     int    `json:"composer_length"`
	ExpectedLength     int    `json:"expected_length"`
	ComposerLineBreaks int    `json:"composer_line_breaks"`
	ExpectedLineBreaks int    `json:"expected_line_breaks"`
	ComposerKind       string `json:"composer_kind"`
	Focused            bool   `json:"focused"`
}

type chatGPTEnterEvaluator interface {
	PressEnter(context.Context) error
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
		Kind  string  `json:"kind"`
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
	if !prepared.OK {
		reason := strings.TrimSpace(prepared.Reason)
		if reason == "" {
			reason = "prompt composer did not retain trusted input"
		}
		kind := strings.TrimSpace(prepared.ComposerKind)
		if kind == "" {
			kind = strings.TrimSpace(composer.Kind)
		}
		if kind == "" {
			kind = "unknown"
		}
		return fmt.Errorf(
			"chatgpt submit: %s after bounded settle (expected_length=%d actual_length=%d expected_line_breaks=%d actual_line_breaks=%d composer_kind=%s focused=%t)",
			reason,
			prepared.ExpectedLength,
			prepared.ComposerLength,
			prepared.ExpectedLineBreaks,
			prepared.ComposerLineBreaks,
			kind,
			prepared.Focused,
		)
	}
	if prepared.ComposerLength <= 0 {
		return fmt.Errorf("chatgpt submit: verified composer is empty")
	}

	return submitVerifiedChatGPTPrompt(ctx, evaluator, input, chatGPTSendWait)
}

func submitVerifiedChatGPTPrompt(ctx context.Context, evaluator Evaluator, input TextInputEvaluator, wait time.Duration) error {
	deadline := time.Now().Add(wait)
	lastReason := "send control is not ready"
	sawExplicitSend := false
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
			Found  bool    `json:"found"`
			Reason string  `json:"reason"`
			X      float64 `json:"x"`
			Y      float64 `json:"y"`
			Action string  `json:"action"`
		}
		if err := json.Unmarshal(raw, &result); err != nil {
			return fmt.Errorf("chatgpt resolve send result: %w", err)
		}
		if result.Found {
			sawExplicitSend = true
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
		if !result.Retry || !time.Now().Before(deadline) {
			if !sawExplicitSend {
				enter, ok := evaluator.(chatGPTEnterEvaluator)
				if !ok {
					return fmt.Errorf("chatgpt submit: %s; evaluator does not support trusted Enter fallback", lastReason)
				}
				if err := enter.PressEnter(ctx); err != nil {
					return fmt.Errorf("chatgpt trusted Enter fallback: %w", err)
				}
				return nil
			}
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
		current.ComposerKind = strings.TrimSpace(current.ComposerKind)
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
  const visible = (el) => {
    if (!el) return false;
    const s = getComputedStyle(el);
    const r = el.getBoundingClientRect();
    return s.display !== 'none' && s.visibility !== 'hidden' && r.width > 0 && r.height > 0 && r.bottom > 0 && r.right > 0 && r.top < window.innerHeight && r.left < window.innerWidth;
  };
  const kindOf = (composer) => {
    if (composer instanceof HTMLTextAreaElement) return 'textarea';
    if (composer instanceof HTMLInputElement) return 'input';
    if (composer.classList && composer.classList.contains('ProseMirror')) return 'prosemirror';
    if (composer.getAttribute('contenteditable') === 'true') return 'contenteditable';
    return 'other';
  };
  const selectors = ['#prompt-textarea','textarea[data-testid*="prompt" i]','div.ProseMirror[contenteditable="true"]','[contenteditable="true"][role="textbox"]'];
  for (const selector of selectors) {
    for (const el of document.querySelectorAll(selector)) {
      if (!visible(el)) continue;
      const r = el.getBoundingClientRect();
      const maxX = Math.max(0, window.innerWidth - 1);
      const maxY = Math.max(0, window.innerHeight - 1);
      return {
        found: true,
        x: Math.min(maxX, Math.max(0, r.left + Math.min(r.width / 2, 48))),
        y: Math.min(maxY, Math.max(0, r.top + r.height / 2)),
        kind: kindOf(el)
      };
    }
  }
  return {found:false,x:0,y:0,kind:'missing'};
})()`

const chatGPTVerifyPromptExpressionTemplate = `(() => {
  const prompt = %s;
  const visible = (el) => {
    if (!el) return false;
    const s = getComputedStyle(el);
    const r = el.getBoundingClientRect();
    return s.display !== 'none' && s.visibility !== 'hidden' && r.width > 0 && r.height > 0;
  };
  const firstVisible = (selectors) => {
    for (const selector of selectors) {
      for (const el of document.querySelectorAll(selector)) {
        if (visible(el)) return el;
      }
    }
    return null;
  };
  const kindOf = (composer) => {
    if (composer instanceof HTMLTextAreaElement) return 'textarea';
    if (composer instanceof HTMLInputElement) return 'input';
    if (composer.classList && composer.classList.contains('ProseMirror')) return 'prosemirror';
    if (composer.getAttribute('contenteditable') === 'true') return 'contenteditable';
    return 'other';
  };
  const normalize = (value) => String(value || '')
    .replace(/\r\n/g, '\n')
    .replace(/\r/g, '\n')
    .replace(/\u00a0/g, ' ')
    .replace(/[\u200b\ufeff]/g, '')
    .trim();
  const lineBreakCount = (value) => (String(value || '').match(/\n/g) || []).length;
  const readCandidates = (composer) => {
    if (composer instanceof HTMLTextAreaElement || composer instanceof HTMLInputElement) {
      return [String(composer.value || '')];
    }
    const candidates = [];
    const add = (value) => {
      const raw = String(value || '');
      if (raw !== '' && !candidates.includes(raw)) candidates.push(raw);
    };
    add(composer.innerText);
    add(composer.textContent);
    if (composer.children && composer.children.length > 0) {
      const blocks = Array.from(composer.children);
      const blockTags = new Set(['P','DIV','LI','PRE','BLOCKQUOTE']);
      const separator = blocks.some((block) => blockTags.has(String(block.tagName || '').toUpperCase())) ? '\n' : '';
      add(blocks.map((block) => String(block.innerText || block.textContent || '')).join(separator));
      add(blocks.map((block) => String(block.textContent || '')).join(separator));
    }
    return candidates;
  };
  const expected = normalize(prompt);
  const composer = firstVisible(['#prompt-textarea','textarea[data-testid*="prompt" i]','div.ProseMirror[contenteditable="true"]','[contenteditable="true"][role="textbox"]']);
  if (!composer) return {
    ok:false,
    reason:'composer not found',
    composer_length:0,
    expected_length:expected.length,
    composer_line_breaks:0,
    expected_line_breaks:lineBreakCount(expected),
    composer_kind:'missing',
    focused:false
  };
  const candidates = readCandidates(composer).map(normalize);
  const matched = candidates.find((value) => value === expected) || '';
  const diagnostic = matched || candidates.reduce((best, value) => {
    if (best === '') return value;
    return Math.abs(value.length - expected.length) < Math.abs(best.length - expected.length) ? value : best;
  }, '');
  const actual = matched || diagnostic;
  const active = document.activeElement;
  const focused = Boolean(active && (active === composer || composer.contains(active) || (active.shadowRoot && active.shadowRoot.activeElement === composer)));
  const composerKind = kindOf(composer);
  const composerLength = actual.length;
  const composerLineBreaks = lineBreakCount(actual);
  const expectedLineBreaks = lineBreakCount(expected);
  if (actual !== expected) return {
    ok:false,
    reason:'composer text mismatch',
    composer_length:composerLength,
    expected_length:expected.length,
    composer_line_breaks:composerLineBreaks,
    expected_line_breaks:expectedLineBreaks,
    composer_kind:composerKind,
    focused
  };
  if (actual.length === 0) return {
    ok:false,
    reason:'composer is empty after trusted input',
    composer_length:0,
    expected_length:expected.length,
    composer_line_breaks:0,
    expected_line_breaks:expectedLineBreaks,
    composer_kind:composerKind,
    focused
  };
  return {
    ok:true,
    reason:'',
    composer_length:actual.length,
    expected_length:expected.length,
    composer_line_breaks:lineBreakCount(actual),
    expected_line_breaks:expectedLineBreaks,
    composer_kind:composerKind,
    focused
  };
})()`

const chatGPTResolveSendExpression = `(() => {
  const visible = (el) => {
    if (!el) return false;
    const s = getComputedStyle(el);
    const r = el.getBoundingClientRect();
    return s.display !== 'none' && s.visibility !== 'hidden' && r.width > 0 && r.height > 0 && r.bottom > 0 && r.right > 0 && r.top < window.innerHeight && r.left < window.innerWidth;
  };
  const selectors = [
    'button[data-testid="send-button"]',
    'button[data-testid*="send" i]',
    'button[aria-label*="send" i]',
    'button[aria-label*="submit" i]',
    'button[aria-label*="gửi" i]',
    'button[title*="send" i]',
    'button[title*="gửi" i]'
  ];
  for (const selector of selectors) {
    for (const el of document.querySelectorAll(selector)) {
      if (!visible(el)) continue;
      const disabled = el.disabled || el.hasAttribute('disabled') || el.getAttribute('aria-disabled') === 'true';
      const r = el.getBoundingClientRect();
      return {
        ok: !disabled,
        retry: disabled,
        found: true,
        reason: disabled ? 'send button disabled' : '',
        x: r.left + r.width / 2,
        y: r.top + r.height / 2,
        action: 'button'
      };
    }
  }
  return {ok:false,retry:true,found:false,reason:'send button not found',x:0,y:0,action:''};
})()`

const chatGPTResolveCancelExpression = `(() => {
  const visible = (el) => { if (!el) return false; const s = getComputedStyle(el); const r = el.getBoundingClientRect(); return s.display !== 'none' && s.visibility !== 'hidden' && r.width > 0 && r.height > 0; };
  const selectors = ['button[data-testid="stop-button"]','button[aria-label*="stop" i]'];
  for (const selector of selectors) for (const el of document.querySelectorAll(selector)) if (visible(el)) { const r = el.getBoundingClientRect(); return {found:true,x:r.left+r.width/2,y:r.top+r.height/2}; }
  return {found:false,x:0,y:0};
})()`
