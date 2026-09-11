# AINOVEL Desktop — G05.6 Browser Lane Pool Contract v1

Status: `G05.6 — IMPLEMENTATION CANDIDATE / CONTRACT FROZEN`.

Parent authority: `docs/desktop-g05-multirun-authority-v1.md`.

Parent accepted head: `961850c2edc2e12a2269e429c8872c2d541c3e15` (`G05.5 PASS / LOCKED`).

## 1. Gate mission

G05.6 introduces a bounded Browser Lane Pool above the existing WebAI `SessionManager` without changing WEB-only Gemini execution, the Engine, story persistence, scheduler/resource-lock authority, or Run Center UI.

A lane is one opaque execution slot that owns one isolated visible Chrome/Gemini Web session/profile lifecycle. The pool controls lane allocation, release, stop/restart recovery and a sanitized projection suitable for later AppRuntime integration.

G05.7 remains CLOSED and owns operational multi-run orchestration and Run Center UI integration.

## 2. Bounded capacity and identity

The pool capacity is explicit and bounded:

- minimum: `1` lane;
- maximum: `8` lanes (`MaxBrowserLaneCount`);
- no unbounded goroutine/browser fan-out is authorized.

Lane IDs are deterministic opaque `domain.BrowserLaneID` values:

- `lane-001`
- `lane-002`
- ... up to configured capacity.

The pool never accepts a raw browser profile path from GUI/runtime commands and never derives a lane ID from a filesystem path.

## 3. Allocation policy

`Allocate(runID)` is deterministic and bounded:

1. validate the opaque `RunID` through the frozen G05.1 identity rules;
2. repeated allocation for the same run is idempotent;
3. otherwise choose the first free lane in lane-ID order;
4. reserve the lane for that run before starting its browser lifecycle;
5. if no lane is free, return `ErrNoBrowserLaneAvailable` rather than creating an extra browser or falling back to an AI API.

A lane is never preempted from another run by this pool. Waiting/priority policy remains scheduler/orchestrator authority and is not implemented in G05.6.

`AUTH_REQUIRED` is a valid allocated lane state because the visible browser may require manual login. A transient readiness failure may leave the allocated lane `DEGRADED` so the same lane can be recovered. A fatal launch failure releases the reservation.

## 4. Profile isolation

Each lane owns a distinct persistent browser profile lifecycle.

If the base `SessionConfig` supplies an explicit internal `ProfileDir`, each lane uses a private child directory:

`<base-profile-dir>/<lane-id>`

If the base config uses `ProfileName`, each lane receives a private profile name:

`<base-profile-name>-<lane-id>`

If no base name is supplied, the base name is `default`.

The profile remains outside project/story persistence and survives lane stop/restart so Chrome-owned login state can persist. Profiles are never concurrently shared by two lanes.

Browser conversations remain transient execution state and are never project memory.

## 5. Release and reuse

`Release(runID)` stops the owned Chrome process before clearing transient run ownership and returning the lane to the free pool.

This ordering prevents a subsequent run from acquiring a lane whose prior visible browser process is still active. Reuse keeps the same lane-specific persistent login profile but starts a new visible browser lifecycle.

Repeated release of an already-unassigned run is idempotent.

## 6. Per-lane recovery ownership

`Recover(laneID)` is synchronous and bounded:

1. the lane must exist and be allocated;
2. mark only that lane `RECOVERING`;
3. stop that lane's current `SessionManager` process;
4. restart the same `SessionManager` with the same isolated lane profile;
5. keep the same lane ID and run owner;
6. increment only that lane's recovery counter.

Recovery does not move the run to another lane, delete login state, change story/resource authority, or use an AI API fallback.

`StopAll()` is the pool shutdown boundary: all lane browser processes are stopped and transient run allocations are cleared, while persistent browser profiles remain untouched.

## 7. Sanitized lane projection

`BrowserLaneProjection` contains exactly the operationally safe lane facts needed by later AppRuntime integration:

- `lane_id`
- `state`
- optional `run_id`
- `recovery_count`
- `changed_at`

Projection states are:

- `STARTING`
- `AUTH_REQUIRED`
- `READY`
- `BUSY`
- `DEGRADED`
- `FAILED`
- `RECOVERING`
- `STOPPED`

A READY session projects `BUSY` while it is owned by a run and `READY` only while unowned.

The projection MUST NOT contain:

- `BrowserPath`;
- `ProfileDir` or profile name;
- PID/process handles;
- cookies, credentials, tokens or browser storage;
- raw provider payloads;
- raw `SessionManager` error/reason strings that may contain local paths.

The existing internal `SessionSnapshot` remains an implementation detail and is not a safe desktop DTO.

## 8. Integration boundary

G05.6 deliberately does not replace the existing single-session production bootstrap in `host.startWebRuntime`.

That compatibility path remains unchanged so current single-run W6/W5 production behavior is preserved. G05.7 owns wiring run scheduling/orchestration to `BrowserLanePool` and projecting sanitized lane facts through the existing AppRuntime five-method seam.

G05.6 therefore adds no direct desktop-to-WebAI method, no new GUI command, and no Run Center view.

## 9. Persistence and authority boundary

Browser lane allocation is transient orchestration state in G05.6. It does not become story truth and it does not introduce a runtime database.

G05.6 does not modify:

- `meta/progress.json`;
- `meta/run.json`;
- scheduler ordering facts;
- G05.5 resource-lock ownership;
- chapter/draft/summary persistence;
- Engine/Host creative flow.

Later orchestration may project the opaque `LaneID` into the already frozen run metadata/DTO seams, but browser profile paths and credentials remain outside project files.

## 10. Explicitly CLOSED after G05.6

G05.6 does not authorize:

- G05.7 multi-run orchestrator integration;
- Run Center UI;
- GUI lane selection;
- direct desktop `SessionManager` access;
- hidden/headless browser substitution;
- AI API/provider fallback;
- unlimited browser concurrency;
- browser profile deletion/migration;
- Engine forks or alternate story persistence;
- changes to scheduler priority or G05.5 resource-lock ordering.

## 11. Regression requirements

The G05.6 candidate must prove at minimum:

- invalid/unbounded lane capacity is rejected;
- deterministic first-free lane allocation;
- bounded exhaustion fails with `ErrNoBrowserLaneAvailable`;
- same-run allocation is idempotent;
- simultaneous lanes use distinct profile directories;
- sanitized projection does not serialize browser/profile paths or PID;
- release stops a lane before deterministic reuse;
- recovery keeps lane ID, run owner and lane profile while restarting the browser;
- transient DEGRADED readiness can recover to READY/BUSY on the same lane;
- `StopAll` clears transient ownership without deleting profiles.

## 12. Acceptance boundary

The G05.6 candidate must remain a bounded Browser Lane Pool delta on top of exact accepted G05.5 head `961850c2edc2e12a2269e429c8872c2d541c3e15`.

Acceptance requires exact-head repository checks, including CI + W6A + W6B + W6C, followed by:

- branch-head re-read with no drift;
- exact G05.5-parent-to-candidate boundary verification;
- zero blocking PR review threads;
- `main` unchanged from the active G05 baseline;
- WEB-only / NO-API invariants preserved.

Only after G05.6 becomes PASS / AUTHOR-CONFIRMED / LOCKED may G05.7 be considered for a separate AUTHOR authorization. G05.7 must not open automatically.
