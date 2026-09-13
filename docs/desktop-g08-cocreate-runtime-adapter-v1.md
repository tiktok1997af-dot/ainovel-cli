# G08.3 — CoCreate runtime adapter + G05 fail-closed ownership integration

Status: IMPLEMENTATION CANDIDATE — NOT AUTHOR-CONFIRMED / NOT LOCKED.

Parent authority: G08.2 locked at `9a2f899271bffae7771ee2b40aff25a3df4d2b4a`.

## Purpose

G08.3 establishes the dedicated AppRuntime routing and ownership boundary required before any desktop CoCreate Host execution is allowed.

The frozen `cocreate.*` command family now routes to a dedicated CoCreate adapter rather than falling through to the generic mutation plane.

## G05 ownership rule

Run Center remains the only scheduler/resource/browser-lane authority.

Every CoCreate dispatch is serialized through the existing `Runtime.commandMu`. While holding that authority, AppRuntime checks the existing `run.hasManagedWork()` predicate. Any queued, blocked, active, paused/recoverable or otherwise scheduled Run Center work causes CoCreate to fail closed before Host/WebAI execution.

The adapter does not allocate a browser lane, does not create a scheduler/resource claim, and does not expose or accept caller `RunID`, `TaskID`, or `Resource` authority.

## Successor boundaries remain closed

G08.3 deliberately performs no Host CoCreate operation yet:

- `cold_start` turn execution remains G08.4;
- cold-start accepted draft -> existing `start(mode=new)` handoff remains G08.4;
- stage begin/turn/finish/cancel execution remains G08.5;
- desktop CoCreate workspace/controller remains G08.6;
- Folder View remains G08.8+.

Valid commands that pass the G08.2 typed transport contract and G05 ownership gate therefore reach the dedicated adapter and return staged `ErrNotImplemented` until their authorized successor step opens.

## Preserved invariants

- one existing Host only;
- one existing Engine only;
- one existing Run Center scheduler/resource/lane authority only;
- WEB-only Gemini execution only;
- no direct WebAI/SessionManager access from desktop;
- no new CoCreate Store/DB or persistent conversation truth;
- no raw model thinking/reasoning/provider payload exposure;
- no API/provider fallback;
- G08.4+ remain closed.
