package sites

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// Gemini is the first concrete WEB-ONLY readiness adapter. It only inspects
// the visible page; it never submits prompts or reads authentication secrets.
type Gemini struct{}

func (Gemini) Name() string { return "gemini-web" }

func (Gemini) TargetScore(rawURL string) int {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return 0
	}
	host := strings.ToLower(u.Hostname())
	switch host {
	case "gemini.google.com":
		return 100
	case "accounts.google.com":
		if strings.Contains(strings.ToLower(rawURL), "gemini.google.com") {
			return 80
		}
	}
	return 0
}

type geminiReadinessPayload struct {
	Host                    string   `json:"host"`
	Path                    string   `json:"path"`
	HasAccountControl       bool     `json:"has_account_control"`
	HasComposer             bool     `json:"has_composer"`
	HasSignIn               bool     `json:"has_sign_in"`
	SecurityChallenge       bool     `json:"security_challenge"`
	CandidateAccountLink    bool     `json:"candidate_account_link"`
	CandidateAccountAria    bool     `json:"candidate_account_aria"`
	CandidateAccountData    bool     `json:"candidate_account_data"`
	CandidateAccountImg     bool     `json:"candidate_account_img"`
	HasOpenShadowRoot       bool     `json:"has_open_shadow_root"`
	CandidateAccountsIframe bool     `json:"candidate_accounts_iframe"`
	CandidateOGSIframe      bool     `json:"candidate_ogs_iframe"`
	CandidateGoogleIframe   bool     `json:"candidate_google_iframe"`
	FrameLocations          []string `json:"frame_locations"`
}

func (Gemini) Probe(ctx context.Context, evaluator Evaluator) (Result, error) {
	raw, err := evaluator.Eval(ctx, geminiReadinessExpression)
	if err != nil {
		return Result{}, err
	}
	var payload geminiReadinessPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return Result{}, fmt.Errorf("gemini readiness payload: %w", err)
	}
	host := strings.ToLower(strings.TrimSpace(payload.Host))
	if host == "accounts.google.com" || payload.SecurityChallenge {
		return Result{State: ReadinessAuthRequired, Reason: "Google sign-in/security page requires manual user action"}, nil
	}
	if host != "gemini.google.com" {
		return Result{State: ReadinessDegraded, Reason: "Gemini page target is not active"}, nil
	}

	// A visible sign-in control is authoritative even if Gemini exposes a
	// public composer or preloads Google account infrastructure.
	if payload.HasSignIn {
		return Result{State: ReadinessAuthRequired, Reason: "Gemini sign-in is required"}, nil
	}

	if payload.HasComposer && payload.HasAccountControl {
		return Result{State: ReadinessReady, Reason: "authenticated Gemini composer is ready"}, nil
	}

	// Current Gemini can render the signed-in Google account shell in
	// cross-origin accounts.google.com + ogs.google.com iframes. The parent page
	// cannot inspect those iframe DOMs, so the conservative authenticated signal
	// is: composer present, no visible sign-in control, and both Google account
	// infrastructure frames present. A clean profile is kept AUTH_REQUIRED by
	// the HasSignIn check above.
	if payload.HasComposer && payload.CandidateAccountsIframe && payload.CandidateOGSIframe {
		return Result{State: ReadinessReady, Reason: "authenticated Gemini composer is ready via Google account iframe shell"}, nil
	}

	if !payload.HasAccountControl {
		reason := "authenticated Google account control not detected"
		if payload.HasComposer {
			frames := strings.Join(payload.FrameLocations, ",")
			if frames == "" {
				frames = "none"
			}
			reason = fmt.Sprintf(
				"Gemini composer detected without authenticated account control (link=%t aria=%t data=%t img=%t shadow=%t accounts_iframe=%t ogs_iframe=%t google_iframe=%t frames=%s)",
				payload.CandidateAccountLink,
				payload.CandidateAccountAria,
				payload.CandidateAccountData,
				payload.CandidateAccountImg,
				payload.HasOpenShadowRoot,
				payload.CandidateAccountsIframe,
				payload.CandidateOGSIframe,
				payload.CandidateGoogleIframe,
				frames,
			)
		}
		return Result{State: ReadinessAuthRequired, Reason: reason}, nil
	}
	return Result{State: ReadinessDegraded, Reason: "authenticated Gemini page detected but composer is not ready"}, nil
}

