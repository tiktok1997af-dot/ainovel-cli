# AINOVEL Desktop — G04 Creative Studio Authority v1

Status: `G04.2 — AUTHORITY / DESKTOP SHELL CONTRACT CANDIDATE`

Roadmap parent: **AINOVEL Desktop Optimized v2 — 8 GATE / 60 steps**.

Branch: `feat/desktop-roadmap-v2-g04-creative-studio`

Baseline: `main@022d1b79cbce1bd5150a97592d94c8f6684fc490`.

Parent authority chain:

- `docs/desktop-g01-foundation-authority-v1.md`
- `docs/desktop-g02-appruntime-contract-v1.md`
- `docs/desktop-g03-roadmap-successor-authority-v1.md`
- `docs/desktop-g03-final-authority-v1.md`
- `docs/desktop-g04-shell-ui-boundary-contract-v1.md`

This document is a forward authority for G04. It preserves the approved AINOVEL Desktop product target from G01 and the AppRuntime/core contracts locked by G02-G03. It does not claim to recover missing historical wording for G04 from an unavailable prior roadmap artifact.

## 1. G04.1 inherited audit decision

`G04.1 — Authority / Roadmap Audit & Creative Studio Scope Freeze` is accepted as `PASS / INHERITED INTO THIS AUTHORITY` when this G04.2 exact-head candidate passes its gate.

The audit established:

1. G03 is merged and verified on `main@022d1b79cbce1bd5150a97592d94c8f6684fc490`.
2. G03 deliberately stops at the AppRuntime-backed Project / Knowledge model and does not create the Creative Studio GUI workspace.
3. No authoritative G04 implementation commit, PR, or complete verbatim G04 child-step authority exists on the audited baseline.
4. G04 therefore continues forward from the locked G01-G03 invariants rather than inventing historical provenance.
5. G04 is the desktop Creative Studio presentation / interaction layer above AppRuntime. It is not a second core, Store, engine, browser transport, or project persistence authority.

## 2. G04 mission

G04 creates the primary Creative Studio desktop experience required by the G01 product contract while keeping all story/runtime correctness behind AppRuntime.

G04 owns desktop presentation and interaction for:

- common application shell and navigation;
- Project / Chapter / Outline workspace;
- supported Documents catalog/view;
- Knowledge Studio views for Context, Canon, Characters, World and Timeline;
- Creative Studio writing workspace presentation;
- lifecycle control surface for Start / Pause / Resume / Stop / Cancel / Retry;
- activity, status, error, recovery and progress presentation;
- safe UI binding to the read/write contracts already locked in G03;
- responsive desktop/tablet/mobile layout behavior required by G01.

G04 does not redefine the novel engine or persistence model.

## 3. Non-negotiable architecture boundary

The only supported dependency direction is:

```text
Creative Studio UI
        |
        v
Desktop Shell / UI Adapter / View Models
        |
        v
AppRuntime
  Snapshot / Query / Dispatch / Subscribe / Close
        |
        v
narrow Host seams
        |
        v
existing canonical Store / domain owners
```

Forbidden direct dependencies remain:

```text
Creative Studio UI -> Host                  X
Creative Studio UI -> Store                 X
Creative Studio UI -> WebAI                 X
Creative Studio UI -> Engine / Workers      X
Creative Studio UI -> filesystem mutation   X
Core packages -> Creative Studio UI          X
```

Frontend-local state may contain presentation facts such as selected route, active panel, scroll position, local draft form dirtiness and ephemeral view preferences. Any fact that affects canonical story state, runtime state, accepted history, browser execution, recovery, conflicts or revision semantics must be sourced from or committed through AppRuntime.

## 4. G04 read-plane authority

G04 consumes the existing G03 query catalog; it does not create a parallel read backend.

The frozen G03 queries are:

```text
project.overview
chapters.list
chapters.get
outline.get
documents.list
documents.get
knowledge.context
knowledge.canon
knowledge.characters
knowledge.world
knowledge.timeline
```

Rules:

1. Large content remains bounded/paged/ranged according to the existing query contracts.
2. Global shell state must not load all chapter prose or all knowledge data eagerly.
3. Documents remain a supported artifact catalog/view, never a generic filesystem browser or write surface.
4. Context remains computed/ephemeral; Canon remains provenance-bearing projection.
5. Chapter plan, draft, editable workspace and accepted ChapterRecord remain visibly distinct facts where the UI exposes them.

