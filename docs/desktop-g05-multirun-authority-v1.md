# AINOVEL Desktop — G05 Multi-Run Orchestration Authority v1

Status: `G05 — FORWARD AUTHORITY / SCOPE FROZEN`

Roadmap parent: **AINOVEL Desktop Optimized v2 — 8 GATE / 60 steps**.

Baseline: `main@e0a8ce46d1b2c17e0b0799d7d35af21685a6e951` (`G04 COMPLETE / MAIN VERIFIED`).

Implementation branch: `feat/desktop-roadmap-v2-g05-multirun`.

Parent authority chain:

- `docs/desktop-g01-foundation-authority-v1.md`
- `docs/desktop-g02-appruntime-contract-v1.md`
- `docs/desktop-g03-roadmap-successor-authority-v1.md`
- `docs/desktop-g04-creative-studio-authority-v1.md`
- `docs/desktop-g04-final-authority-v1.md`

## 1. Provenance and audit decision

The audited G04-merged baseline contains no authoritative `desktop-g05-*` file and repository code search does not recover a verbatim historical G05 child-step sequence. This document therefore does **not** claim to reproduce missing historical wording.

G05 continues as a forward-only successor authority reconstructed from the locked product/runtime invariants that already reserve multi-run capabilities:

- G01 requires a final product with a multi-run scheduler, queue, priority, resource locks, isolated browser lanes, retry/watchdog/recovery and a Run Center;
- G01 requires `meta/run.json` to remain readable for single-run projects, requires existing runtime queue/task logs to be extended rather than replaced, and requires additive multi-run metadata under `meta/runtime/runs/`;
- G02 freezes AppRuntime as the sole desktop-to-core seam and already reserves `RunID`, `TaskID` and `Resource` fields in command/event DTOs while explicitly deferring locks, worker pools and Browser Lane Pool to a later gate;
- G04 intentionally leaves Run Center and all multi-run/resource-lock/browser-lane ownership closed.

These facts are sufficient to freeze a forward G05 mission and child sequence without changing earlier contracts.

## 2. G05 mission

G05 owns **Multi-Run Orchestration & Run Center**.

G05 may extend the existing single-run runtime into a bounded, recoverable multi-run control plane while preserving one Engine implementation, canonical project Store facts and WEB-only Gemini execution.

G05 owns:

- stable opaque run/task/resource/lane identities;
- additive per-run runtime metadata/history;
- deterministic scheduler queue and priority policy;
- logical resource locking/serialization;
- isolated browser lane allocation and per-lane readiness/recovery projection;
- multi-run orchestration behind AppRuntime;
- run-aware events/snapshots/queries/commands;
- Run Center UI for active / queued / completed / failed runs and browser lanes;
- bounded recovery/restart semantics for queued and active runs;
- concurrency, starvation, cancellation, restart and compatibility regressions.

G05 does not replace story/project persistence, the Engine, Host domain logic or WebAI transport.

## 3. Source-of-truth and persistence boundary

The following are non-negotiable:

1. Store/project files remain canonical story truth.
2. Browser conversations remain transient execution state, never project memory.
3. Existing `meta/run.json` remains readable for legacy/single-run projects.
4. Existing `meta/runtime/queue.jsonl` and `meta/runtime/tasks/` remain durable runtime audit/replay sources and may gain run-aware fields; they are not replaced by an unrelated queue database.
5. New durable per-run metadata, when G05.3 opens it, lives additively under `meta/runtime/runs/`.
6. No desktop-only SQLite/JSON database becomes canonical runtime authority.
7. Scheduler ordering facts must not be written into canonical story progress files.
8. No destructive project-format migration is authorized by G05.

The scheduler may maintain bounded in-memory indexes/caches, but after restart it must reconstruct authoritative state from the canonical additive runtime facts rather than from GUI memory.

## 4. AppRuntime boundary

Desktop UI continues to use exactly:

`Snapshot / Query / Dispatch / Subscribe / Close`.

G05 may add serialization-safe run-aware DTO fields, Query kinds and Command kinds additively under `ainovel.desktop.v1`, but must not add a second frontend-to-core transport.

The existing `CommandRequest.RunID`, `TaskID`, `Resource` and corresponding result/event fields are reserved integration seams, not permission for the GUI to choose locks or filesystem paths.

Rules:

- scheduler/lock/lane decisions live behind AppRuntime;
- GUI may request supported run operations and priority values through typed commands only;
- GUI never calls Host, Store, WebAI, SessionManager or Engine directly;
- GUI never acquires/releases a resource lock itself;
- GUI never supplies an absolute path as a resource authority;
- command acceptance remains distinct from asynchronous run completion;
- events + fresh snapshots/queries remain the reconciliation path.

## 5. Scheduler / queue / priority ownership

The scheduler is a core orchestration component behind AppRuntime.

It MUST eventually:

- choose runnable work deterministically from persisted run intent;
- honor bounded priority policy frozen in G05.4;
- prevent starvation with an explicit deterministic policy rather than ad-hoc goroutine timing;
- distinguish queued, blocked-on-resource, blocked-on-lane, running, terminal and recovery states without inventing a competing GUI state machine;
- emit run-aware durable transitions suitable for replay;
- keep cancellation/retry idempotent where the underlying lifecycle contract permits it.

The UI may present/reorder only through supported typed commands. Direct mutation of queue files is prohibited.

Priority is scheduler policy, not a thread/process OS priority and not a browser priority. Exact levels, tie-breaking and aging semantics belong to G05.4 and are not guessed by G05.1-G05.3.

