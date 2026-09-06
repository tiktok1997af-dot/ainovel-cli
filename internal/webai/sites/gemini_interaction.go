package sites

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	geminiSendWait = 6 * time.Second
	geminiSendPoll = 100 * time.Millisecond
)

func (Gemini) Conversation(ctx context.Context, evaluator Evaluator) (ConversationSnapshot, error) {
	raw, err := evaluator.Eval(ctx, geminiConversationExpression)
	if err != nil {
		return ConversationSnapshot{}, err
	}
	var snapshot ConversationSnapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return ConversationSnapshot{}, fmt.Errorf("gemini conversation snapshot: %w", err)
	}
	if snapshot.ResponseCount < 0 {
		return ConversationSnapshot{}, fmt.Errorf("gemini conversation snapshot: negative response count")
	}
	if snapshot.UserMessageCount < 0 {
		return ConversationSnapshot{}, fmt.Errorf("gemini conversation snapshot: negative user message count")
	}
	if !snapshot.ComposerPresent {
		snapshot.ComposerEmpty = false
	}
	snapshot.LastResponse = strings.TrimSpace(snapshot.LastResponse)
	return snapshot, nil
}

func (Gemini) Submit(ctx context.Context, evaluator Evaluator, prompt string) error {
	if strings.TrimSpace(prompt) == "" {
		return fmt.Errorf("gemini submit: prompt is empty")
	}
	encoded, err := json.Marshal(prompt)
	if err != nil {
		return fmt.Errorf("gemini submit: encode prompt: %w", err)
	}

	// Keep the side-effecting submit path synchronous from CDP's perspective.
	// Poll readiness from Go, then execute exactly one short synchronous click.
	// A click is not considered delivery acknowledgement; the transport performs
	// a separate read-only SEND ACK phase after this method returns.
	prepareExpression := fmt.Sprintf(geminiPreparePromptExpressionTemplate, string(encoded))
	raw, err := evaluator.Eval(ctx, prepareExpression)
	if err != nil {
		return err
	}
	var prepared struct {
		OK     bool   `json:"ok"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(raw, &prepared); err != nil {
		return fmt.Errorf("gemini prepare prompt result: %w", err)
	}
	if !prepared.OK {
		reason := strings.TrimSpace(prepared.Reason)
		if reason == "" {
			reason = "prompt composer is not ready"
		}
		return fmt.Errorf("gemini submit: %s", reason)
	}

	deadline := time.Now().Add(geminiSendWait)
	lastReason := "send control is not ready"
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		raw, err := evaluator.Eval(ctx, geminiClickSendExpression)
		if err != nil {
			return err
		}
		var result struct {
			OK     bool   `json:"ok"`
			Retry  bool   `json:"retry"`
			Reason string `json:"reason"`
		}
		if err := json.Unmarshal(raw, &result); err != nil {
			return fmt.Errorf("gemini click send result: %w", err)
		}
		if result.OK {
			return nil
		}
		if reason := strings.TrimSpace(result.Reason); reason != "" {
			lastReason = reason
		}
		if !result.Retry {
			return fmt.Errorf("gemini submit: %s", lastReason)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("gemini submit: %s after bounded wait", lastReason)
		}

		timer := time.NewTimer(geminiSendPoll)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (Gemini) Cancel(ctx context.Context, evaluator Evaluator) (bool, error) {
	raw, err := evaluator.Eval(ctx, geminiCancelExpression)
	if err != nil {
		return false, err
	}
	var result struct {
		Clicked bool `json:"clicked"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return false, fmt.Errorf("gemini cancel result: %w", err)
	}
	return result.Clicked, nil
}

