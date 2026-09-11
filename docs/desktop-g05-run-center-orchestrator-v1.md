# AINOVEL Desktop — G05.7 Multi-Run Orchestrator + Run Center v1

Status: `G05.7 — IMPLEMENTATION CANDIDATE / CONTRACT FROZEN`

Parent authority: `docs/desktop-g05-multirun-authority-v1.md`.

Parent accepted head: `7811a60c9f0fae18c065ba8d8253fecf42708e3c` (`G05.6 PASS / LOCKED`).

## 1. Mission

G05.7 operationalizes the already frozen run control vocabulary without replacing
Host, Engine, Store, WebAI transport, scheduler, resource locks, or browser-lane
authority.

The implementation connects:

`AppRuntime run.* commands / runs.* queries`
→ `G05.4 scheduler`
→ `G05.5 canonical story resource lock`
→ `G05.6 Browser Lane Pool`
→ `existing Host / Engine execution context`

and exposes a Run Center presentation controller only through the existing
`Snapshot / Query / Dispatch / Subscribe / Close` desktop seam.

G05.8 remains closed and owns final concurrency/restart/orphan recovery regression
and G05 final merge authority.

## 2. One Engine, multiple run intents

One opened project continues to own exactly one Host and one Engine implementation.

Multiple run intents may be represented and queued concurrently. Physical story
mutation is serialized by the canonical story resource. G05.7 does not claim that
two writers may mutate the same project concurrently.

The current project execution context therefore adopts one physical lane,
`lane-001`, backed by the exact `SessionManager` already used by Host models.
This is a real lane ownership boundary rather than a display-only projection:
allocation reserves the same visible Gemini/Chrome session used by the Engine.

The G05.6 pool implementation remains capable of bounded multiple isolated lanes;
G05.7 deliberately does not create a second Host/Engine merely to consume them.

## 3. Run lifecycle

G05.7 freezes these persisted registry states:

- `queued`
- `blocked_resource`
- `blocked_lane`
- `starting`
- `running`
- `pausing`
- `paused`
- `stopping`
- `stopped`
- `cancelling`
- `cancelled`
- `recovering`
- `failed`
- `completed`

`run.start` creates a registry record when the opaque run ID is new and schedules
resume-mode project execution. New-project requirement/planning remains owned by
the existing single-run creation flow.

`run.pause`, `run.stop`, and `run.cancel` distinguish queued/blocked runs from the
active Engine run. `run.resume` requeues paused work. `run.retry` requeues failed,
cancelled, or stopped work. Terminal cleanup releases scheduler claims, story
resource claims, and browser-lane ownership.

Command acceptance remains separate from asynchronous completion.

## 4. Authority ordering

Scheduler priority chooses the next candidate unless an older persisted resource
waiter constrains which queued run is currently runnable.

The orchestrator never lets scheduler priority bypass G05.5 FIFO lock ownership.
A specific selected runnable ticket is durably dequeued only after:

1. the run exists in the G05.3 registry;
2. the canonical story resource is acquired;
3. a browser lane is allocated and ready.

Lane/resource failure does not create an alternate database or API path.

## 5. Durable state and events

Lifecycle state is persisted in `meta/runtime/runs/<run-id>/run.json`.

Every run transition appends sanitized `RUN / run_state` history under the run's
existing G05.3 history file.

Run Center queries reconstruct scheduler priority, resource ownership/waiting, and
lane ownership from their owning authorities. These facts are not copied into a
desktop database.

Live run transitions emit `RUN / run_state` AppRuntime events. UI reconciliation
uses a fresh `runs.*` query rather than trusting GUI memory.

G05.8 owns restart/orphan recovery hardening; G05.7 does not invent final restart
semantics ahead of that gate.

## 6. Browser lane safety

The adopted lane exposes only:

- opaque lane ID;
- readiness/busy/degraded/failed state;
- opaque run ID;
- recovery count;
- changed timestamp.

Browser path, profile directory, PID, cookies, credentials, browser storage, raw
provider payloads, and raw readiness errors are excluded from Run Center DTOs.

Releasing a run stops the adopted Chrome session before freeing the lane. Host may
then warm the same SessionManager again so legacy single-run compatibility remains
available. No AI API/provider fallback exists.

## 7. Run Center UI

Run Center owns:

- run list and state/status;
- priority/wait reason/resource/lane projection;
- sanitized lane list;
- selected-run task/activity projection;
- Start/Pause/Resume/Stop/Cancel/Retry controls.

The desktop package imports only AppRuntime. It does not import Host, Store, WebAI,
Engine, Workers, Arbiter, Tools, filesystem APIs, or network transport.

Run controls use `NewDesktopRunControlCommand`; the GUI supplies an opaque run ID
only and never supplies a resource key, task authority, browser profile, or path.

## 8. Compatibility and closed scope

Preserved:

- `meta/run.json` legacy projection;
- existing `meta/runtime/queue.jsonl` scheduler/resource audit;
- existing `meta/runtime/tasks/`;
- additive `meta/runtime/runs/`;
- WEB-only Gemini execution;
- one Engine implementation;
- canonical Store story truth;
- existing single-run lifecycle when no Run Center execution owns the Engine.

Still CLOSED:

- G05.8 concurrency/restart/orphan finalization;
- G05 merge to `main`;
- AI API fallback;
- hidden browser execution;
- direct desktop core access;
- destructive project migration;
- second Store/Engine/runtime database.

## 9. Acceptance

G05.7 may be locked only if the exact candidate passes:

- CI;
- W6A;
- W6B;
- W6C real packaged WEB-only desktop smoke;
- branch-head no-drift recheck;
- exact G05.6→G05.7 boundary audit;
- zero blocking review threads;
- unchanged `main@e0a8ce46d1b2c17e0b0799d7d35af21685a6e951`;
- WEB-only / NO-API invariants.

Only then may roadmap progress advance from `37/60 = 61.7%` to
`38/60 = 63.3%`.

G05.8 must not open automatically.
