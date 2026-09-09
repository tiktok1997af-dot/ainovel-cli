# AINOVEL Desktop — G02.1 AppRuntime Contract & Core-Bridge Boundary v1

Status: `G02.1 — APP RUNTIME CONTRACT & CORE-BRIDGE BOUNDARY`

Parent authority: `docs/desktop-g01-foundation-authority-v1.md`

Baseline inherited from G01: `main@af14099d9b8d50794473dc4171a31a548caace3a`, preserving `ainovel-cli v0.1.3` compatibility and the WEB-only Gemini execution invariant.

This document freezes the only supported boundary between the future desktop GUI and the existing Go core. It is an architecture contract, not yet the full implementation of G02.

## 1. G02.1 decision

The desktop GUI MUST communicate with the existing core through one facade named **AppRuntime**.

The GUI MUST NOT directly import, retain, or call:

- `internal/host.Host`;
- `internal/store.Store` or any store sub-service;
- `internal/webai.SessionManager` / browser transport;
- Engine / Workers / Arbiter / local Tools;
- Host CoCreate internals;
- raw Host event/stream channels.

The core implementation remains authoritative. AppRuntime is an adapter/orchestration facade, not a second engine and not a second database.

## 2. Current seam audit

### 2.1 Host

`internal/host.Host` currently owns the runtime shell: configuration, Store, Engine, WebAI session, observer, chapter gate, lifecycle state and CoCreate occupancy. The desktop product therefore wraps Host rather than reconstructing its responsibilities.

### 2.2 UISnapshot

`host.UISnapshot` already aggregates UI-oriented facts including runtime state, phase/flow, chapter progress, recovery label, agent snapshots, outline/character summaries and checkpoint/review summaries.

Decision: **EXTEND + PROJECT**, not expose directly to the desktop frontend.

The desktop contract receives an immutable DTO projection derived from `UISnapshot` plus browser/run data. This prevents the frontend from becoming coupled to TUI-specific Go structs.

### 2.3 Observer / events

Host observer is explicitly a pure observer and already:

- emits structured `host.Event` values;
- tracks agent activity;
- emits stream deltas;
- persists completed/system events into `Store.Runtime`.

Decision: AppRuntime consumes this existing pipeline and projects it to a desktop event stream. It MUST NOT add a parallel observer inside the GUI.

### 2.4 Store

Store remains the canonical source of truth. `RuntimeStore` already persists `meta/runtime/queue.jsonl` and per-task logs under `meta/runtime/tasks/` with sequence numbers and task IDs.

Decision: AppRuntime is the only desktop-side path to Store-backed reads/writes. GUI code never owns a Store pointer or writes project files directly.

### 2.5 WebAI

`webai.SessionManager` owns one visible Chrome process and its persistent browser profile. It exposes lifecycle/readiness through `SessionSnapshot` and deliberately does not own credentials.

Decision: desktop code sees only a projected browser status. Browser start/refresh/stop operations are commands through AppRuntime. Direct SessionManager access is prohibited.

### 2.6 CoCreate

CoCreate already lives in Host and uses the same model/store/session infrastructure, including streamed progress and durable session logs.

Decision: desktop CoCreate is a view + command projection over the existing Host capability. No second CoCreate engine may be created for the GUI.

## 3. AppRuntime four-plane contract

The stable facade has exactly four functional planes plus close/disposal.

```go
// Conceptual authority. Concrete Go types are implemented in G02.2-G02.6.
type AppRuntime interface {
    Snapshot(ctx context.Context) (DesktopSnapshot, error)
    Query(ctx context.Context, req QueryRequest) (QueryResult, error)
    Dispatch(ctx context.Context, cmd CommandRequest) (CommandResult, error)
    Subscribe(ctx context.Context, cursor EventCursor) (EventSubscription, error)
    Close(ctx context.Context) error
}
```

The exact DTO field set may grow additively in later G02 steps, but the five-method facade shape is frozen by G02.1.

### 3.1 Snapshot plane

Purpose: cheap, side-effect-free current-state projection for shell/header/status panels.

Required properties:

