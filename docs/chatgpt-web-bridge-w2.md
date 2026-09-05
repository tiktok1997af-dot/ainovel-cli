# W2 — Browser Session & Persistent Web Login v0.1

Status: **OPEN — W2A–W2C LOCKED / W2D IMPLEMENTATION IN PROGRESS**

Repository: `tiktok1997af-dot/ainovel-cli`  
Branch: `w2-browser-session`  
Base: W1 locked main commit `afafe906487b61295c2ab4c3a59feaeeafddc753`

## Locked product rule

W2 remains **WEB-ONLY / NO-API**.

This stage does not add an AI API transport, API key, HTTP-provider fallback, or prompt submission. Its job is only to establish a visible local Chrome session whose login profile survives tool restarts and whose login readiness can be inspected locally.

Target lifecycle:

```text
STOPPED
  -> STARTING
  -> AUTH_REQUIRED  (user logs in manually in visible Chrome)
  -> READY          (site readiness probe confirms authenticated web UI)
  -> DEGRADED/FAILED when browser/site inspection cannot safely continue
  -> STOPPED
```

`BUSY` is reserved for later prompt work and is not a W2 readiness verdict.

## W2A–W2C — PASS / LOCKED

### W2A — Browser executable resolution

- explicit Chrome executable override;
- common Chrome locations on Windows/macOS/Linux;
- fail visibly when Chrome cannot be resolved;
- never fall back to an API/cloud browser.

### W2B — Persistent profile isolation

- default profile root: `~/.ainovel/browser/profiles/<name>`;
- profile is outside novel/project data;
- Chrome owns cookies/login storage;
- ainovel does not store passwords;
- Stop does not delete the profile;
- browser artifacts are ignored if a developer intentionally points a profile into a checkout.

### W2C — Browser process/session lifecycle

- visible Chrome launch;
- dedicated `--user-data-dir`;
- loopback `--remote-debugging-address=127.0.0.1`;
- dynamic `--remote-debugging-port=0`;
- start/stop/PID tracking;
- unexpected browser exit -> FAILED/STOPPED;
- fake launcher tests in CI.

### W2A–W2C runtime verification

Exact verified commit: `fb02c8426684aebae8e27d8195d7b553456250e2`  
GitHub Actions CI run: `33979454518` / run #16.

- Ubuntu: format PASS; installer syntax PASS; `go vet ./...` PASS; full `go test -buildvcs=false -count=1 ./...` PASS; selected race tests PASS.
- Windows: format PASS; `go vet ./...` PASS; full `go test -buildvcs=false -count=1 ./...` PASS.

Decision: **W2A–W2C PASS / LOCKED**.

## W2D — Local Chrome DevTools readiness adapter

Implementation boundary:

- read Chrome's `DevToolsActivePort` from the dedicated profile;
- contact only `127.0.0.1:<active-port>`;
- reject non-loopback or port-mismatched DevTools WebSocket targets;
- locate the configured AI-site page;
- evaluate a read-only DOM readiness expression;
- return only coarse operational booleans/state;
- never read cookies, auth tokens, localStorage, account identifiers, conversation text or project data;
- never submit prompts;
- never bypass CAPTCHA, 2FA, anti-bot or account security challenges.

### First concrete site — Gemini Web

Gemini readiness is fail-closed because Gemini may expose some web functionality without login. `READY` therefore requires both:

1. an authenticated Google account control; and
2. a visible Gemini prompt composer.

Classification:

```text
Google sign-in/security page -> AUTH_REQUIRED
Gemini public composer without authenticated account control -> AUTH_REQUIRED
authenticated account + composer -> READY
authenticated account but composer not ready -> DEGRADED
unknown/non-Gemini target -> DEGRADED
```

`NewSessionManager(SessionConfig{Site: "gemini-web"})` automatically selects the Gemini start URL and concrete DevTools readiness probe unless a test/custom probe is explicitly injected.

### W2D deterministic CI coverage

W2D tests use a local fake DevTools HTTP/WebSocket server only. They cover:

- authenticated Gemini -> READY;
- public/unauthenticated Gemini composer -> AUTH_REQUIRED;
- authenticated Gemini without composer -> DEGRADED;
- loopback-only WebSocket enforcement;
- site-target scoring;
- default Gemini probe wiring.

## Security boundary

Session snapshots may expose operational facts only: state, site, browser path, profile path, PID and timestamps/reason.

They must never expose or persist passwords, cookies, auth tokens, local-storage values, account identity, or browser database contents.

## Remaining W2 gate

W2 can be fully locked only after:

1. W2D Linux/Windows CI passes.
2. A clean real Chrome profile deterministically yields `AUTH_REQUIRED`.
3. The same real profile after manual login yields `READY`.
4. Restarting ainovel/Chrome with that profile preserves `READY` when the web login remains valid.
5. Logout/security challenge returns to an explicit user-action state without bypass attempts.

Prompt submission and response capture are explicitly outside W2 and begin in W3.
