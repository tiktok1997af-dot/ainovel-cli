from pathlib import Path
import re

path = Path("internal/webai/sites/gemini_interaction.go")
text = path.read_text(encoding="utf-8")


def sub_once(pattern: str, replacement: str, label: str) -> None:
    global text
    matches = list(re.finditer(pattern, text, flags=re.S))
    if len(matches) != 1:
        raise SystemExit(f"{label}: expected exactly one structural match, got {len(matches)}")
    match = matches[0]
    text = text[: match.start()] + replacement + text[match.end() :]


sub_once(
    r"    composer\.dispatchEvent\(new Event\('change', \{bubbles: true\}\)\);\n  \}\n  return \{ok: true, reason: ''\};\n\}\)\(\)`\n\nconst geminiClickSendExpression",
    """    composer.dispatchEvent(new Event('change', {bubbles: true}));
  }
  const preparedText = String(
    (composer instanceof HTMLTextAreaElement || composer instanceof HTMLInputElement)
      ? composer.value
      : (composer.innerText || composer.textContent || '')
  ).trim();
  if (preparedText !== String(prompt).trim()) {
    return {ok: false, reason: 'prompt composer did not retain prepared text'};
  }
  return {ok: true, reason: ''};
})()`

const geminiClickSendExpression""",
    "prepare verification",
)

native_block = """  const sendSelectors = [
    '[data-test-id="send-button-container"] button[aria-label*="Send message" i]',
    '[data-test-id="send-button-container"] button[aria-label*="Send" i]',
    '[data-test-id="send-button-container"] button',
    'button[aria-label*="Send message" i]',
    'button[aria-label="Send"]',
    'button[aria-label*="Submit" i]',
    'button[aria-label*="Gửi" i]',
    'button.send-button',
    '[data-test-id="send-button"]',
    'gem-icon-button.send-button[aria-disabled="false"]',
    'gem-icon-button.submit[aria-disabled="false"]',
    '[data-test-id="send-button-container"] gem-icon-button',
    'gem-icon-button[aria-label*="Send" i]',
    'gem-icon-button[aria-label*="Submit" i]',
    'gem-icon-button[aria-label*="Gửi" i]',
    '[role="button"][aria-label*="Send" i]',
    '[role="button"][aria-label*="Submit" i]',
    '[role="button"][aria-label*="Gửi" i]'
  ];
  const resolveClickTarget = (control) => {
    if (!control) return null;
    if (control instanceof HTMLButtonElement) return control;
    if (typeof control.querySelector === 'function') {
      const nested = Array.from(control.querySelectorAll('button')).find(visible);
      if (nested) return nested;
    }
    try {
      if (control.shadowRoot) {
        const shadowButton = Array.from(control.shadowRoot.querySelectorAll('button')).find(visible);
        if (shadowButton) return shadowButton;
      }
    } catch (_) {}
    return control;
  };
  const isDisabled = (el) => {
    for (let node = el; node && node !== document.body; node = node.parentElement) {
      if (Boolean(node.disabled) || node.hasAttribute('disabled') || node.getAttribute('aria-disabled') === 'true') {
        return true;
      }
    }
    return false;
  };
  const resolved = (control) => {
    const target = resolveClickTarget(control);
    if (!target || !visible(target)) return null;
    return {control, target};
  };
  const findSend = () => {
    const direct = firstVisible(sendSelectors);
    if (direct) {
      const hit = resolved(direct);
      if (hit) return hit;
    }
    for (const control of document.querySelectorAll('button, gem-icon-button, [role="button"]')) {
      if (!visible(control)) continue;
      const aria = String(control.getAttribute('aria-label') || '').trim().toLowerCase();
      const icon = control.querySelector('mat-icon');
      const iconText = String(icon && (icon.getAttribute('data-mat-icon-name') || icon.getAttribute('fonticon') || icon.textContent) || '').trim().toLowerCase();
      if (aria.includes('send') || aria.includes('submit') || aria.includes('gửi') ||
          iconText === 'send' || iconText === 'send_spark' || iconText === 'arrow_upward') {
        const hit = resolved(control);
        if (hit) return hit;
      }
    }
    return null;
  };

  const send = findSend();
  if (!send) return {ok: false, retry: true, reason: 'send control not found'};
  if (isDisabled(send.control) || isDisabled(send.target)) {
    return {ok: false, retry: true, reason: 'send control is disabled'};
  }
  const clickTarget = send.target;"""

sub_once(
    r"  const sendSelectors = \[\n.*?  const disabled = Boolean\(sendButton\.disabled\) \|\| sendButton\.hasAttribute\('disabled'\) \|\| sendButton\.getAttribute\('aria-disabled'\) === 'true';\n  if \(disabled\) return \{ok: false, retry: true, reason: 'send control is disabled'\};",
    native_block,
    "native send target resolution",
)

sub_once(
    r"  sendButton\.click\(\);\n  return \{ok: true, retry: false, reason: ''\};",
    "  clickTarget.click();\n  return {ok: true, retry: false, reason: ''};",
    "single click target",
)

native = '[data-test-id="send-button-container"] button[aria-label*="Send message" i]'
custom = 'gem-icon-button.send-button[aria-disabled="false"]'
if text.find(native) < 0 or text.find(custom) < 0 or text.find(native) > text.find(custom):
    raise SystemExit("native send selector is not prioritized ahead of custom wrapper")
if text.count("clickTarget.click();") != 1 or "sendButton.click();" in text:
    raise SystemExit("submit side effect is not exactly one resolved-target click")
if "preparedText !== String(prompt).trim()" not in text:
    raise SystemExit("prepared composer verification missing")
if "KeyboardEvent" in text or "dispatchKeyEvent" in text:
    raise SystemExit("unsafe keyboard fallback detected")

path.write_text(text, encoding="utf-8")
