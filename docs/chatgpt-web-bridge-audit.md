# AI Transport / Web Bridge Architecture Audit

Status: **AUDIT BASELINE — no runtime code changed**

Repository: `tiktok1997af-dot/ainovel-cli`
Branch: `chatgpt-dev`
Baseline: `16952591683dc5d99564cde5ab301d219c526469`

## 1. Goal

Preserve the existing deterministic Engine, multi-agent workflow, tools, checkpoints and filesystem Store, while allowing AI requests to use a browser/web session authenticated by the user instead of requiring an API key.

The web UI must remain a transport/intermediary only. It must not become the source of truth for novel state, workflow state or tool side effects.

## 2. Current request path

```text
TUI / headless
    -> host.Host
       -> deterministic Engine / Arbiter
          -> architect / writer / editor
             -> agentcore.ChatModel
                -> bootstrap.ModelSet
                   -> createModelFromConfig(...)
                      -> litellm llm.NewModel(...)
                         -> provider HTTP API

Workers -> Tools -> Store remains local and authoritative.
```

The project does not scatter direct API calls through each creative agent. `internal/agents/build.go` receives models from `bootstrap.ModelSet`, and `internal/bootstrap/models.go` is the centralized model construction boundary. This is the safest insertion point for a second transport.

## 3. Recommended target architecture

```text
Engine / Arbiter / Workers
          |
    agentcore.ChatModel
          |
    bootstrap.ModelSet
       /          \
 API ChatModel     Web ChatModel
 (existing)        (new)
                       |
             BrowserSessionManager
                       |
              persistent Chrome profile
                       |
              target AI web application
```

### Core rule

Do **not** teach Architect, Writer, Editor, Flow, Tools or Store how to automate a browser. The browser implementation must satisfy the existing `agentcore.ChatModel` boundary.

## 4. Why this is feasible

The project already depends on the following useful abstractions:

- `agentcore.ChatModel` is injected into subagents.
- `bootstrap.ModelSet` centralizes model selection, swapping and failover.
- Arbiter direct structured calls go through `internal/llmcontract`.
- `llmcontract` already has a `prompt_contract` path when native JSON Schema is unavailable.
- `internal/llmretry` only needs a minimal `Generate` interface and typed retryable errors.
- TUI and headless both call `host.New(...)`; neither entry point owns provider HTTP logic.

Therefore the core story engine can remain intact.

## 5. Hardest compatibility requirement: tool calls

A browser adapter cannot merely scrape prose from a web page.

Writer and the other Workers use `agentcore` tools. A model turn may need to return a real `agentcore.ToolCallBlock` with `StopReasonToolUse`, after which the local tool executes and its result is fed to the next model turn.

A Web ChatModel therefore needs a deterministic textual tool protocol:

1. Serialize current messages and available `ToolSpec` definitions into the web prompt.
2. Require the web model to return either normal assistant text or a strict tool-call envelope.
3. Parse and validate that envelope locally.
4. Convert it to `agentcore.ToolCallBlock` and `StopReasonToolUse`.
5. Never execute a browser-produced tool name/arguments without validating it against the locally supplied tool registry/schema.

Without this layer, `plan_chapter`, `draft_chapter`, `commit_chapter`, reviews and other existing Worker behavior will break.

## 6. Structured Arbiter output

A browser-backed model should initially declare native JSON Schema as unsupported/unknown. Existing `internal/llmcontract` will then use its prompt-contract mode, append the schema to the prompt, extract JSON from the returned text and perform local validation/correction.

No rewrite of Arbiter business logic is required for the first web-transport version.

## 7. Browser/session policy

Recommended first implementation is a native desktop Chrome/CDP session with a persistent user-data directory.

- User performs login manually in the real browser window.
- The application reuses that browser profile/session on later runs.
- Do not store account passwords in project config.
- Do not bypass CAPTCHA, 2FA, anti-bot or security challenges.
- If authentication expires or a security challenge appears, return a typed non-retryable authentication error and surface an explicit re-login state.
- Website selectors and conversation behavior belong behind site-specific adapters because web DOMs are inherently unstable.

For the first release, native Windows/macOS/Linux should be the supported web mode. Docker/browser-login support should not be treated as equivalent to the current API/Ollama Docker workflow.

## 8. Streaming strategy

For v0.1, correctness is more important than visual token streaming.

`GenerateStream` may submit the web request, wait until the final response is stable, then emit a completed stream event. True DOM-delta streaming can be added later after the transport and tool-call contracts are stable.

## 9. Error mapping

Web transport errors must preserve the retry semantics expected by `internal/llmretry` and the subagent runtime.

Retryable examples:

- temporary page/network failure,
- stale DOM / navigation race,
- transient response timeout,
- target page reload required.

Non-retryable examples:

- login required,
- account/security challenge,
- CAPTCHA/2FA awaiting user,
- unsupported site layout/version,
- malformed tool-call protocol after bounded correction,
- permission/account restriction.

Retryable web errors should implement the existing `agentcore.RetryableError` contract and optionally `RetryHinter`.

## 10. Usage and budget semantics

A consumer web UI normally does not provide authoritative API token/cost usage metadata. The application must not silently record web calls as real `$0` API usage and then claim budget enforcement is active.

For web mode:

- mark provider token/cost data as unavailable unless it can be measured reliably,
- display transport usage separately from API billing usage,
- disable or explicitly mark API-cost hard-stop as unavailable for the web provider,
- avoid repeated generic "missing usage" warnings on every normal web response.

## 11. Exact implementation surface

### New files/packages — required

