# AINOVEL Desktop — G04.6 Creative Lifecycle Control Plane v1

Status: `G04.6 — IMPLEMENTATION CANDIDATE`

Parent authority:

- `docs/desktop-g01-foundation-authority-v1.md`
- `docs/desktop-g02-appruntime-contract-v1.md`
- `docs/desktop-g04-creative-studio-authority-v1.md`
- `docs/desktop-g04-shell-ui-boundary-contract-v1.md`
- accepted G04.5 head `e125e0517f167447ed83ab9686cabd2fb7dba630`

## 1. Decision

G04.6 activates the six global lifecycle controls already reserved by the
Creative Studio shell:

- `Start`
- `Pause`
- `Resume`
- `Stop`
- `Cancel`
- `Retry`

Every control is bound only through `RuntimeClient.Dispatch`, which is the UI
adapter form of the locked `AppRuntime.Dispatch` plane. No Host, Store, WebAI,
Engine, Worker, filesystem, or browser transport object is exposed to the UI.

G04.6 does not create a lifecycle engine or a second lifecycle state machine.
The existing AppRuntime lifecycle implementation remains authoritative.

## 2. Authoritative lifecycle sources

The UI may display lifecycle state only from these existing AppRuntime outputs:

1. `CommandResult.Status` for immediate command acceptance state;
2. projected `DesktopEvent` values of category `LIFECYCLE` and type
   `lifecycle_state` for asynchronous transitions;
3. a fresh `DesktopSnapshot.Runtime.State` after every command result and every
   accepted lifecycle event.

The controller keeps only transient command presentation state:

- whether one UI command request is pending;
- its generated command ID;
- the requested action;
- whether AppRuntime accepted it;
- the status returned by AppRuntime;
- the most recently observed AppRuntime state;
- a structured UI-safe error projection.

None of these fields is canonical project/runtime storage.

## 3. Exact command catalog

G04.6 exposes exactly this mapping and no generic command dispatcher:

| UI action | AppRuntime command |
| --- | --- |
| `start` | `appruntime.CommandStart` |
| `pause` | `appruntime.CommandPause` |
| `resume` | `appruntime.CommandResume` |
| `stop` | `appruntime.CommandStop` |
| `cancel` | `appruntime.CommandCancel` |
| `retry` | `appruntime.CommandRetry` |

Unknown UI actions are rejected locally as `invalid_argument` before
`AppRuntime.Dispatch`.

The global G04.6 `Start` control uses the G02 safe default semantics explicitly:
`StartCommandPayload{Mode: StartModeResume}`. New-project preparation is not
opened by this gate. Project/editor write workflows remain owned by later gates.

G04.6 does not populate `RunID`, `TaskID`, or `Resource`. Those fields are not
used to introduce G05 scheduling/resource ownership early.

## 4. Enablement and command pending behavior

The shell's existing lifecycle eligibility projection remains the only UI
preflight table and mirrors the locked AppRuntime state rules:

- `Start`: ready / stopped / cancelled / failed;
- `Pause`: running;
- `Resume`: paused;
- `Stop`: running / paused;
- `Cancel`: running / paused;
- `Retry`: failed / cancelled / stopped.

This is a UX preflight only. AppRuntime still re-validates command legality and
may reject a command if the authoritative state changes between snapshot and
dispatch.

After a valid authoritative snapshot, an eligible control becomes enabled.
While one lifecycle command is pending, all six controls are disabled to prevent
duplicate UI dispatch. This pending guard is transient presentation state and is
not a replacement lifecycle state.

## 5. Stable command identity

The Creative Studio controller generates monotonic UI command IDs in the form:

`desktop-lifecycle-N`

The ID is sent through `CommandRequest.ID` and the returned
`CommandResult.CommandID` must match. A mismatch is treated as a safe internal
protocol error and never accepted optimistically.

`CommandRequest.ContractVersion` is always `ainovel.desktop.v1`.
`CommandResult.ContractVersion` must match the same locked contract before the
result can be accepted by the UI.

## 6. Command result reconciliation

The controller follows this order:

1. verify runtime/subscription/snapshot readiness;
2. verify the requested action belongs to the six-command catalog;
3. apply the existing lifecycle eligibility preflight;
4. set transient pending state and disable lifecycle controls;
5. call `RuntimeClient.Dispatch` once;
6. validate structured error/result contract and command identity;
7. project AppRuntime's returned status without inventing a transition;
8. clear transient pending state;
9. request a fresh `RuntimeClient.Snapshot`;
10. derive controls again from the fresh authoritative state.

A command result with `Accepted=false` and no structured error is treated as a
safe `command_rejected` result rather than UI success.