- read-only;
- safe for repeated/concurrent calls;
- does not start browser/AI work;
- does not mutate Store;
- returns value/DTO data only, never core pointers;
- includes a monotonically comparable revision/timestamp so the frontend can ignore stale projections.

The snapshot eventually contains these top-level sections:

```text
DesktopSnapshot
├── Product
├── Project
├── Runtime
├── CurrentChapter
├── Agents
├── Browser
├── Recovery
└── QualitySummary
```

`host.UISnapshot` is the primary source for current runtime/project/agent facts; additional sections may be composed from existing canonical core sources.

### 3.2 Query plane

Purpose: bounded read operations that are too large or too specific for the global snapshot.

Examples for later gates:

- chapter list/detail;
- Arc/Volume hierarchy;
- documents;
- Context/Canon/Characters/World/Timeline;
- run history/task logs;
- review details;
- Folder View metadata/preview.

Rules:

1. Queries are read-only.
2. Query DTOs are serialization-safe and contain no open file handles or internal pointers.
3. Large datasets use bounded/paged/range requests rather than bloating `DesktopSnapshot`.
4. A Query MUST NOT be used as a hidden write path.

### 3.3 Command plane

Purpose: every state-changing desktop action.

All of the following ultimately enter through `Dispatch`:

- Start / Pause / Resume / Stop / Cancel / Retry;
- create/open/change project actions;
- AI writing actions;
- CoCreate send/commit/apply actions;
- review/repair/re-run/promote actions;
- browser lifecycle actions;
- settings mutations that affect runtime state.

Rules:

1. GUI code cannot call Host mutation methods directly.
2. Commands carry a stable command ID for idempotency/audit.
3. Commands are context-cancellable.
4. Mutating commands are ordered by AppRuntime/orchestrator policy; the frontend cannot bypass serialization/resource locks by issuing concurrent direct core calls.
5. Command acceptance and command completion are distinct concepts. `CommandResult` may acknowledge an asynchronous run while later events report progress/completion.
6. Exact Start/Pause/Resume/Stop/Cancel/Retry semantics are frozen in G02.5 and must not be guessed by the GUI.

### 3.4 Event subscription plane

Purpose: realtime desktop updates without polling every core component.

`Subscribe(ctx, cursor)` provides a single desktop event feed that projects existing sources:

```text
Host structured Event ─┐
Host stream delta ─────┼─> AppRuntime event projector ─> DesktopEvent
Host stream clear ─────┤
Browser readiness ─────┤
Command/run state ─────┘
```

Rules:

1. Preserve existing Host event semantics; do not change Engine decisions from the observer path.
2. Desktop events have an ordered cursor/sequence suitable for reconnect/replay.
3. Where durable runtime queue records exist, AppRuntime uses them for replay rather than inventing a second persistent event log.
4. Stream deltas may be transient, but durable state transitions/checkpoints remain recoverable from canonical Store facts.
5. Subscription backpressure must not block Engine/Host execution. A slow GUI is not allowed to stall writing.
6. The frontend receives desktop DTOs, not raw Go channels owned by Host.

## 4. Dependency direction

Allowed dependency direction:

```text
Desktop UI / Desktop transport
            |
            v
      AppRuntime contract
            |
            v
      AppRuntime facade
       /    |     \
    Host   Store   WebAI
      |      |       |
   Engine  Canon   Chrome/Gemini Web
```

Forbidden direction:

```text
Desktop UI -> Host          X
Desktop UI -> Store         X
Desktop UI -> WebAI         X
Desktop UI -> Engine/Tools  X
Host/Store -> Desktop UI    X
```

The AppRuntime implementation may depend on core packages. Core packages MUST NOT depend on the desktop frontend.

## 5. Package boundary authority

Target Go package introduced in G02.2:

`internal/appruntime`

Recommended internal split:

```text
internal/appruntime/
├── runtime.go        # facade and construction
├── contract.go       # public internal DTO/request interfaces
├── snapshot.go       # Host/core -> DesktopSnapshot projection
├── events.go         # event subscription/projection
├── commands.go       # command dispatch adapter
├── queries.go        # read query adapter
└── errors.go         # stable desktop error projection
```

This is a logical authority, not a requirement to create all files in G02.2 at once.

