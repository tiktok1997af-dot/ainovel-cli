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
	if snapshot.ComposerLength < 0 {
		return ConversationSnapshot{}, fmt.Errorf("gemini conversation snapshot: negative composer length")
	}
	if !snapshot.ComposerPresent {
		snapshot.ComposerEmpty = false
		snapshot.ComposerLength = 0
	}
	snapshot.SubmitAction = strings.TrimSpace(snapshot.SubmitAction)
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
	// Prompt preparation may probe/mutate only the composer. The send phase polls
	// readiness without side effects until one canonical actionable control is
	// found, then performs exactly one click. A click is not delivery ACK; the
	// transport independently confirms a rendered user turn/BUSY/response.
	prepareExpression := fmt.Sprintf(geminiPreparePromptExpressionTemplate, string(encoded))
	raw, err := evaluator.Eval(ctx, prepareExpression)
	if err != nil {
		return err
	}
	var prepared struct {
		OK             bool   `json:"ok"`
		Reason         string `json:"reason"`
		ComposerLength int    `json:"composer_length"`
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
	if prepared.ComposerLength <= 0 {
		return fmt.Errorf("gemini submit: prepared composer is empty")
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
  const signature = (text) => {
    const value = String(text || '');
    let hash = 2166136261;
    for (let i = 0; i < value.length; i++) {
      hash ^= value.charCodeAt(i);
      hash = Math.imul(hash, 16777619);
    }
    return value.length + ':' + String(hash >>> 0);
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

  // A send click seeds sanitized capture state containing only response count,
  // a length/hash signature, composer length, action strategy and timestamps.
  // Gemini can briefly hide its Stop control while a response is still changing.
  // Treat recent DOM text activity as BUSY for a short bounded grace independent
  // of any protocol markers. No prompt or response text is stored in page state.
  const last = texts.length ? texts[texts.length - 1] : '';
  const captureState = window.__ainovelWebCaptureState;
  if (captureState && last) {
    const sig = signature(last);
    const changedFromBaseline = texts.length > Number(captureState.responseCount || 0) || sig !== String(captureState.lastSignature || '');
    if (changedFromBaseline) {
      if (!captureState.observedChange) {
        captureState.observedChange = true;
        captureState.currentSignature = sig;
        captureState.lastChangedAt = Date.now();
      } else if (sig !== String(captureState.currentSignature || '')) {
        captureState.currentSignature = sig;
        captureState.lastChangedAt = Date.now();
      }
      const activityGraceMs = 2500;
      const elapsed = Date.now() - Number(captureState.lastChangedAt || 0);
      if (elapsed < activityGraceMs) {
        busy = true;
      } else {
        window.__ainovelWebCaptureState = null;
      }
    }
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

  const max = 1048576;
  return {
    busy,
    response_count: texts.length,
    user_message_count: userMessageCount,
    composer_present: Boolean(composer),
    composer_empty: Boolean(composer) && composerText.length === 0,
    composer_length: composerText.length,
    submit_action: captureState ? String(captureState.submitAction || '') : '',
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
  const readComposer = (composer) => String(
    (composer instanceof HTMLTextAreaElement || composer instanceof HTMLInputElement)
      ? composer.value
      : (composer.innerText || composer.textContent || '')
  );
  const samePrompt = (actual) => String(actual || '').replace(/\r\n/g, '\n').trim() === String(prompt).replace(/\r\n/g, '\n').trim();

  const composer = firstVisible([
    'rich-textarea .ql-editor[contenteditable="true"]',
    'rich-textarea [contenteditable="true"]',
    'div.ql-editor[contenteditable="true"]',
    '[contenteditable="true"][role="textbox"]',
    '[aria-label="Enter a prompt here"]',
    'textarea[aria-label*="prompt" i]'
  ]);
  if (!composer) return {ok: false, reason: 'prompt composer not found', composer_length: 0};
  composer.focus();

  if (composer instanceof HTMLTextAreaElement || composer instanceof HTMLInputElement) {
    const proto = composer instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
    const setter = Object.getOwnPropertyDescriptor(proto, 'value')?.set;
    if (setter) setter.call(composer, prompt); else composer.value = prompt;
    try {
      composer.dispatchEvent(new InputEvent('beforeinput', {bubbles: true, cancelable: true, inputType: 'insertText', data: prompt}));
    } catch (_) {}
    composer.dispatchEvent(new InputEvent('input', {bubbles: true, inputType: 'insertText', data: prompt}));
    composer.dispatchEvent(new Event('change', {bubbles: true}));
  } else {
    const selection = window.getSelection();
    const range = document.createRange();
    range.selectNodeContents(composer);
    selection.removeAllRanges();
    selection.addRange(range);
    try {
      composer.dispatchEvent(new InputEvent('beforeinput', {bubbles: true, cancelable: true, inputType: 'insertText', data: prompt}));
    } catch (_) {}
    let inserted = false;
    try { inserted = document.execCommand('insertText', false, prompt); } catch (_) {}
    if (!inserted || !samePrompt(readComposer(composer))) {
      // Last-resort framework-compatible mutation. The event is dispatched after
      // the DOM value is present so controlled editors can read the actual value.
      composer.textContent = prompt;
      composer.dispatchEvent(new InputEvent('input', {bubbles: true, inputType: 'insertText', data: prompt}));
    }
    composer.dispatchEvent(new Event('change', {bubbles: true}));
  }

  const actual = readComposer(composer);
  if (!samePrompt(actual)) {
    return {ok: false, reason: 'prompt composer did not retain prepared text', composer_length: String(actual || '').trim().length};
  }
  const composerLength = String(actual || '').trim().length;
  if (composerLength === 0) {
    return {ok: false, reason: 'prompt composer is empty after prepare', composer_length: 0};
  }
  return {ok: true, reason: '', composer_length: composerLength};
})()`

const geminiClickSendExpression = `(() => {
  const visible = (el) => {
    if (!el) return false;
    const style = window.getComputedStyle(el);
    if (style.display === 'none' || style.visibility === 'hidden') return false;
    const rect = el.getBoundingClientRect();
    return rect.width > 0 && rect.height > 0;
  };
  const disabled = (el) => Boolean(el && (
    el.disabled || el.hasAttribute('disabled') || el.getAttribute('aria-disabled') === 'true'
  ));
  const signature = (text) => {
    const value = String(text || '');
    let hash = 2166136261;
    for (let i = 0; i < value.length; i++) {
      hash ^= value.charCodeAt(i);
      hash = Math.imul(hash, 16777619);
    }
    return value.length + ':' + String(hash >>> 0);
  };
  const composerSelectors = [
    'rich-textarea .ql-editor[contenteditable="true"]',
    'rich-textarea [contenteditable="true"]',
    'div.ql-editor[contenteditable="true"]',
    '[contenteditable="true"][role="textbox"]',
    '[aria-label="Enter a prompt here"]',
    'textarea[aria-label*="prompt" i]'
  ];
  const firstVisible = (selectors) => {
    for (const selector of selectors) {
      for (const el of document.querySelectorAll(selector)) {
        if (visible(el)) return el;
      }
    }
    return null;
  };
  const explicitSendSemantic = (el) => {
    if (!el) return false;
    const aria = String(el.getAttribute('aria-label') || '').trim().toLowerCase();
    if (aria.includes('send') || aria.includes('submit') || aria.includes('gửi')) return true;
    const cls = String(el.className || '').toLowerCase();
    if (cls.includes('send-button') || cls.includes('submit')) return true;
    const testID = String(el.getAttribute('data-test-id') || '').toLowerCase();
    if (testID === 'send-button') return true;
    const icon = el.querySelector && el.querySelector('mat-icon');
    const iconText = String(icon && (icon.getAttribute('data-mat-icon-name') || icon.getAttribute('fonticon') || icon.textContent) || '').trim().toLowerCase();
    return iconText === 'send' || iconText === 'send_spark' || iconText === 'arrow_upward';
  };
  const nativeDescendant = (candidate) => {
    if (!candidate) return null;
    if (candidate instanceof HTMLButtonElement) return candidate;
    for (const root of [candidate.shadowRoot, candidate]) {
      if (!root || !root.querySelectorAll) continue;
      for (const button of root.querySelectorAll('button')) {
        if (visible(button)) return button;
      }
    }
    return null;
  };
  const canonicalAction = (candidate) => {
    if (!candidate) return null;
    const native = nativeDescendant(candidate);
    if (native) {
      return {
        element: native,
        strategy: candidate instanceof HTMLButtonElement
          ? 'native-button'
          : (candidate.shadowRoot && candidate.shadowRoot.contains(native) ? 'shadow-native-button' : 'nested-native-button')
      };
    }
    const tag = String(candidate.tagName || '').toLowerCase();
    if (tag === 'gem-icon-button' && explicitSendSemantic(candidate)) {
      return {element: candidate, strategy: 'custom-send-host'};
    }
    if (candidate.getAttribute('role') === 'button' && explicitSendSemantic(candidate)) {
      return {element: candidate, strategy: 'semantic-role-button'};
    }
    return null;
  };
  const sendSelectors = [
    '[data-test-id="send-button-container"] button',
    'button[data-test-id="send-button"]',
    'button[aria-label*="Send message" i]',
    'button[aria-label="Send"]',
    'button[aria-label*="Submit" i]',
    'button[aria-label*="Gửi" i]',
    'button.send-button',
    'gem-icon-button.send-button',
    'gem-icon-button.submit',
    '[data-test-id="send-button-container"] gem-icon-button',
    '[data-test-id="send-button"]',
    'gem-icon-button[aria-label*="Send" i]',
    'gem-icon-button[aria-label*="Submit" i]',
    'gem-icon-button[aria-label*="Gửi" i]',
    '[role="button"][aria-label*="Send" i]',
    '[role="button"][aria-label*="Submit" i]',
    '[role="button"][aria-label*="Gửi" i]'
  ];
  const findSendAction = () => {
    const composer = firstVisible(composerSelectors);
    if (composer) {
      const form = composer.closest && composer.closest('form');
      if (form) {
        for (const button of form.querySelectorAll('button[type="submit"], button:not([type])')) {
          if (!visible(button) || !explicitSendSemantic(button)) continue;
          return canonicalAction(button);
        }
      }
    }
    for (const selector of sendSelectors) {
      for (const candidate of document.querySelectorAll(selector)) {
        if (!visible(candidate)) continue;
        const action = canonicalAction(candidate);
        if (action) return action;
      }
    }
    for (const control of document.querySelectorAll('button, gem-icon-button, [role="button"]')) {
      if (!visible(control) || !explicitSendSemantic(control)) continue;
      const action = canonicalAction(control);
      if (action) return action;
    }
    return null;
  };

  const composer = firstVisible(composerSelectors);
  if (!composer) return {ok: false, retry: false, reason: 'prompt composer disappeared before send'};
  const composerText = String(
    (composer instanceof HTMLTextAreaElement || composer instanceof HTMLInputElement)
      ? composer.value
      : (composer.innerText || composer.textContent || '')
  ).trim();
  if (!composerText) return {ok: false, retry: false, reason: 'prompt composer is empty before send'};

  const action = findSendAction();
  if (!action) return {ok: false, retry: true, reason: 'actionable send control not found'};
  if (disabled(action.element)) return {ok: false, retry: true, reason: 'actionable send control is disabled'};

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
  const previous = texts.length ? texts[texts.length - 1] : '';
  const previousSignature = signature(previous);
  window.__ainovelWebCaptureState = {
    responseCount: texts.length,
    lastSignature: previousSignature,
    currentSignature: previousSignature,
    lastChangedAt: Date.now(),
    observedChange: false,
    composerLengthBeforeClick: composerText.length,
    submitAction: action.strategy
  };

  action.element.focus();
  action.element.click();
  return {ok: true, retry: false, reason: '', action: action.strategy};
})()`

const geminiCancelExpression = `(() => {
  const visible = (el) => {
    if (!el) return false;
    const style = window.getComputedStyle(el);
    if (style.display === 'none' || style.visibility === 'hidden') return false;
    const rect = el.getBoundingClientRect();
    return rect.width > 0 && rect.height > 0;
  };
  const disabled = (el) => Boolean(el && (
    el.disabled || el.hasAttribute('disabled') || el.getAttribute('aria-disabled') === 'true'
  ));
  const explicitStopSemantic = (el) => {
    if (!el) return false;
    const aria = String(el.getAttribute('aria-label') || '').trim().toLowerCase();
    if (aria.includes('stop') || aria.includes('dừng')) return true;
    const icon = el.querySelector && el.querySelector('mat-icon');
    const iconText = String(icon && (icon.getAttribute('data-mat-icon-name') || icon.getAttribute('fonticon') || icon.textContent) || '').trim().toLowerCase();
    return iconText === 'stop' || iconText === 'stop_circle';
  };
  const canonicalAction = (candidate) => {
    if (!candidate) return null;
    if (candidate instanceof HTMLButtonElement) return candidate;
    for (const root of [candidate.shadowRoot, candidate]) {
      if (!root || !root.querySelectorAll) continue;
      for (const button of root.querySelectorAll('button')) {
        if (visible(button)) return button;
      }
    }
    if (String(candidate.tagName || '').toLowerCase() === 'gem-icon-button' && explicitStopSemantic(candidate)) return candidate;
    if (candidate.getAttribute('role') === 'button' && explicitStopSemantic(candidate)) return candidate;
    return null;
  };
  const selectors = [
    'button[aria-label*="Stop response" i]',
    'button[aria-label*="Stop generating" i]',
    'button[aria-label*="Dừng" i]',
    '[data-test-id="stop-button"] button',
    'button[data-test-id="stop-button"]',
    'button.stop-button',
    'gem-icon-button[aria-label*="Stop" i]',
    'gem-icon-button[aria-label*="Dừng" i]',
    '[data-test-id="stop-button"]'
  ];
  for (const selector of selectors) {
    for (const candidate of document.querySelectorAll(selector)) {
      if (!visible(candidate)) continue;
      const action = canonicalAction(candidate);
      if (!action || disabled(action)) continue;
      action.focus();
      action.click();
      return {clicked: true};
    }
  }
  for (const control of document.querySelectorAll('button, gem-icon-button, [role="button"]')) {
    if (!visible(control) || !explicitStopSemantic(control)) continue;
    const action = canonicalAction(control);
    if (!action || disabled(action)) continue;
    action.focus();
    action.click();
    return {clicked: true};
  }
  return {clicked: false};
})()`