## 5. G04 write-plane authority

G04 may bind only the mutation catalog already locked by G03 unless a later G04 child gate proves a narrowly additive AppRuntime contract is required.

The frozen G03 mutation catalog is:

```text
project.metadata.update
project.premise.update
chapter.plan.save
chapter.draft.save
chapter.workspace.save
outline.tail.revise
outline.arc.expand
outline.volume.append
outline.compass.update
knowledge.characters.core.replace
knowledge.world.rules.replace
knowledge.timeline.append
knowledge.relationships.update
knowledge.foreshadow.update
```

Rules:

1. All writes enter through `AppRuntime.Dispatch`.
2. UI forms must preserve optimistic/precondition/revision guards exposed by the contract.
3. The UI must surface `target_not_found`, validation, precondition conflict, stale conflict, read failure and write failure as structured product states rather than silently overwriting canonical facts.
4. UI save controls never become a raw `writeFile(path, bytes)` escape hatch.
5. G04 does not add a DocumentStore, ContextStore, CanonStore, alternate Character/Timeline store, desktop database or project-format migration.
6. Unsupported product wishes remain visibly unsupported until an explicit authority adds a safe owning core/AppRuntime seam.

## 6. Lifecycle control authority

The Creative Studio must expose lifecycle controls as real product behavior, not decorative buttons:

```text
Start
Pause
Resume
Stop
Cancel
Retry
```

The UI must bind these controls to the lifecycle command semantics locked by G02 through `AppRuntime.Dispatch` and then reconcile state from command results, events and fresh snapshots.

The UI MUST NOT:

- infer that a command succeeded merely because the user clicked a button;
- optimistically force runtime state locally;
- call Host lifecycle methods directly;
- create a second state machine competing with AppRuntime;
- hide illegal-state failures by silently changing local button state.

Button enabled/disabled/busy/error presentation is derived from authoritative lifecycle state and command acceptance.

## 7. Creative Studio product shell

G04 preserves the G01 approved product navigation target:

1. `Tổng quan`
2. `Dự án`
3. `Sáng tác`
4. `Tri thức`
5. `Review`
6. `Run Center`
7. `Cài đặt`

G04 implementation scope is limited to the shell and Creative Studio views whose underlying AppRuntime contracts already exist. Later roadmap gates remain responsible for capabilities not yet backed by locked runtime contracts, including full Review/12-Gate orchestration and multi-run/resource-lock/browser-lane scheduling.

The shell must allow later workspaces to be added without redesigning the core boundary.

## 8. Layout authority

The inherited visual/layout target remains:

- desktop-first dark UI;
- common application shell;
- wide studio layout approximately `256px | flexible | 360px`;
- tablet collapses toward two functional panes;
- mobile uses one focused pane with tabs/drawers;
- status and runtime state are understandable without reading raw terminal logs;
- Folder View, when implemented under its owning later scope, may use a wider diagnostic layout.

G04.2 freezes structure and state ownership, not final colors, iconography, typography micro-tuning or animation polish.

## 9. Desktop shell state model

The shell is organized into four UI state classes:

### 9.1 Authoritative projected state

Comes from `AppRuntime.Snapshot`, `Query`, `Dispatch` results and `Subscribe` events.

Examples:

- current project identity;
- current chapter facts;
- runtime lifecycle state;
- browser readiness projection;
- recovery/error state;
- quality summary already present in snapshot;
- command acceptance/completion;
- query/mutation results.

### 9.2 Route/selection state

Frontend-local and disposable:

- selected workspace;
- selected chapter/document/knowledge tab;
- selected inspector section;
- drawer/panel visibility.

### 9.3 Form/editor transient state

Frontend-local until explicitly dispatched:

- unsaved form text;
- dirty flags;
- local validation hints;
- cursor/selection/scroll position.

A transient edit must never be presented as canonical or accepted merely because it exists in memory.

### 9.4 Derived presentation state

Calculated from authoritative state without becoming source of truth:

