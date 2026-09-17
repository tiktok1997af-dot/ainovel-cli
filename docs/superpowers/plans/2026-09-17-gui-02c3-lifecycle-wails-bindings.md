# GUI-02C.3 Lifecycle Wails Bindings Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add typed Wails `CreateProject`, `OpenProject`, `SwitchProject`, and `CloseProject` bindings on the existing Gateway while preserving the single serialized project-scoped AppRuntime authority locked by GUI-02C.1/02C.2.

**Architecture:** Gateway remains the sole Wails-bound authority. Path-bearing lifecycle requests are normalized to absolute project roots, runtime construction uses `bootstrap.LoadConfigForProject(projectRoot)` plus an explicit project output/config path, and activation/closure goes through `projectSessionController` exclusive ownership. The event bridge is session-scoped: the old subscription is stopped before the old runtime closes and before the replacement becomes active; the replacement subscription is established before activation. No React lifecycle UI, generic Invoke, CWD mutation, or business S01-S10 work is included.

**Tech Stack:** Go, Wails v2, existing `internal/bootstrap`, `internal/host`, `internal/appruntime`, `internal/desktopui`, GitHub Actions.

**Spec:** GUI-02C.1 OPTION A authority lock and GUI-02C.2 exact parent `f9cb6b0d8ac1d7e3442f3098171fbadc4a7b4ef2`.

## Global Constraints

- Exact parent: `f9cb6b0d8ac1d7e3442f3098171fbadc4a7b4ef2`.
- Only one active project-scoped AppRuntime.
- Gateway is the only Wails-bound authority.
- Public lifecycle surface is exactly `CreateProject`, `OpenProject`, `SwitchProject`, `CloseProject` in this gate.
- Snapshot / Query / Dispatch authority remains unchanged.
- No generic Invoke; no React filesystem access; no Host/Store/WebAI/engine exposure.
- No `os.Chdir()`.
- Project configuration and config-write target must be path-explicit.
- Runtime calls serialize with replacement/Close.
- Old event subscription stops before a new session becomes active.
- Event topic remains exactly `ainovel:desktop:event`.
- GUI-02C.4, GUI-02C.5, and S01-S10 remain CLOSED.

---

### Task 1: Lock typed lifecycle contract with RED tests

**Files:**
- Create: `desktop/project_lifecycle_test.go`
- No production changes in RED commit.

**Interfaces:**
- Consumes: existing `Gateway`, `projectSessionController`, `desktopui.RuntimeClient`.
- Produces expectation for `ProjectLifecycleRequest`, `ProjectLifecycleResult`, and Gateway methods `CreateProject`, `OpenProject`, `SwitchProject`, `CloseProject`.

- [ ] Add tests proving the four public typed methods exist, lifecycle requests carry only a project root, and no generic `Invoke` surface appears.
- [ ] Add behavioral tests with fake runtimes/factory proving Open activates a runtime, Switch waits for an in-flight Gateway lease, old event subscription stops before replacement activation, Close leaves Snapshot fail-closed, and invalid lifecycle state/path returns typed safe errors.
- [ ] Run desktop tests and record RED. Initial missing lifecycle symbols are acceptable for contract RED; before production behavior is considered GREEN, run the behavioral tests and confirm failures are caused by missing lifecycle behavior rather than test-harness defects.

### Task 2: Add path-explicit runtime construction seam

**Files:**
- Create: `desktop/project_runtime_factory.go`
- Modify: `internal/bootstrap/project_config.go`
- Modify: `internal/host/options.go`
- Modify: `internal/host/host.go` only to consume an additive explicit config-path option; do not refactor Host behavior.
- Test: `internal/bootstrap/configfile_project_test.go`
- Test: `internal/host/host_lifecycle_test.go` or a focused new host option test if required.

**Interfaces:**
- Produces: `bootstrap.ProjectConfigPath(projectRoot) (string, error)`; additive `host.WithConfigPath(path string) NewOption`; internal `projectRuntimeFactory` used only by Gateway.
- Runtime factory loads config with `LoadConfigForProject(root)`, forces `cfg.OutputDir = root`, loads assets from that root, constructs `host.New(..., host.WithConfigPath(explicitProjectConfigPath))`, then `appruntime.New(core)`.

