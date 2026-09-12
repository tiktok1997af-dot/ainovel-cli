# AINOVEL Desktop — G06 Review / 12-Gate Quality Authority v1

Status: `G06.1 — FORWARD SUCCESSOR AUTHORITY / CANDIDATE`

Roadmap parent: **AINOVEL Desktop Optimized v2 — 8 GATE / 60 steps**.

Baseline: `main@a2e722528b9c729a3a1232ce9775fb5c8cfae3b6` (`G05 COMPLETE / MAIN VERIFIED`).

Implementation branch: `feat/desktop-roadmap-v2-g06-review-quality`.

Parent authority chain:

- `docs/desktop-g01-foundation-authority-v1.md`
- `docs/desktop-g02-appruntime-contract-v1.md`
- `docs/desktop-g03-roadmap-successor-authority-v1.md`
- `docs/desktop-g03-final-authority-v1.md`
- `docs/desktop-g04-creative-studio-authority-v1.md`
- `docs/desktop-g04-final-authority-v1.md`
- `docs/desktop-g05-multirun-authority-v1.md`
- `docs/desktop-g05-final-authority-v1.md`

## 1. Provenance and audit decision

The verified G05-merged baseline contains no authoritative `desktop-g06-*` document, no accessible PR titled as G06, and no recoverable verbatim historical G06 child-step sequence. G05 explicitly states that it does not allocate or rename G06-G08 child steps.

This authority therefore does **not** claim to recover missing historical wording. It is a forward-only successor authority derived from product obligations that are already locked by G01-G05 and from existing core review/evaluation seams on the verified baseline.

The audit establishes:

1. G01 requires a product `Review` workspace with **12 quality gates, repair/re-run, and promote official**.
2. G04 intentionally leaves full Review/12-Gate orchestration to a later roadmap gate and prohibits inventing a second Review engine inside the Creative Studio shell.
3. G05 consumes Multi-Run / Run Center ownership but explicitly leaves G06-G08 unallocated.
4. The core already owns review/evaluation facts and evaluators: `domain.ReviewEntry`, `diag`, `stylestat`, `internal/eval`, chapter/revision records, Editor review flow and `save_review` tooling.
5. Existing `ChapterAdvanceGate` review mode is a chapter-advance user policy and is **not** the same subsystem as the product Review/12-Gate quality workflow.

These facts are sufficient to freeze G06 as the product quality/review gate without creating a duplicate evaluator, duplicate Store, or parallel orchestration engine.

## 2. G06 mission

G06 owns **Review / 12-Gate Quality Workflow**.

G06 turns the existing review, diagnostic, style-statistics and accepted-revision facts into a typed desktop quality workflow behind AppRuntime. It must provide a deterministic, auditable path from evidence collection to quality status, bounded repair/re-run, and explicit official promotion while preserving Store/project facts as canonical truth.

G06 owns:

- a stable 12-gate product catalog and gate-state semantics;
- typed AppRuntime Review queries/commands/events/DTOs;
- deterministic quality evidence aggregation from existing core facts;
- review run orchestration using the G05 scheduler/resource/lane authority;
- bounded repair and re-run workflows for affected chapters/scopes;
- explicit, revision-safe `promote official` semantics;
- Review workspace UI projection and controls;
- end-to-end quality/recovery/compatibility regressions.

G06 does not replace the Engine, Editor, Store, `diag`, `stylestat`, `internal/eval`, ChapterRecord/revision authority, or WEB-only Gemini transport.

## 3. Roadmap allocation rule

G01-G05 account for **39/60** locked roadmap steps. Twenty-one steps remain for G06-G08.

This successor allocates **seven** steps to G06 only. It does not name, allocate internally, or redefine G07/G08; fourteen roadmap steps remain reserved for later successor authorities.

This 7-step G06 allocation is a forward governance decision, not a claim about unavailable historical wording.

Completion of all seven G06 steps would therefore bring the roadmap to **46/60**. G07 remains CLOSED until G06.7 is merged and resulting `main` is verified.

## 4. Existing quality/review authority that G06 must reuse

G06 must reuse, not clone, the existing seams:

