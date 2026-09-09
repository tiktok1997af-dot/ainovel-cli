# AINOVEL Desktop — G01 Foundation Authority v1

Status target: `G01 — FOUNDATION`

Roadmap authority: **Optimized v2 — 8 GATE / 60 steps**.

This document freezes the implementation foundation for the desktop product. It does not replace the existing WEB-only runtime authority in `README.md` and `docs/architecture.md`; it defines how the new desktop UI must wrap and extend it.

## G01.1 — Baseline freeze

Canonical baseline:

- repository: `tiktok1997af-dot/ainovel-cli`
- version/tag: `v0.1.3`
- commit: `56636d18710ca97cb585bb44bbfd9d41e2df96ca`
- default branch at freeze time: `main`

Rules:

1. Never mutate or reinterpret the `v0.1.3` tag.
2. Desktop work is additive and must preserve existing project compatibility.
3. No gate may silently restore an AI API path, provider fallback, hidden browser, or credential extraction.

## G01.2 — Implementation branch

Foundation branch:

`feat/desktop-roadmap-v2-g01-foundation`

All G01 changes are isolated from `main` until the gate is reviewed and passed.

## G01.3 — Current architecture seam audit

Current entry surfaces:

- `internal/entry/tui` — existing interactive TUI.
- `internal/entry/headless` — non-TUI entry using the same browser-backed runtime.
- `internal/entry/startup` — startup/setup composition.

Current core seams that the desktop product must reuse:

- `internal/host` — lifecycle/orchestration, snapshots, observer/events and co-create.
- `internal/domain` — persistent runtime/project contracts.
- `internal/store` — local source of truth, checkpoints, runtime queue/logs.
- `internal/webai` — visible Chrome/Gemini Web session, transport and watchdog recovery.
- Engine/Workers/Arbiter/local Tools — deterministic local orchestration and side effects.

Desktop rule: add a desktop entry and AppRuntime facade; do not fork the Engine into a second implementation.

## G01.4 — KEEP / EXTEND / WRAP / DEPRECATE matrix

| Area | Decision | Desktop rule |
|---|---|---|
| Engine / Workers / Arbiter | KEEP | One orchestration core only. |
| Store / project files / checkpoints | KEEP | Remain canonical source of truth. |
| WebChatModel / GeminiWebTransport | KEEP | One AI execution path remains WEB-only. |
| SessionManager | EXTEND | Later support isolated browser lanes without breaking single-session compatibility. |
| AutoRecoveryTransport/watchdog | EXTEND | Reuse per lane/run; preserve bounded replay safety. |
| Host observer/events | EXTEND | Add run-aware projection while preserving existing event semantics. |
| `UISnapshot` | EXTEND | Becomes source material for desktop view models, not duplicated state. |
| CoCreate host/runtime | WRAP | Desktop CoCreate workspace wraps existing capability. |
| TUI | KEEP | Compatibility/admin surface; not the new primary UX. |
| Headless | KEEP | Compatibility surface; no new AI transport. |
| Desktop GUI | ADD | New primary product surface. |
| AI API/provider fallback | DEPRECATE/PROHIBIT | Must not re-enter production runtime. |
| Duplicate desktop-only Store | PROHIBIT | Desktop must not create a second canonical project database. |

## G01.5 — Compatibility and data contract freeze

The desktop roadmap must preserve these v0.1.3 invariants unless a later gate introduces an explicit migration with backward compatibility tests:

1. Local Store/project files remain canonical facts.
2. Browser conversation is not persistent project memory.
3. Existing `meta/progress.json` semantics remain readable.
4. Existing `meta/run.json` remains readable for single-run projects.
5. Existing checkpoints and recovery artifacts remain resumable.
6. Runtime queue/task logs remain local and may be extended with run IDs rather than replaced by an unrelated queue.
7. Existing Chrome profile/session data remains browser-owned; credentials/cookies are not extracted by ainovel-cli.
8. Existing projects created by v0.1.3 must open without destructive migration.
9. New multi-run metadata must live under an additive runtime namespace such as `meta/runtime/runs/` and must not overload canonical story progress.
10. Official/canonical story data may only be modified through explicit product workflows; transient browser/run state never becomes canonical by itself.

## G01.6 — Desktop product and visual contract freeze

Primary navigation/workspaces:

1. **Tổng quan**
2. **Dự án** — Chapters, Arc/Volume, Documents, Folder View
3. **Sáng tác** — Viết AI + Đồng sáng tác
4. **Tri thức** — Context, Canon, Nhân vật, Thế giới, Timeline
5. **Review** — 12 quality gates, repair/re-run, promote official
6. **Run Center** — active/queue/completed/failed + browser lanes
7. **Cài đặt**

Required product screens matching the approved visual target:

- Dashboard
- Project/Chapter workspace
- AI Editor
- CoCreate workspace
- Knowledge workspace
- Review/12-Gate workspace
- Multi-Run Center
- Folder View
- Settings

Global lifecycle controls are mandatory product behavior, not decorative UI:

`Start / Pause / Resume / Stop / Cancel`

Required runtime capabilities retained in the final target:

- AI writing
- CoCreate
- Context/Canon/Characters/World/Timeline
- 12 quality gates
- Repair/re-run/promote official
- autosave/checkpoint
- multi-run scheduler/queue/priority
- resource locks
- isolated browser lanes
- retry/watchdog/recovery
- responsive desktop/tablet/mobile behavior
- Windows packaging

Desktop layout authority:

- desktop-first dark UI;
- common app shell;
- studio wide layout approximately `256px | flexible | 360px`;
- tablet collapses to two panes;
- mobile uses one pane with focused tabs/drawers;
- Folder View may use full-width file/diagnostic layout;
- runtime state must be understandable without reading raw terminal logs.

## G01.7 — Baseline regression authority

The baseline CI workflow at `v0.1.3` defines the minimum regression floor for all later desktop gates:

- WEB-only residue audit;
- fork/release lock checks;
- Go format check;
- `go vet ./...`;
- `go test -buildvcs=false -count=1 ./...` on Linux and Windows;
- Linux race tests for critical host/store/tools paths.

The baseline commit's `main` CI run must be recorded as successful before G01 can pass. Every later gate must keep these tests green while adding desktop-specific tests rather than replacing them.

## G01.8 — G01 gate criteria

G01 is PASS only when all of the following are true:

- [x] baseline SHA/version verified;
- [x] isolated G01 implementation branch exists;
- [x] current entry/core seams audited;
- [x] KEEP/EXTEND/WRAP/DEPRECATE matrix frozen;
- [x] compatibility/data invariants frozen;
- [x] desktop visual/product contract frozen;
- [x] v0.1.3 baseline CI is verified green;
- [ ] this authority change itself passes PR CI and is ready to merge.

Until the final item is satisfied, **G02 MUST NOT OPEN**.
