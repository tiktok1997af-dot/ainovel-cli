# AINOVEL Desktop — G04.2 Desktop Shell / UI Boundary Contract v1

Status: `G04.2 — CONTRACT CANDIDATE`

Parent: `docs/desktop-g04-creative-studio-authority-v1.md`

Baseline: `main@022d1b79cbce1bd5150a97592d94c8f6684fc490`.

Purpose: freeze the desktop shell/frontend boundary before any Creative Studio production UI implementation begins.

## 1. Contract goal

The Creative Studio frontend is a presentation/client layer over AppRuntime. It owns navigation, local interaction state and rendering. It does not own canonical story/runtime state or core execution.

The boundary is:

```text
UI Components
    |
    v
Workspace Controllers / View Models
    |
    v
Desktop Runtime Adapter
    |
    v
AppRuntime
Snapshot | Query | Dispatch | Subscribe | Close
```

The `Desktop Runtime Adapter` is a serialization/transport binding around the existing AppRuntime contract. It is not an alternate business-logic service.

## 2. Allowed frontend responsibilities

Frontend code may own:

- route and tab selection;
- pane/drawer visibility;
- filters/sorts that do not redefine canonical facts;
- form input prior to submission;
- unsaved/dirty indicators;
- request cancellation and stale-response suppression;
- loading/empty/error/conflict presentation;
- command button enablement derived from authoritative state;
- responsive presentation state;
- accessibility/focus/keyboard behavior;
- local display preferences that do not affect story/runtime correctness.

## 3. Forbidden frontend responsibilities

Frontend code must not:

- import or call `internal/host`;
- import or call `internal/store`;
- import or call `internal/webai`;
- import Engine/Workers/Arbiter/local Tools;
- read/write arbitrary project files as a persistence mechanism;
- construct canonical chapter/outline/knowledge facts without AppRuntime;
- persist Context/Canon mirrors as desktop truth;
- manage browser credentials/cookies;
- decide lifecycle state transitions independently from AppRuntime;
- accept/promote canonical story output through local-only state;
- create a frontend database that competes with Store/project files.

## 4. Adapter surface

G04 shell code must depend on an abstract desktop runtime client matching the existing five-plane facade conceptually:

```text
snapshot()                  -> DesktopSnapshot
query(QueryRequest)         -> QueryResult
dispatch(CommandRequest)    -> CommandResult
subscribe(EventCursor)      -> DesktopEvent stream
close()                     -> completion/error
```

Concrete language/IPC binding is implementation-specific, but it must preserve the locked `ainovel.desktop.v1` / SchemaVersion 1 serialization contract unless an explicit later authority changes that version.

No adapter method may expose Host/Store pointers, Go error objects, filesystem handles, WebAI/session objects, browser storage or provider objects to UI code.

## 5. Shell bootstrap sequence

The target shell startup sequence is:

```text
1. create/connect Desktop Runtime Adapter
2. request initial AppRuntime Snapshot
3. establish DesktopEvent subscription from a valid cursor
4. render shell using authoritative projected state
5. lazily issue bounded Query requests for the active workspace
6. reconcile command/query/event responses by request/revision identity
```

Failure to load one bounded workspace query must not force the UI to invent fallback canonical data.

## 6. Navigation model

The common shell reserves these primary product destinations from the G01 contract:

```text
overview       Tổng quan
project        Dự án
creative       Sáng tác
knowledge      Tri thức
review         Review
run_center     Run Center
settings       Cài đặt
```

During G04, only destinations backed by existing authority may be functional. Later-gate destinations may be present as explicit disabled placeholders but may not simulate core behavior.

Route identity is frontend-local. Project/chapter/runtime identity is authoritative AppRuntime data.

## 7. Wide-layout regions

The default desktop Creative Studio layout targets:

```text
┌───────────────┬──────────────────────────────────┬──────────────────────┐
│ Navigation    │ Main Workspace                   │ Context / Inspector  │
│ ~256 px       │ flexible                         │ ~360 px              │
└───────────────┴──────────────────────────────────┴──────────────────────┘
```

The shell also provides a persistent or readily visible runtime/status area for lifecycle state, command progress, recovery and errors.

Responsive behavior:

- desktop: three-region studio when useful;
- tablet: collapse to two functional regions;
- mobile: one focused region with tabs/drawers;
- collapsing a pane must not discard canonical data or silently submit transient edits.

## 8. Global header/status contract

The shell may project these classes of facts from `DesktopSnapshot` and events:

- product/project identity;
- current chapter summary;
- runtime lifecycle state;
- browser readiness/status projection;
- recovery state;
- agent/activity summary where already provided;
- quality summary where already provided;
- active command progress/error state.

The shell must not require raw terminal logs for normal runtime understanding.

## 9. Lifecycle control binding

Global control set:

```text
Start | Pause | Resume | Stop | Cancel | Retry
```

Interaction contract:

1. User action creates the appropriate typed lifecycle `CommandRequest`.
2. UI enters a local `command_pending` presentation state associated with that command ID.
3. `AppRuntime.Dispatch` determines acceptance/rejection.
4. Accepted asynchronous work is followed through `DesktopEvent` and fresh snapshots.
5. UI updates authoritative lifecycle presentation only from returned/projected AppRuntime facts.
6. Rejection/error restores a deterministic presentation state and surfaces the structured error.

The UI must not model `click = state transition succeeded`.

## 10. Query-to-workspace mapping

