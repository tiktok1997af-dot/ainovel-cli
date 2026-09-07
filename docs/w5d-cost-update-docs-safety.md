# W5D — COST / UPDATE / DOCS SAFETY

Status: **W5D-A AUDIT PASS / LOCKED**

Authority base: `24dd076bbb3db0b55b63b4d5dbad30a5077af05c`
Working branch: `w5d-cost-update-docs-safety`

W5D is a safety cleanup stage. It MUST NOT change the locked browser bridge contract from W1–W5C and MUST NOT introduce an AI API, API key, provider fallback, Ollama execution path, hidden browser, credential extraction, or website-side local file execution.

## 1. Locked product facts

The only AI execution path remains:

```text
Engine / Workers
  -> agentcore.ChatModel
  -> WebChatModel
  -> owned visible Chrome / persistent profile
  -> logged-in Gemini Web
  -> deterministic response/tool protocol
  -> local Tools / Store
```

`WebChatModel` explicitly reports provider usage telemetry as unsupported. Its response contains no authoritative token count, cache billing, or dollar cost. W5D MUST NOT reconstruct API billing from an API-era model catalog and MUST NOT present a zero-dollar value as proof that the browser account incurred no cost.

## 2. Audit classification

| Area | Current residue | Classification | Locked action |
|---|---|---|---|
| `internal/models` pricing/catalog | OpenRouter HTTP pricing refresh, vendor/model prices, cache prices, API-era context metadata | **REMOVE** | Remove active OpenRouter pricing/catalog dependency once callers are removed. No provider metadata HTTP refresh in WEB-only product. |
| `internal/host/usage.go` | provider/model billing resolution, USD cost/savings, provider prompt-cache diagnostics, OpenAI `include_usage` assumptions | **REWRITE WEB-ONLY** | Retain only genuinely provider-neutral/local telemetry if mechanically available. Missing web Usage is normal, not an error. Do not display invented token/cost/cache values. |
| `internal/host/usage_replay.go` + persisted usage | replays provider/model Usage and API billing history | **REWRITE WEB-ONLY** | Current WEB runtime must not rebuild API-dollar cost from old sessions. Historical API-era billing data must not be treated as current browser billing. |
| `BudgetConfig` / `BudgetSentinel` | `budget.book_usd`, warn ratio, hard-stop based on reconstructed USD cost | **REMOVE** | Remove active USD budget/stop policy. Legacy `budget` config must fail loudly with migration guidance because browser Gemini does not expose authoritative billing telemetry. |
| `internal/version/update.go` generic updater | verified GitHub release download + checksum/staging | **RETAIN PROVIDER-NEUTRAL** | Keep generic updater implementation. |
| `cmd/ainovel-cli` self-update caller | hard-coded `voocel/ainovel-cli` | **REWRITE WEB-ONLY** | Update only from `tiktok1997af-dot/ainovel-cli`; add regression protection so upstream cannot overwrite this fork. |
| `scripts/install.sh` | raw/release URLs hard-coded to `voocel/ainovel-cli` | **REWRITE WEB-ONLY** | Install only this fork's release artifacts/checksums. |
| Docker compose/runtime packaging | compose pulls `ghcr.io/voocel/ainovel-cli`; Alpine runtime has no owned visible Chrome | **REMOVE AS SUPPORTED RUNTIME** | Do not advertise or publish Docker as a supported WEB-only runtime until an explicit visible-browser bridge exists. No upstream image pull is allowed. |
| release notes workflow | injects Gemini/Anthropic/OpenAI API keys and script directly calls those APIs | **REMOVE AI API BEHAVIOR** | Generate release notes deterministically from Git history; no AI API secrets or network model calls. |
| `README.md` | API-key/provider/Ollama/multi-model/Docker setup instructions | **REWRITE WEB-ONLY** | Document Chrome + manual Gemini Web login, browser profile, no API key, `/model` read-only status and `/config` Chrome/profile only. |
| `HUONG_DAN_SU_DUNG.md` | API/Ollama/provider switching and Docker instructions | **REWRITE WEB-ONLY** | Same product truth as README; remove unsupported execution paths. |
| active TUI `/model` + `/config` help presentation | already says Gemini Web status and Chrome/profile configuration | **RETAIN WEB-ONLY** | Preserve and add regression coverage that active help does not advertise API keys/Base URLs/Ollama/provider switching. |
| `docs/architecture.md` | stale budget/model-management/usage wording | **REWRITE WEB-ONLY** | Keep Engine/Workers/Tools/Store architecture; replace model management/budget semantics with browser-session/readiness semantics. |
| `docs/context-management.md` | mostly provider-neutral, but mentions API-call waste/provider Usage | **RETAIN + TARGETED REWRITE** | Preserve local context strategy; describe browser turns and local estimates instead of API billing/usage. |
| `docs/import-pipeline.md` | mostly provider-neutral, some stale model/usage assumptions | **RETAIN + TARGETED REWRITE** | Preserve semantic import architecture; fixed `web/gemini-web` identity, no billing telemetry. |
| `docs/observability.md` | local Store/diagnostic workflow | **RETAIN PROVIDER-NEUTRAL** | Preserve unless a stale API execution claim is found by final audit. |
| `docs/prompt-cache-design.md` | current-tense LiteLLM/OpenAI/Anthropic API caching and pricing design | **REMOVE FROM CURRENT PRODUCT DOCS** | The browser bridge deliberately does not expose/control provider prompt-cache protocol. Historical W0–W5 migration docs are sufficient provenance. |
| W0–W5 browser/audit/proof docs | historical evidence describing removed API-era code and locked WEB-only migration | **RETAIN AS PROVENANCE** | API terms inside explicit historical audit/proof documents are allowed evidence, not supported product instructions. |

