# G08.6 — Desktop CoCreate workspace/controller

Status: IMPLEMENTATION CANDIDATE — NOT AUTHOR-CONFIRMED / NOT LOCKED.

Parent authority: G08.5 locked at `9a1dbc86bf5fad34ac5d189e875bc06277dddc50`.

## Purpose

G08.6 opens only the desktop presentation/controller layer over the already-locked AppRuntime CoCreate contract and G08.4/G08.5 execution semantics.

The implementation lives in `internal/desktopui` and follows the existing desktop boundary: UI state talks only to `RuntimeClient`; it never imports or calls Host, Store, Engine, WebAI or SessionManager directly.

## Workspace state

`CoCreateWorkspaceState` is transient presentation state only. It carries:

- current `cold_start` or `stage` mode;
- sanitized alternating history using the frozen AppRuntime DTO;
- current cumulative draft, ready flag and bounded suggestions;
- stage-active presentation state;
- pending action / command ID / safe projected error state;
- explicit cold-start handoff acceptance state.

No CoCreate history or workspace field is persisted as Canon, project memory, Store/DB truth, scheduler state or browser identity.

## UX command wiring

G08.6 exposes the bounded controller flow:

- **Send** in cold-start mode -> `cocreate.turn(mode=cold_start)`;
- **Begin stage** -> `cocreate.stage.begin`;
- **Send** in stage mode -> `cocreate.turn(mode=stage)`;
- **Finish stage** -> `cocreate.stage.finish` with the currently rendered cumulative draft;
- **Cancel stage** -> `cocreate.stage.cancel`;
- **Start new** after explicit user acceptance -> the already-existing `start` command with `mode=new` and `requirement=<accepted draft>`.

Model `ready=true` remains presentation data, not start authority. The explicit desktop Start action is the acceptance that triggers the existing start-new lifecycle path.

## Fail-closed presentation ordering

- only one CoCreate UI command may be pending at a time;
- all controls are disabled while a command is pending;
- stage Send/Finish/Cancel are unavailable until stage begin has succeeded;
- cold-start Send/start-new is unavailable while stage is active;
- failed turns do not append local user/assistant continuity;
- history reserves both user and assistant slots and never exceeds the frozen `MaxCoCreateHistoryItems` bound;
- runtime result envelopes are decoded with unknown-field rejection and are rechecked for mode, stage state, history count and finite message/draft/suggestion bounds before rendering;
- unknown/raw runtime failures map to the existing safe desktop error projection.

Stage begin/finish/cancel and cold-start start-new refresh the authoritative AppRuntime snapshot after acceptance so the shell observes Host/lifecycle transitions without inventing UI-owned lifecycle truth.

## Navigation boundary

G08.6 does not add or replace primary navigation. CoCreate is a subordinate desktop workspace/controller, preserving the locked G01/G04 shell navigation and avoiding a second desktop shell.

## Preserved authority

- AppRuntime remains the sole desktop-to-core facade.
- G05 remains the sole scheduler/resource/browser-lane authority.
- The existing Host/Engine remain the only creation runtime.
- Gemini Web remains the only AI execution path.
- No direct WebAI/SessionManager access is introduced.
- No API/provider fallback is introduced.

## Still closed

G08.6 does not open:

- G08.7 CoCreate boundary/final regression gate;
- G08.8+ Folder View;
- new Host/Engine/scheduler/resource/browser lane;
- new CoCreate DB/persistence authority;
- direct provider/browser/session internals in desktop state;
- PR merge.

## Bounded regression target

Acceptance candidate regression is bounded to:

- new `internal/desktopui` CoCreate workspace/controller state and tests;
- the minimal `Controller` ownership/initialization seam;
- existing AppRuntime CoCreate command vocabulary only;
- exact-head validation against locked G08.5 parent.

G08.7 remains CLOSED until a separate AUTHOR instruction.