- button enablement;
- badges;
- progress labels;
- empty/loading/error panels;
- conflict prompts;
- status summaries.

## 10. Loading / empty / error / stale-state rules

Every G04 workspace must define deterministic states for:

```text
initial
loading
ready
empty
refreshing
saving / command_pending
validation_error
conflict
runtime_error
unavailable / unsupported
```

Stale async responses must not overwrite a newer route/project/revision projection. View models therefore carry request identity/revision context sufficient to discard obsolete results.

The shell may render cached frontend presentation while refreshing, but canonical claims are always reconciled with the latest AppRuntime projection.

## 11. No premature later-gate implementation

G04 MUST NOT prematurely implement:

- G05 resource locking, worker-pool scheduling or multi-run orchestration;
- Browser Lane Pool or parallel Gemini session architecture;
- a new Review/12-Gate execution engine;
- alternate AI providers or direct AI APIs;
- project-format v3 or destructive migration;
- browser conversation memory as story authority;
- generic project file editing;
- credential/cookie extraction;
- replacement of the existing TUI/headless compatibility surfaces.

Visible navigation placeholders for later work are allowed only when clearly disabled/coming later and do not create fake local behavior.

## 12. G04 child-step sequence

The forward sequence is locked as:

```text
G04.1  Authority / roadmap audit & Creative Studio scope freeze       PASS / inherited into G04.2 gate
G04.2  Creative Studio authority + Desktop Shell/UI boundary contract ACTIVE / this candidate
G04.3  Creative Studio workspace shell                                CLOSED until G04.2 PASS
G04.4  Project / Chapter / Outline workspace                          CLOSED until G04.3 PASS
G04.5  Knowledge Studio                                               CLOSED until G04.4 PASS
G04.6  Creative lifecycle control plane                               CLOSED until G04.5 PASS
G04.7  Creative editor + safe write-plane UI                          CLOSED until G04.6 PASS
G04.8  Integrated UX / boundary regression / final authority gate     CLOSED until G04.7 PASS
```

No successor child step opens merely because this file exists. G04.2 must first pass the exact-head acceptance rule.

## 13. G04.2 deliverables

G04.2 contains authority only:

- this Creative Studio authority;
- `docs/desktop-g04-shell-ui-boundary-contract-v1.md`;
- no production Creative Studio implementation;
- no editor implementation;
- no new core/AppRuntime/Host/Store/WebAI behavior;
- no project-format or persistence changes.

## 14. G04.2 exact-head acceptance rule

The exact commit containing both G04.2 authority documents becomes `G04.2 — PASS / AUTHORITY LOCKED` only when all applicable branch checks for that same SHA pass, including:

1. repository CI concludes SUCCESS;
2. WEB-only / NO-API residue audit remains green;
3. Linux format/vet/tests and required critical race tests remain green;
4. Windows format/vet/tests remain green according to workflow authority;
5. no production source or project persistence file is changed by this authority-only commit;
6. branch head still equals the exact tested SHA when PASS is declared.

Release packaging / installation / real packaged desktop smoke are not required solely to validate a documentation-only G04.2 contract unless the active repository workflow explicitly runs and requires them for this branch.

If the head changes, G04.2 reopens and the new exact head must be validated.

## 15. G04.2 PASS effect

Only after section 14 passes:

- `G04.1 = PASS / LOCKED`;
- `G04.2 = PASS / AUTHORITY LOCKED`;
- `G04.3 = OPEN / AUTHORIZED`;
- production Creative Studio shell code may begin;
- G04.4+ remain closed.

## 16. Inherited global locks

G04 preserves all of the following:

- baseline ancestry remains compatible with `ainovel-cli v0.1.3`;
- Store/project files remain canonical story truth;
- browser conversation is not persistent memory;
- Desktop UI reaches core only through AppRuntime;
- project format remains v2 unless a later explicit migration authority proves otherwise;
- WEB-only Gemini remains the sole AI execution path;
- no direct AI API/provider fallback is permitted;
- browser credentials/session secrets never become project DTO/state;
- existing TUI/headless surfaces remain compatibility paths;
- no second desktop engine or duplicate canonical database may be created.

This authority is forward-only and additive. It may be clarified by later G04 child authorities, but no child authority may silently weaken these locks.