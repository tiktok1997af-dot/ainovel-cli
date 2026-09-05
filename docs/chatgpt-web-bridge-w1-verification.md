# W1 — Web ChatModel Contract & Deterministic Tool Protocol — Verification Record

Status: **SOURCE-LEVEL HARDENING COMPLETE / RUNTIME VERIFICATION HOLD**

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

Verified signatures/contracts include:

- `agentcore.ChatModel`: `Generate`, `GenerateStream`, `SupportsTools`.
- `agentcore.ProviderNamer` and `agentcore.ModelNamer`.
- `llm.CapabilityProvider`, `llm.ModelInfo` and `llm.Capabilities`.
- `agentcore.Message`, content types and `AssertMessageSequence`.
- `agentcore.ToolCall` including `ArgsInvalid` and provider `ThoughtSignature` fields.
- `subagent.Runner.Run(ctx, agent, task)`.
- `agentcore.NewFuncTool(...)`.
- `agentcore.RetryableError` and `agentcore.RetryHinter`.
- provider-neutral error sentinels `ErrProviderAuth`, `ErrProviderNetwork`, `ErrProviderTimeout`.
- existing `llmcontract.Plan` fallback to `prompt_contract` when native JSON Schema is unsupported.

Compile-time interface assertions are present in W1 source so a real Go compiler rejects interface drift.

A local source compile harness was also executed for the three runtime files using stubs that mirror the inspected v1.8.2 signatures. Result: **PASS**. This catches Go syntax, unused imports and internal type/signature inconsistencies in W1 runtime source, but it is not a substitute for compiling against the real module dependency.

## 3. Final hardening findings and repairs

The final static review found and repaired the following source-level edge cases.

### 3.1 Ambiguous local tool registry

Before hardening, duplicate tool names would collapse into a map during response parsing. W1 now rejects:

- empty tool names;
- names with surrounding whitespace;
- duplicate tool names.

The same registry validation is applied before prompt serialization and before response parsing.

### 3.2 Malformed or ambiguous historical tool transcript

W1 now validates the message sequence before serialization and rejects:

- incomplete/orphaned tool transcripts reported by `agentcore.AssertMessageSequence`;
- tool calls without IDs or names;
- tool-call IDs/names with surrounding whitespace;
- duplicate historical `tool_call_id` values;
- `ToolCall.ArgsInvalid` history;
- missing/non-object/malformed JSON tool arguments;
- tool results without a valid correlation ID.

### 3.3 Unsupported content no longer disappears silently

The web protocol is text/tool-call only in W1. It now fails closed for unsupported transferable content instead of silently dropping it:

- images;
- tool-reference blocks;
- unknown content block types;
- unsupported message roles.

Provider reasoning/thinking blocks are intentionally not replayed to the consumer website. If dropping provider-only reasoning would leave a message with no transferable semantic content, W1 rejects the transcript instead of sending an empty message.

### 3.4 Provider reasoning metadata isolation

The web projection strips:

- thinking/reasoning block text;
- `ToolCall.ThoughtSignature`;
- arbitrary message metadata;
- usage/provider/model telemetry;
- timestamps.

Only local semantic conversation state needed for the next web turn is projected.

### 3.5 Retry semantics are now fail-safe

Authentication/security challenge, protocol violation and unsupported-site errors are always non-retryable, even if a caller accidentally constructs them with `Retry=true`. `RetryAfter()` also returns zero for these states.

Retryable transport and timeout failures can still use the existing agentcore retry interfaces.

### 3.6 Capability metadata consistency

`Model.Info()` now advertises chat, tool calling and streaming, matching `Capabilities()` where streaming is explicitly partial/final-only in W1.

## 4. Security/data-boundary verification completed

Not sent to the web transport:

