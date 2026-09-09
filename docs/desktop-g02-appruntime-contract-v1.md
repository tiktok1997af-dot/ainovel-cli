# AINOVEL Desktop — G02.1 AppRuntime Contract & Core-Bridge Boundary v1

Status: `G02 — APP RUNTIME — IN PROGRESS`

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

G02.4 implementation additionally locks these details:

- exactly one AppRuntime event hub consumes the Host event/stream channels and fans out to desktop subscribers;
- each subscriber has its own bounded non-blocking buffer using drop-oldest behavior under backpressure;
- durable replay uses the existing `RuntimeStore` sequence and `EventCursor.AfterSeq`;
- the hub tails the canonical runtime queue for newly persisted durable events, so no second event database exists;
- `host.StreamClearSentinel` is projected as `stream_clear` and never crosses the desktop boundary raw;
- `utils.ThinkingSep` is projected as explicit `stream_mode` transitions and never crosses the desktop boundary raw;
- prose/thinking deltas are emitted as `stream_delta` JSON payloads with an explicit `mode`;
- closing AppRuntime stops the event hub before closing Host, and subscription cancellation removes only that subscriber.

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

Desktop entry code may import `internal/appruntime`. Desktop frontend bindings must be generated/wrapped from AppRuntime DTOs rather than importing other internal core packages.

## 6. Ownership and source-of-truth rules

1. Host/Store remain authoritative for project/runtime facts.
2. AppRuntime owns desktop-facing orchestration and projection only.
3. Browser state is execution state, never canonical project state.
4. Browser conversations are not persistent novel memory.
5. Checkpoint/recovery remains based on local canonical project facts.
6. AppRuntime must not create a second project database.
7. Desktop UI state such as selected tab/panel may live in the frontend, but any fact that affects story/runtime correctness must come through AppRuntime.

## 7. Concurrency boundary

G02 freezes the boundary needed by later multi-run work without implementing G05 concurrency early:

- frontend may submit multiple commands but cannot directly run Host methods concurrently;
- AppRuntime is the future serialization/orchestration point;
- run/task/resource IDs are part of command/event DTO design;
- resource locks, worker pools and Browser Lane Pool remain later-gate responsibilities;
- G02 must not introduce fake parallelism over the current single Host/browser assumptions.

## 8. Lifecycle boundary

The user-visible target remains:

`Start / Pause / Resume / Stop / Cancel / Retry`

G02.1 locks only their route:

```text
Desktop control
  -> AppRuntime.Dispatch(command)
  -> core lifecycle adapter
  -> Host/Engine/Store/WebAI as required
  -> DesktopEvent(s)
  -> updated DesktopSnapshot
```

Exact semantics and illegal-state behavior are implemented/frozen in G02.5. The frontend is never allowed to infer lifecycle transitions and mutate local state optimistically as if the core already accepted them.

## 9. Error boundary

Core errors must not be serialized ad hoc as Go implementation strings throughout the UI.

G02.6 finalizes a stable projected error shape with categories such as validation/runtime/browser/store/recovery/internal while preserving diagnostic detail for logs. Until then, the facade must keep errors behind the AppRuntime call boundary.

## 10. Compatibility invariants inherited from G01

AppRuntime MUST preserve all of these:

- `ainovel-cli v0.1.3` remains the core source baseline.
- Existing TUI and headless entrypoints are not deleted by G02.
- Existing project directories remain readable.
- Store remains canonical.
- WEB-only Gemini execution remains in force; no paid/direct AI API runtime may be reintroduced.
- Browser credentials/session secrets are not copied into project files or desktop DTOs.
- Existing resume/checkpoint/watchdog behavior is wrapped or extended, not replaced by a competing implementation.

## 11. G02 progress authority

- `G02.1 — AppRuntime Contract & Core-Bridge Boundary`: PASS.
- `G02.2 — AppRuntime Facade / Package Skeleton`: PASS.
- `G02.3 — UISnapshot -> Desktop View Model Projection`: PASS.
- `G02.4 — Host Observer/Event Stream -> Desktop Event Bridge`: PASS.
- `G02.5 — Lifecycle Command Contract`: OPEN / AUTHORIZED NEXT STEP.
- `G02.6 — Error/Status/Serialization Contracts`: CLOSED.
- `G02.7 — Contract Tests + Regression + G02 Gate`: CLOSED.

G03 remains CLOSED until all G02 steps pass.