const geminiConversationExpression = `(() => {
  const visible = (el) => {
    if (!el) return false;
    const style = window.getComputedStyle(el);
    if (style.display === 'none' || style.visibility === 'hidden') return false;
    const rect = el.getBoundingClientRect();
    return rect.width > 0 && rect.height > 0;
  };
  const firstVisible = (selectors) => {
    for (const selector of selectors) {
      for (const el of document.querySelectorAll(selector)) {
        if (visible(el)) return el;
      }
    }
    return null;
  };

  const stopSelectors = [
    'button[aria-label*="Stop response" i]',
    'button[aria-label*="Stop generating" i]',
    'button[aria-label*="Dừng" i]',
    'gem-icon-button[aria-label*="Stop" i]',
    'gem-icon-button[aria-label*="Dừng" i]',
    '[data-test-id="stop-button"]',
    'button.stop-button'
  ];
  let busy = stopSelectors.some((selector) => Array.from(document.querySelectorAll(selector)).some(visible));
  if (!busy) {
    for (const control of document.querySelectorAll('button, gem-icon-button, [role="button"]')) {
      if (!visible(control)) continue;
      const icon = control.querySelector('mat-icon');
      const iconText = String(icon && (icon.getAttribute('data-mat-icon-name') || icon.getAttribute('fonticon') || icon.textContent) || '').trim().toLowerCase();
      if (iconText === 'stop' || iconText === 'stop_circle') {
        busy = true;
        break;
      }
    }
  }

  const responseRoots = Array.from(document.querySelectorAll(
    'model-response, [data-test-id="model-response"], .model-response'
  )).filter(visible);
  const texts = [];
  for (const root of responseRoots) {
    const body = root.querySelector(
      'message-content, .model-response-text, .markdown, [class*="response-text"]'
    ) || root;
    const text = String(body.innerText || body.textContent || '').trim();
    if (text) texts.push(text);
  }

  // Count rendered user turns without returning their text. Gemini has used
  // user-query as the stable custom element; the fallbacks cover current test-id
  // and role-based variants. A Set prevents the same element from being counted
  // twice when more than one selector matches it.
  const userElements = new Set();
  for (const selector of [
    'user-query',
    '[data-test-id="user-query"]',
    '.user-query',
    '[data-message-author-role="user"]'
  ]) {
    for (const el of document.querySelectorAll(selector)) {
      if (visible(el)) userElements.add(el);
    }
  }
  let userMessageCount = 0;
  for (const root of userElements) {
    const text = String(root.innerText || root.textContent || '').trim();
    if (text) userMessageCount++;
  }

  const composer = firstVisible([
    'rich-textarea .ql-editor[contenteditable="true"]',
    'rich-textarea [contenteditable="true"]',
    'div.ql-editor[contenteditable="true"]',
    '[contenteditable="true"][role="textbox"]',
    '[aria-label="Enter a prompt here"]',
    'textarea[aria-label*="prompt" i]'
  ]);
  const composerText = composer
    ? String((composer instanceof HTMLTextAreaElement || composer instanceof HTMLInputElement) ? composer.value : (composer.innerText || composer.textContent || '')).trim()
    : '';

  const last = texts.length ? texts[texts.length - 1] : '';
  const max = 1048576;
  return {
    busy,
    response_count: texts.length,
    user_message_count: userMessageCount,
    composer_present: Boolean(composer),
    composer_empty: Boolean(composer) && composerText.length === 0,
    last_response: last.length > max ? last.slice(0, max) : last,
    truncated: last.length > max
  };
})()`

const geminiPreparePromptExpressionTemplate = `(() => {
  const prompt = %s;
  const visible = (el) => {
    if (!el) return false;
    const style = window.getComputedStyle(el);
    if (style.display === 'none' || style.visibility === 'hidden') return false;
    const rect = el.getBoundingClientRect();
    return rect.width > 0 && rect.height > 0;
  };
  const firstVisible = (selectors) => {
    for (const selector of selectors) {
      for (const el of document.querySelectorAll(selector)) {
        if (visible(el)) return el;
      }
    }
    return null;
  };

  const composer = firstVisible([
    'rich-textarea .ql-editor[contenteditable="true"]',
    'rich-textarea [contenteditable="true"]',
    'div.ql-editor[contenteditable="true"]',
    '[contenteditable="true"][role="textbox"]',
    '[aria-label="Enter a prompt here"]',
    'textarea[aria-label*="prompt" i]'
  ]);
  if (!composer) return {ok: false, reason: 'prompt composer not found'};
  composer.focus();

  if (composer instanceof HTMLTextAreaElement || composer instanceof HTMLInputElement) {
    const proto = composer instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
    const setter = Object.getOwnPropertyDescriptor(proto, 'value')?.set;
    if (setter) setter.call(composer, prompt); else composer.value = prompt;
    composer.dispatchEvent(new InputEvent('input', {bubbles: true, inputType: 'insertText', data: prompt}));
    composer.dispatchEvent(new Event('change', {bubbles: true}));
  } else {
    const selection = window.getSelection();
    const range = document.createRange();
    range.selectNodeContents(composer);
    selection.removeAllRanges();
    selection.addRange(range);
    let inserted = false;
    try { inserted = document.execCommand('insertText', false, prompt); } catch (_) {}
    if (!inserted || String(composer.innerText || '').trim() !== String(prompt).trim()) {
      composer.textContent = prompt;
      composer.dispatchEvent(new InputEvent('input', {bubbles: true, inputType: 'insertText', data: prompt}));
    }
    composer.dispatchEvent(new Event('change', {bubbles: true}));
  }
  return {ok: true, reason: ''};
})()`

