package sites

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"unicode/utf8"
)

// ChatGPT is the WEB-only ChatGPT readiness/model-observation adapter. It reads
// only coarse visible DOM structure and model-picker labels. It never reads
// cookies, tokens, localStorage, account identity, or conversation contents.
type ChatGPT struct{}

func (ChatGPT) Name() string { return "chatgpt-web" }

func (ChatGPT) TargetScore(rawURL string) int {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return 0
	}
	host := strings.ToLower(u.Hostname())
	switch host {
	case "chatgpt.com", "www.chatgpt.com":
		return 100
	case "auth.openai.com", "login.openai.com", "auth0.openai.com":
		return 80
	case "accounts.google.com":
		path := strings.ToLower(strings.TrimSpace(u.Path))
		if strings.Contains(path, "/signin/") || strings.Contains(path, "/o/oauth2/") {
			return 70
		}
		return 0
	default:
		return 0
	}
}

type chatGPTReadinessPayload struct {
	Host              string `json:"host"`
	Path              string `json:"path"`
	HasComposer       bool   `json:"has_composer"`
	HasSignIn         bool   `json:"has_sign_in"`
	HasAccountControl bool   `json:"has_account_control"`
	SecurityChallenge bool   `json:"security_challenge"`
}

func (ChatGPT) Probe(ctx context.Context, evaluator Evaluator) (Result, error) {
	raw, err := evaluator.Eval(ctx, chatGPTReadinessExpression)
	if err != nil {
		return Result{}, err
	}
	var payload chatGPTReadinessPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return Result{}, fmt.Errorf("chatgpt readiness payload: %w", err)
	}
	host := strings.ToLower(strings.TrimSpace(payload.Host))
	if isChatGPTAuthHost(host) || payload.SecurityChallenge {
		return Result{State: ReadinessAuthRequired, Reason: "ChatGPT sign-in/security page requires manual user action"}, nil
	}
	if host != "chatgpt.com" && host != "www.chatgpt.com" {
		return Result{State: ReadinessDegraded, Reason: "ChatGPT page target is not active"}, nil
	}
	if payload.HasSignIn {
		return Result{State: ReadinessAuthRequired, Reason: "ChatGPT sign-in is required"}, nil
	}
	if payload.HasComposer && payload.HasAccountControl {
		return Result{State: ReadinessReady, Reason: "authenticated ChatGPT composer is ready"}, nil
	}
	if payload.HasComposer && !payload.HasAccountControl {
		return Result{State: ReadinessAuthRequired, Reason: "ChatGPT composer is visible but authenticated account control is not verified"}, nil
	}
	if payload.HasAccountControl {
		return Result{State: ReadinessDegraded, Reason: "authenticated ChatGPT shell detected but composer is not ready"}, nil
	}
	return Result{State: ReadinessAuthRequired, Reason: "ChatGPT authenticated session is not verified"}, nil
}

func isChatGPTAuthHost(host string) bool {
	switch strings.ToLower(strings.TrimSpace(host)) {
	case "auth.openai.com", "login.openai.com", "auth0.openai.com", "accounts.google.com":
		return true
	default:
		return false
	}
}

type chatGPTModelPayload struct {
	ActiveLabel string `json:"active_label"`
	Models      []struct {
		Label     string `json:"label"`
		Available bool   `json:"available"`
	} `json:"models"`
}

// ObserveModels reads only model-switcher/picker labels. A model ID is derived
// from the currently observed label rather than hard-coding volatile marketing
// names in core. If the active model cannot be observed, model preflight must
// fail closed.
func (ChatGPT) ObserveModels(ctx context.Context, evaluator Evaluator) (ModelCatalogSnapshot, error) {
	raw, err := evaluator.Eval(ctx, chatGPTModelExpression)
	if err != nil {
		return ModelCatalogSnapshot{}, err
	}
	var payload chatGPTModelPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ModelCatalogSnapshot{}, fmt.Errorf("chatgpt model payload: %w", err)
	}
	activeLabel := sanitizeModelLabel(payload.ActiveLabel)
	if activeLabel == "" {
		return ModelCatalogSnapshot{}, fmt.Errorf("chatgpt active model is not observable")
	}

	byID := make(map[string]ModelOption)
	for _, observed := range payload.Models {
		label := sanitizeModelLabel(observed.Label)
		if label == "" {
			continue
		}
		id := observedModelID(label)
		byID[id] = ModelOption{ID: id, Label: label, Available: observed.Available}
	}
	activeID := observedModelID(activeLabel)
	if existing, ok := byID[activeID]; ok {
		existing.Available = true
		byID[activeID] = existing
	} else {
		byID[activeID] = ModelOption{ID: activeID, Label: activeLabel, Available: true}
	}

	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	models := make([]ModelOption, 0, len(ids))
	for _, id := range ids {
		models = append(models, byID[id])
	}
	return ModelCatalogSnapshot{
		Models:        models,
		ActiveModelID: activeID,
		Revision:      modelCatalogRevision(activeID, models),
	}, nil
}

