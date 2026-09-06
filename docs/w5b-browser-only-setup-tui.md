# W5B — BROWSER-ONLY SETUP & TUI MIGRATION v0.1

Status: PASS / LOCKED

Base authority: W5A PASS / LOCKED at `995d280db154503724185460425af29fceeb7ad1`.

Lock-authorizing implementation head: `c14184c1c51b5f07392d12e1af72c3e68d057a5f`.

## Product UX boundary

For the active WEB-only product path, setup and TUI expose browser/session concepts only. Users never choose or enter an AI API provider, API key, Base URL, API protocol, provider fallback, or Ollama/local inference path.

Initial supported AI website: Gemini Web.

## First-run setup

The setup wizard now:
1. chooses creative language;
2. explains that AI runs through the user's visible logged-in Gemini Web session;
3. optionally accepts a Chrome executable path (blank = auto-detect);
4. accepts a persistent browser profile name (default `default`);
5. saves `web.enabled=true`, `web.site=gemini-web` and no `providers` credential entries.

It does not ask for API Key, Base URL, provider protocol, provider name, or model ID.

## `/model`

In WEB-only mode `/model` is a read-only browser AI status surface, not a provider/model switcher. It reports non-secret browser/session information such as:
- transport: WEB-only;
- site/model label: Gemini Web;
- readiness state (STARTING/AUTH_REQUIRED/READY/BUSY/DEGRADED/FAILED/STOPPED);
- browser PID/profile where useful;
- manual-login guidance for AUTH_REQUIRED.

Provider/model switching and API fallback are not reachable from this surface.

## `/config`

In WEB-only mode `/config` edits browser settings only:
- Chrome executable path (blank = auto-detect);
- persistent profile name;
- site fixed to `gemini-web` in W5B.

Changes are persisted safely and take effect on the next application restart; the current owned browser session is not silently replaced during an active writing run.

No cookies, Google credentials, session tokens, API credentials, Base URLs, or provider definitions are displayed or accepted.

## Example configuration contract

All shipped example configs are synchronized and WEB-only. After JSONC comments are removed they parse as valid JSON, contain `web.enabled=true` + `web.site=gemini-web`, contain no provider credential registry, and resolve through `FillDefaults()` to the compatibility runtime identity `web/gemini-web` before `ValidateBase()` passes.

## Compatibility boundary

Legacy provider-shaped internals remain temporarily compiled only for W5C migration/removal. W5B makes them unreachable from a newly configured WEB-only product. W5C is the blocking gate that deletes the legacy API runtime itself.

## Verification evidence

Lock authorization was established on clean implementation head `c14184c1c51b5f07392d12e1af72c3e68d057a5f` by GitHub Actions CI run **#68** (`34013675990`):

- Ubuntu 24.04 / Go 1.25.5:
  - gofmt: PASS
  - installer syntax: PASS
  - `go vet ./...`: PASS
  - `go test -buildvcs=false -count=1 ./...`: PASS
  - critical-state race tests: PASS
- Windows / Go 1.25.5:
  - gofmt: PASS
  - `go vet ./...`: PASS
  - `go test -buildvcs=false -count=1 ./...`: PASS
- deterministic setup/config tests proving no API credential/provider entry is created: PASS
- TUI WEB-only `/model` non-switching contract: PASS
- TUI WEB-only `/config` browser-only contract: PASS
- browser config persistence contract: PASS
- temporary CI repair workflows used during development were removed before the lock-authorizing run: PASS

W5B is locked. The lock commit itself must also remain green under the repository's standard CI before integration into the W5 branch. W5C may open only after that integration succeeds.
