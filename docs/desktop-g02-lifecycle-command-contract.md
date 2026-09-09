# G02.5 — Desktop Lifecycle Command Contract

Status: **AUTHORITATIVE FOR G02.5**  
Baseline: `ainovel-cli v0.1.3`  
Boundary: Desktop UI → `internal/appruntime` → Host/Engine

## Purpose

The desktop product exposes six run controls through `AppRuntime.Dispatch`:

- `start`
- `pause`
- `resume`
- `stop`
- `cancel`
- `retry`

Desktop code must never call Host/Engine lifecycle methods directly.

## Desktop lifecycle

```text
READY
  └─ start ─→ STARTING ─→ RUNNING

RUNNING
  ├─ pause  ─→ PAUSING    ─→ PAUSED
  ├─ stop   ─→ STOPPING   ─→ STOPPED
  └─ cancel ─→ CANCELLING ─→ CANCELLED

PAUSED
  └─ resume ─→ RESUMING ─→ RUNNING

FAILED / CANCELLED / STOPPED
  └─ retry ─→ RECOVERING ─→ RUNNING

RUNNING may also converge to COMPLETED or to the Host's paused/idle state when
core policy stops the engine independently of a desktop command.
```

## Start

`start` is deliberately safe by default.

Payload:

```json
{
  "mode": "resume | new",
  "requirement": "required only for mode=new"
}
```

An omitted payload uses `mode=resume`. This prevents a generic Start button from
silently resetting an existing book. `mode=new` is explicit and delegates to
`Host.PrepareUserRules` followed by `Host.StartPrepared`.

## Pause

Pause uses the existing Host/Engine abort primitive.

The Engine contract already defines abort as **pause semantics with checkpoint
losslessness**. AppRuntime therefore does not invent a second checkpoint or
rollback mechanism.

Desktop semantics:

```text
RUNNING → PAUSING → PAUSED
```

`resume` is permitted only after the physical Engine goroutine has stopped.

## Resume

Resume delegates to `Host.Resume`, which reconstructs execution from durable
checkpoint + progress facts. No browser conversation is treated as project
memory.

Desktop semantics:

```text
PAUSED → RESUMING → RUNNING
```

A missing resume label is a rejected command, not a fabricated successful run.

## Stop

Stop and Pause currently share the Host's lossless cancellation primitive, but
they are intentionally **different run-level semantics**.

Stop means:

- terminate the current run;
- retain all durable checkpoint/progress/log facts;
- end in `STOPPED`, not `PAUSED`;
- `resume` is not the continuation control for a stopped run;
- a later `start(mode=resume)` or `retry` creates the next run attempt from
  durable facts.

```text
RUNNING → STOPPING → STOPPED
PAUSED  → STOPPING → STOPPED
```

This distinction lets G05 add persistent run identities without changing the
core checkpoint model.

## Cancel

Cancel means abandon the **current attempt** immediately.

Already committed project facts are not rolled back. AppRuntime must not claim
transactional rollback that the v0.1.3 Host does not provide.

```text
RUNNING → CANCELLING → CANCELLED
PAUSED  → CANCELLING → CANCELLED
```

`CANCELLED` remains retryable from durable project facts.

## Retry

Retry is allowed from:

- `FAILED`
- `CANCELLED`
- `STOPPED`

It delegates to `Host.Resume` and therefore uses the same durable recovery path
as ordinary resume.

```text
FAILED / CANCELLED / STOPPED → RECOVERING → RUNNING
```

## Command acceptance rule

The GUI must not optimistically mutate lifecycle state.

AppRuntime first validates the command and asks the Host/core to accept the
operation. Only after acceptance does AppRuntime emit lifecycle events and
change the desktop lifecycle projection.

Rejected commands return stable command errors and leave the previous state
unchanged, except for a genuine core start/resume failure, which is surfaced as
`FAILED` so Retry has an explicit recovery source state.

## Physical engine stop guard

Host sets its logical lifecycle to `paused` before the Engine goroutine has
necessarily finished unwinding. AppRuntime therefore uses the read-only
`Host.DesktopEngineRunning()` fact to complete:

- `PAUSING → PAUSED`
- `STOPPING → STOPPED`
- `CANCELLING → CANCELLED`

The accessor exposes only a boolean and does not expose Engine mutation APIs.

## Event contract

Accepted lifecycle changes are emitted through the existing AppRuntime event
fan-out as:

```text
Category: LIFECYCLE
Type:     lifecycle_state
```

Payload contains:

- command ID
- command kind
- previous desktop state
- new desktop state
- optional detail

No second event database is introduced.

## Invariants

1. GUI never calls `Host`, `Engine`, `Store`, or WebAI lifecycle APIs directly.
2. Pause keeps durable checkpoint/progress state.
3. Stop is not Pause: final state is `STOPPED` and same-run Resume is rejected.
4. Cancel is not Stop: final state is `CANCELLED` and the attempt is abandoned.
5. Cancel does not falsely promise rollback of already committed facts.
6. Retry always re-enters through durable Host recovery.
7. Transition commands are serialized inside AppRuntime.
8. UI state changes only after AppRuntime/core acceptance.
9. Physical Engine shutdown is observed before terminal interrupt states are finalized.
10. G03 remains closed until all of G02 passes.