const geminiClickSendExpression = `(() => {
  const visible = (el) => {
    if (!el) return false;
    const style = window.getComputedStyle(el);
    if (style.display === 'none' || style.visibility === 'hidden') return false;
    const rect = el.getBoundingClientRect();
    return rect.width > 0 && rect.height > 0;
  };
  const firstVisible = (selectors) => {
    for (const selector of selectors) {
      for (const el of document.querySelectorAll(selector)) {
        if (visible(el)) return el;
      }
    }
    return null;
  };
  const sendSelectors = [
    'gem-icon-button.send-button[aria-disabled="false"]',
    'gem-icon-button.submit[aria-disabled="false"]',
    '[data-test-id="send-button-container"] gem-icon-button',
    '[data-test-id="send-button-container"] button',
    '[data-test-id="send-button"]',
    'button[aria-label*="Send message" i]',
    'button[aria-label="Send"]',
    'button[aria-label*="Submit" i]',
    'button[aria-label*="Gửi" i]',
    'gem-icon-button[aria-label*="Send" i]',
    'gem-icon-button[aria-label*="Submit" i]',
    'gem-icon-button[aria-label*="Gửi" i]',
    '[role="button"][aria-label*="Send" i]',
    '[role="button"][aria-label*="Submit" i]',
    '[role="button"][aria-label*="Gửi" i]',
    'button.send-button'
  ];
  const findSend = () => {
    const direct = firstVisible(sendSelectors);
    if (direct) return direct;
    for (const control of document.querySelectorAll('button, gem-icon-button, [role="button"]')) {
      if (!visible(control)) continue;
      const aria = String(control.getAttribute('aria-label') || '').trim().toLowerCase();
      if (aria.includes('send') || aria.includes('submit') || aria.includes('gửi')) return control;
      const icon = control.querySelector('mat-icon');
      const iconText = String(icon && (icon.getAttribute('data-mat-icon-name') || icon.getAttribute('fonticon') || icon.textContent) || '').trim().toLowerCase();
      if (iconText === 'send' || iconText === 'send_spark' || iconText === 'arrow_upward') return control;
    }
    return null;
  };

  const sendButton = findSend();
  if (!sendButton) return {ok: false, retry: true, reason: 'send control not found'};
  const disabled = Boolean(sendButton.disabled) || sendButton.hasAttribute('disabled') || sendButton.getAttribute('aria-disabled') === 'true';
  if (disabled) return {ok: false, retry: true, reason: 'send control is disabled'};
  sendButton.click();
  return {ok: true, retry: false, reason: ''};
})()`

const geminiCancelExpression = `(() => {
  const visible = (el) => {
    if (!el) return false;
    const style = window.getComputedStyle(el);
    if (style.display === 'none' || style.visibility === 'hidden') return false;
    const rect = el.getBoundingClientRect();
    return rect.width > 0 && rect.height > 0;
  };
  const selectors = [
    'button[aria-label*="Stop response" i]',
    'button[aria-label*="Stop generating" i]',
    'button[aria-label*="Dừng" i]',
    'gem-icon-button[aria-label*="Stop" i]',
    'gem-icon-button[aria-label*="Dừng" i]',
    '[data-test-id="stop-button"]',
    'button.stop-button'
  ];
  for (const selector of selectors) {
    for (const button of document.querySelectorAll(selector)) {
      if (!visible(button)) continue;
      button.click();
      return {clicked: true};
    }
  }
  for (const control of document.querySelectorAll('button, gem-icon-button, [role="button"]')) {
    if (!visible(control)) continue;
    const icon = control.querySelector('mat-icon');
    const iconText = String(icon && (icon.getAttribute('data-mat-icon-name') || icon.getAttribute('fonticon') || icon.textContent) || '').trim().toLowerCase();
    if (iconText === 'stop' || iconText === 'stop_circle') {
      control.click();
      return {clicked: true};
    }
  }
  return {clicked: false};
})()`
