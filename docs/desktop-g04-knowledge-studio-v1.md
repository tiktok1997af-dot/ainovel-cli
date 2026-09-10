# AINOVEL Desktop — G04.5 Knowledge Studio Authority v1

Status: `G04.5 — IMPLEMENTATION CANDIDATE / EXACT-HEAD REQUIRED`

Roadmap: **AINOVEL Desktop Optimized v2 — 8 GATE / 60 steps**.

Branch: `feat/desktop-roadmap-v2-g04-creative-studio`.

Parent accepted head: `G04.4 @ 1a8a5a0aea18ee825d81cd01b6ae1e6bcd103fb6`.

## 1. Mission

G04.5 implements the renderer-neutral **Knowledge Studio** above the locked G04.3 shell and G04.4 Project workspace.

This child gate is deliberately read-only. It exposes the five G03 Knowledge query contracts through `RuntimeClient.Query` / `AppRuntime.Query` and projects their typed DTOs into UI state.

It does not open lifecycle commands, the creative editor, any Knowledge mutation surface, scheduling, browser lanes, alternate persistence, or direct AI APIs.

## 2. Locked read contracts

G04.5 may issue exactly these existing G03 query kinds:

```text
knowledge.context
knowledge.canon
knowledge.characters
knowledge.world
knowledge.timeline
```

No new query kind is introduced.

The G03 DTOs remain authoritative. G04.5 does not reinterpret project files, rebuild canon locally, or create a second Knowledge database.

## 3. Knowledge Studio tabs

The renderer-neutral workspace exposes five tabs:

```text
Context
Canon
Characters
World
Timeline
```

The `Tri thức` primary route becomes enabled in this child gate. `Tổng quan` and `Dự án` remain enabled from earlier gates.

`Sáng tác`, `Review`, `Run Center`, and later-owned Settings behavior remain unavailable according to their owning gates.

## 4. Bounded query policy

G04.5 uses `KnowledgeWorkspaceQueryLimit = 100`.

Rules:

1. `knowledge.context` uses `max_items=100`.
2. `knowledge.canon` uses page `limit=100`.
3. `knowledge.characters` uses page `limit=100`.
4. `knowledge.world` uses `limit=100` across selected world sections.
5. `knowledge.timeline` uses `limit=100` within the requested chapter window.
6. Opening Knowledge Studio loads only the default Context view; it does not eagerly load all five knowledge surfaces.
7. The default Context request uses the current projected chapter and `overview` scope.

This preserves the G04 authority that global shell startup must not eagerly load all knowledge data.

## 5. Projection semantics

Knowledge DTOs are projected without becoming new canonical stores.

### Context

Context remains computed/ephemeral. Sections and item provenance returned by AppRuntime remain intact in the projection.

### Canon

Canon remains a provenance-bearing projection. G04.5 does not convert displayed facts into local authority.

### Characters

Characters remain the typed G03 projection, including core/cast origin fields exposed by AppRuntime.

### World

World remains the typed projection of rules, foreshadow, relationships, and state changes.

### Timeline

Timeline remains the typed chapter/time/event projection returned by AppRuntime.

## 6. Stale-response protection

Every Knowledge query ticket carries:

```text
request id
query kind
shell snapshot revision
route
active Knowledge tab
```

A result is accepted only when all of those still match current UI state.

Therefore a result is discarded if:

- a newer request of the same query kind supersedes it;
- the user leaves the Knowledge route;
- a fresher shell snapshot revision is accepted;
- the user switches to a different Knowledge tab before the result returns.

A stale result never overwrites a newer projected state.

## 7. Loading / empty / error states

Knowledge Studio uses the shared G04 load states:

```text
initial
loading
ready
empty
refreshing
validation_error
conflict
runtime_error
```

Errors from a Knowledge query remain workspace-scoped and do not replace the shell's authoritative Snapshot error projection.

Local invalid selectors are rejected before `AppRuntime.Query`.