Desktop entry code may import `internal/appruntime`. Desktop frontend bindings must be generated/wrapped from AppRuntime DTOs rather than importing other internal core packages.

## 6. Ownership and source-of-truth rules

### Project/story facts

Owner: Store/domain artifacts.

AppRuntime may project or command changes but never keeps a competing canonical copy.

### Runtime lifecycle

Owner: Host today; later Run Orchestrator extends this authority.

AppRuntime translates desktop commands to the owner and projects status back.

### Browser session

Owner: WebAI SessionManager / later browser-lane owner.

AppRuntime exposes status/commands only.

### CoCreate state

Owner: existing Host/Store CoCreate path.

Desktop workspace keeps only presentation state (selection, scroll position, unsent local draft).

### GUI presentation state

Owner: frontend.

Examples: selected tab, panel width, filters, sort order. Such state is not canonical novel/runtime state unless explicitly persisted through a future settings command.

## 7. Concurrency contract

G02.1 freezes these concurrency rules:

- Snapshot and Query may execute concurrently when the underlying core read is safe.
- Dispatch is the only mutation entrance.
- AppRuntime must be safe when UI threads issue overlapping calls.
- AppRuntime must not expose Host mutexes/channels to the frontend.
- `context.Context` cancellation means the caller no longer waits; it does not automatically mean "Cancel Run" unless the command semantics explicitly say so.
- Multi-run/resource-lock scheduling belongs to G05, but the G02 contract must not prevent it: command and event DTOs therefore carry optional `RunID`, `TaskID` and `Resource` identity fields additively.

## 8. Serialization / transport neutrality

AppRuntime is not tied to a specific desktop IPC implementation.

Contract DTOs must be representable in JSON-compatible form so the same facade can be bound through a desktop bridge without leaking Go implementation details.

Rules:

- explicit string enums rather than frontend-visible Go pointer identity;
- timestamps serialized deterministically;
- durations represented with a stable unit/format;
- no `any`/opaque internal payload exposed to the frontend contract without a typed envelope;
- no secrets, cookies, browser credentials or filesystem handles in desktop DTOs.

Exact status/error serialization is completed in G02.6.

## 9. Compatibility rules inherited from G01

AppRuntime implementation MUST preserve:

- WEB-only Gemini execution; no AI API fallback;
- visible Chrome + persistent user-owned login profile;
- Store as canonical source of truth;
- readable existing `meta/progress.json`, `meta/run.json`, checkpoints and project files;
- TUI and headless compatibility;
- existing Host/Engine regression tests;
- browser conversation is not project memory;
- no credential/cookie extraction.

The desktop GUI is additive. It is not permitted to make existing v0.1.3 projects desktop-only.

## 10. Migration rule for the existing TUI

The existing TUI may continue to call `host.Host` during G02 implementation. G02 does not require an immediate TUI rewrite.

However:

- new desktop code MUST use AppRuntime from its first implementation;
- no new desktop-only capability may be implemented by bypassing AppRuntime;
- shared behavior should progressively move behind AppRuntime/core services rather than duplicate logic in desktop entry code.

This preserves production compatibility while avoiding a risky all-at-once TUI migration.

## 11. G02.1 acceptance checklist

- [x] Host ownership seam audited.
- [x] `UISnapshot` projection authority audited.
- [x] observer/event seam audited.
- [x] Store/runtime queue seam audited.
- [x] WebAI SessionManager ownership audited.
- [x] CoCreate ownership audited.
- [x] single AppRuntime facade selected.
- [x] Snapshot / Query / Dispatch / Subscribe / Close facade shape frozen.
- [x] direct GUI -> core package access prohibited.
- [x] source-of-truth and concurrency rules frozen.
- [x] transport-neutral serialization boundary frozen.
- [x] G01 compatibility invariants preserved.

## 12. Gate state

`G02.1 — PASS` when this authority file is committed on `feat/desktop-roadmap-v2-g02-appruntime` and the branch still descends from the G01 merge head.

Next authorized step after G02.1 PASS:

`G02.2 — CREATE APPRUNTIME FACADE / PACKAGE SKELETON`

G03 remains CLOSED until all G02 steps pass.
