# W5C — LEGACY API RUNTIME REMOVAL v0.1

Status: OPEN

Base authority: W5B PASS / LOCKED, integrated at `dc0b417cbbd36e6581422f1644b1b3cdc5916fa3`.

Branch: `w5c-remove-legacy-api-runtime`.

## Non-negotiable target

The compiled product must have exactly one AI execution path:

`Host -> SessionManager -> GeminiWebTransport -> WebChatModel -> Agents/Arbiter`

No provider HTTP API, API credential, Ollama/local inference, provider fallback, or provider/model hot-swap path may remain reachable.

Legacy API-shaped configuration must fail with an explicit WEB-only migration error. It must never be interpreted as permission to construct an API client.

## Initial audit findings

### `internal/bootstrap/models.go`

Legacy runtime remains compiled and must be removed:
- `NewModelSet` API/provider constructor;
- `createModelFromConfig`;
- `llm.NewModel`;
- `llm.WithAPIKey`;
- `llm.WithBaseURL`;
- `llm.WithProviderExtra` / `llm.WithExtra`;
- role fallback targets and `failoverModel`;
- provider/model `Swap` path;
- candidate `ApplyPrepared` hot-replacement path.

The verified `NewWebModelSet` path is retained.

### `internal/agents/build.go`

Workers still call `ForRoleWithFailover` and install provider-failover logging. W5C replaces this with the fixed browser-backed model path. Retry may remain provider-neutral inside agentcore, but resubmission to a different AI provider is forbidden.

### `internal/host/host.go`

Host still contains a conditional fallback to `bootstrap.NewModelSet(cfg)` when WEB mode is disabled. W5C removes that branch: production Host must require WEB mode and always start the browser runtime.

### `internal/bootstrap/config.go` / `configfile.go`

API-era provider/credential/model/fallback schema and merge helpers still compile. W5C must:
- require WEB-only configuration at runtime;
- reject legacy API-only configs with a clear migration message;
- remove active API credential persistence and provider/model fallback routing;
- preserve creative settings, browser settings, local context-compaction settings, notifications and other provider-neutral project settings.

### `internal/host/model_config.go`

The obsolete runtime provider-management surface still compiles even though W5B made it unreachable from WEB-only TUI. It contains API key mutation, provider/model configuration, connection tests, hot model replacement and API model construction. W5C removes this API-era runtime service and its production call paths.

### Dependency boundary

`go.mod` still declares `github.com/voocel/litellm v1.8.10` directly. After API constructor removal W5C will run module/dependency audit; remove the direct dependency if no product source imports it. `agentcore/llm` capability types may remain because they are provider-neutral interfaces used by WebChatModel/structured-output policy.

## Execution gates

### W5C-C1 — HARD-CUT RUNTIME
- `Config.ValidateBase` rejects non-WEB runtime configs with migration guidance;
- `Host.New` has no API fallback branch;
- workers use the browser-backed model directly, not provider failover;
- `llm.NewModel` and API constructor helpers are deleted from production bootstrap.

### W5C-C2 — REMOVE PROVIDER MANAGEMENT
- delete API-key/provider/model/fallback persistence and connection-test runtime services;
- delete provider/model hot-swap/failover product paths;
- simplify ModelSet to fixed WEB-only identity while preserving the interfaces actually needed by Engine/Agents/Arbiter/context management.

### W5C-C3 — CONFIG MIGRATION CONTRACT
- old API-shaped configs fail loudly with one clear WEB-only migration error;
- WEB-only config remains loadable and writable;
- no credential is copied into browser config or project data.

### W5C-C4 — DEPENDENCY & REPOSITORY AUDIT
- no production `llm.NewModel`, `WithAPIKey`, `WithBaseURL`, API-provider constructor or Ollama active path;
- direct `litellm` dependency removed if unused;
- test-only historical references may remain only where unreachable and non-secret.

## W5C PASS gate

W5C may be locked only after:
- C1–C4 PASS;
- `gofmt` PASS;
- `go vet ./...` PASS;
- full Ubuntu + Windows `go test ./...` PASS;
- critical race tests PASS on Linux;
- repository audit proves no active API runtime constructor/fallback remains.

W5D remains closed until W5C is PASS / LOCKED.
