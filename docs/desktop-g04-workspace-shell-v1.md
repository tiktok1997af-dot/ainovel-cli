# AINOVEL Desktop — G04.3 Creative Studio Workspace Shell v1

Status: `G04.3 — IMPLEMENTATION CANDIDATE`

Parent authority:

- `docs/desktop-g04-creative-studio-authority-v1.md`
- `docs/desktop-g04-shell-ui-boundary-contract-v1.md`

Parent exact head: `3903518afbadefb0f51f29608d3d32448cd379df`.

## 1. Scope

G04.3 implements the renderer-neutral **Creative Studio workspace shell** only.

The implementation introduces `internal/desktopui` as a presentation/client layer over `internal/appruntime`. It does not choose or lock a native GUI framework, does not create an alternate runtime, and does not move canonical project state out of the existing Store/domain owners.

Authorized shell capabilities in this candidate:

- the seven primary product destinations reserved by G01/G04;
- responsive desktop/tablet/mobile pane state;
- global product/project/chapter header projection;
- runtime/browser/recovery status projection;
- inspector and bounded activity presentation state;
- deterministic initial/loading/ready/empty/refreshing/runtime-error states;
- request-identity and snapshot-revision stale-response suppression;
- ordered/deduplicated durable event projection;
- a renderer-neutral runtime client boundary matching the five AppRuntime methods;
- shell bootstrap via Snapshot + Subscribe only;
- lifecycle control presentation reserved as Start / Pause / Resume / Stop / Cancel / Retry, with eligibility derived from authoritative lifecycle state but all controls deliberately unwired and disabled until G04.6.

## 2. Dependency boundary

Production files under `internal/desktopui` may depend on the AINOVEL core only through:

```text
internal/appruntime
```

The following remain forbidden:

```text
internal/host
internal/store
internal/webai
Engine / Workers / Arbiter / Tools
filesystem mutation as project persistence
```

Boundary tests inspect production imports so later shell edits cannot silently cross this rule.

## 3. Navigation boundary

The shell reserves:

```text
overview       Tổng quan
project        Dự án
creative       Sáng tác
knowledge      Tri thức
review         Review
run_center     Run Center
settings       Cài đặt
```

Only `overview` is selectable in G04.3. Successor workspaces stay explicit disabled placeholders:

- Project: G04.4
- Knowledge: G04.5
- Creative/editor behavior: G04.7
- Run Center / multi-run behavior: G05+
- Review and Settings behavior: later owning gates

A placeholder never simulates unavailable core behavior.

## 4. Responsive shell boundary

The renderer-neutral layout state follows the inherited G01/G04 target:

- desktop `>= 1200px`: navigation `256px`, flexible main workspace, optional inspector `360px`;
- tablet `>= 760px`: navigation + main workspace, inspector represented as a drawer;
- mobile `< 760px`: one focused main region, inspector represented as a drawer.

Responsive changes alter presentation state only. They do not discard, submit, or redefine authoritative project/runtime data.

## 5. Snapshot and event projection

The shell accepts only `ainovel.desktop.v1` / SchemaVersion 1 snapshots.

Snapshot rules:

1. each refresh carries a request identity;
2. a response for an obsolete request identity is ignored;
3. an older snapshot revision cannot replace a newer accepted projection;
4. empty project identity produces an explicit empty state;
5. structured AppRuntime errors map to product error views;
6. unknown raw errors use a safe runtime-unavailable message and do not expose filesystem/provider internals.

Event rules:

1. non-zero durable event sequence is monotonic and deduplicated;
2. obsolete durable events are ignored;
3. transient sequence-zero projected events may still be presented;
4. activity history is bounded in frontend memory;
5. event errors use the structured AppError projection.

## 6. Bootstrap boundary

G04.3 Controller is intentionally narrow:

```text
Snapshot -> Shell projection
Subscribe -> bounded activity/status projection
Close -> subscription then RuntimeClient close
```

It does **not** issue `Query` or `Dispatch` calls. Project/Chapter/Outline bounded query behavior opens in G04.4. Lifecycle Dispatch wiring opens in G04.6. Editor/write behavior opens in G04.7.

## 7. Lifecycle presentation lock

The required controls are visible to future renderers as reserved presentation models:

```text
Start / Pause / Resume / Stop / Cancel / Retry
```

Eligibility is derived from AppRuntime lifecycle states so the presentation model cannot invent a competing state machine. However, `Enabled` remains false for every lifecycle control in G04.3 and the reason explicitly points to G04.6.

Therefore G04.3 adds no lifecycle command execution.

## 8. Non-goals

This candidate does not implement:

- Project / Chapter / Outline workspace queries or writes;
- Knowledge workspace queries or mutations;
- creative editor or chapter editing;
- lifecycle command dispatch;
- Review/12-Gate execution;
- multi-run scheduler, queue orchestration or resource locks;
- Browser Lane Pool;
- direct Host/Store/WebAI/filesystem access;
- new persistence/database/project format;
- direct AI API/provider fallback;
- a GUI-framework decision.

WEB-only Gemini and all inherited G01-G04.2 locks remain unchanged.

## 9. Candidate files

```text
internal/desktopui/runtime_client.go
internal/desktopui/shell.go
internal/desktopui/controller.go
internal/desktopui/shell_test.go
internal/desktopui/controller_test.go
internal/desktopui/boundary_test.go
docs/desktop-g04-workspace-shell-v1.md
```

## 10. G04.3 exact-head acceptance

G04.3 becomes `PASS / LOCKED` only when the exact candidate SHA containing these files satisfies all active repository gates for that same head, including:

- CI SUCCESS;
- WEB-only / NO-API audit SUCCESS;
- Linux/Windows regression SUCCESS;
- W6A SUCCESS;
- W6B SUCCESS;
- W6C SUCCESS, including the real packaged Windows WEB-only smoke when the workflow runs;
- branch head still equals the tested SHA at declaration time;
- no unresolved blocking review thread;
- the diff remains inside the G04.3 shell boundary.

Until then, verified locked roadmap progress remains `25/60 = 41.7%` and G04.4 remains CLOSED.

Only after this exact-head acceptance may `G04.4 — Project / Chapter / Outline workspace` open.
