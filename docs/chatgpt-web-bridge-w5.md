# W5 — FULL WEB-ONLY INTEGRATION & API REMOVAL v0.1

Status: OPEN

This stage turns the browser implementation proven in W1–W4 into the only active AI path of ainovel-cli.

## Non-negotiable product boundary

- AI transport is the user's visible logged-in browser session only.
- No AI API keys.
- No OpenAI/Gemini/Anthropic/OpenRouter/provider HTTP API fallback.
- No Ollama/local-inference provider path in the active product.
- The web page never receives direct filesystem or shell authority.
- Local tools remain schema-validated and executed only by the local ainovel runtime.
- No CAPTCHA, 2FA, anti-bot, or login-security bypass.

## W5A — Runtime bootstrap migration

Replace provider/API model construction with one browser-owned runtime:

`Config -> SessionManager -> GeminiWebTransport -> WebChatModel -> ModelSet -> Agents/Arbiter`

Requirements:
- persistent Chrome profile;
- browser session lifetime owned by the application runtime and closed on Host.Close;
- all roles use the web model unless a future website-native selector is explicitly implemented and verified;
- no API fallback/failover;
- context compaction remains local and provider-neutral.

Gate: production Host can boot with WEB-only configuration and no API credential.

## W5B — Setup and TUI migration

First-run setup and runtime configuration must expose browser concepts only:
- site (initially `gemini-web`);
- Chrome executable path (optional auto-detect);
- browser profile name;
- login/readiness status;
- existing creative language/style settings where applicable.

Remove API key, Base URL, provider protocol, API model registry and API connection-test UX.

The old `/model` workflow must not imply provider/API switching. It may be removed or repurposed to show the active web session/model label only.

## W5C — Legacy API runtime removal

Remove or make unreachable, then delete:
- `llm.NewModel` API construction;
- `WithAPIKey`, `WithBaseURL`, provider-extra/body configuration;
- API provider presets and API credential persistence;
- role-level provider/model fallback routing;
- Ollama/Bedrock/API-provider active paths;
- provider-rate-limit/network failover semantics that resubmit to another provider.

Legacy configs containing API-era keys must fail with a clear migration message; they must never be silently used.

## W5D — Cost/update/docs safety

- Disable API pricing refresh and API-dollar budget semantics when usage/cost is unavailable from the website.
- Keep token/usage accounting only where locally observable and truthful; never invent web billing data.
- Disable upstream self-update that could overwrite this customized WEB-only fork.
- Rewrite example config and user docs for browser login.

## W5.5 — NO-API AUDIT (blocking)

Repository-wide audit must prove the released product has no active AI API path.

Search/audit terms include at minimum:
- `api_key`, `APIKey`, `WithAPIKey`;
- `base_url`, `WithBaseURL`;
- `llm.NewModel`;
- OpenAI/Anthropic/Gemini API/OpenRouter/DeepSeek/Qwen/GLM/Grok/Bedrock/Ollama provider setup;
- provider HTTP fallback/failover;
- upstream self-update target.

Test/verifier-only historical references are allowed only when they cannot be reached by the product runtime and do not contain credentials.

Gate: deterministic Linux+Windows CI PASS + repository audit PASS + real Windows product smoke PASS.

## W5E — Real product smoke

The actual `ainovel-cli` build (not only a dedicated verifier) must:
1. start with no AI API credential;
2. locate/open the persistent Chrome profile;
3. reach `READY` on the logged-in Gemini Web session;
4. execute a bounded real WebChatModel request through the production bootstrap;
5. close cleanly without orphaning the owned browser session.

Only after W5A–E and W5.5 PASS may W5 be AUTHOR-CONFIRMED / LOCKED and W6 open.
