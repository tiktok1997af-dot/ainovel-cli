# G08.5 — Stage CoCreate Host-wrap semantics

Status: IMPLEMENTATION CANDIDATE — NOT AUTHOR-CONFIRMED / NOT LOCKED.

Parent authority: G08.4 locked at `bf15d99e374d9847b0644e72da9d57552be54a1b`.

## Purpose

G08.5 opens only the stage CoCreate execution slots already frozen by G08.2:

- `cocreate.stage.begin`;
- `cocreate.turn` with `mode=stage`;
- `cocreate.stage.finish`;
- `cocreate.stage.cancel`.

The AppRuntime adapter wraps the existing Host seams only:

- `PauseForCoCreate`;
- `StageCoCreateStream`;
- `ResumeFromCoCreate`;
- `CancelCoCreate`.

No second CoCreate engine, scheduler, browser lane, persistence layer or provider path is introduced.

## Stage ordering

Stage execution is fail-closed:

1. `stage.begin` must succeed before a stage turn can execute.
2. A duplicate `stage.begin` is rejected.
3. While the stage window is active, cold-start CoCreate turns are rejected.
4. A stage turn failure keeps the stage window active so the caller may retry or cancel.
5. `stage.finish` terminates the AppRuntime stage window after invoking the existing Host resume/injection seam.
6. `stage.cancel` terminates the stage window through the existing Host cancel seam.
7. `stage.finish` or `stage.cancel` without an active stage is rejected.

`stageActive` is transient AppRuntime presentation/session state only. It is not Canon, project memory, a scheduler resource, a browser session identifier or persistent story truth.

## Host semantics preserved

`stage.begin` delegates occupancy and pause semantics to Host `PauseForCoCreate`.

`stage.turn` delegates model interaction and current-story-state prompt construction to Host `StageCoCreateStream`.

`stage.finish` delegates the accepted stage draft to Host `ResumeFromCoCreate`, which injects the stage-plan intervention through the existing Continue/Arbiter path and resumes creation.

`stage.cancel` delegates to Host `CancelCoCreate`, preserving Host's paused-state behavior.

AppRuntime does not access WebAI, SessionManager, Store internals or provider responses directly.

## DTO/privacy boundary

Stage turn results use the existing frozen `CoCreateTurnResultDTO` with:

- `mode=stage`;
- bounded `history_count`;
- `stage_active=true`;
- sanitized message/draft/ready/suggestions only.

Host raw XML/provider output never crosses AppRuntime.

Stage begin/finish/cancel use the existing `CoCreateStageStateResultDTO`.

## G05 and WEB-only invariants

- Existing `Runtime.commandMu` remains the command serialization authority.
- Existing `run.hasManagedWork()` remains the G05 admission guard before any CoCreate Host execution.
- CoCreate allocates no Run Center resource or browser lane.
- Gemini Web remains the only AI execution path.
- No API/provider fallback is added.

## Still closed

G08.5 does not open:

- desktop CoCreate workspace/controller;
- any new CoCreate UI state machine;
- Folder View;
- direct WebAI/SessionManager access;
- a new Host, Engine, scheduler, resource lock, browser lane or Store/DB;
- API/provider fallback.

Those remain G08.6+ scope.

## Bounded regression target

Acceptance candidate regression is bounded to:

- `internal/appruntime` CoCreate contract/runtime tests;
- existing `internal/host` CoCreate stage/protocol tests;
- G05 managed-work fail-closed regression;
- exact-head validation against locked G08.4 parent.

G08.6 must remain CLOSED until a separate AUTHOR instruction.
