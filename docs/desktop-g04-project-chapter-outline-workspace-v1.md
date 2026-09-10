# AINOVEL Desktop — G04.4 Project / Chapter / Outline Workspace v1

Status: `G04.4 — IMPLEMENTATION CANDIDATE`

Parent gate: `G04.3 — PASS / LOCKED`

Accepted parent SHA: `1b3268eb879dffc92b988c76634731b3774048d1`

Branch: `feat/desktop-roadmap-v2-g04-creative-studio`

## 1. Mission

G04.4 makes the `Dự án` destination functional as a read-only Creative Studio workspace above the AppRuntime contract already locked by G03.

The child gate owns presentation/controller state for:

- project overview;
- bounded chapter catalog;
- one selected chapter detail projection without prose content;
- bounded outline projection;
- local tab/selection state;
- deterministic loading, empty, validation and runtime-error states;
- request identity and snapshot-revision stale-response suppression.

G04.4 does not add or change canonical project facts.

## 2. Allowed AppRuntime read surface

G04.4 may call exactly these existing query kinds:

```text
project.overview
chapters.list
chapters.get
outline.get
```

The UI/controller binds them only through `RuntimeClient.Query`, which mirrors `AppRuntime.Query`.

No new query kind or AppRuntime contract version is introduced.

## 3. Read bounds

Chapter and outline collection requests are explicitly bounded:

```text
ProjectWorkspaceQueryLimit = 100
```

This remains below the G03 contract maximum and prevents the desktop workspace from eagerly loading the whole project.

The query shapes are:

```text
project.overview
  payload: {}

chapters.list
  payload: { offset, limit: 100 }

chapters.get
  payload: { chapter, include_content: false }

outline.get
  payload: { from_chapter, limit: 100 }
```

`chapters.get` keeps `include_content=false` in G04.4. Full prose editing/presentation belongs to G04.7 and must not be pulled into this child gate.

## 4. Workspace model

The renderer-neutral `ProjectWorkspaceState` contains separate projections for:

```text
Overview
Chapters
SelectedChapter
Outline
```

It also owns disposable presentation state:

```text
active tab
selected chapter
load state
workspace error
pending query identity
```

The model does not own persistence, accepted history or lifecycle state.

## 5. Navigation effect

After this gate candidate is installed:

```text
Tổng quan   enabled   G04.3
Dự án       enabled   G04.4
Sáng tác    disabled  G04.7
Tri thức    disabled  G04.5
Review      disabled  later gate
Run Center  disabled  G05+
Cài đặt     disabled  later gate
```

Enabling `Dự án` does not authorize any write action.

## 6. Project workspace tabs

The G04.4 local tab identities are:

```text
overview
chapters
outline
```

Tab selection is frontend-local and disposable.

The canonical data shown in each tab remains sourced from AppRuntime Query results.

## 7. Bootstrap and refresh behavior

`OpenProjectWorkspace`:

1. selects the authorized `Dự án` route;
2. loads `project.overview`;
3. loads the first bounded `chapters.list` page;
4. loads the first bounded `outline.get` window;
5. renders only accepted query projections.

`LoadChapterPage` and `LoadOutlineWindow` can refresh bounded windows.

`LoadChapter` retrieves metadata/plan/status for one chapter and never asks AppRuntime for prose content in G04.4.

## 8. Stale-response rule

Every Project workspace request captures:

```text
request identity
query kind
current shell snapshot revision
current route
selected chapter where relevant
```

A response is accepted only when it still matches the pending request and the same shell snapshot revision/route.

A superseded request, route change or newer snapshot causes the obsolete result to be discarded rather than overwrite current UI state.

This presentation guard does not replace AppRuntime canonical revision/precondition rules.

## 9. Query response validation

Before projection, the controller verifies:

- AppRuntime query returned without a transport/runtime error;
- result-level `AppError` is absent;
- `ContractVersion == ainovel.desktop.v1`;
- returned query kind equals the requested kind;
- returned data is non-empty valid JSON;
- typed DTO decoding succeeds;
- a `chapters.get` result identifies the requested chapter.

Protocol mismatches map to a generic safe internal UI error and do not expose raw internals.

## 10. Error state behavior

Local invalid selectors are rejected before reaching `AppRuntime.Query`.

Examples:

```text
chapter <= 0
chapter list offset < 0
outline from_chapter < 0
```

Structured AppRuntime errors are projected into the existing `ErrorView`.

Workspace query failures remain scoped to `ProjectWorkspaceState`; they do not destroy an otherwise valid global shell snapshot.

## 11. Canonical semantic preservation

The Project workspace preserves the G03 distinctions already present in DTOs:

```text
chapter plan
draft-present metadata
final-present metadata
accepted ChapterRecord metadata
outline structure
```

G04.4 never equates a plan, draft, final artifact or workspace projection with accepted canonical history.

## 12. Forbidden behavior

G04.4 MUST NOT:

- call `AppRuntime.Dispatch`;
- add mutation controls;
- load full chapter prose through `chapters.get`;
- implement the Creative editor;
- implement Knowledge Studio;
- wire Start / Pause / Resume / Stop / Cancel / Retry;
- import Host, Store, WebAI, Engine, Workers, Arbiter or Tools;
- read/write arbitrary project files;
- create a desktop database or alternate canonical store;
- modify project format;
- add direct AI API/provider fallback;
- implement G05 scheduling/resource locks/browser-lane orchestration.

## 13. Production files in this candidate

Expected G04.4 production delta:

```text
internal/desktopui/shell.go
internal/desktopui/controller.go
internal/desktopui/project_workspace.go
internal/desktopui/project_controller.go
```

Test delta:

```text
internal/desktopui/shell_test.go
internal/desktopui/project_workspace_test.go
```

Authority delta:

```text
docs/desktop-g04-project-chapter-outline-workspace-v1.md
```

No AppRuntime/core/persistence production file is changed.

## 14. Test obligations

The candidate tests verify at least:

- only Overview and Project routes are enabled;
- later destinations remain disabled;
- opening Project uses only the authorized query kinds;
- bounded `chapters.list` and `outline.get` payloads;
- `chapters.get` always uses `include_content=false`;
- no Dispatch call is made;
- overview/chapter/outline projections reach deterministic ready state;
- invalid selectors are rejected before Query;
- structured query errors remain safe and workspace-scoped;
- stale request identity/revision is suppressed;
- mismatched result contract is rejected;
- empty projections produce deterministic empty state;
- existing import boundary tests continue to protect AppRuntime-only dependency direction.

## 15. Exact-head acceptance rule

G04.4 becomes `PASS / LOCKED` only when the exact candidate SHA passes:

1. repository CI;
2. WEB-only / NO-API audit;
3. Linux regression and required critical race tests;
4. Windows regression;
5. W6A release packaging gate;
6. W6B install/update integrity gate;
7. W6C release artifact desktop smoke, including real packaged Windows WEB-only smoke when triggered;
8. branch head recheck with no drift;
9. no blocking review thread;
10. candidate diff remains within the G04.4 read-only workspace boundary.

Any code correction changes the exact head and resets acceptance to the new SHA.

## 16. Gate effect

Until section 15 passes:

```text
G04.4 = IMPLEMENTED / EXACT-HEAD VALIDATION
G04.5 = CLOSED
locked progress = 26/60 = 43.3%
```

Only after PASS:

```text
G04.4 = PASS / LOCKED
G04.5 = OPEN / AUTHORIZED
locked progress = 27/60 = 45.0%
```

G04.5 will own Knowledge Studio and must not be implemented by this candidate.
