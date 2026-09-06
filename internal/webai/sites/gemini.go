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
	Host                 string `json:"host"`
	Path                 string `json:"path"`
	HasAccountControl    bool   `json:"has_account_control"`
	HasComposer          bool   `json:"has_composer"`
	HasSignIn            bool   `json:"has_sign_in"`
	SecurityChallenge    bool   `json:"security_challenge"`
	CandidateAccountLink bool   `json:"candidate_account_link"`
	CandidateAccountAria bool   `json:"candidate_account_aria"`
	CandidateAccountData bool   `json:"candidate_account_data"`
	CandidateAccountImg  bool   `json:"candidate_account_img"`
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
	if !payload.HasAccountControl {
		reason := "authenticated Google account control not detected"
		if payload.HasSignIn {
			reason = "Gemini sign-in is required"
		} else if payload.HasComposer {
			reason = fmt.Sprintf(
				"Gemini public composer detected without authenticated account control (candidate_link=%t candidate_aria=%t candidate_data=%t candidate_img=%t)",
				payload.CandidateAccountLink,
				payload.CandidateAccountAria,
				payload.CandidateAccountData,
				payload.CandidateAccountImg,
			)
		}
		return Result{State: ReadinessAuthRequired, Reason: reason}, nil
	}
	if payload.HasComposer {
		return Result{State: ReadinessReady, Reason: "authenticated Gemini composer is ready"}, nil
	}
	return Result{State: ReadinessDegraded, Reason: "authenticated Gemini page detected but composer is not ready"}, nil
}

// The expression returns booleans and coarse page location only. It does not
// read cookies, tokens, localStorage, account email/name, conversation text or
// any project data.
const geminiReadinessExpression = `(() => {
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
  const account = firstVisible([
    'a[href*="SignOutOptions"]',
    'a[href*="ManageAccount"]',
    '[aria-label*="Google Account" i]',
    '[aria-label*="Tài khoản Google" i]'
  ]);
  const candidateAccountLink = firstVisible([
    'a[href*="myaccount.google.com"]',
    'a[href*="accounts.google.com/SignOutOptions"]',
    'a[href*="accounts.google.com/ManageAccount"]'
  ]);
  const candidateAccountAria = firstVisible([
    'button[aria-label*="account" i]',
    'button[aria-label*="tài khoản" i]',
    '[role="button"][aria-label*="account" i]',
    '[role="button"][aria-label*="tài khoản" i]'
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
  for (const el of document.querySelectorAll('a,button')) {
    if (!visible(el)) continue;
    const href = String(el.getAttribute('href') || '').toLowerCase();
    const text = String(el.textContent || '').trim().toLowerCase();
    if (href.includes('accounts.google.com') || text === 'sign in' || text === 'đăng nhập') {
      signIn = true;
      break;
    }
  }
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
    candidate_account_img: Boolean(candidateAccountImg)
  };
})()`
