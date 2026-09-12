# AINOVEL Desktop — G05.3 Additive Run Registry / History Persistence v1

Status: `G05.3 — IMPLEMENTATION CANDIDATE / PERSISTENCE-ONLY`.

Parent authority: `docs/desktop-g05-multirun-authority-v1.md`.

Accepted parent head: `d0304859e9366a212046c400589fe86b3839650e` (`G05.2 PASS / LOCKED`).

## 1. Gate mission

G05.3 opens only the durable per-run fact boundary reserved by G05.1-G05.2.

It adds an authoritative additive namespace under:

```text
meta/runtime/runs/
  <opaque-run-id>/
    run.json
    history.jsonl
```

The existing project Store remains the only persistence authority. No desktop-only database is introduced.

## 2. Persisted run facts

`RunRegistryRecord` stores only serialization-safe facts:

- schema version;
- opaque `RunID`;
- persistence source;
- opaque lifecycle `State` / human-readable `Status`;
- optional current `TaskID`;
- lifecycle timestamps.

G05.3 deliberately does **not** define the allowed scheduler lifecycle states. The strings are persistence slots only. Exact scheduler states, ordering, priority, starvation and recovery transitions remain owned by G05.4 and later gates.

`Priority`, `WaitReason`, `ResourceKey` ownership and `BrowserLaneID` allocation are not persisted by this gate because their semantics remain closed under G05.4-G05.6.

## 3. Per-run history

`history.jsonl` is append-only and receives a store-assigned monotonically increasing per-run sequence.

Each `RunHistoryRecord` contains only:

- sequence and timestamp;
- `RunID` and optional `TaskID`;
- category / type / level;
- bounded human-readable summary.

Arbitrary payload bodies, credentials, cookies, browser profile paths and provider secrets are excluded.

History may be appended only for a persisted registry run. G05.3 does not decide which lifecycle transitions should emit history; owning orchestration gates will call this persistence boundary later.

## 4. Legacy single-run projection

Legacy compatibility is read-only and non-destructive.

When `meta/runtime/runs/` has no persisted runs, `RuntimeStore.ListRunsWithLegacyProjection()` may project the existing single-run project as the reserved synthetic ID:

`legacy-single-run`

The projection:

- reads existing `meta/run.json` and `meta/progress.json`;
- exposes their available start time / project phase as compatibility facts;
- does not create `meta/runtime/runs/`;
- never rewrites `meta/run.json`;
- cannot receive persisted G05.3 history;
- disappears from the projected list once authoritative additive run records exist.

The reserved synthetic ID cannot be used for a persisted registry run.

This is a compatibility projection, not a destructive import or project-format migration.

## 5. Existing runtime compatibility

G05.3 extends `RuntimeStore`; it does not replace the existing runtime sources:

- `meta/runtime/queue.jsonl` remains unchanged;
- `meta/runtime/tasks/` remains unchanged;
- `meta/run.json` remains readable;
- existing `RuntimeStore.Reset()` retains its pre-G05 queue/task reset meaning and does not delete the new durable registry/history facts.

Future gates may add run-aware fields to queue/task audit records only when their owning scheduler/orchestrator semantics are frozen.

## 6. Safety boundary

All filesystem paths are derived only after existing `domain.RunIdentity` validation. Path-like, whitespace/control-character and invalid identities are rejected before path construction.

`run.json` uses the Store's existing atomic JSON write path. `history.jsonl` uses the existing durable append+sync path.

Directory enumeration is fail-closed: malformed run directories, missing `run.json`, mismatched IDs or invalid persisted records are surfaced as errors rather than silently ignored.

## 7. Explicitly CLOSED after G05.3

G05.3 does not authorize:

- G05.4 deterministic scheduler execution, priority values, queue ordering or starvation policy;
- G05.5 resource-key derivation, lock acquisition/release or conflict serialization;
- G05.6 Browser Lane Pool, profile creation, lane allocation or recovery ownership;
- G05.7 multi-run orchestrator or Run Center UI;
- operational `runs.*` AppRuntime query routing or `run.*` command dispatch;
- direct desktop access to Store/Host/WebAI/Engine;
- direct AI API/provider fallback;
- destructive migration of legacy project files.

## 8. Acceptance boundary

The candidate must remain a bounded persistence delta on top of exact accepted G05.2 head `d0304859e9366a212046c400589fe86b3839650e`.

Acceptance requires the repository-required exact-head regression gates, including CI and active WEB-only/NO-API release checks, followed by:

- branch-head re-read with no drift;
- parent-to-candidate boundary verification;
- zero blocking review threads;
- verification that `main` has not moved from the accepted G04 baseline while this cumulative G05 PR remains unmerged.

Only after G05.3 is PASS / LOCKED may G05.4 be considered under a separate AUTHOR authorization. G05.4 must not open automatically.