- `internal/domain.ReviewEntry`, `ConsistencyIssue`, `DimensionScore` and existing review verdict semantics;
- `internal/diag` for deterministic fact/runtime findings;
- `internal/stylestat` for deterministic cross-chapter style regression;
- `internal/eval` for existing Go in-process evaluation aggregation where applicable;
- current Editor / `save_review` review facts and repair targets;
- `ChapterRecord` and revision semantics for accepted canonical chapter facts;
- G03 typed mutation/revision guards;
- G05 run scheduler, resource locking, browser lane and restart recovery ownership;
- AppRuntime as the sole Desktop UI-to-core seam.

A new product 12-gate projection may aggregate these sources, but it must not redefine what counts as a valid ChapterRecord, checkpoint, runtime transition, diagnostic finding, or style statistic.

## 5. Twelve-gate product semantics

G06.2 freezes the exact stable identifiers and DTO contract for the twelve product gates. G06.1 freezes only the governing requirements:

1. Exactly twelve stable product gate identities are exposed to the Review workspace.
2. Every gate has typed scope, state, evidence summary, freshness/revision context and actionable/non-actionable status.
3. Gate state must distinguish at least `not_run`, `running`, `pass`, `warn`, `fail`, `stale`, and `unavailable` where semantically valid.
4. Deterministic hard failure must be grounded in canonical evidence or deterministic evaluators; a free-form LLM score alone must not silently become canonical hard authority.
5. LLM/editor semantic review may produce verdicts, issues, repair targets and advisory quality judgments using existing review facts.
6. Evidence must remain bounded and serialization-safe across AppRuntime. Raw browser transcripts, credentials, cookies and unrestricted project files never become Review DTOs.
7. A gate result is stale when its referenced canonical revision/fingerprint no longer matches current project facts.
8. Review presentation must not mutate canonical facts merely by reading or rendering a gate result.

The exact twelve gate names are not guessed by G06.1; they are frozen in G06.2 after seam-to-product mapping audit so every gate has a real owning evidence source.

## 6. Quality evidence boundary

Quality evidence is projected from existing authority, not copied into a second truth database.

G06 may persist only narrowly necessary review-run/result metadata when a later child gate proves it is required for restart/history. Any additive metadata must identify its canonical source revision/fingerprint and must not duplicate full chapter/project truth.

G06 MUST NOT introduce:

- `ReviewStore` as a second canonical project database;
- a second diagnostic engine that reparses project state independently of `diag`;
- a second style-statistics implementation competing with `stylestat`;
- a second ChapterRecord/official chapter database;
- browser conversation as review memory;
- opaque local UI state presented as accepted quality truth.

## 7. Repair and re-run boundary

Repair/re-run is an explicit workflow, not an unrestricted text rewrite shortcut.

Rules:

- Review identifies typed affected scope/chapters and evidence.
- Repair actions enter through typed AppRuntime commands.
- Actual execution reuses existing Engine/Workers/Tools and G05 run orchestration.
- Mutating work obeys canonical resource locking and browser-lane ownership.
- Re-run must produce fresh evidence against the post-repair revision/fingerprint.
- Old results remain historical but become stale; they are never silently rewritten as if they evaluated new content.
- Cancellation/retry/restart behavior must preserve G05 recovery invariants.
- UI may request repair/re-run but never invokes Editor, Host, Store, WebAI or filesystem writes directly.

## 8. Official promotion boundary

`Promote official` is a privileged explicit product transition.

G06 promotion MUST:

- require a typed target and expected current revision/fingerprint;
- reject stale/precondition-conflicting promotion;
- preserve ChapterRecord and revision history rather than overwriting history in place;
- write canonical facts only through existing owning Store/Host/domain seams;
- be idempotent when the exact target revision is already official;
- surface validation/conflict/write failures explicitly;
- never infer approval from a UI selection, LLM verdict, browser response or cached gate result alone;
- never promote while required hard-fail evidence for the same target revision remains unresolved under the frozen G06 gate policy.

G06 does not authorize destructive rollback of later canonical story facts merely to promote an older revision.

## 9. AppRuntime and UI boundary

Desktop continues to use only:

`Snapshot / Query / Dispatch / Subscribe / Close`.

G06 may add Review-specific Query/Command/Event kinds and serialization-safe DTOs under the existing desktop contract. It must not add a second frontend transport.

The Review workspace eventually presents:

- twelve gate cards/statuses;
- bounded evidence and issue summaries;
- affected chapter/scope projection;
- freshness/stale information;
- review history where authoritative metadata exists;
- repair and re-run controls;
- explicit promote-official control;
- run/progress/error/recovery projection for review work.

