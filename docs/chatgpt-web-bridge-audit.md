# Web-Only AI Transport Architecture Audit

Status: **W0 LOCKED — WEB-ONLY / NO API**

Repository: `tiktok1997af-dot/ainovel-cli`
Branch: `chatgpt-dev`
Baseline: `16952591683dc5d99564cde5ab301d219c526469`

## 1. Author requirement

The fork must remove API-based AI execution entirely.

There is no supported runtime mode that asks the user for an AI API key, sends prompts to an AI provider HTTP API, falls back to an API provider, or uses API billing/budget semantics.

The only AI execution path is:

1. The Tool opens or attaches to a real browser session.
2. The user signs in to the AI website manually with their own account.
3. The Tool submits model prompts through that logged-in web session.
4. The Tool captures the web response.
5. The Tool converts that response back into the existing local `agentcore.ChatModel` contract.
6. All novel state, tools, checkpoints and artifacts remain local and authoritative.

The website is only an AI transport/intermediary. It is never the source of truth for project data or workflow state.

## 2. Existing architecture to preserve

```text
TUI / headless
    -> host.Host
       -> deterministic Engine / Arbiter
          -> architect / writer / editor
             -> agentcore.ChatModel

Workers -> Tools -> Store
                -> Progress
                -> Checkpoints
                -> Artifacts
```

Architect, Writer and Editor already receive an injected `agentcore.ChatModel`. They do not need to know whether the model is reached through an API or a browser.

`internal/bootstrap/models.go` is the current centralized model construction boundary. The existing API implementation ends in `llm.NewModel(...)`; the web-only fork will replace that construction path rather than teaching every agent about browser automation.

## 3. Locked target architecture

```text
TUI / headless
      |
    Host
      |
Engine / Arbiter / Workers
      |
agentcore.ChatModel
      |
 WebChatModel
      |
 BrowserSessionManager
      |
 persistent Chrome profile
      |
 logged-in AI website
      |
 response capture
      |
 local protocol parser
      |
 text or ToolCallBlock
      |
 local Tools -> local Store
```

There is deliberately **no API ChatModel branch**.

### Core rule

Do not teach Architect, Writer, Editor, Flow, Tools or Store how to automate a browser. Browser behavior stays behind `WebChatModel` and site/session adapters.

## 4. API functionality to remove

The final fork must not expose or rely on the following as supported product behavior:

- AI provider API keys.
- OpenAI / Anthropic / Gemini / OpenRouter / DeepSeek / Qwen / GLM / Grok / Bedrock AI HTTP provider configuration.
- API base URL configuration for AI execution.
- API protocol selection (`openai`, `anthropic`, `gemini`) for AI execution.
- API provider failover.
- API connection tests.
- API usage-price accounting and API budget hard-stop semantics.
- API model switching that constructs HTTP clients.
- Self-update behavior that can overwrite this fork with upstream API-oriented binaries.

Ollama is also outside the locked web-only product path. The goal is not “free API/local inference”; the goal is “logged-in web account as the only AI transport”.

If API-related types remain temporarily during migration only to keep intermediate commits compiling, they are transitional implementation residue and must be removed or made unreachable before W6.

## 5. WebChatModel contract

`internal/webai/model.go` will implement the existing `agentcore.ChatModel` boundary:

- `Generate(...)`
- `GenerateStream(...)`
- `SupportsTools()`
- model identity / capability reporting needed by the current runtime

For v0.1, `GenerateStream` may wait for the complete stable web response and then emit a final stream event. Correctness takes priority over visual token-by-token DOM streaming.

The web model should report native JSON Schema as unsupported/unknown initially. Existing `internal/llmcontract` can use prompt-contract mode and perform local JSON extraction, schema validation and correction.

## 6. Critical compatibility requirement: local tool calling

The browser adapter cannot merely scrape prose.

Writer and other Workers rely on local tools such as:

- `plan_chapter`
- `draft_chapter`
- `edit_chapter`
- `check_consistency`
- `commit_chapter`
- review / summary / foundation tools

