# W1 — Web ChatModel Contract & Deterministic Tool Protocol — Verification Record

Status: **IMPLEMENTATION COMPLETE / RUNTIME VERIFICATION HOLD**

Repository: `tiktok1997af-dot/ainovel-cli`
Branch: `chatgpt-dev`
Architecture authority: `docs/chatgpt-web-bridge-audit.md`

## 1. Scope completed

W1 implements the browser-neutral contract layer only. It deliberately does not contain live Chrome/DOM automation.

Implemented package:

- `internal/webai/model.go`
- `internal/webai/protocol.go`
- `internal/webai/errors.go`
- `internal/webai/model_test.go`
- `internal/webai/protocol_test.go`

The model boundary remains `agentcore.ChatModel`. Browser/site implementation is deferred to W2/W3 behind `Transport`.

## 2. Source-contract verification completed

The implementation was checked against the actual `github.com/voocel/agentcore v1.8.2` contracts used by this repository.

Verified signatures/contracts:

- `agentcore.ChatModel`: `Generate`, `GenerateStream`, `SupportsTools`.
- `agentcore.ProviderNamer` and `agentcore.ModelNamer`.
- `llm.CapabilityProvider` and `llm.Capabilities`.
- `agentcore.ToolCall` fields `ID`, `Name`, `Args`.
- `subagent.Runner.Run(ctx, agent, task)`.
- `agentcore.NewFuncTool(...)`.
- `agentcore.RetryableError` and `agentcore.RetryHinter`.
- Existing `llmcontract.Plan` falls back to `prompt_contract` when native JSON Schema is reported unsupported.

Compile-time interface assertions are present in W1 source so an actual Go compiler will reject interface drift.

## 3. Security/data-boundary verification completed

The web request projection deliberately excludes legacy provider-only or internal fields.

Not sent to the web transport:

- API keys.
- provider session IDs.
- prompt-cache routing keys.
- arbitrary provider-specific tool-choice objects.
- `Message.Usage`.
- message timestamps.
- arbitrary `Message.Metadata`.
- provider telemetry/model billing metadata.
- `ToolSpec.Strict`.
- `ToolSpec.DeferLoading`.

Only the semantic conversation/tool state needed by the browser AI is projected.

Tool results keep only the local correlation fields required to continue a tool-call transcript:

- `tool_call_id`
- `tool_name`
- `is_error`

The website never receives direct filesystem/Store authority.

## 4. Deterministic tool protocol implemented

The browser AI is required to return exactly one bounded response envelope.

Normal text:

```text
<<<AINOVEL_WEB_RESPONSE>>>
{"kind":"text","text":"..."}
<<<END_AINOVEL_WEB_RESPONSE>>>
```

Tool request:

```text
<<<AINOVEL_WEB_RESPONSE>>>
{"kind":"tool_calls","tool_calls":[{"name":"exact_local_tool","arguments":{}}]}
<<<END_AINOVEL_WEB_RESPONSE>>>
```

Local parser behavior:

1. Reject content outside the envelope.
2. Reject unknown JSON fields.
3. Reject unknown tool names.
4. Reject non-object tool arguments.
5. Bound the number of tool calls per response.
6. Generate stable deterministic tool-call IDs.
7. Convert valid calls to `agentcore.ToolCallBlock` + `StopReasonToolUse`.
8. Leave final JSON-Schema validation and tool execution to the existing local `agentcore` loop.

## 5. Contract tests written

W1 tests cover:

- normal text response;
- deterministic registry-bound tool call;
- unknown tool rejection;
- non-object argument rejection;
- content outside envelope rejection;
- unknown response field rejection;
- provider credential/cache/session non-leakage;
- provider ToolSpec flag non-leakage;
- message telemetry/internal metadata non-leakage;
- tool-result correlation requirement;
- cancellation propagation;
- retry/auth error contracts;
- prompt-contract structured output fallback;
- successful local tool execution through a real `subagent.Runner`;
- schema-invalid tool request never executing the local tool;
- local tool result returning to the following web-model turn.

## 6. Runtime verification blocker

The repository contains a CI workflow that runs formatting, `go vet`, full `go test ./...`, Windows/Linux matrix tests and selected race tests.

A controlled probe temporarily added `chatgpt-dev` to the workflow push branch list. A push commit was created and GitHub Actions was queried by exact head SHA. GitHub returned zero workflow runs.

The probe was then fully removed from branch history; `.github/workflows/ci.yml` is back to its upstream content and `main` was never modified.

Conclusion: the fork currently does not schedule GitHub Actions workflows. This is an infrastructure/repository setting limitation, not a W1 code test failure.

The local execution sandbox available to this ChatGPT session also cannot resolve `github.com` or `proxy.golang.org`, and it has no cached `agentcore` module. Therefore it cannot honestly produce an executed `go test` result for this Go module.

## 7. Gate decision

### PASS now

- W1 architecture boundary: PASS.
- W1 ChatModel source contract: PASS by direct dependency inspection + compile-time assertions.
- W1 deterministic protocol design: PASS.
- W1 local authority/security boundary: PASS by source audit.
- W1 test suite authored: PASS.

### HOLD

- `gofmt` execution evidence: HOLD.
- `go vet ./...` execution evidence: HOLD.
- `go test -buildvcs=false -count=1 ./...` execution evidence: HOLD.
- Linux/Windows CI evidence: HOLD.

### Overall

**W1 remains HOLD, not PASS, until an actual Go runner executes the repository test suite successfully.**

W2 must not be marked PASS before W1 runtime verification exists. Implementation work that does not weaken the W1 contract may continue on `chatgpt-dev`, but the formal W2 gate remains closed.
