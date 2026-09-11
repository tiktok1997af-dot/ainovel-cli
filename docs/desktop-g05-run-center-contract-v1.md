# AINOVEL Desktop — G05.2 AppRuntime Run Center Contract v1

Status: `G05.2 — IMPLEMENTATION CANDIDATE / CONTRACT-ONLY`.

Parent authority: `docs/desktop-g05-multirun-authority-v1.md`.

Parent accepted head: `1f845592581e6a421e367a770e693ce6b68f07c6` (`G05.1 PASS / LOCKED`).

## 1. Gate mission

G05.2 freezes the typed AppRuntime wire vocabulary required by the future Run Center while preserving the existing five-method facade:

`Snapshot / Query / Dispatch / Subscribe / Close`.

This gate is contract-only. It does not make multi-run data durable, schedule work, acquire resources, allocate browser lanes, or enable Run Center UI.

## 2. Query contract

G05.2 reserves exactly three additive Run Center query kinds:

- `runs.list`
- `runs.get`
- `runs.activity`

Their typed DTOs are:

- `RunsListQuery` / `RunsListResultDTO`
- `RunsGetQuery` / `RunsGetResultDTO`
- `RunsActivityQuery` / `RunsActivityResultDTO`

The existing G03 `SupportedQueryKinds()` catalog remains untouched. G05.2 publishes its own `CurrentRunCenterContractCatalog()` so staged contract discovery cannot be mistaken for an implemented data route.

Operational query routing remains CLOSED until the owning later gate has an authoritative run registry/history source. G05.2 therefore performs no read from `meta/runtime/runs/` and creates no fallback cache/database.

## 3. Run projection DTOs

`RunSummaryDTO` projects only serialization-safe facts:

- opaque `RunID`;
- lifecycle `State` and human-readable `Status`;
- optional current `TaskID`;
- optional `Priority`, `WaitReason`, `Resource`, and `LaneID` projections;
- lifecycle timestamps.

`Priority` and `WaitReason` are deliberately opaque strings in G05.2. This gate does not define priority levels, queue ordering, starvation policy, lock derivation, wait-state transitions, lane count, or allocation policy. Those semantics belong to G05.4-G05.6.

`Resource` and `LaneID` use the frozen G05.1 opaque domain identities. They are projections only; their presence does not authorize a GUI caller to choose resource ownership or browser profiles.

Per-run task and activity history use `RunTaskDTO` and `RunActivityDTO`. Activity DTOs intentionally omit arbitrary payload bodies and browser/provider secrets.

## 4. Command contract

G05.2 reserves exactly six run-targeted lifecycle command kinds:

- `run.start`
- `run.pause`
- `run.resume`
- `run.stop`
- `run.cancel`
- `run.retry`

These are distinct from the already locked single-run G02 command kinds `start/pause/resume/stop/cancel/retry`.

`NewRunControlCommand` creates only a typed `CommandRequest` envelope. It:

- validates `RunID` through the frozen G05.1 `domain.RunIdentity` rules;
- sets `ContractVersion` and the requested `run.*` kind;
- places the opaque run ID only in `CommandRequest.RunID`;
- leaves `TaskID`, `Resource`, and `Payload` empty;
- rejects command kinds outside the six-item G05.2 catalog.

No `run.*` command is operationally dispatched in G05.2. This is intentional fail-closed staging: a run-targeted command must not accidentally fall through to the existing global single-run lifecycle implementation.

Run creation/enqueue intent, priority mutation, queue reordering, resource lock control and browser-lane selection are not defined here.

## 5. Event contract

G05.2 reserves category `RUN` and exactly three typed event payload families:

- `run_state` → `RunStateEventPayloadDTO`
- `run_progress` → `RunProgressEventPayloadDTO`
- `run_task` → `RunTaskEventPayloadDTO`

The existing `DesktopEvent.RunID` / `TaskID` envelope fields remain the correlation seam. The payloads do not include cookies, credentials, profile paths, browser paths, authorization tokens or raw provider payloads.

G05.2 does not synthesize or persist these events. Emission begins only when later owning gates have authoritative transitions to project.

## 6. Five-method boundary

No sixth AppRuntime method is added. In particular there is no `ListRuns`, `GetRun`, `PauseRun`, scheduler object, Store handle, WebAI handle or direct desktop runtime channel.

Future Run Center UI must continue to:

- read through `Query` / `Snapshot`;
- control through typed `Dispatch` commands;
- reconcile through `Subscribe` plus fresh reads;
- close through `Close`.

The contract remains additive under `ainovel.desktop.v1` and does not change `SchemaVersion` in this gate.

## 7. Explicitly CLOSED after G05.2

G05.2 does not authorize:

- `G05.3` run registry/history persistence or writes under `meta/runtime/runs/`;
- `G05.4` scheduler execution, priority levels, ordering, aging or queue mutation;
- `G05.5` resource-key derivation, lock acquisition/release or conflict serialization;
- `G05.6` Browser Lane Pool, profile creation, lane allocation or recovery ownership;
- `G05.7` Run Center UI or multi-run orchestrator;
- direct Host/Store/WebAI/SessionManager/Engine access from desktop UI;
- direct AI API/provider fallback;
- generic filesystem mutation surfaces.

## 8. Acceptance boundary

The G05.2 candidate must remain a bounded AppRuntime contract delta on top of the exact accepted G05.1 head.

Acceptance requires exact-head CI/W6A/W6B/W6C as required by repository authority, followed by branch-head, parent-to-candidate boundary and blocking-review rechecks.

Only after G05.2 is PASS / LOCKED may G05.3 be considered for a separate AUTHOR authorization. G05.3 must not open automatically.