A model turn may need to become a real `agentcore.ToolCallBlock` with `StopReasonToolUse` so that the local runtime executes the tool and returns the result into the next model turn.

Therefore WebChatModel requires a deterministic textual tool protocol:

1. Serialize conversation messages and the currently allowed `ToolSpec` definitions into the web prompt.
2. Require one of two response envelopes: normal assistant text or a strict tool-call envelope.
3. Parse the envelope locally.
4. Validate tool name against the exact locally supplied tool registry.
5. Validate arguments against the local tool schema before execution.
6. Convert valid calls to `agentcore.ToolCallBlock` + `StopReasonToolUse`.
7. Feed local tool results back into the following WebChatModel request.
8. Never let a website directly mutate the Store or execute arbitrary commands.

Without this layer the existing multi-agent engine would not be behaviorally compatible.

## 7. Browser/session policy

The first supported implementation is native desktop browser automation with a persistent Chrome profile.

- The Tool launches or attaches to Chrome.
- The user performs login manually in the visible browser window.
- The browser profile is persisted and reused on restart.
- Account passwords are never stored in project config.
- The Tool does not bypass CAPTCHA, 2FA, anti-bot or account security challenges.
- Expired login or a security challenge produces an explicit authentication state and waits for user action.
- Site-specific DOM selectors and behavior live behind narrow site adapters because consumer web UIs change over time.

Required readiness states:

```text
STARTING
AUTH_REQUIRED
READY
BUSY
DEGRADED
FAILED
STOPPED
```

Docker is not the primary web-login runtime for v0.1. Native desktop execution is the supported path because a visible persistent browser session is part of the product requirement.

## 8. Request lifecycle

```text
Engine asks model to generate
        |
WebChatModel serializes messages/tools
        |
BrowserSessionManager ensures READY
        |
SiteAdapter opens/reuses conversation
        |
submit prompt
        |
wait for response generation
        |
capture stable final response
        |
Protocol parser
   |             |
 text        tool envelope
                 |
         local validation
                 |
        ToolCallBlock
                 |
        local Tool execution
                 |
           local Store
                 |
        next model turn
```

The website never receives direct filesystem authority.

## 9. Error mapping

Web transport must preserve retry/cancel semantics expected by the current runtime.

Retryable examples:

- temporary browser/page failure,
- stale DOM or navigation race,
- transient response timeout,
- temporary website load failure,
- page reload required.

Non-retryable / user-action states:

- login required,
- CAPTCHA / 2FA / security challenge,
- account restriction,
- unsupported site layout after bounded detection,
- malformed tool envelope after bounded correction,
- browser executable/profile unavailable.

Retryable web errors should implement the existing `agentcore.RetryableError` contract and optionally `RetryHinter` where useful.

## 10. Usage semantics in web-only mode

Consumer web subscriptions are not API billing.

The fork must not pretend that web calls have authoritative API token counts or API dollar costs.

For web-only mode:

- remove API pricing/budget claims from the active product flow,
- do not record unknown web billing as authoritative `$0`,
- keep operational counters only when useful (requests, elapsed time, failures, retries),
- label any estimated text/token metrics clearly as local estimates rather than provider billing data.

## 11. Implementation surface

### New package — required

| Path | Responsibility |
|---|---|
| `internal/webai/model.go` | `agentcore.ChatModel` implementation |
| `internal/webai/protocol.go` | message/tool serialization and validated response parsing |
| `internal/webai/errors.go` | typed browser/auth/site/retry errors |
| `internal/webai/session.go` | browser process and persistent profile lifecycle |
| `internal/webai/state.go` | browser/auth/readiness state machine |
| `internal/webai/sites/site.go` | site adapter interface |
| `internal/webai/sites/<target>.go` | first concrete AI website adapter |
| `internal/webai/*_test.go` | deterministic fake-transport and protocol tests |

### Existing files — mandatory migration

