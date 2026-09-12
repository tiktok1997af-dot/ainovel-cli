# AINOVEL Desktop — G05.8 Concurrency / Recovery Regression / Final Authority v1

Status: `G05.8 — FINAL EXACT-HEAD CANDIDATE`

Roadmap parent: **AINOVEL Desktop Optimized v2 — 8 GATE / 60 steps**.

Branch: `feat/desktop-roadmap-v2-g05-multirun`.

Accepted parent: G05.7 exact head `9f63285dc3efd0b04529815e9f9e19bb2411256d` (`G05.7 PASS / LOCKED`).

Baseline remains `main@e0a8ce46d1b2c17e0b0799d7d35af21685a6e951` until the final G05 merge authority is satisfied.

## 1. Mission

G05.8 is the final concurrency, restart/recovery and cumulative authority gate for G05. It does not open a new feature family. It may only:

1. close restart/orphan gaps deliberately deferred by G05.7;
2. add integrated concurrency/recovery regressions over the already-frozen scheduler, resource-lock, Browser Lane Pool and Run Center contracts;
3. verify legacy compatibility and WEB-only / NO-API boundaries remain intact;
4. establish one exact-head cumulative G05 candidate for CI/W6A/W6B/W6C acceptance;
5. only after that exact-head acceptance, authorize PR #21 to leave Draft and merge.

No G06 scope opens inside this gate.

## 2. Restart authority

After a desktop process restart, no Engine goroutine and no browser-lane allocation from the prior process is trusted as live. Recovery therefore reconstructs intent only from durable Store facts:

- `meta/runtime/runs/<run-id>/run.json`;
- append-only scheduler facts in `meta/runtime/queue.jsonl`;
- append-only resource-lock facts in `meta/runtime/queue.jsonl`;
- canonical project progress/checkpoints consumed by the existing `Resume()` path.

Browser conversation, PID, browser profile paths, renderer memory and prior in-memory `activeRun` are never recovery authority.

Before orchestration resumes, Store `RecoverResourceLocks` releases every stale persisted owner with an explicit recovery fact while preserving FIFO resource waiters.

## 3. Frozen restart transition matrix

G05.8 freezes these restart semantics:

| Persisted state | Restart action |
| --- | --- |
| `queued` | preserve valid scheduler intent; if the scheduler intent was interrupted between dequeue/start boundaries, pass through `recovering` and recreate a bounded scheduler intent |
| `blocked_resource` | preserve valid scheduler ticket and FIFO waiter; missing scheduler intent is repaired through `recovering` → `queued` |
| `blocked_lane` | preserve valid scheduler intent; no browser-lane ownership is trusted across process restart |
| `starting` / `running` | release stale claims, persist `recovering`, requeue, then resume only through the existing durable `Resume()` path |
| `recovering` | ensure one deterministic scheduler intent, then persist `queued` |
| `pausing` | complete the accepted intent as `paused`; never auto-resume |
| `stopping` | complete the accepted intent as `stopped`; never auto-resume |
| `cancelling` | complete the accepted intent as `cancelled`; never auto-resume |
| `paused` / terminal states | remove impossible leftover scheduler/resource/lane claims; never auto-resume |
| empty state with impossible scheduler intent | fail closed as `failed` after cleanup |
| unknown persisted state | fail closed as `failed` after cleanup |

A restarted interrupted active run is safe to requeue because execution uses the existing resume-mode Engine against canonical durable project facts. If no resumable work remains, the existing orchestrator completes the run instead of replaying story mutation.

## 4. Concurrency invariants

The final integrated regression must prove simultaneously:

1. multiple run intents may exist concurrently;
2. one current project Host/Engine owns at most one physical mutating execution at a time;
3. the canonical story resource never has two owners;
4. one adopted Browser Lane is never allocated to two runs simultaneously;
5. a second run becomes a deterministic persisted waiter while the first run owns the story resource;
6. completion releases resource + lane authority before the oldest eligible waiter begins;
7. no terminal cleanup leaks scheduler tickets, resource claims, lane ownership or `activeRun` state;
8. scheduler priority/age and resource FIFO facts survive restart when already durably present;
9. cancellation/stop/pause intent is not lost or inverted by restart.

These regressions extend rather than replace the G05.4 scheduler anti-starvation tests, G05.5 crash-safe resource-lock replay tests and G05.6 lane isolation tests.

## 5. Fail-closed recovery

Recovery never invents a second runtime database and never trusts partial GUI state.

Malformed scheduler/resource replay still causes AppRuntime initialization to fail closed through the existing Store validators. Unsupported per-run lifecycle state is cleaned of execution authority and persisted as `failed` with sanitized history rather than guessed into a runnable state.

A restart-recreated scheduler ticket uses the existing frozen default priority when no durable ticket survives. Existing durable tickets retain their priority and age.

## 6. Boundary preservation

G05.8 does not change:

- Store/project files as canonical story truth;
- AppRuntime `Snapshot / Query / Dispatch / Subscribe / Close` as the desktop seam;
- Run Center as a projection/control surface rather than scheduler authority;
- one Engine implementation per project Host;
- bounded visible Gemini Web browser execution;
- no AI API/provider fallback;
- no credential/cookie extraction;
- no generic filesystem surface;
- no destructive project migration;
- legacy `meta/run.json`, runtime queue and task-log compatibility.

The only new Host surface is a bounded restart-only wrapper over the already-implemented Store `RecoverResourceLocks` primitive.

## 7. Candidate boundary

The G05.8 candidate is exactly one commit on accepted G05.7 head and is restricted to five files:

```text
docs/desktop-g05-final-authority-v1.md
internal/appruntime/runtime.go
internal/appruntime/run_recovery.go
internal/appruntime/g05_final_regression_test.go
internal/host/desktop_run_recovery.go
```

No Store format, scheduler policy, resource-lock algorithm, Browser Lane Pool implementation, Run Center UI, WebAI transport, release workflow or project-format file may be changed by this candidate.

## 8. Exact-head acceptance

G05.8 becomes `PASS / LOCKED` only when the same exact candidate SHA satisfies all of the following:

1. CI SUCCESS;
2. Linux format/vet/full tests SUCCESS;
3. required critical race tests SUCCESS;
4. Windows format/vet/full regression SUCCESS;
5. repository-wide WEB-only / W5.5 NO-API audit SUCCESS;
6. W6A Release Packaging Gate SUCCESS;
7. W6B Install Update Integrity Gate SUCCESS;
8. W6C Release Artifact Desktop Smoke SUCCESS, including real Windows packaged production WEB-only smoke;
9. branch head remains the exact tested SHA;
10. `main` remains `e0a8ce46d1b2c17e0b0799d7d35af21685a6e951` before merge;
11. G05.7→G05.8 remains exactly one commit and only the five authorized files above;
12. blocking review threads = 0 and blocking review submissions = 0;
13. cumulative G05 compare remains behind 0 from the locked G04 baseline.

Any corrective code change creates a replacement candidate SHA and resets exact-head acceptance evidence.

## 9. PASS effect and final merge authority

Only after section 8 is satisfied may authority become:

```text
G05.8 = PASS / LOCKED
G05 = COMPLETE
roadmap progress = 39/60 = 65.0%
```

G05.8 is the only G05 child gate allowed to authorize the cumulative PR #21 to leave Draft and merge.

After merge, the resulting `main` commit must be re-read and verified to contain the exact accepted G05 authority/content before G06 may open. If the merge result or post-merge `main` verification differs from the accepted cumulative authority, G06 remains closed.
