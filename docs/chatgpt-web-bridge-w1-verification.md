# W1 — Web ChatModel Contract & Deterministic Tool Protocol — Verification Record

Status: **PASS / AUTHOR-CONFIRMED / LOCKED**

Repository: `tiktok1997af-dot/ainovel-cli`  
Branch: `chatgpt-dev`  
Architecture authority: `docs/chatgpt-web-bridge-audit.md`

## 1. Locked product rule

AINovel is being converted to **WEB-ONLY / NO-API**.

W1 does not introduce or depend on an AI API transport. The retained `agentcore.ChatModel` and provider-neutral error/capability names are internal contracts only. The intended runtime path is:

```text
ainovel-cli
  -> WebChatModel
  -> browser/session transport
  -> logged-in AI website
  -> response capture
  -> local agentcore/tool execution
  -> local Store/project data
```

No API key, API fallback or provider HTTP route is part of the W1 target architecture.

## 2. W1 implementation

Implemented package:

- `internal/webai/model.go`
- `internal/webai/protocol.go`
- `internal/webai/errors.go`
- `internal/webai/model_test.go`
- `internal/webai/protocol_test.go`

The browser/site implementation remains behind `Transport`; live Chrome/DOM/login work starts in W2/W3.

## 3. Contract and security guarantees

W1 is verified against `github.com/voocel/agentcore v1.8.2` and preserves the existing agent/tool/store authority boundary.

The web boundary does not receive:

- API keys;
- legacy provider session/cache routing values;
- provider billing/usage telemetry;
- arbitrary message metadata;
- message timestamps;
- provider reasoning/thinking history;
- provider thought signatures;
- provider-specific strict/deferred tool flags;
- direct filesystem, Store, process or command authority.

The web side receives only the semantic conversation/tool contract needed for the next model turn.

Tool requests returned by the website are parsed locally, registry-bound and passed into the existing `agentcore` validation/execution path. The website never executes local tools directly.

## 4. Hardening completed

Final W1 hardening includes:

- reject empty, whitespace-mutated or duplicate tool names;
- reject ambiguous tool registries;
- validate historical tool-call/result sequence before serialization;
- reject missing/duplicate tool-call IDs;
- reject malformed/non-object tool arguments;
- reject unsupported message roles/content instead of silently dropping them;
- strip provider reasoning/thought signatures from web projection;
- reject a message if stripping provider-only reasoning would leave no transferable semantic content;
- enforce non-retryable semantics for auth-required, security challenge, protocol violation and unsupported-site failures;
- keep retryable transport/timeout failures compatible with existing agentcore retry contracts;
- report native structured output as unsupported so existing `llmcontract` uses `prompt_contract` rather than an API-specific schema path.

## 5. Runtime CI evidence

GitHub Actions was enabled on the fork and the real repository dependency graph was executed with Go 1.25.5.

### CI run #5

Run ID: `33978428776`  
Verified source head: `22fb7adde49d35d7bd0426716d2c1229d85df7c2`

Ubuntu (`ubuntu-latest`):

- Checkout: PASS
- Set up Go: PASS
- Format check: PASS
- Installer syntax check: PASS
- `go vet ./...`: PASS
- `go test -buildvcs=false -count=1 ./...`: PASS
- `go test -race -buildvcs=false -count=1 ./internal/host ./internal/store ./internal/tools`: PASS

Windows (`windows-latest`):

- Checkout: PASS
- Set up Go: PASS
- Format check: PASS
- `go vet ./...`: PASS
- `go test -buildvcs=false -count=1 ./...`: PASS

### Final clean CI run #8

Run ID: `33978723298`  
Verified final head: `f0924feaad83777cfb0145dcef9087673381375f`

Ubuntu (`ubuntu-latest`): **PASS** for format, installer syntax, `go vet`, full `go test`, and selected race tests.

Windows (`windows-latest`): **PASS** for format, `go vet`, and full `go test`.

This is the final exact-head verification for W1 before merge.

## 6. Baseline issue found during verification

The first real CI run exposed a pre-existing `gofmt` defect in upstream file:

- `internal/entry/tui/panels_outline.go`

Only whitespace formatting was corrected; no behavior was changed. After that correction, format checks passed on both Linux and Windows.

## 7. Cleanup state

Temporary diagnostic workflow changes were fully reverted. `.github/workflows/ci.yml` is restored to the upstream workflow content.

The temporary `docs/.w1-actions-trigger` file was removed.

## 8. Gate decision

W1 acceptance criteria are satisfied:

- architecture boundary: **PASS**;
- WEB-ONLY / NO-API invariant: **PASS**;
- ChatModel contract: **PASS**;
- deterministic tool protocol: **PASS**;
- security/data boundary: **PASS**;
- hardening/static review: **PASS**;
- real dependency `go vet`: **PASS**;
- real dependency full `go test ./...`: **PASS**;
- Linux race tests: **PASS**;
- Windows test matrix: **PASS**.

# W1 FINAL: PASS / AUTHOR-CONFIRMED / LOCKED

Next authorized stage: **W2 — BROWSER SESSION & PERSISTENT WEB LOGIN v0.1**.