G04 workspaces consume the locked G03 read catalog as follows:

| Workspace | Query kinds |
| --- | --- |
| Project overview | `project.overview` |
| Chapters | `chapters.list`, `chapters.get` |
| Outline | `outline.get` |
| Documents | `documents.list`, `documents.get` |
| Context | `knowledge.context` |
| Canon | `knowledge.canon` |
| Characters | `knowledge.characters` |
| World | `knowledge.world` |
| Timeline | `knowledge.timeline` |

This table is routing authority, not permission to broaden query payloads or bypass existing bounds.

## 11. Mutation-to-UI mapping

The shell/editor may eventually expose only the locked G03 mutation kinds:

| UI intent | Mutation kind |
| --- | --- |
| Edit project metadata | `project.metadata.update` |
| Edit premise | `project.premise.update` |
| Save chapter plan | `chapter.plan.save` |
| Save chapter draft | `chapter.draft.save` |
| Save editable chapter workspace | `chapter.workspace.save` |
| Revise future outline tail | `outline.tail.revise` |
| Expand arc | `outline.arc.expand` |
| Append volume | `outline.volume.append` |
| Update outline compass | `outline.compass.update` |
| Replace core characters | `knowledge.characters.core.replace` |
| Replace world rules | `knowledge.world.rules.replace` |
| Append timeline | `knowledge.timeline.append` |
| Update relationships | `knowledge.relationships.update` |
| Update foreshadow state | `knowledge.foreshadow.update` |

G04.2 does not implement these controls. This mapping only prevents later UI work from creating an unapproved alternate write route.

## 12. View-model state contract

Every workspace view model should be expressible with these classes:

```text
route / selection
request identity
source revision / timestamp where available
load state
projected authoritative data
transient local edit state
validation state
conflict state
command state
error state
```

Recommended load-state vocabulary:

```text
initial
loading
ready
empty
refreshing
saving
command_pending
validation_error
conflict
runtime_error
unsupported
```

Names in concrete frontend code may vary, but semantic distinctions must remain testable.

## 13. Stale-response / race rule

A response/event must not overwrite newer UI context merely because it arrives later.

At minimum, workspace controllers must associate async operations with enough context to verify:

- current project identity;
- current route/selection;
- request/command identity;
- relevant authoritative revision/timestamp where supplied.

When context no longer matches, the obsolete result is discarded or used only to update non-conflicting global authoritative projection.

Frontend stale-response suppression is a presentation safety rule. It does not replace G03 stale-write/precondition conflict handling.

## 14. Refresh-after-write rule

After a successful mutation, the UI must reconcile with authoritative post-mutation result and/or issue the bounded query/snapshot refresh required by that contract.

The UI must not assume its submitted payload is byte-for-byte the final canonical projection because core coordination may normalize, reject, deduplicate or preserve protected history.

## 15. Conflict presentation rule

When AppRuntime reports a precondition or stale/revision conflict, UI behavior must be explicit:

- do not auto-overwrite;
- keep the user's unsaved transient text available when safe;
- show that canonical state changed or guard failed;
- allow reload/reconcile/retry only through the typed contract;
- never downgrade a conflict into a generic success toast.

## 16. Chapter semantic boundary

The UI must preserve distinctions among:

```text
chapter plan
chapter draft
editable workspace
accepted ChapterRecord / canonical accepted history
```

A workspace save is not an acceptance/promotion action. G04 must not invent local behavior that rewrites accepted ChapterRecord or bypasses revision workflow.

## 17. Documents boundary

Documents UI is a logical supported-artifact catalog/view.

It must not provide:

- arbitrary filesystem navigation as canonical product behavior;
- path-addressed generic writes;
- recursive project editing;
- write authority merely because a document can be viewed.

Any later Folder View capability must follow its own authority and containment/safety rules.

## 18. Knowledge boundary

G04 knowledge presentation preserves G03 ownership:

- Context: computed/ephemeral bounded projection;
- Canon: provenance-bearing semantic projection, not duplicate persistence;
- core Characters and supporting Cast retain distinct ownership/origin semantics;
- Timeline remains under WorldStore;
- historical/scoped reads retain their leakage boundaries.

The frontend must not merge these into an untyped generic JSON knowledge database.

## 19. Error contract

UI consumes structured `AppError` categories/messages from AppRuntime and maps them to stable product states.

The UI must not display raw internal filesystem paths, raw Store errors, provider/browser internals, cookies/session storage or stack traces as ordinary user-facing errors.

Diagnostic logs may preserve deeper internal information only through existing approved logging boundaries.

## 20. Testing obligations for G04.3+

Every production shell/workspace child gate must add tests appropriate to its layer, covering at least:

- no forbidden core imports/dependencies;
- deterministic loading/empty/error states;
- stale-response suppression;
- lifecycle button state derived from authoritative projection;
- command rejection/error behavior;
- query routing and bounded request construction;
- conflict-safe mutation UI where writes are added;
- responsive layout state without canonical data loss;
- WEB-only / NO-API regression remains green.

## 21. G04.2 non-implementation lock

This contract does not authorize production UI code by itself. Until its exact-head gate passes:

- no Creative Studio source implementation is authorized;
- no editor implementation is authorized;
- no core/AppRuntime/Host/Store/WebAI modification is authorized;
- G04.3 remains closed.

After G04.2 exact-head PASS, G04.3 may implement the common Creative Studio workspace shell strictly within this boundary.