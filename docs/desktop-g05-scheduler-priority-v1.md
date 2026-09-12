# AINOVEL Desktop — G05.4 Deterministic Scheduler Queue & Priority Authority v1

Status: `G05.4 — IMPLEMENTATION AUTHORITY / CANDIDATE`

Parent authority: `docs/desktop-g05-multirun-authority-v1.md`

Accepted parent head: `dde588db2e4f647744adf9c5c21638542936b492` (`G05.3 PASS / LOCKED`)

Roadmap progress before G05.4 acceptance: **34/60 = 56.7%**.

## 1. Purpose

G05.4 freezes the deterministic scheduler queue and bounded priority policy needed by later multi-run orchestration.

This child gate owns only:

- persisted run scheduling intent on the existing runtime queue;
- deterministic replay after restart;
- bounded scheduler priority values;
- deterministic selection/tie-breaking;
- explicit starvation prevention;
- enqueue, reprioritize, dequeue and remove queue primitives behind the existing Store.

G05.4 does **not** start a run or make a run executable. Dequeue means only that a scheduler intent has been selected/removed from the scheduling queue.

## 2. Source of truth

The authoritative scheduling journal remains:

`meta/runtime/queue.jsonl`

G05.4 extends `domain.RuntimeQueueItem` additively with:

- `run_id`;
- `run_priority`.

Existing fields and legacy queue records remain readable. Scheduler records use the existing legacy audit-channel priority `control`; product scheduling priority is carried separately by `run_priority` so the two concepts are never overloaded.

No SQLite database, desktop-only state database, alternate queue file or destructive migration is authorized.

Per-run registry/history under `meta/runtime/runs/` from G05.3 remains intact and is not replaced by scheduler state.

## 3. Bounded priority policy

The only supported scheduler priorities are:

```text
high
normal
low
```

`normal` is the default for a newly enqueued run when no explicit priority is supplied.

Base ranks are:

```text
high   = 0
normal = 1
low    = 2
```

Lower rank is selected first.

Unknown persisted priority values are corruption/unsupported facts and must fail closed. They are never silently mapped to a supported level.

## 4. Deterministic starvation prevention

G05.4 uses persisted-transition aging, not wall-clock aging.

A scheduler turn is one persisted G05.4 scheduler event in `queue.jsonl`.

A waiting ticket gains one rank every **8 scheduler turns** after its enqueue turn, capped at rank `0`:

```text
effective_rank = max(0, base_rank - floor(age_turns / 8))
```

Consequences:

- `normal` reaches effective `high` after 8 scheduler transitions;
- `low` reaches effective `normal` after 8 transitions and effective `high` after 16;
- continuous newer `high` arrivals cannot starve an older `low` run indefinitely;
- restart/replay produces the same effective ranks because no current wall clock participates.

Reprioritizing a queued run preserves its original enqueue sequence and enqueue turn. Priority changes therefore cannot reset or manufacture queue age.

## 5. Deterministic tie-breaking

Selection order is exactly:

1. lower effective rank;
2. lower durable enqueue `queue.jsonl` sequence;
3. lexicographically lower opaque `run_id` as the final deterministic fallback.

No goroutine timing, map iteration order, process priority, browser timing or current timestamp participates in ordering.

## 6. Durable scheduler event protocol

G05.4 owns four scheduler queue categories:

```text
run_scheduler.enqueued
run_scheduler.priority_changed
run_scheduler.dequeued
run_scheduler.removed
```

Replay rules:

- non-scheduler legacy queue records are ignored by scheduler reconstruction;
- `enqueued` creates one active ticket and captures its durable sequence/turn;
- `priority_changed` updates only priority and preserves enqueue age/order;
- `dequeued` removes the selected ticket from the active queue;
- `removed` removes a queued ticket without assigning cancellation/retry semantics;
- duplicate or impossible persisted scheduler transitions fail closed rather than being guessed through.

An exact duplicate enqueue with the same priority is idempotent. Changing priority through enqueue is rejected; the explicit reprioritize primitive must be used.

## 7. Store boundary

`RuntimeStore` exposes bounded scheduler primitives:

- `LoadScheduledRuns`
- `EnqueueScheduledRun`
- `SetScheduledRunPriority`
- `DequeueNextScheduledRun`
- `RemoveScheduledRun`

Only registered additive G05.3 runs may be enqueued. The synthetic `legacy-single-run` compatibility projection is read-only and cannot become a scheduler run.

`DequeueNextScheduledRun` only journals deterministic scheduler selection/removal. It must not:

- invoke Engine/Workers/Arbiter;
- dispatch an AppRuntime command;
- acquire a resource lock;
- allocate a browser lane;
- open Chrome/Gemini;
- change canonical story progress.

## 8. Compatibility rules

G05.4 preserves all earlier invariants:

- legacy `meta/run.json` stays readable and unmodified;
- G05.3 `meta/runtime/runs/` records/history stay additive and durable;
- old `queue.jsonl` records without `run_id` / `run_priority` remain readable;
- legacy `RuntimeQueuePriority` values `control/background` remain unchanged;
- WEB-only / NO-API runtime authority remains unchanged;
- `Reset` retains its pre-existing runtime queue reset semantics and does not delete G05.3 run registry/history.

## 9. Explicitly closed scope

G05.4 MUST NOT open or implement:

- G05.5 resource-key derivation, resource locks or conflict ownership;
- G05.6 Browser Lane Pool, profile allocation or lane recovery;
- G05.7 multi-run execution/orchestrator wiring or Run Center UI;
- AppRuntime operational routing for `run.cancel`, `run.retry`, `run.set_priority`, `run.pause` or `run.resume`;
- cancellation/retry lifecycle semantics;
- blocked-on-resource or blocked-on-lane transitions;
- hidden browser execution or AI API fallback;
- OS/thread/process priority;
- a second Store, Engine or runtime database.

## 10. Acceptance

G05.4 becomes `PASS / LOCKED` only when the exact candidate head satisfies all of the following:

- domain/store tests prove bounded priorities and default `normal`;
- deterministic priority/FIFO selection is proven;
- restart replay returns the same active queue/order;
- reprioritization preserves queue age;
- a sustained stream of higher-priority work cannot starve an older low-priority run under the frozen aging policy;
- malformed persisted scheduler facts fail closed;
- legacy/non-scheduler queue records remain compatible;
- exact-head CI passes on Linux and Windows;
- WEB-only / NO-API checks pass;
- active W6A/W6B/W6C release gates pass;
- branch head is re-read with no drift;
- G05.3-to-G05.4 boundary contains only authorized files;
- blocking review threads are zero;
- `main` remains the accepted G05 baseline until G05.8 authorizes merge.

Only after those checks pass may roadmap progress advance to **35/60 = 58.3%** and G05.5 open.
