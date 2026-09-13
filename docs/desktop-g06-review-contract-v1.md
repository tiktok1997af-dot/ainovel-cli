# AINOVEL Desktop — G06.2 Review Contract & 12-Gate Catalog v1

Status target: `G06.2 — AppRuntime Review contract + 12-gate catalog / projection DTOs`.

Parent authority: `docs/desktop-g06-review-quality-authority-v1.md`.

Accepted parent candidate: `G06.1@3bceaa5c1177448040c101c5378b939411003b0e`.

This document freezes the G06.2 serialization vocabulary only. It does not operationalize evidence aggregation, repair/re-run, promotion, or Review UI behavior. Those remain owned by later G06 child gates.

## 1. Seam-to-evidence audit

G06.2 reuses the existing quality owners rather than inventing a second review engine:

- `domain.ReviewEntry` and `save_review` already own Editor semantic review facts, chapter contract fulfillment, issue targets and the seven review dimensions.
- `assets/prompts/editor.md` defines the seven native dimensions: `consistency`, `character`, `pacing`, `continuity`, `foreshadow`, `hook`, `aesthetic`.
- `stylestat` owns deterministic cross-chapter style facts and is explicitly the deterministic quality backbone in `docs/evaluation-system.md`.
- `diag` owns deterministic fact findings with stable categories `flow`, `quality`, `planning`, `context`.
- `internal/eval` consumes `diag`/`stylestat`/review facts and explicitly forbids reimplementing their checks.
- structured `user_rules` are already required by the Editor prompt to be mapped into the seven native dimensions; they are not a thirteenth product gate.
- `diag.quality` is not exposed as a separate product gate because its rules summarize or correlate existing review/contract/hook/rewrite/word-count signals; exposing it separately would double-count quality evidence.
- ChapterRecord revision/hash facts provide freshness provenance; they are not a quality score of their own.

## 2. Frozen twelve-gate catalog

The stable ordered product catalog is exactly:

| # | Gate ID | Class | Evidence owner |
|---|---|---|---|
| 1 | `contract_fulfillment` | semantic | `ReviewEntry.ContractStatus/ContractMisses` + chapter contract |
| 2 | `consistency` | semantic | `ReviewEntry.Dimension("consistency")` |
| 3 | `character` | semantic | `ReviewEntry.Dimension("character")` |
| 4 | `pacing` | semantic | `ReviewEntry.Dimension("pacing")` |
| 5 | `continuity` | semantic | `ReviewEntry.Dimension("continuity")` |
| 6 | `foreshadow` | semantic | `ReviewEntry.Dimension("foreshadow")` |
| 7 | `hook` | semantic | `ReviewEntry.Dimension("hook")` |
| 8 | `aesthetic` | semantic | `ReviewEntry.Dimension("aesthetic")` |
| 9 | `style_regression` | deterministic | `stylestat` |
| 10 | `flow_integrity` | deterministic | `diag` category `flow` |
| 11 | `planning_integrity` | deterministic | `diag` category `planning` |
| 12 | `context_integrity` | deterministic | `diag` category `context` |

The identifiers and ordering above are serialization authority for the desktop Review workspace. Later gates may add labels/help text but must not silently rename/reorder IDs or change the owning evidence source.

## 3. Gate state vocabulary

A projected gate uses only:

- `not_run`
- `running`
- `pass`
- `warn`
- `fail`
- `stale`
- `unavailable`

G06.2 does not invent score thresholds. G06.3 must map existing evidence into these states while preserving the current rule that deterministic hard failure comes from canonical/deterministic evidence, not from a free-form model score alone.

## 4. Scope and freshness vocabulary

Review scope is typed as:

- `chapter`
- `arc`
- `global`

A target is identified by the fields appropriate to its scope (`chapter`, or `volume`+`arc`+`through_chapter`, or `through_chapter`).

Freshness is projected using:

- an opaque deterministic `fingerprint` owned by the later evidence aggregator;
- bounded chapter revision references `{chapter, revision, content_sha256}`;
- `evaluated_at`.

A result is stale when the fingerprint/revision references no longer match canonical ChapterRecord facts. G06.2 defines the DTO only; G06.3 owns calculation and comparison.

## 5. AppRuntime query vocabulary

Frozen Review query kinds:

- `review.catalog`
- `review.status`
- `review.history`

`review.catalog` returns the static twelve-gate contract.

`review.status` projects the latest authoritative/bounded state for one typed target.

`review.history` is a paged metadata projection. It must not return raw browser transcripts or unrestricted chapter/project content.

Operational query handlers and evidence aggregation are G06.3 scope.

## 6. AppRuntime command vocabulary

Frozen Review command kinds:

- `review.run`
- `review.repair`
- `review.rerun`
- `review.promote_official`

Payloads carry a typed target and, where mutation is possible, an `expected_fingerprint` plus bounded revision refs/affected chapters as appropriate.

G06.2 freezes payload DTOs only. Dispatch/Engine integration for `run`, `repair`, `rerun` is G06.4 scope. `promote_official` operational behavior is G06.5 scope. Therefore G06.2 MUST NOT add command routing that pretends these commands are executable.

## 7. AppRuntime event vocabulary

Review event category: `REVIEW`.

Frozen event types:

- `review_state`
- `review_gate`
- `review_action`

Events use bounded DTO payloads and canonical identifiers. They may announce projected state but cannot make UI-local optimistic state authoritative.

Event production is operationalized only by later G06 child gates.

## 8. Projection DTO boundary

The frozen DTO family includes:

- `ReviewGateDefinitionDTO`
- `ReviewTargetDTO`
- `ReviewRevisionRefDTO`
- `ReviewFreshnessDTO`
- `ReviewEvidenceDTO`
- `ReviewGateResultDTO`
- catalog/status/history result DTOs
- command payload DTOs
- state/gate/action event payload DTOs

Evidence DTOs expose bounded summaries/codes/chapter references only. They do not contain raw browser conversation, cookies, credentials, arbitrary filesystem paths, or unrestricted project files.

## 9. G06.2 closed scope

This gate MUST NOT:

- run `diag` or `stylestat` from AppRuntime;
- persist Review result/history metadata;
- dispatch Editor/Writer/Engine work;
- create repair runs;
- promote any chapter official;
- add Review workspace UI;
- modify ChapterRecord semantics;
- add a second Store/evaluator;
- open G06.3 before exact-head acceptance.

## 10. G06.2 acceptance

G06.2 may lock only when:

1. the exact twelve IDs and ordering above are represented by code and tests;
2. typed query/command/event/DTO vocabulary compiles and round-trips through JSON;
3. no Review command is operationalized prematurely;
4. the cumulative PR remains based on the accepted G05 main baseline with G06.1 as parent;
5. exact-head CI and repository-required release gates are green;
6. branch head/base/boundary and zero blocking review threads are revalidated;
7. an acceptance comment records the exact accepted SHA and G06.2 boundary.

PASS effect: roadmap becomes `41/60 = 68.3%`, G06.3 opens, and G06.4-G06.7 remain closed.
