# AINOVEL Desktop — G04.8 Integrated UX / Boundary Regression / Final Authority v1

Status: `G04.8 — FINAL EXACT-HEAD CANDIDATE`

Roadmap parent: **AINOVEL Desktop Optimized v2 — 8 GATE / 60 steps**.

Branch: `feat/desktop-roadmap-v2-g04-creative-studio`

Accepted parent: G04.7 exact head `e1361e851921c76728c4ae395aba1efe0ee782aa`.

G05 remains `CLOSED` throughout this gate.

## 1. Mission

G04.8 is the final integration and authority gate for the Creative Studio work completed in G04.1 through G04.7. It does not open a new product feature family. Its responsibilities are to:

1. audit the inherited G04 authority against the integrated implementation;
2. verify navigation, projected state, loading/error/conflict behavior, responsive layout and renderer-facing accessibility semantics;
3. verify all UI-to-core access remains behind the five-method AppRuntime facade;
4. verify G05 scheduler/resource-lock/browser-lane ownership remains absent;
5. make only bounded fixes required to satisfy already-locked G04 contracts;
6. establish one exact-head candidate for final CI/W6A/W6B/W6C acceptance.

## 2. Audit result before bounded fixes

The integrated G04.1-G04.7 review found two blocking G04-level gaps and no requirement for a new core/AppRuntime contract.

### 2.1 Supported Documents catalog/view was missing from Project workspace

The locked G04 authority explicitly includes a supported Documents catalog/view and the G03 read catalog already provides:

```text
documents.list
documents.get
```

The existing G04.7 Project workspace exposed only Overview / Chapters / Outline. G04.8 therefore adds a `Documents` Project tab as a bounded read-only projection through those two existing queries.

This is not Folder View. The renderer receives only catalog DTOs and document content returned by AppRuntime. It never resolves a catalog path into a filesystem operation and exposes no document write mutation.

### 2.2 Creative chapter reads lacked stale-response identity

Project and Knowledge reads already reject stale query responses using request identity plus route/snapshot revision context. Creative `chapters.get(include_content=true)` did not carry an equivalent ticket, so an obsolete response could overwrite a newer Creative chapter selection.

G04.8 adds a Creative query ticket with:

- request ID;
- route;
- snapshot revision;
- selected chapter.

Only the current ticket may update the Creative canonical projection. Reconciliation after a chapter mutation also refuses to pull the editor back to an older chapter if the user has selected another chapter while the mutation was in flight.

## 3. Documents bounded-fix contract

The G04 Project workspace now supports:

```text
ProjectTabDocuments
LoadDocumentsPage(offset, kind, prefix)
LoadDocument(id)
```

Rules:

1. `documents.list` is bounded to `ProjectWorkspaceQueryLimit = 100`.
2. Filters are passed only through the typed G03 `DocumentsListQuery` contract.
3. `documents.get` uses only the opaque catalog document ID.
4. A returned document must match the requested ID and must declare `ReadOnly=true`; otherwise the UI fails closed as a protocol error.
5. Paths are presentation metadata only and are never opened directly by the desktop UI layer.
6. Invalid local selectors fail before `AppRuntime.Query` where the UI can determine invalidity safely.
7. No Dispatch call exists for the Documents surface because G03 exposes no document mutation.

## 4. Creative stale-state bounded-fix contract

Creative chapter selection now distinguishes:

- `Selected`: the renderer's current requested chapter;
- `Chapter`: the chapter represented by the accepted canonical query result.

A response is accepted only when its ticket still matches the current route, snapshot revision, selected chapter and request ID. Obsolete responses are discarded without replacing the newer canonical projection.

Dirty plan/draft/workspace preservation remains limited to same-chapter mutation reconciliation. A late reconciliation for an old chapter may refresh the global AppRuntime snapshot but must not issue a Creative chapter query that would pull the editor away from the user's newer selection.

## 5. Integrated UX regression matrix

The final candidate must preserve all of the following simultaneously.

### Navigation

Enabled G04 routes:

```text
Tổng quan
Dự án
Sáng tác
Tri thức
```

Successor placeholders remain disabled:

```text
Review
Run Center
Cài đặt
```

Every navigation item has a stable route ID, visible label and availability description. Disabled successor routes cannot change the selected route.

### Responsive layout

Locked breakpoint behavior remains:

```text
< 760px       mobile: one focused main region; inspector may be a drawer
760-1199px    tablet: navigation + main; inspector may be a drawer
>= 1200px     desktop: 256px navigation + flexible main + optional 360px inspector
```

Resizing and route changes must not discard the accepted AppRuntime snapshot/revision.