- [ ] Write/extend failing tests for explicit project config path and Host option before implementation.
- [ ] Verify RED.
- [ ] Implement only the additive path-explicit helpers/options needed by desktop lifecycle.
- [ ] Verify targeted tests GREEN.

### Task 3: Implement serialized Gateway lifecycle

**Files:**
- Create: `desktop/project_lifecycle.go`
- Modify: `desktop/gateway.go`
- Modify: `desktop/session_controller.go` only as needed for one exclusive lifecycle transition primitive.
- Test: `desktop/project_lifecycle_test.go`
- Test: existing `desktop/gateway_session_integration_test.go` and GUI-02B gateway tests.

**Interfaces:**
- `ProjectLifecycleRequest { ProjectRoot string }`.
- `ProjectLifecycleResult { ContractVersion string; ProjectRoot string; Data *appruntime.DesktopSnapshot; Error *appruntime.AppError }`.
- `CreateProject(req)`, `OpenProject(req)`, `SwitchProject(req)`, `CloseProject()` return `ProjectLifecycleResult` and never raw Go errors to JS.
- Gateway owns a lifecycle mutex so public lifecycle transitions cannot overlap.
- Controller owns the exclusive runtime swap; callers cannot extract the runtime pointer and unlock before Close/replacement.

- [ ] Normalize/validate project roots without changing process CWD.
- [ ] CreateProject creates only the requested project root if absent, then constructs and activates the project runtime; it does not implement business S01-S10 data.
- [ ] OpenProject requires an existing directory and no currently active session.
- [ ] SwitchProject requires an active session and an existing different project directory.
- [ ] CloseProject stops the event bridge and closes/deactivates the active runtime.
- [ ] During Open/Create/Switch, build the candidate runtime first; under exclusive transition ownership stop the old bridge, close old runtime, subscribe the new runtime, then activate it. If preparation/subscription fails, close the candidate and remain fail-closed rather than exposing a half-active session.
- [ ] Keep one event topic exactly `ainovel:desktop:event`.
- [ ] Verify focused desktop lifecycle tests GREEN, then run GUI-02B regression tests.

### Task 4: Wails binding/build and authority verification

**Files:**
- Modify generated/frontend binding artifacts only if Wails generation/build requires it; do not hand-create React lifecycle UI.
- Modify CI workflow only if the existing workflow cannot exercise the new Wails methods without broadening scope.

- [ ] Run desktop Go tests.
- [ ] Run race-sensitive desktop/session tests where supported.
- [ ] Run root Go tests and existing GUI-02B authority regression.
- [ ] Run bootstrap config tests.
- [ ] Audit no `os.Chdir`, no generic Invoke, no frontend filesystem/core bypass, no S01-S10 changes.
- [ ] Run frontend build and Wails production build.
- [ ] If gate is runnable, capture evidence only from the real executable; never generate/mock screenshots.
- [ ] Compare candidate against exact parent `f9cb6b0d...`; scope/lineage must be clean.

### Task 5: Fresh Review

- [ ] Re-read the locked GUI-02C.1/02C.2 constraints against the final diff.
- [ ] Confirm tests/build/race/serialization/event handoff/path-explicit config all PASS with blockers = 0.
- [ ] Keep the PR DRAFT / OPEN / NOT MERGED.
- [ ] Present exact candidate SHA to AUTHOR for `CHỐT GUI-02C.3` only; do not auto-lock and do not open GUI-02C.4.

## Self-review

- Spec coverage: all four lifecycle bindings, serialization, event cutoff, path-explicit config, fail-closed behavior, and authority boundaries are mapped to tasks.
- Placeholder scan: no implementation placeholders or deferred scope inside GUI-02C.3.
- Type consistency: lifecycle request/result and runtime-factory boundaries are defined once and reused by tests/implementation.