- API keys;
- provider session IDs;
- prompt-cache routing keys;
- arbitrary provider-specific tool-choice objects;
- `Message.Usage`;
- message timestamps;
- arbitrary `Message.Metadata`;
- provider telemetry/model billing metadata;
- reasoning/thinking history;
- provider thought signatures;
- `ToolSpec.Strict`;
- `ToolSpec.DeferLoading`.

Tool results keep only the correlation fields required to continue a local tool-call transcript:

- `tool_call_id`
- `tool_name`
- `is_error`

The website never receives direct filesystem, Store, process or arbitrary command authority.

## 5. Deterministic tool protocol implemented

The browser AI must return exactly one bounded response envelope.

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

1. reject content outside the envelope;
2. reject unknown JSON fields;
3. reject ambiguous local tool registries;
4. reject unknown/whitespace-mutated tool names;
5. reject non-object or malformed tool arguments;
6. bound tool calls per response;
7. generate stable deterministic tool-call IDs;
8. convert valid calls to `agentcore.ToolCallBlock` + `StopReasonToolUse`;
9. leave final JSON-Schema validation and execution to the existing local `agentcore` loop.

## 6. Contract tests authored

W1 tests now cover:

- normal text response;
- deterministic registry-bound tool calls;
- unknown/ambiguous/whitespace tool rejection;
- non-object argument rejection;
- content outside envelope rejection;
- unknown response field rejection;
- provider credential/cache/session non-leakage;
- provider ToolSpec flag non-leakage;
- message telemetry/internal metadata non-leakage;
- reasoning/thought-signature non-leakage;
- tool-result correlation requirement;
- invalid message sequence rejection;
- unsupported role/content rejection;
- thinking-only/empty-after-projection rejection;
- malformed historical tool-call rejection;
- duplicate historical tool-call ID rejection;
- cancellation propagation;
- retry/auth/security/protocol error contracts;
- capability metadata consistency;
- prompt-contract structured-output fallback;
- successful local tool execution through a real `subagent.Runner` test definition;
- schema-invalid tool request never executing the local tool;
- local tool result returning to the following web-model turn.

## 7. Formatting/static execution evidence

The exact revised W1 files were passed through local `gofmt`; `gofmt -l` returned no W1 files.

A local compile harness for the three runtime files returned:

```text
? github.com/voocel/ainovel-cli/internal/webai [no test files]
```

This is useful source-level evidence only. The harness uses inspected-signature stubs because the session sandbox cannot download the real Go dependencies.

## 8. Remaining runtime verification blocker

The repository CI workflow normally runs formatting, `go vet`, full `go test ./...`, Windows/Linux matrix tests and selected race tests.

A controlled probe temporarily allowed pushes to `chatgpt-dev`. GitHub still created zero Actions runs for the exact pushed SHA. The probe was then removed from branch history and `.github/workflows/ci.yml` was restored unchanged.

The local execution sandbox cannot resolve GitHub/Go module endpoints and does not have `agentcore v1.8.2` cached, so it cannot run the repository against the real dependency graph.

Therefore there is no observed code failure, but there is also no valid executed full-module test result.

## 9. Gate decision

### PASS at source/static level

- W1 architecture boundary: **PASS**.
- W1 ChatModel source contract: **PASS**.
- W1 deterministic protocol design: **PASS**.
- W1 final static hardening: **PASS**.
- W1 local authority/security boundary: **PASS**.
- W1 W1-file formatting check: **PASS**.
- W1 runtime-source compile harness: **PASS with stub-dependency limitation**.
- W1 test suite authored: **PASS**.

### Single remaining blocker class

**Executed real-module test runner**:

- `go vet ./...`;
- `go test -buildvcs=false -count=1 ./...`;
- Linux/Windows CI evidence;
- selected race tests from the repository workflow.

### Overall

**W1 remains HOLD, not PASS, solely because no runner has executed the real repository dependency graph yet.**

No additional source-level blocker is known after final hardening review. W2 remains formally closed until real-module runtime verification is available.