| Path | Responsibility |
|---|---|
| `internal/webai/model.go` | `agentcore.ChatModel` implementation: `Generate`, `GenerateStream`, `SupportsTools`, model info/capabilities |
| `internal/webai/protocol.go` | Serialize conversation/tool specs and parse validated text/tool-call envelopes |
| `internal/webai/errors.go` | Typed auth, site, timeout and retryable transport errors |
| `internal/webai/session.go` | Browser process/profile/session lifecycle |
| `internal/webai/sites/site.go` | Site adapter interface (`AuthState`, submit, wait/capture response, new conversation) |
| `internal/webai/sites/<target>.go` | DOM adapter for the first supported AI website |
| `internal/webai/*_test.go` | Deterministic protocol/model/session tests with fakes; no live account in CI |

A Go-native CDP library is preferable for the first implementation to preserve a single-binary architecture. A Node/Playwright sidecar is possible but increases packaging and lifecycle complexity.

### Existing files — mandatory changes

| Path | Required change |
|---|---|
| `go.mod` | Add selected browser/CDP dependency if using a Go-native browser worker |
| `internal/bootstrap/config.go` | Add transport kind (`api` / `web`) and web-session configuration; validation must not require API key/litellm provider type for web transport |
| `internal/bootstrap/config.example.jsonc` | Document web-login configuration separately from API providers |
| `internal/bootstrap/setup.go` | First-run choice between API/local provider flow and Web Login flow |
| `internal/bootstrap/models.go` | Dispatch model construction: existing `llm.NewModel` for API, `webai.NewModel` for web; own/close web model resources correctly |
| `internal/host/host.go` | Browser/model lifecycle shutdown; web transport readiness/auth state; budget/usage integration |
| `internal/host/model_config.go` | Extend provider snapshots/drafts and connection testing to web auth/readiness instead of API-key-only semantics |
| `internal/entry/tui/command_config.go` | TUI controls/status for transport type, browser profile and login/readiness |
| `internal/entry/tui/command_config_test.go` | Test web configuration UI/state transitions |
| `internal/host/usage.go` | Represent unavailable web token/cost data without treating it as authoritative zero-cost API usage |
| `cmd/ainovel-cli/main.go` | Disable or repoint self-update: it currently targets `voocel/ainovel-cli`, which could replace a customized fork with upstream binaries |
| `README.md` | Describe the fork and web-login workflow; remove API-only assumptions where applicable |
| `HUONG_DAN_SU_DUNG.md` | Add browser login/session/re-auth/recovery documentation |

### Existing files — likely small/conditional changes

| Path | Possible change |
|---|---|
| `internal/entry/tui/command_model.go` + tests | Display/select web-backed model/site identity and auth status |
| `.github/workflows/ci.yml` | Keep browser protocol tests deterministic; never require a real logged-in web account in CI |
| `Dockerfile` / Docker docs | Declare web mode unsupported initially or provide a separate explicit GUI/browser architecture later |
| `.goreleaser.yml` | Validate release packaging after browser dependency is introduced |

### Files/subsystems that should remain unchanged in v0.1

- `internal/flow/*`
- `internal/store/*`
- `internal/tools/*`
- core creative behavior in `internal/agents/build.go`
- `internal/llmcontract/*`
- most of `internal/arbiter/*`
- existing prompt/content assets unless web tool-protocol prompting proves a narrowly scoped addition is required

Changing those areas would increase regression risk without helping the transport problem.

## 12. Recommended implementation gates

| Gate | Deliverable | Pass condition |
|---|---|---|
| W0 | This audit/spec | Architecture boundary and file surface frozen before runtime edits |
| W1 | WebChatModel + protocol with fake transport | Unit tests prove text response, tool call, invalid tool call, cancellation and typed errors |
| W2 | Browser session + manual login + first site adapter | Persistent profile can reach `AUTH_REQUIRED` and `READY` deterministically without bypassing security controls |
| W3 | Request/response capture | One prompt can be submitted and final answer captured; cancel/timeout/retry semantics verified |
| W4 | Tool-use E2E | Browser model can request a local tool; local tool result returns to model; a bounded Worker task completes |
| W5 | Bootstrap/TUI integration | Web transport selectable without API key; restart preserves profile/config and reports readiness accurately |
| W6 | Regression/release | Existing API/Ollama path still passes tests; web tests pass; docs/update behavior fixed; PR ready for merge |

## 13. Risks

### High — web tool calling

Consumer chat websites do not expose the same native tool-call wire protocol as API models. The local text protocol/parser is therefore a critical compatibility layer and must be schema-validated.

### High — DOM/site drift

Website UI selectors, message containers and loading states can change. Keep every site-specific selector behind a narrow adapter and fail visibly rather than guessing.

### Medium — session/auth lifecycle

Manual login is easy; reliable detection of logged-out/challenge/ready states across restarts is harder. Make it an explicit state machine.

### Medium — usage/cost observability

Web subscriptions and API billing are different systems. Avoid implying that API budget calculations apply to a consumer web session.

### Medium — upstream updates

This fork should continue receiving upstream changes, but changes touching `bootstrap/models.go`, `config.go`, Host model lifecycle or TUI model configuration are expected merge-conflict hotspots.

## 14. Decision

**GO, with a transport-adapter architecture.**

The requested web-login direction does not require rewriting the novel engine. The safest implementation is to preserve `agentcore.ChatModel` as the seam, add a browser-backed implementation beneath `ModelSet`, and leave Engine/Flow/Tools/Store authoritative and provider-agnostic.

The first code gate should be **W1 — WebChatModel contract + deterministic tool protocol tests**, before any live DOM automation is added.