// The expression returns booleans and coarse page/frame locations only. It
// strips iframe query strings and fragments and does not read cookies, tokens,
// localStorage, account email/name, conversation text or project data.
const geminiReadinessExpression = `(() => {
  const visible = (el) => {
    if (!el) return false;
    const style = window.getComputedStyle(el);
    if (style.display === 'none' || style.visibility === 'hidden') return false;
    const rect = el.getBoundingClientRect();
    return rect.width > 0 && rect.height > 0;
  };
  const roots = [document];
  let hasOpenShadowRoot = false;
  for (let i = 0; i < roots.length; i++) {
    const root = roots[i];
    for (const el of root.querySelectorAll('*')) {
      if (el.shadowRoot) {
        hasOpenShadowRoot = true;
        roots.push(el.shadowRoot);
      }
    }
  }
  const firstVisible = (selectors) => {
    for (const root of roots) {
      for (const selector of selectors) {
        for (const el of root.querySelectorAll(selector)) {
          if (visible(el)) return el;
        }
      }
    }
    return null;
  };
  const host = String(location.hostname || '').toLowerCase();
  const path = String(location.pathname || '');
  const strongAccountSelectors = [
    'a[href*="SignOutOptions"]',
    'a[href*="ManageAccount"]',
    'a[href*="myaccount.google.com"]',
    '[aria-label*="Google Account" i]',
    '[aria-label*="Tài khoản Google" i]',
    'button[aria-label*="account" i]',
    'button[aria-label*="tài khoản" i]',
    '[role="button"][aria-label*="account" i]',
    '[role="button"][aria-label*="tài khoản" i]',
    '[data-test-id*="account" i]',
    '[data-testid*="account" i]',
    '[data-test-id*="avatar" i]',
    '[data-testid*="avatar" i]',
    'button img[src*="googleusercontent.com"]',
    'a img[src*="googleusercontent.com"]',
    '[role="button"] img[src*="googleusercontent.com"]'
  ];
  const account = firstVisible(strongAccountSelectors);
  const candidateAccountLink = firstVisible([
    'a[href*="myaccount.google.com"]',
    'a[href*="accounts.google.com/SignOutOptions"]',
    'a[href*="accounts.google.com/ManageAccount"]'
  ]);
  const candidateAccountAria = firstVisible([
    'button[aria-label*="account" i]',
    'button[aria-label*="tài khoản" i]',
    '[role="button"][aria-label*="account" i]',
    '[role="button"][aria-label*="tài khoản" i]',
    '[aria-label*="Google Account" i]',
    '[aria-label*="Tài khoản Google" i]'
  ]);
  const candidateAccountData = firstVisible([
    '[data-test-id*="account" i]',
    '[data-testid*="account" i]',
    '[data-test-id*="avatar" i]',
    '[data-testid*="avatar" i]'
  ]);
  const candidateAccountImg = firstVisible([
    'button img[src*="googleusercontent.com"]',
    'a img[src*="googleusercontent.com"]',
    '[role="button"] img[src*="googleusercontent.com"]'
  ]);
  const composer = firstVisible([
    'div.ql-editor',
    'rich-textarea [contenteditable="true"]',
    '[aria-label="Enter a prompt here"]',
    '[contenteditable="true"][role="textbox"]',
    'textarea[aria-label*="prompt" i]'
  ]);
  let signIn = false;
  for (const root of roots) {
    for (const el of root.querySelectorAll('a,button')) {
      if (!visible(el)) continue;
      const href = String(el.getAttribute('href') || '').toLowerCase();
      const text = String(el.textContent || '').trim().toLowerCase();
      if (href.includes('accounts.google.com') || text === 'sign in' || text === 'đăng nhập') {
        signIn = true;
        break;
      }
    }
    if (signIn) break;
  }
  let candidateAccountsIframe = false;
  let candidateOGSIframe = false;
  let candidateGoogleIframe = false;
  const frameLocations = [];
  for (const frame of document.querySelectorAll('iframe')) {
    const raw = String(frame.getAttribute('src') || '');
    if (!raw) continue;
    try {
      const u = new URL(raw, location.href);
      const h = String(u.hostname || '').toLowerCase();
      const p = String(u.pathname || '/');
      if (h === 'accounts.google.com') candidateAccountsIframe = true;
      if (h === 'ogs.google.com') candidateOGSIframe = true;
      if (h === 'accounts.google.com' || h === 'ogs.google.com' || h === 'myaccount.google.com' || h.endsWith('.google.com')) {
        candidateGoogleIframe = true;
      }
      if ((h === 'accounts.google.com' || h === 'ogs.google.com' || h === 'myaccount.google.com' || h.endsWith('.google.com')) && frameLocations.length < 8) {
        frameLocations.push(h + p);
      }
    } catch (_) {}
  }
  frameLocations.sort();
  const securityChallenge = host === 'accounts.google.com' || /\/challenge(?:\/|$)/i.test(path);
  return {
    host,
    path,
    has_account_control: Boolean(account),
    has_composer: Boolean(composer),
    has_sign_in: signIn,
    security_challenge: securityChallenge,
    candidate_account_link: Boolean(candidateAccountLink),
    candidate_account_aria: Boolean(candidateAccountAria),
    candidate_account_data: Boolean(candidateAccountData),
    candidate_account_img: Boolean(candidateAccountImg),
    has_open_shadow_root: hasOpenShadowRoot,
    candidate_accounts_iframe: candidateAccountsIframe,
    candidate_ogs_iframe: candidateOGSIframe,
    candidate_google_iframe: candidateGoogleIframe,
    frame_locations: frameLocations
  };
})()`
