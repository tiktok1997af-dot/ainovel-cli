# G08.4 — Cold-start CoCreate turn + existing `start(mode=new)` handoff

Status: IMPLEMENTATION CANDIDATE — NOT AUTHOR-CONFIRMED / NOT LOCKED.

Parent authority: G08.3 locked at `60933c4e2ca076a10f9e88ee061b0483b129b25a`.

## Purpose

G08.4 opens only the cold-start `cocreate.turn` execution slot already frozen by G08.2/G08.3.

The dedicated AppRuntime adapter now reconstructs sanitized Host CoCreate continuity, calls the existing Host `CoCreateStream` path, and projects only the frozen desktop `CoCreateTurnResultDTO`.

## Existing start-new handoff

G08.4 does not create a second start command or automatically start a book when a turn reports `ready=true`.

The accepted cumulative `draft` returned by `cocreate.turn` is the exact creative requirement handed to the already-existing lifecycle command:

```json
{
  "kind": "start",
  "payload": {
    "mode": "new",
    "requirement": "<accepted cocreate draft>"
  }
}
```

That existing path remains the sole authority for `PrepareUserRules` + `StartPrepared`, lifecycle transitions and Engine start.

## Preserved authority and privacy

- G05 remains the only Run Center scheduler/resource/browser-lane authority.
- `Runtime.commandMu` still serializes CoCreate admission against command authority.
- Any G05 managed work still blocks CoCreate before Host/WebAI execution.
- AppRuntime calls the existing Host `CoCreateStream`; it does not access WebAI/SessionManager directly.
- Host raw XML/provider output is not returned to desktop. Only message/draft/ready/suggestions + bounded session metadata cross the contract.
- CoCreate history remains transient transport state; no new Store/DB/session truth is introduced.
- WEB-only / NO-API remains unchanged.

## Still closed

G08.4 does not open:

- stage `cocreate.turn`;
- `cocreate.stage.begin`;
- `cocreate.stage.finish`;
- `cocreate.stage.cancel`;
- desktop CoCreate workspace/controller;
- Folder View;
- direct WebAI/SessionManager access;
- new Host/Engine/scheduler/browser lane;
- API/provider fallback.

Those remain G08.5+ scope.

## Bounded regression target

Acceptance candidate regression is bounded to:

- `internal/appruntime` CoCreate contract/runtime + lifecycle start-new handoff;
- existing `internal/host` CoCreate protocol tests;
- exact-head validation against the G08.3 locked parent.

G08.5 must remain CLOSED until a separate AUTHOR instruction.
