package sites

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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
	expression := fmt.Sprintf(geminiSubmitExpressionTemplate, string(encoded))
	raw, err := evaluator.Eval(ctx, expression)
	if err != nil {
		return err
	}
	var result struct {
		OK     bool   `json:"ok"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return fmt.Errorf("gemini submit result: %w", err)
	}
	if !result.OK {
		reason := strings.TrimSpace(result.Reason)
		if reason == "" {
			reason = "Gemini composer/send control is not ready"
		}
		return fmt.Errorf("gemini submit: %s", reason)
	}
	return nil
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
  const stopSelectors = [
    'button[aria-label*="Stop response" i]',
    'button[aria-label*="Stop generating" i]',
    'button[aria-label*="Dừng" i]',
    '[data-test-id="stop-button"]',
    'button.stop-button'
  ];
  let busy = stopSelectors.some((selector) => Array.from(document.querySelectorAll(selector)).some(visible));
  if (!busy) {
    for (const button of document.querySelectorAll('button')) {
      if (!visible(button)) continue;
      const icon = button.querySelector('mat-icon');
      const iconText = String(icon && icon.textContent || '').trim().toLowerCase();
      if (iconText === 'stop' || iconText === 'stop_circle') {
        busy = true;
        break;
      }
    }
  }

  const roots = Array.from(document.querySelectorAll(
    'model-response, [data-test-id="model-response"], .model-response'
  )).filter(visible);
  const texts = [];
  for (const root of roots) {
    const body = root.querySelector(
      'message-content, .model-response-text, .markdown, [class*="response-text"]'
    ) || root;
    const text = String(body.innerText || body.textContent || '').trim();
    if (text) texts.push(text);
  }
  const last = texts.length ? texts[texts.length - 1] : '';
  const max = 1048576;
  return {
    busy,
    response_count: texts.length,
    last_response: last.length > max ? last.slice(0, max) : last,
    truncated: last.length > max
  };
})()`

const geminiSubmitExpressionTemplate = `(async () => {
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
  }

  await new Promise((resolve) => setTimeout(resolve, 120));
  const send = firstVisible([
    'button[aria-label*="Send message" i]',
    'button[aria-label="Send"]',
    'button[aria-label*="Gửi" i]',
    '[data-test-id="send-button"]',
    'button.send-button'
  ]);
  let sendButton = send;
  if (!sendButton) {
    for (const button of document.querySelectorAll('button')) {
      if (!visible(button)) continue;
      const icon = button.querySelector('mat-icon');
      const iconText = String(icon && icon.textContent || '').trim().toLowerCase();
      if (iconText === 'send') { sendButton = button; break; }
    }
  }
  if (!sendButton) return {ok: false, reason: 'send button not found'};
  if (sendButton.disabled || sendButton.getAttribute('aria-disabled') === 'true') {
    return {ok: false, reason: 'send button is disabled'};
  }
  sendButton.click();
  return {ok: true, reason: ''};
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
  for (const button of document.querySelectorAll('button')) {
    if (!visible(button)) continue;
    const icon = button.querySelector('mat-icon');
    const iconText = String(icon && icon.textContent || '').trim().toLowerCase();
    if (iconText === 'stop' || iconText === 'stop_circle') {
      button.click();
      return {clicked: true};
    }
  }
  return {clicked: false};
})()`