| Path | Required change |
|---|---|
| `go.mod` | add chosen browser/CDP dependency; remove direct API-only dependencies when no longer referenced |
| `internal/bootstrap/config.go` | replace provider/API-key centered configuration with web/browser/site/profile configuration |
| `internal/bootstrap/config.example.jsonc` | replace API examples with web-login configuration |
| `internal/bootstrap/setup.go` | remove provider/API-key wizard; create browser/site/profile setup flow |
| `internal/bootstrap/models.go` | remove `llm.NewModel(...)` API construction path; construct only WebChatModel instances |
| `internal/host/host.go` | own browser/model lifecycle and expose readiness/auth state |
| `internal/host/model_config.go` | replace provider/API model management with web session/site/model identity management |
| `internal/entry/tui/command_config.go` | show browser path, profile, site, auth/readiness; remove API-key UI |
| `internal/entry/tui/command_config_test.go` | replace API configuration tests with web-session tests |
| `internal/entry/tui/command_model.go` | adapt model selection semantics to models available through the chosen web UI, where reliably selectable |
| `internal/host/usage.go` | remove API-cost authority from active web-only behavior; retain only truthful local observability |
| `cmd/ainovel-cli/main.go` | disable or repoint upstream self-update so customized web-only builds cannot be overwritten |
| `README.md` | rewrite installation/configuration around web login; state no API key is required or supported |
| `HUONG_DAN_SU_DUNG.md` | document login, persistent session, re-auth, browser recovery and supported sites |
| `.github/workflows/ci.yml` | keep CI deterministic with fake browser/site adapters; no real account credentials in CI |

### Subsystems that should remain behaviorally unchanged

- `internal/flow/*`
- `internal/store/*`
- `internal/tools/*`
- core worker responsibilities in `internal/agents/build.go`
- `internal/llmcontract/*`
- most of `internal/arbiter/*`
- checkpoint / recovery semantics
- novel artifact formats

The migration should change how AI is reached, not how novel facts are managed.

## 12. Migration gates

| Gate | Deliverable | Pass condition |
|---|---|---|
| W0 | Web-only architecture lock | This document states no API runtime is allowed |
| W1 | WebChatModel + deterministic tool protocol | Unit tests cover text, tool call, invalid call, cancel and typed errors with fake transport |
| W2 | Browser lifecycle + persistent login | `AUTH_REQUIRED` and `READY` are deterministic across clean and persisted profiles |
| W3 | Prompt submit + response capture | One prompt can be submitted and final response captured with cancel/timeout/retry behavior |
| W4 | Local tool-use E2E | Web model requests a local tool, Tool executes locally, result returns to model, bounded Worker task completes |
| W5 | Web-only bootstrap/TUI migration | No API key/provider setup remains in normal UI; restart preserves browser profile and readiness state |
| W5.5 | API removal gate | Search/build tests confirm API construction, API provider fallback and API-key product paths are unreachable/removed |
| W6 | Full regression/release | Engine/Flow/Tools/Store tests pass, web tests pass, docs are web-only, update path is safe, PR ready |

## 13. Risks

### High — tool-call emulation

Consumer AI websites do not expose the same native tool-call wire protocol as APIs. The local protocol/parser is the most important compatibility component.

### High — site DOM drift

Websites can change selectors and generation behavior without notice. Site adapters must fail visibly and be independently replaceable.

### Medium — authentication lifecycle

Persistent manual login is straightforward; reliable detection of logout/challenge/ready states across restarts requires a strict state machine.

### Medium — context/conversation drift

A consumer web conversation may retain UI-side history. WebChatModel must intentionally control conversation reuse/new-thread behavior so the website does not become an uncontrolled second memory system.

### Medium — upstream merges

Future upstream changes touching model/bootstrap/TUI configuration will be conflict hotspots because this fork deliberately removes API-oriented product behavior.

## 14. Locked decision

**WEB-ONLY. API execution is removed from the product requirement.**

The project keeps its deterministic Engine, multi-agent Workers, local Tools, Store and recovery architecture. The single AI transport is a browser-backed `WebChatModel` using a persistent logged-in user session.

No API fallback is permitted.

The next implementation gate is **W1 — WEB CHATMODEL CONTRACT & DETERMINISTIC TOOL PROTOCOL v0.1**. W1 must be completed with fake/deterministic transport tests before live site DOM automation begins.