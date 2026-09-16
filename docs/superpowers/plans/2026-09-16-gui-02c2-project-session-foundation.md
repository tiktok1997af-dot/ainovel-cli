# GUI-02C.2 Project Session Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the project-session concurrency foundation and path-explicit project config loading required by locked GUI-02C.1 Option A, without opening GUI-02C.3 lifecycle bindings.

**Architecture:** Keep one Wails Gateway authority. Introduce a desktop-internal serialized project session holder that owns at most one `desktopui.RuntimeClient` and keeps a read lock for the full duration of runtime calls so future switch/close operations cannot close a runtime in use. Add an additive `bootstrap.LoadConfigForProject(projectRoot)` helper that reads `<projectRoot>/.ainovel/config.json` without process CWD mutation while preserving the existing global/project merge semantics. GUI-02C.2 does not expose Create/Open/Switch/Close to React; those lifecycle operations belong to GUI-02C.3.

**Tech Stack:** Go 1.x, Wails v2 desktop module, existing `internal/bootstrap`, existing `desktopui.RuntimeClient`, GitHub Actions.

**Spec:** AUTHOR-locked GUI-02C.1 Option A on parent `35ce9fdbb35131ac5ce33e42b4e1419b8a371397`.

## Global Constraints

- Exact parent authority: `GUI-02B@35ce9fdbb35131ac5ce33e42b4e1419b8a371397`.
- GUI-02C.1 Option A is `PASS / AUTHOR-CONFIRMED / LOCKED`.
- Gateway remains the sole Wails-bound authority.
- At most one active project-scoped runtime.
- No `os.Chdir()` for project selection/config resolution.
- No frontend filesystem, Host, Store, WebAI, engine, or generic `Invoke` exposure.
- Existing `Snapshot / Query / Dispatch / ainovel:desktop:event` contract remains unchanged.
- Backend/domain/runtime behavior is frozen; only the additive bootstrap config helper authorized by GUI-02C.1 may be added.
- GUI-02C.3 stays CLOSED: no Create/Open/Switch/Close Wails lifecycle surface in this gate.

---

### Task 1: Path-explicit project config loader

**Files:**
- Modify: `internal/bootstrap/configfile_test.go`
- Modify: `internal/bootstrap/configfile.go`

**Interfaces:**
- Produces: `func LoadConfigForProject(projectRoot string) (Config, error)`
- Semantics: global config from `DefaultConfigPath()` + optional `<projectRoot>/.ainovel/config.json`; no CWD mutation; project override retains existing merge behavior; corrupt/legacy handling matches `LoadConfig()`.

- [ ] **Step 1: Write failing tests** for project-root lookup while CWD points elsewhere, global+project merge, missing project config, and corrupt project config.
- [ ] **Step 2: Run bootstrap tests and verify RED** because `LoadConfigForProject` does not exist.
- [ ] **Step 3: Implement minimal helper** by sharing the existing load/merge flow with an explicit project config path; leave `LoadConfig()` behavior unchanged.
- [ ] **Step 4: Run bootstrap tests and verify GREEN** including existing config tests.

### Task 2: Serialized project session holder

**Files:**
- Create: `desktop/project_session_test.go`
- Create: `desktop/project_session.go`

**Interfaces:**
- Produces: `projectSessionController` (desktop-internal, not Wails-bound).
- Produces: runtime read access that keeps an `RWMutex.RLock` for the full callback.
- Produces: exclusive runtime replacement/clear primitive for GUI-02C.3 to use later.
- Invariant: controller owns at most one runtime pointer; replacement never closes a runtime itself in 02C.2 because close/switch sequencing belongs to GUI-02C.3.

- [ ] **Step 1: Write failing tests** proving no-runtime fail-closed access, active runtime callback access, and exclusive replacement waits for an in-flight read callback.
- [ ] **Step 2: Run desktop tests and verify RED** because the controller does not exist.
- [ ] **Step 3: Implement minimal controller** with `sync.RWMutex`, full-call read lease, and exclusive swap/clear primitives.
- [ ] **Step 4: Run desktop tests and verify GREEN**.

### Task 3: Gateway adopts session read lease without behavior drift

**Files:**
- Modify: `desktop/gateway_test.go`
- Modify: `desktop/gateway.go`

**Interfaces:**
- `newGateway(runtime, emit)` remains source-compatible for existing tests/callers.
- Gateway stores/uses `projectSessionController` rather than reading a runtime pointer and unlocking before a call.
- `Snapshot`, `Query`, and `Dispatch` execute inside the session read lease.
- Startup/shutdown event behavior remains unchanged for the initial runtime in this gate; dynamic event rebinding remains GUI-02C.3.

- [ ] **Step 1: Add failing concurrency test** where an in-flight Gateway query blocks an exclusive runtime clear/replacement until the query returns.
- [ ] **Step 2: Run desktop tests and verify RED** against the current Gateway implementation.
- [ ] **Step 3: Refactor Gateway minimally** to use the session holder while preserving all locked error/contract behavior.
- [ ] **Step 4: Run desktop tests and verify GREEN**, including GUI-02B Gateway regression tests.

### Task 4: Verification and scope audit

**Files:**
- Modify only if needed: existing CI workflow to ensure desktop tests remain executed; no product source change solely for evidence.

- [ ] **Step 1: Run `go test ./internal/bootstrap/...` and desktop gateway/session tests** on the exact candidate.
- [ ] **Step 2: Run repository CI, vet/format/race paths already applicable to the branch**.
- [ ] **Step 3: Compare exact parent `35ce9fdb...` to candidate** and verify no GUI-02C.3 lifecycle binding, no frontend filesystem authority, no backend/domain/runtime refactor.
- [ ] **Step 4: Fresh Audit** with blockers = 0 before presenting `CHỐT GUI-02C.2`.

## Self-Review

- Spec coverage: path-explicit config, single-runtime session ownership foundation, full-call serialization, fail-closed no-runtime behavior, and no-bypass constraints are covered.
- Deferred intentionally to GUI-02C.3: Create/Open/Switch/Close Wails methods, Host/AppRuntime factory, preflight path validation, event rebinding, close/reopen and project lease runtime scenarios.
- Placeholder scan: no implementation placeholders are required to execute this plan.
- Type consistency: GUI-02C.2 introduces only an additive bootstrap helper and desktop-internal session controller; locked public Gateway call shapes remain unchanged.