A failed Dispatch is also followed by a fresh snapshot when possible, because a
state race may have occurred even when the requested command was rejected.

## 7. Event reconciliation

`Controller.PumpEvent` continues to consume only the existing AppRuntime
subscription. It does not create another observer or event database.

For a valid projected lifecycle event:

1. the event first passes the shell's existing contract/sequence guard;
2. the lifecycle state is read from the event payload, with the event summary as
   a bounded fallback only when it is a known lifecycle state;
3. the displayed state/control eligibility is updated from that AppRuntime
   event;
4. the controller immediately requests a fresh AppRuntime snapshot;
5. the fresh snapshot wins as the final projection.

Command acceptance and asynchronous completion therefore remain distinct. For
example, `Pause` may return `pausing`; a later lifecycle event and snapshot may
then project `paused`.

## 8. Error boundary

G04.6 uses only `appruntime.AppError` fields that are already safe for the
desktop boundary. Known AppRuntime errors retain their structured code,
category, safe message, and retryable flag.

The UI additionally guards malformed/mismatched command results:

- contract mismatch -> `contract_mismatch`;
- command ID mismatch -> safe `internal` error;
- unaccepted result without error -> `command_rejected`.

Raw provider/browser/core paths, stack traces, credentials, cookies, prompts,
or model response bodies are not surfaced by this control plane.

## 9. Files and dependency boundary

G04.6 candidate delta from accepted G04.5 is intentionally limited to four
files:

- `internal/desktopui/controller.go` — activate post-snapshot and lifecycle-event
  reconciliation hooks;
- `internal/desktopui/lifecycle_controller.go` — six-command control plane;
- `internal/desktopui/lifecycle_controller_test.go` — exact command/event/error
  boundary tests;
- `docs/desktop-g04-lifecycle-control-plane-v1.md` — this authority record.

Production desktop UI code continues to import only `internal/appruntime` from
AINOVEL internals.

No AppRuntime/core/Host/Store/WebAI/persistence/project-format file is modified.

## 10. Explicitly closed scope

G04.6 MUST NOT implement or open:

- the Creative editor route;
- chapter prose editing;
- project/chapter/outline mutations;
- Knowledge mutations;
- generic `AppRuntime.Dispatch` access from renderer code;
- direct Host/Store/WebAI/filesystem access;
- a new canonical DB or project format;
- a second lifecycle state machine;
- browser credential/cookie extraction;
- AI API/provider fallback;
- G05 queue scheduling;
- G05 resource locks;
- G05 isolated browser-lane pools;
- multi-run prioritization.

`G04.7 — Creative editor + safe write-plane UI` remains CLOSED until G04.6
passes exact-head acceptance.

G05 remains CLOSED.

## 11. Tests required by the candidate

The G04.6 tests must prove:

- authoritative paused snapshot enables only Resume/Stop/Cancel;
- all six UI actions map to exactly the six locked AppRuntime command kinds;
- Start carries only safe `resume` mode;
- no command populates G05 `RunID`/`TaskID`/`Resource` fields;
- command IDs are generated by the controller and matched on result;
- post-command fresh snapshot occurs;
- lifecycle event is followed by fresh snapshot reconciliation;
- command acceptance (`pausing`) remains distinct from completion (`paused`);
- structured Dispatch errors survive as UI-safe errors;
- failed command still refreshes authoritative state;
- unknown actions never reach Dispatch;
- one pending command disables all lifecycle controls;
- mismatched command-result contracts are rejected.

Existing Project and Knowledge read-only tests remain intact.

## 12. Exact-head acceptance

G04.6 may become `PASS / LOCKED` only when one exact candidate SHA satisfies all
of the following:

- candidate parent is the accepted G04.5 SHA
  `e125e0517f167447ed83ab9686cabd2fb7dba630`;
- parent-to-candidate is exactly one commit;
- candidate diff remains exactly inside the four-file G04.6 boundary above;
- CI succeeds, including Linux/Windows regression and WEB-only/NO-API audit;
- W6A Release Packaging Gate succeeds;
- W6B Install Update Integrity Gate succeeds;
- W6C Release Artifact Desktop Smoke succeeds, including real packaged Windows
  WEB-only smoke when triggered;
- branch head is rechecked after all workflows with no drift;
- no blocking PR review thread remains.

Until those conditions are satisfied:

`G04.6 = IMPLEMENTED / EXACT-HEAD VALIDATION ACTIVE`

and locked roadmap progress remains:

`28/60 = 46.7%`

Only after exact-head PASS may progress become:

`29/60 = 48.3%`

and `G04.7 — Creative editor + safe write-plane UI` become OPEN / AUTHORIZED.

G05 remains CLOSED regardless of G04.6 acceptance.
