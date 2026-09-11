# AINOVEL Desktop — G05.5 Resource Lock Manager & Conflict Serialization v1

Status: `G05.5 — CANDIDATE / EXACT-HEAD ACCEPTANCE REQUIRED`

Parent authority:
- `docs/desktop-g05-multirun-authority-v1.md`
- accepted G05.4 exact head `52b3d623dd62326a4c47b41bb7cb3f1494b89af0`

## Mission

G05.5 freezes the logical story-resource lock authority required before browser lanes and the multi-run orchestrator may open.

This gate is intentionally conservative: one canonical project-wide story resource is the minimum safe mutation boundary. Finer-grained chapter/document resources remain closed until their non-overlap can be proven.

## Canonical resource key

`RuntimeStore.CanonicalStoryResourceKey()` derives the resource from the Store's canonical project root:

1. resolve absolute project root;
2. resolve filesystem aliases/symlinks;
3. normalize platform path identity (including Windows case folding);
4. hash the canonical identity with a versioned domain separator;
5. persist only `story:<sha256>`.

The raw project path is never stored in the resource key and is never exposed as lock authority.

## Durable lock protocol

G05.5 extends the existing `meta/runtime/queue.jsonl` audit/replay source rather than creating another runtime database.

Typed resource facts are:
- `run_resource.wait_requested`
- `run_resource.acquired`
- `run_resource.released`
- `run_resource.wait_cancelled`
- `run_resource.recovery_released`

All use the existing `control` audit priority and carry typed `RunID` + opaque `ResourceKey`.

## Conflict policy

For one resource:
- at most one owner exists;
- a repeated request by the owner is idempotent;
- conflicts persist one wait claim per run;
- wait claims are FIFO by durable queue sequence;
- after release, only the oldest waiter may acquire;
- a later waiter cannot bypass the oldest waiter;
- non-owner release fails closed;
- malformed replay that implies double ownership or owner mismatch fails closed.

Scheduler priority remains owned by G05.4. The scheduler decides which run reaches the lock manager; once a conflict is persisted, lock-layer ordering is deterministic FIFO and cannot be changed by goroutine timing.

## Crash/recovery and cleanup

`RecoverResourceLocks()` explicitly releases every replayed owner using `run_resource.recovery_released`. Wait claims are preserved in original FIFO order. This is correct before G05.7 because mutating work does not survive the owning desktop process; a recovered run must reacquire before mutation resumes.

`ReleaseAllRunResourceClaims(runID)` is the terminal cleanup primitive for later orchestration. It releases held resources and cancels persisted waits, preventing leaked claims.

## Boundary kept closed

G05.5 does not:
- allocate browser lanes or profiles (G05.6);
- start/dispatch Engine or WebAI work;
- route AppRuntime lifecycle commands;
- create a second scheduler;
- add Run Center UI;
- expose raw filesystem paths as resource authority;
- add a desktop runtime database;
- change WEB-only / NO-API constraints.

G05.6+ remain closed until G05.5 exact-head acceptance passes.

## Acceptance

Exact candidate must pass:
- `gofmt` / build / vet / unit tests on Linux and Windows;
- resource-key alias/stability/opacity regression;
- no-double-owner and non-owner-release regression;
- deterministic FIFO conflict regression;
- restart replay regression;
- crash/recovery owner cleanup with waiter preservation;
- terminal no-leak cleanup;
- repository WEB-only / NO-API checks;
- active W6A/W6B/W6C release gates;
- branch-head no-drift re-read;
- parent-to-head boundary verification;
- zero blocking review threads;
- unchanged `main@e0a8ce46d1b2c17e0b0799d7d35af21685a6e951`.

Only after those pass may G05.5 be `PASS / LOCKED` and progress advance from `35/60 = 58.3%` to `36/60 = 60.0%`.