## 3. D1 — Cost / usage / budget safety gate

PASS requires all of the following:

1. No outbound OpenRouter pricing/model metadata refresh in active product code.
2. No active API-price based USD cost or cache-savings calculation.
3. No active `budget.book_usd` safety gate or `BudgetSentinel` pretending to enforce browser-account spending.
4. Missing `agentcore.Usage` from `WebChatModel` is treated as expected capability, not an OpenAI streaming failure.
5. User-facing UI does not display authoritative-looking `$0`, token totals, provider cache hit rates, or budget protection when Gemini Web did not provide such telemetry.
6. Legacy budget/API billing configuration fails loudly rather than silently changing behavior.

## 4. D2 — Update / install / release safety gate

PASS requires all of the following:

1. `ainovel-cli update` can only fetch releases from `tiktok1997af-dot/ainovel-cli`.
2. Installer examples and release downloads can only target this fork.
3. No supported install/runtime path pulls `voocel/ainovel-cli` or another upstream binary/image.
4. Release automation contains no Gemini/Anthropic/OpenAI API key, endpoint, or AI changelog generation.
5. Release notes are generated deterministically from Git history.
6. Docker is not advertised/published as a functional WEB-only runtime while it cannot own the required visible Chrome session.

## 5. D3 — Documentation / help truthfulness gate

PASS requires all of the following:

1. README and Vietnamese user guide describe WEB-only Gemini/Chrome login, not API/Ollama/provider setup.
2. First-run instructions match the implemented browser-only setup wizard.
3. `/model` is documented as read-only Gemini Web/browser status.
4. `/config` is documented as Chrome/profile settings only.
5. No user-facing current-product documentation instructs users to enter AI API keys, Base URLs, choose API providers, configure Ollama, or depend on API-dollar budget enforcement.
6. Historical migration/audit documents remain clearly historical and may contain old API terminology as evidence.

## 6. D4 — Verification gate

W5D may be called PASS / LOCKED only after:

- implementation D1–D3 is complete;
- `gofmt` / `go vet ./...` / `go test ./...` pass;
- Linux race critical paths pass;
- Windows full tests pass;
- a repository-wide W5D residue audit verifies no active AI API/release-secret/upstream-overwrite/current-doc instruction path remains;
- all temporary patch/audit workflows or scripts are removed;
- final CI passes again on the exact W5D lock head.

Only then may W5D be integrated into `w5-web-only-integration-api-removal` and W5.5 be opened.
