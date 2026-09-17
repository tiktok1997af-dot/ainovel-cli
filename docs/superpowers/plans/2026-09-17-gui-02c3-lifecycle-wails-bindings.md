# GUI-02C.3 Lifecycle Wails Bindings Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the locked typed `CreateProject`, `OpenProject`, `SwitchProject`, and `CloseProject` Wails bindings behind the existing Gateway and serialized Project Session Controller.

**Architecture:** Gateway remains the sole Wails-bound authority. Project lifecycle transitions are serialized separately from runtime operation leases: target preflight is read-only before disturbing the active session, the old event bridge is stopped, the old runtime is exclusively closed, the new runtime is built from a path-explicit project root, then activated and subscribed on the existing `ainovel:desktop:event` topic. Invalid switch preflight preserves the current session; a failure while constructing the replacement after teardown leaves the gateway fail-closed with no active runtime.

**Tech Stack:** Go, Wails v2, existing `internal/bootstrap`, `internal/host`, `internal/appruntime`, `internal/desktopui`.

**Spec:** GUI-02C.1 — Project Session Contract & Authority Design, OPTION A (AUTHOR-CONFIRMED / LOCKED), implemented on top of GUI-02C.2 locked SHA `f9cb6b0d8ac1d7e3442f3098171fbadc4a7b4ef2`.

## Global Constraints

- One active project-scoped AppRuntime at a time.
- Gateway remains the only Wails-bound authority.
- Keep Snapshot / Query / Dispatch contract and event topic `ainovel:desktop:event` unchanged.
- No generic Invoke, React filesystem access, Host/Store/WebAI/engine exposure, or `os.Chdir()`.
- Project config/runtime scope is path-explicit.
- Runtime operations remain leased for the full call; replacement/Close wait for in-flight operations.
- Old event pump is stopped before a new project becomes active.
- `CreateProject` only accepts a nonexistent or truly empty target and never silently overwrites.
- `OpenProject` preflight is read-only and must not require `meta/format.json` for legacy-v1 compatibility.
- Invalid `SwitchProject` preflight preserves the existing active project.
- Replacement construction failure after teardown leaves no active project; do not resurrect the old runtime.
- GUI-02C.4 and S01–S10 remain CLOSED.

---

### Task 1: Lock the typed Wails lifecycle surface

**Files:**
- Create: `desktop/gateway_lifecycle_test.go`
- Modify after RED only: `desktop/gateway.go`

**Interfaces:**
- Consumes: existing `Gateway` Wails authority.
- Produces: exported typed methods `CreateProject`, `OpenProject`, `SwitchProject`, `CloseProject` and typed request/result envelopes.

- [ ] Write a reflection-based failing test proving exactly the four lifecycle methods are absent at the locked parent.
- [ ] Run `cd desktop && go test -run TestGatewayExposesTypedProjectLifecycleBindings ./...` and confirm RED is due to the missing bindings.
- [ ] Add only the minimal typed method/result surface needed to make the method-set test GREEN.
- [ ] Re-run the targeted test.

### Task 2: Lifecycle preflight and runtime factory boundary

**Files:**
- Modify: `desktop/gateway_lifecycle_test.go`
- Create after RED: `desktop/project_lifecycle.go`
- Modify after RED: `desktop/gateway.go`

**Interfaces:**
- Consumes: project root string, `bootstrap.LoadConfigForProject`, existing Host/AppRuntime constructors.
- Produces: canonical project root, read-only open/switch preflight, create-empty preflight, and a test-injectable runtime factory returning `desktopui.RuntimeClient`.

- [ ] Add RED tests for blank root, non-directory/missing Open, Create refusing non-empty targets, legacy-v1 Open without `meta/format.json`, and path canonicalization.
- [ ] Confirm failures are behavioral, not test-harness errors.
- [ ] Implement minimal preflight helpers and factory seam.
- [ ] Factory sets `cfg.OutputDir` to the canonical project root and never changes process CWD.
- [ ] Re-run targeted lifecycle tests.

### Task 3: Serialized transition and fail-closed replacement semantics

**Files:**
- Modify: `desktop/gateway_lifecycle_test.go`
- Modify after RED: `desktop/gateway.go`
- Modify only if required by the test: `desktop/session_controller.go`

**Interfaces:**
- Consumes: Project Session Controller full-call leases and runtime factory.
- Produces: serialized Create/Open/Switch/Close transitions.

- [ ] Add RED tests proving lifecycle replacement waits for an in-flight Gateway call.
- [ ] Add RED test proving invalid Switch preflight preserves project A.
- [ ] Add RED test proving replacement construction failure after teardown leaves `runtime_unavailable` rather than resurrecting A.
- [ ] Implement the smallest transition path satisfying those tests.
- [ ] Re-run targeted and existing GUI-02B/02C.2 tests.

### Task 4: Event bridge handoff

**Files:**
- Modify: `desktop/gateway_lifecycle_test.go`
- Modify after RED: `desktop/gateway.go`

**Interfaces:**
- Consumes: current runtime `Subscribe`, existing `desktopEventTopic`.
- Produces: one event pump bound to the active runtime only.

- [ ] Add RED test proving old subscription/pump is closed before the new runtime is subscribed/active.
- [ ] Add RED test proving events after switch still use only `ainovel:desktop:event`.
- [ ] Implement explicit stop/start event bridge around lifecycle replacement.
- [ ] Re-run event and lifecycle tests.

### Task 5: Real project runtime construction

**Files:**
- Create or modify: `desktop/project_runtime_factory.go`
- Modify only if correctness requires an additive path-explicit constructor option: `internal/host/options.go`, `internal/host/host.go`.

**Interfaces:**
- Consumes: `bootstrap.LoadConfigForProject(projectRoot)`, `assets.LoadWithLanguage`, `host.New`, `appruntime.New`.
- Produces: one project-scoped AppRuntime with `cfg.OutputDir == canonical projectRoot`.

- [ ] Add RED unit coverage for path-explicit configuration/root propagation without launching a real browser session.
- [ ] Implement minimal factory composition and close Host on AppRuntime construction failure.
- [ ] Do not refactor backend/domain/runtime for UI convenience.

### Task 6: Verification and Fresh Review

**Files:**
- No behavior expansion.

- [ ] Run desktop Go tests and targeted race-sensitive lifecycle/session tests.
- [ ] Run root Go tests, bootstrap config tests, GUI-02B Gateway regression, no-bypass audit, frontend build, and Wails build required by workflow.
- [ ] If runnable GUI evidence is required, use only the actual Wails executable and real screenshot capture.
- [ ] Compare candidate to exact locked parent `f9cb6b0d8ac1d7e3442f3098171fbadc4a7b4ef2`; require merge-base exact, behind 0, and scope-only diff.
- [ ] Restore PR base to the GUI-02C.2 authority lineage after any temporary `main` retarget used solely to trigger Actions.
- [ ] Fresh Review with blockers = 0 before presenting a candidate SHA to AUTHOR.
- [ ] Do not lock GUI-02C.3 or open GUI-02C.4 without a separate explicit AUTHOR command.