## 6. Resource lock ownership

Resource locks are logical orchestration locks, not user-visible file locks.

G05.5 owns:

- canonical resource-key derivation;
- lock acquisition/release around mutating run work;
- deterministic conflict ordering;
- crash/recovery cleanup;
- no-leak/no-double-owner invariants.

At minimum, two mutating runs must never concurrently own the same canonical story resource. Finer-grained resources may be introduced only when their safety can be proven; optimistic frontend concurrency does not bypass this rule.

The GUI may display lock owner/wait reason but never becomes lock authority.

## 7. Browser Lane Pool ownership

G05.6 extends the existing WebAI `SessionManager` model into a bounded Browser Lane Pool without changing WEB-only execution.

A browser lane:

- has an opaque lane ID;
- owns an isolated visible Chrome/Gemini Web session/profile lifecycle;
- exposes readiness/busy/degraded/failed/recovery projection through AppRuntime;
- never exports cookies, credentials or browser secrets to project files or desktop DTOs;
- is allocated/released by orchestration, not selected through a raw profile path from the GUI.

Lane count must be explicitly bounded. G05 does not authorize hidden browser execution or an AI API fallback when lanes are exhausted.

## 8. Multi-run orchestration boundary

Multi-run means the product may have multiple run intents/runs represented and scheduled concurrently, subject to resource and lane availability. It does **not** mean concurrent writes to the same canonical story resource are automatically safe.

Each run must have stable run identity and its own projected lifecycle/history. Engine/Workers/Arbiter remain one implementation reused per execution context; G05 must not fork a second novel engine.

Recovery must distinguish:

- queued but never started;
- blocked on resource/lane;
- active with recoverable checkpoint;
- terminal completed/cancelled/failed;
- orphaned/uncertain state after process restart.

Exact recovery transitions are frozen in the owning child gates before implementation.

## 9. Run Center ownership

Run Center is a desktop projection/control workspace, not the scheduler itself.

It eventually presents:

- active runs;
- queued/blocked runs;
- completed/failed/cancelled runs;
- per-run task/activity history;
- priority and wait reason;
- resource ownership/waiting facts;
- browser lane state/allocation;
- supported run lifecycle/retry/cancel controls.

All facts come through AppRuntime Snapshot/Query/Subscribe. All mutations use typed AppRuntime Dispatch commands. No Run Center view may directly edit `meta/runtime/*` files.

## 10. Closed scope

G05 MUST NOT introduce:

- direct AI API/provider fallback;
- hidden/non-visible browser as a substitute for a lane;
- credential/cookie extraction;
- a second Store, Engine, project database or desktop canonical runtime DB;
- generic filesystem write/delete/move surfaces;
- direct desktop Host/Store/WebAI/Engine calls;
- Review/12-Gate execution features not already required for run scheduling;
- unrelated Settings or Folder View ownership;
- destructive project-format migration;
- unlimited worker/browser concurrency;
- OS-process priority as product scheduling policy.

## 11. Forward-frozen G05 child sequence

The authoritative forward sequence is:

```text
G05.1  Run identity / compatibility foundation                       OPEN after G05 authority freeze
G05.2  AppRuntime Run Center query/command/event contracts            CLOSED until G05.1 PASS
G05.3  Additive run registry / history persistence + legacy projection CLOSED until G05.2 PASS
G05.4  Deterministic scheduler queue + priority policy                 CLOSED until G05.3 PASS
G05.5  Resource lock manager + conflict serialization                  CLOSED until G05.4 PASS
G05.6  Browser Lane Pool + isolated session/recovery ownership         CLOSED until G05.5 PASS
G05.7  Multi-run orchestrator + Run Center UI integration              CLOSED until G05.6 PASS
G05.8  Concurrency/recovery regression + final authority/merge gate    CLOSED until G05.7 PASS
```

This is a forward authority. It is not labeled as recovered verbatim history.

With G01=8, G02=7, G03=8, G04=8 and this G05=8, completion of G05 would bring the roadmap to 39/60. This document does not allocate or rename the remaining G06-G08 child steps.

## 12. G05.1 scope

G05.1 is deliberately narrow. It freezes stable serialization-safe identity primitives and compatibility rules required by every later G05 child gate.

G05.1 MAY:

- add opaque typed `RunID`, `TaskID`, `ResourceKey` and `BrowserLaneID` primitives in the core domain;
- add validation preventing empty/ambiguous/path-like/control-character identities;
- add tests for deterministic identity validation;
- document the legacy single-run compatibility rule.

G05.1 MUST NOT:

- add scheduler execution;
- persist a run registry;
- define priority levels/tie-breaking;
- acquire resource locks;
- start more than the existing browser/session behavior;
- open Run Center UI;
- register new AppRuntime run queries/commands before G05.2;
- rewrite `meta/run.json` or runtime queue files.

## 13. Acceptance rule

Every G05 child candidate must be validated on its exact head with the repository-required regression gates. At minimum this means CI + WEB-only/NO-API checks and any active release gates (W6A/W6B/W6C) required by the repository.

A child gate becomes PASS/LOCKED only after:

- exact-head checks succeed;
- branch head is re-read with no drift;
- authorized parent-to-head boundary is verified;
- blocking review threads are zero;
- its successor remains closed until that acceptance completes.

G05.8 alone may authorize the cumulative G05 PR to leave Draft and merge. Resulting `main` must be verified before any G06 authority opens.
