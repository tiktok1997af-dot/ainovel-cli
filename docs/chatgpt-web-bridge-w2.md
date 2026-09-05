# W2 — Browser Session & Persistent Web Login v0.1

Status: **OPEN — W2A–W2E DETERMINISTIC PASS / REAL-BROWSER GATE PENDING**

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

## W2D — Local Chrome DevTools readiness adapter — DETERMINISTIC PASS

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

### W2D deterministic verification

Exact verified commit: `aafcc8846946b03b4be0030d7c2a14275d7afe62`  
GitHub Actions CI run: `33980197843` / run #20.

- Ubuntu: format PASS; installer syntax PASS; `go vet ./...` PASS; full tests PASS; selected race tests PASS.
- Windows: format PASS; `go vet ./...` PASS; full tests PASS.

Decision: **W2D DETERMINISTIC PASS**.

## W2E — Local real-browser verification harness — DETERMINISTIC PASS

W2E adds a dedicated verifier that bypasses the legacy setup/config path entirely and therefore never asks for an AI API key:

```text
cmd/ainovel-w2-verify
scripts/w2-verify-windows.cmd
.github/workflows/w2-verifier.yml
```

One-click Windows sequence:

```text
START_W2_VERIFY.cmd
  -> ainovel-w2-verify.exe
  -> create a fresh dedicated profile
  -> open visible Chrome/Gemini
  -> observe AUTH_REQUIRED
  -> user manually logs in
  -> observe READY
  -> Tool stops Chrome
  -> Tool restarts the same profile
  -> observe READY after restart
  -> write local evidence JSON
```

The verifier never logs the Google account out automatically. An optional watch mode can record a later transition back to `AUTH_REQUIRED`, but the Tool will not mutate account state merely to manufacture evidence.

Evidence defaults to `~/.ainovel/browser/evidence/<verification-profile>.json` and contains only:

- schema/version;
- site name;
- generated verification profile name;
- timestamps;
- coarse state/reason transitions;
- gate booleans.

Evidence does **not** contain browser/profile paths, cookies, tokens, localStorage, account identity, page text, conversation content or project data.

### W2E deterministic verification

Exact verified implementation commit: `0b309f1764be0f636cf549389073cf881171ab79`.

- CI run #21: Ubuntu format/vet/full tests/race PASS; Windows format/vet/full tests PASS.
- W2 Browser Verifier Build run #1: Windows build PASS and artifact upload PASS.
- Artifact name: `ainovel-w2-verifier-windows`.
- Artifact archive digest: `sha256:f2a4bda2fc4050757e7111ee003d87c3847caabc26dcd6d058bc87a56a47db1f`.

Decision: **W2E DETERMINISTIC IMPLEMENTATION PASS**.

## Security boundary

Session snapshots may expose operational facts only: state, site, browser path, profile path, PID and timestamps/reason in memory.

Persisted W2E evidence is more restrictive and excludes browser/profile paths and all authentication/account/page data.

Passwords, cookies, auth tokens, local-storage values, account identity, conversation text and project data must never be persisted by W2 evidence.

## Remaining W2 real-browser gate

W2 can be fully locked only after the Windows verifier is executed on the user's real machine and records:

1. clean local Chrome profile -> `AUTH_REQUIRED`;
2. same profile after manual login -> `READY`;
3. automatic restart with the same profile -> `READY` while the web login remains valid;
4. logout/security challenge -> explicit user-action state without bypass attempts. The Tool will not force logout automatically; this branch may be observed later when naturally/manual triggered.

Prompt submission and response capture are explicitly outside W2 and begin in W3.
