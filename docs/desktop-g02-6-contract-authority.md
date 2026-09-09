# AINOVEL Desktop — G02.6 Contract Authority

Status: **PASS / LOCKED**  
Roadmap: Optimized v2 — G02 APP RUNTIME  
Baseline: `ainovel-cli v0.1.3`

## 1. Transport version

The desktop bridge contract is versioned as:

- `ContractVersion = ainovel.desktop.v1`
- `SchemaVersion = 1`

Every server-side result/event advertises the current contract. During G02 migration, an empty request version remains accepted; a non-empty incompatible version is rejected as `contract_mismatch`.

## 2. Desktop-safe envelopes

Only AppRuntime DTOs may cross the desktop boundary:

- `DesktopSnapshot`
- `QueryRequest` / `QueryResult`
- `CommandRequest` / `CommandResult`
- `EventCursor` / `DesktopEvent`
- `AppError`

Raw `error`, `host.*`, `webai.*`, Store/Engine pointers and implementation ownership objects are not transport types.

Explicit local configuration fields intentionally modeled by a desktop DTO (for example `BrowserViewSnapshot.browser_path` and `profile_dir`) are allowed because Settings must be able to display/manage those local values. This does not permit arbitrary internal paths to leak through errors, logs or diagnostic payloads.

## 3. Structured AppError

`AppError` carries only:

- stable `code`
- stable `category`
- safe user-facing `message`
- `retryable`

The Go-only `Cause` is excluded from JSON while preserving `errors.Is` / `errors.As` inside the core.

Stable categories:

- `validation`
- `conflict`
- `browser`
- `ai`
- `store`
- `recovery`
- `runtime`
- `internal`

Mappings cover malformed requests, lifecycle conflicts, browser auth/transport/timeout/protocol failures, AI/provider failures, Store read/write failures, recovery failures and uncategorized internal failures.

## 4. Stable status projection

Desktop lifecycle state is normalized by AppRuntime rather than exposing Host lifecycle internals directly.

Browser status is normalized to:

`STARTING | AUTH_REQUIRED | READY | BUSY | DEGRADED | FAILED | STOPPED | UNKNOWN`

Unknown future browser states project to `UNKNOWN` instead of leaking an implementation value.

## 5. Error privacy boundary

Raw error causes, error-derived filesystem/profile paths and raw diagnostic payloads are not emitted to the desktop transport.

Host events are projected through a bounded desktop payload instead of serializing the complete `host.Event`. `ERROR` category / error-level events are sanitized to a structured `AppError`, safe summary and no raw payload before delivery to subscribers, including durable queue replay.

Explicit configuration fields declared in desktop DTOs are not considered diagnostic leakage and remain available to Settings/diagnostic UX under the AppRuntime contract.

## 6. Serialization guarantees

Contract tests cover:

- contract-version acceptance/rejection
- `AppError` JSON round-trip
- no serialized Go cause leakage
- validation/conflict/browser/store/recovery mappings
- browser `UNKNOWN` projection
- JSON serialization of Snapshot, Query, Command, Event and Cursor envelopes
- error-event payload sanitization
- preservation of `errors.Is` inside Go

## 7. Verification

Final G02.6 code head before this authority document passed:

- WEB-only residue audit
- Ubuntu format check
- Ubuntu vet
- Ubuntu full tests
- Ubuntu critical-state race tests
- Windows format check
- Windows vet
- Windows full tests

## 8. Gate rule

G02.6 is PASS. G02 is **not** complete until G02.7 full contract/regression gate passes. G03 remains CLOSED until G02.7 passes and the cumulative G02 PR is merged into `main`.