Unknown/raw errors are normalized through the existing safe desktop error projection.

## 8. Local selector validation

G04.5 accepts only selector values already accepted by the locked G03 contracts.

Context scopes:

```text
""
writer
editor
overview
```

Canon scopes:

```text
""
all
continuity
state
relationship
foreshadow
```

Character scopes:

```text
""
all
core
cast
```

World sections:

```text
rules
foreshadow
relationships
state_changes
```

Timeline chapter bounds must be non-negative and `from_chapter` cannot exceed `to_chapter` when both are non-zero.

## 9. Write-plane remains closed

Although G03 has already locked Knowledge mutation contracts, G04.5 does **not** bind them to UI controls.

The following remain unbound in this child gate:

```text
knowledge.characters.core.replace
knowledge.world.rules.replace
knowledge.timeline.append
knowledge.relationships.update
knowledge.foreshadow.update
```

No G04.5 production path calls `RuntimeClient.Dispatch`.

Knowledge editing belongs to the later safe write-plane UI authority rather than being smuggled into the read-only Knowledge Studio gate.

## 10. G04.6 lifecycle remains closed

G04.5 does not wire:

```text
Start
Pause
Resume
Stop
Cancel
Retry
```

Those controls remain presentation-only with `Enabled=false` until G04.6.

G04.5 must not create a competing lifecycle state machine or call Host lifecycle seams directly.

## 11. G04.7 editor remains closed

G04.5 does not implement:

- creative editor;
- chapter prose editing;
- chapter workspace save;
- project metadata write forms;
- outline write forms;
- Knowledge mutation forms;
- generic project file editing.

No chapter prose is loaded as part of Knowledge Studio.

## 12. G05 remains closed

G04.5 does not implement:

- multi-run scheduling;
- worker/resource locks;
- browser lane pools;
- parallel Gemini session orchestration;
- queue priority;
- later-gate recovery orchestration.

## 13. Core boundary

The dependency direction remains:

```text
Knowledge Studio UI state
        |
        v
RuntimeClient.Query
        |
        v
AppRuntime.Query
        |
        v
existing Host read seams
        |
        v
canonical Store/domain owners
```

Forbidden:

```text
Knowledge Studio -> Host
Knowledge Studio -> Store
Knowledge Studio -> WebAI
Knowledge Studio -> filesystem reads/writes
Knowledge Studio -> alternate database
Knowledge Studio -> direct AI API/provider
```

## 14. Candidate implementation files

The G04.5 candidate is limited to:

```text
internal/desktopui/knowledge_workspace.go
internal/desktopui/knowledge_controller.go
internal/desktopui/knowledge_workspace_test.go
internal/desktopui/project_workspace_test.go
internal/desktopui/shell.go
internal/desktopui/shell_test.go
docs/desktop-g04-knowledge-studio-v1.md
```

No AppRuntime/core/Host/Store/WebAI/persistence source file is changed.

## 15. Acceptance criteria

G04.5 becomes `PASS / LOCKED` only if the same exact candidate SHA satisfies all of the following:

1. candidate parent is the accepted G04.4 SHA;
2. parent-to-head delta is exactly one candidate commit;
3. diff remains within the seven-file G04.5 boundary above;
4. CI succeeds;
5. Linux format/vet/tests and required race checks succeed;
6. Windows regression succeeds;
7. WEB-only / NO-API audit succeeds;
8. W6A release packaging succeeds;
9. W6B install/update integrity succeeds;
10. W6C release artifact desktop smoke succeeds, including real packaged Windows WEB-only smoke when triggered;
11. branch head still equals the exact tested candidate SHA;
12. blocking review threads equal zero.

If the branch head changes, exact-head acceptance resets.

## 16. PASS effect

Only after section 15 passes:

```text
G04.5 = PASS / LOCKED
progress = 28 / 60 = 46.7%
G04.6 — Creative lifecycle control plane = OPEN / AUTHORIZED
```

G04.7 and G05 remain closed.