func sanitizeModelLabel(label string) string {
	label = strings.Join(strings.Fields(strings.TrimSpace(label)), " ")
	if label == "" {
		return ""
	}
	const maxRunes = 120
	if utf8.RuneCountInString(label) <= maxRunes {
		return label
	}
	runes := []rune(label)
	return string(runes[:maxRunes])
}

func observedModelID(label string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.Join(strings.Fields(label), " "))))
	return "observed-" + hex.EncodeToString(sum[:12])
}

func modelCatalogRevision(activeID string, models []ModelOption) string {
	var b strings.Builder
	b.WriteString(activeID)
	b.WriteByte('\n')
	for _, model := range models {
		b.WriteString(model.ID)
		b.WriteByte('|')
		b.WriteString(model.Label)
		if model.Available {
			b.WriteString("|1\n")
		} else {
			b.WriteString("|0\n")
		}
	}
	sum := sha256.Sum256([]byte(b.String()))
	return "sha256:" + hex.EncodeToString(sum[:16])
}

const chatGPTReadinessExpression = `(() => {
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
  const host = String(location.hostname || '').toLowerCase();
  const path = String(location.pathname || '');
  const composer = firstVisible([
    '#prompt-textarea',
    'textarea[data-testid*="prompt" i]',
    '[contenteditable="true"][data-testid*="composer" i]',
    '[contenteditable="true"][role="textbox"]'
  ]);
  const account = firstVisible([
    'button[data-testid="profile-button"]',
    'button[data-testid*="profile" i]',
    '[data-testid*="user-menu" i]',
    'button[aria-label*="profile" i]',
    'button[aria-label*="account" i]',
    '[role="button"][aria-label*="profile" i]',
    '[role="button"][aria-label*="account" i]'
  ]);
  let signIn = false;
  for (const el of document.querySelectorAll('a,button')) {
    if (!visible(el)) continue;
    const href = String(el.getAttribute('href') || '').toLowerCase();
    const text = String(el.textContent || '').trim().toLowerCase();
    if (href.includes('/auth/login') || href.includes('auth.openai.com') || text === 'log in' || text === 'login' || text === 'sign in' || text === 'đăng nhập') {
      signIn = true;
      break;
    }
  }
  const securityChallenge = /challenge|captcha|verify/i.test(path) && host !== 'chatgpt.com';
  return {
    host,
    path,
    has_composer: Boolean(composer),
    has_sign_in: signIn,
    has_account_control: Boolean(account),
    security_challenge: securityChallenge
  };
})()`

const chatGPTModelExpression = `(() => {
  const visible = (el) => {
    if (!el) return false;
    const style = window.getComputedStyle(el);
    if (style.display === 'none' || style.visibility === 'hidden') return false;
    const rect = el.getBoundingClientRect();
    return rect.width > 0 && rect.height > 0;
  };
  const clean = (value) => String(value || '').replace(/\s+/g, ' ').trim().slice(0, 160);
  const activeSelectors = [
    'button[data-testid="model-switcher-dropdown-button"]',
    'button[data-testid*="model-switcher" i]',
    '[data-testid*="model-switcher" i][role="button"]',
    'button[aria-label*="model" i]'
  ];
  let active = '';
  for (const selector of activeSelectors) {
    const el = Array.from(document.querySelectorAll(selector)).find(visible);
    if (el) {
      active = clean(el.getAttribute('data-model') || el.getAttribute('aria-label') || el.textContent);
      if (active) break;
    }
  }
  const candidates = [];
  const seen = new Set();
  const optionSelectors = [
    '[data-testid*="model-option" i]',
    '[data-testid*="model-menu" i] [role="menuitem"]',
    '[data-testid*="model" i][role="menuitem"]',
    '[data-testid*="model" i][role="option"]'
  ];
  for (const selector of optionSelectors) {
    for (const el of document.querySelectorAll(selector)) {
      if (!visible(el)) continue;
      const label = clean(el.getAttribute('data-model') || el.getAttribute('aria-label') || el.textContent);
      if (!label || seen.has(label)) continue;
      seen.add(label);
      const disabled = el.getAttribute('aria-disabled') === 'true' || el.hasAttribute('disabled');
      candidates.push({label, available: !disabled});
      if (candidates.length >= 32) break;
    }
    if (candidates.length >= 32) break;
  }
  return {active_label: active, models: candidates};
})()`