All authoritative state is reconciled from AppRuntime results/events/fresh queries. The UI never marks a gate PASS or a chapter official by local optimistic state alone.

## 10. ChapterAdvanceGate separation

Existing chapter `review` advance mode remains a user policy controlling whether a new forward chapter may start. G06 quality Review is a separate concern.

G06 MUST NOT:

- reinterpret `/review on|off` as running the 12-gate Review workspace;
- consume or manufacture ChapterAdvance permits as a quality verdict;
- merge the advance-gate state machine into the Review gate state machine;
- bypass an active chapter hold/permit policy during repair execution.

Integration is allowed only through existing lifecycle facts and typed orchestration boundaries.

## 11. Closed scope

G06 MUST NOT introduce:

- direct AI API/provider fallback;
- a hidden browser path;
- credential/cookie extraction;
- a second Engine/Editor/Store/project DB;
- generic filesystem write/delete/move APIs;
- direct Desktop UI calls to Host/Store/WebAI/Engine/eval packages;
- unlimited parallel review or repair execution;
- destructive project-format migration;
- unrelated Folder View or Settings ownership;
- G07/G08 implementation;
- a new general-purpose eval platform unrelated to the product Review workflow.

WEB-only Gemini remains the sole AI execution path whenever a semantic review/repair requires model execution.

## 12. Forward-frozen G06 child sequence

The authoritative forward sequence is:

```text
G06.1  Authority / roadmap audit & Review/12-Gate scope freeze          ACTIVE / this candidate
G06.2  AppRuntime Review contract + 12-gate catalog / projection DTOs   CLOSED until G06.1 PASS
G06.3  Deterministic quality evidence aggregation                       CLOSED until G06.2 PASS
G06.4  Review orchestration + bounded repair / re-run workflow          CLOSED until G06.3 PASS
G06.5  Revision-safe official promotion workflow                        CLOSED until G06.4 PASS
G06.6  Review workspace UI integration                                  CLOSED until G06.5 PASS
G06.7  End-to-end Review regression + final authority / merge gate      CLOSED until G06.6 PASS
```

No successor child step opens merely because this file exists. G06.1 must first pass exact-head acceptance.

## 13. G06.1 deliverable

G06.1 is authority-only:

- this file;
- no production code;
- no AppRuntime Review contract yet;
- no twelve-gate identifiers yet;
- no review execution or persistence changes;
- no UI implementation.

## 14. G06.1 exact-head acceptance rule

G06.1 becomes `PASS / AUTHOR-CONFIRMED / LOCKED` only when:

1. branch parent is verified as `main@a2e722528b9c729a3a1232ce9775fb5c8cfae3b6`;
2. the G06.1 candidate boundary is authority-only and exact;
3. repository CI succeeds on the exact candidate SHA;
4. WEB-only / NO-API residue audit remains green;
5. Linux format/vet/tests and required race tests remain green;
6. Windows format/vet/tests remain green according to active workflow authority;
7. any repository-required release gates automatically triggered for that exact SHA are green before acceptance;
8. branch head is re-read with no drift;
9. `main` has not unexpectedly drifted from the verified G05 merge baseline;
10. blocking review threads are zero;
11. an acceptance record/comment identifies the exact accepted SHA and boundary.

If the head changes, G06.1 reopens and the new exact head must be revalidated.

## 15. G06.1 PASS effect

Only after section 14 passes:

- `G06.1 = PASS / AUTHOR-CONFIRMED / LOCKED`;
- roadmap progress becomes **40/60 = 66.7%**;
- `G06.2 = OPEN / AUTHORIZED`;
- G06.3-G06.7 remain CLOSED.

The cumulative G06 PR remains Draft until G06.7 final authority/merge gate.

## 16. Inherited non-negotiable locks

G06 preserves all locked G01-G05 invariants, including:

- compatibility lineage from `ainovel-cli v0.1.3`;
- Store/project files are canonical story truth;
- browser conversation is transient execution state, not project memory;
- Desktop UI reaches core only through AppRuntime;
- existing project/read/write/revision contracts remain compatible unless a child authority proves an additive extension;
- G05 scheduler/resource/lane/recovery authority remains the sole multi-run orchestration owner;
- WEB-only Gemini remains the sole AI execution path;
- no direct AI API/provider fallback;
- no duplicate canonical database or second Engine;
- no destructive project migration without a later explicit migration authority.

This authority is forward-only and additive. Later G06 child authorities may clarify implementation details but may not silently weaken these locks.