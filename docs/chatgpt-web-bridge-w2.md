# W2 — Browser Session & Persistent Web Login v0.1

Status: **OPEN — IMPLEMENTATION IN PROGRESS**

Repository: `tiktok1997af-dot/ainovel-cli`  
Branch: `w2-browser-session`  
Base: W1 locked main commit `afafe906487b61295c2ab4c3a59feaeeafddc753`

## Locked product rule

W2 remains **WEB-ONLY / NO-API**.

This stage does not add an AI API transport, API key, HTTP-provider fallback, or prompt submission. Its job is only to establish a visible local Chrome session whose login profile survives tool restarts.

Target lifecycle:

```text
STOPPED
  -> STARTING
  -> AUTH_REQUIRED  (user logs in manually in visible Chrome)
  -> READY          (site readiness probe confirms logged-in web UI)
  -> DEGRADED/FAILED when browser/site inspection cannot safely continue
  -> STOPPED
```

`BUSY` is reserved for later prompt work and is not a W2 readiness verdict.

## W2 slices

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
- `--remote-debugging-port=0` reserved for bounded readiness inspection;
- start/stop/PID tracking;
- unexpected browser exit -> FAILED/STOPPED;
- fake launcher tests in CI.

### W2D — Site readiness adapter

Next implementation slice after W2A–C tests pass:

- inspect the local Chrome DevTools endpoint only;
- identify the configured AI website tab;
- classify login/security state as `AUTH_REQUIRED`, `READY`, `DEGRADED`, or `FAILED`;
- do not submit prompts;
- do not bypass CAPTCHA, 2FA, anti-bot, or security challenges.

## Security boundary

Session snapshots may expose operational facts only: state, site, browser path, profile path, PID and timestamps/reason.

They must never expose or persist passwords, cookies, auth tokens, local-storage values, or browser database contents.

## W2 gate

W2 can be locked only after:

1. Linux/Windows CI passes for browser/session contracts.
2. A clean profile deterministically yields `AUTH_REQUIRED` in the concrete site adapter.
3. The same profile after manual login yields `READY`.
4. Restarting ainovel/Chrome with that profile preserves `READY` when the web login remains valid.
5. Logout/security challenge returns to an explicit user-action state without bypass attempts.

Prompt submission and response capture are explicitly outside W2 and begin in W3.