### Accessibility semantics at the renderer-neutral boundary

Because G04 freezes renderer-neutral view models rather than a specific GUI toolkit, final accessibility authority requires the data needed by a renderer to expose accessible controls and status:

- every navigation item has a non-empty visible label;
- lifecycle controls have unique IDs and non-empty labels;
- disabled lifecycle controls provide a non-empty reason;
- structured errors/conflicts provide code, category and non-empty user-safe message;
- conflict states remain explicit `LoadConflict` states rather than silent overwrites.

Toolkit-specific focus rings, screen-reader APIs, keyboard shortcuts and visual contrast implementation remain renderer responsibilities when a concrete renderer is selected; G04.8 does not lock a GUI framework merely to simulate them.

### State / error / conflict

The integrated model must continue to preserve deterministic states for loading, ready, empty, refreshing, validation error, conflict and runtime error. Structured AppRuntime errors remain renderer-safe and canonical changes are reconciled through fresh Snapshot/Query reads.

### Lifecycle

`Start / Pause / Resume / Stop / Cancel / Retry` remain bound only through `AppRuntime.Dispatch`. Lifecycle truth remains AppRuntime-owned and is reconciled from command result, lifecycle event and fresh snapshot. No second state machine is introduced.

### Write plane

The 14 frozen G03 mutations remain the complete G04 typed write catalog. Plan, draft, editable workspace and accepted ChapterRecord remain distinct. Accepted ChapterRecord remains read-only because no promote/overwrite mutation exists in G03.

## 6. Final architecture boundary regression

Production `internal/desktopui` code must continue to depend on AINOVEL internals only through:

```text
internal/appruntime
```

The final boundary tests additionally reject direct desktop UI imports of filesystem/network execution surfaces including:

```text
os
os/exec
io/fs
path/filepath
net
net/http
```

This prevents the final integration gate from accidentally turning Documents or another UI surface into a direct filesystem/browser/network transport.

The final AST regression also rejects explicit `RunID`, `TaskID` or `Resource` ownership in desktop UI `appruntime.CommandRequest` literals. Those fields remain available in the AppRuntime contract for later roadmap ownership but are not claimed by G04.

## 7. G05 closure

G04.8 does not implement or authorize:

- scheduler or queue ownership;
- multi-run prioritization;
- resource lock acquisition/lease/release;
- browser-lane pool or parallel Gemini sessions;
- Run Center execution semantics;
- direct AI API/provider fallback;
- direct Host/Store/WebAI/Engine/filesystem access;
- a frontend canonical database;
- a new project format or migration;
- generic document/file writing.

`G05 = CLOSED` until G04.8 itself passes and the final G04 PR authority/merge gate is completed.

## 8. Candidate boundary

The G04.8 candidate is one commit on the exact G04.7 accepted parent and is restricted to these final integration files:

```text
internal/desktopui/project_workspace.go
internal/desktopui/project_controller.go
internal/desktopui/creative_workspace.go
internal/desktopui/creative_controller.go
internal/desktopui/boundary_test.go
internal/desktopui/g04_final_regression_test.go
docs/desktop-g04-final-authority-v1.md
```

No AppRuntime, Host, Store, WebAI, Engine, persistence, release workflow or project-format file may be changed by this candidate.

## 9. Exact-head acceptance

G04.8 becomes `PASS / LOCKED` only when the same exact candidate SHA satisfies all of the following:

1. CI SUCCESS;
2. Linux format/vet/full test SUCCESS;
3. required critical race tests SUCCESS;
4. Windows format/vet/full regression SUCCESS;
5. WEB-only residue / repository-wide W5.5 NO-API audit SUCCESS;
6. W6A Release Packaging Gate SUCCESS;
7. W6B Install Update Integrity Gate SUCCESS;
8. W6C Release Artifact Desktop Smoke SUCCESS, including real Windows packaged production WEB-only smoke when triggered;
9. branch head remains the exact tested SHA after all workflows complete;
10. blocking review threads = 0;
11. parent-to-candidate remains exactly one commit and only the seven authorized files above;
12. `main` remains the locked G03 baseline until the explicit final PR authority/merge decision.

If any implementation fix changes the candidate SHA, all exact-head acceptance evidence resets to the replacement SHA.

## 10. PASS effect

Only after section 9 is satisfied may authority state change to:

```text
G04.8 = PASS / LOCKED
G04 = COMPLETE
progress = 31/60 = 51.7%
```

At that point the next action is the final PR #20 authority/merge gate. G05 does not open merely because G04.8 tests pass; G05 remains CLOSED until the final cumulative G04 PR authority/merge decision is completed and its resulting `main` authority is verified.
