# W5B — BROWSER-ONLY SETUP & TUI MIGRATION v0.1

Status: OPEN

Base authority: W5A PASS / LOCKED at `995d280db154503724185460425af29fceeb7ad1`.

## Product UX boundary

For the active WEB-only product path, setup and TUI expose browser/session concepts only. Users never choose or enter an AI API provider, API key, Base URL, API protocol, provider fallback, or Ollama/local inference path.

Initial supported AI website: Gemini Web.

## First-run setup

The setup wizard must:
1. choose creative language;
2. explain that AI runs through the user's visible logged-in Gemini Web session;
3. optionally accept a Chrome executable path (blank = auto-detect);
4. accept a persistent browser profile name (default `default`);
5. save `web.enabled=true`, `web.site=gemini-web` and no `providers` credential entries.

It must not ask for API Key, Base URL, provider protocol, provider name, or model ID.

## `/model`

In WEB-only mode `/model` is a read-only browser AI status surface, not a provider/model switcher. It shows only non-secret data:
- transport: WEB-only;
- site/model label: Gemini Web;
- readiness state (STARTING/AUTH_REQUIRED/READY/BUSY/DEGRADED/FAILED/STOPPED);
- browser PID/profile where useful;
- manual-login guidance for AUTH_REQUIRED.

No provider switching or API fallback is reachable from this surface.

## `/config`

In WEB-only mode `/config` edits browser settings only:
- Chrome executable path (blank = auto-detect);
- persistent profile name;
- site fixed to `gemini-web` in W5B.

Changes are persisted safely and take effect on the next application restart; the current owned browser session is not silently replaced during an active writing run.

No cookies, Google credentials, session tokens, API credentials, Base URLs, or provider definitions are displayed or accepted.

## Compatibility boundary

Legacy provider-shaped internals remain temporarily compiled only for W5C migration/removal. W5B makes them unreachable from a newly configured WEB-only product. W5C remains the blocking gate that deletes the legacy API runtime itself.

## W5B gate

PASS requires:
- deterministic setup/config tests proving no API credential/provider entry is created;
- TUI tests proving WEB-only `/model` cannot switch providers;
- TUI tests proving WEB-only `/config` contains browser fields and no API Key/Base URL/provider controls;
- config persistence test;
- gofmt / go vet / full Linux + Windows tests PASS.

Only then may W5B be locked and W5C opened.
