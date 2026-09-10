# AINOVEL Desktop — G03 Final Regression / Authority / Merge Gate v1

Status: `G03.8 — FINAL AUTHORITY / ACCEPTANCE GATE`

Forward authority: `docs/desktop-g03-roadmap-successor-authority-v1.md`.

Parent authority chain:

- `docs/desktop-g01-foundation-authority-v1.md`
- `docs/desktop-g02-appruntime-contract-v1.md`
- `docs/desktop-g03-project-knowledge-authority-v1.md`
- `docs/desktop-g03-query-contract-v1.md`
- `docs/desktop-g03-project-read-handlers-v1.md`
- `docs/desktop-g03-document-catalog-v1.md`
- G03.5 exact-head PR gate record
- `docs/desktop-g03-mutation-contract-v1.md`
- `docs/desktop-g03-mutation-handlers-v1.md`

Final-gate branch: `feat/desktop-roadmap-v2-g03-project-knowledge`.

Pre-authority implementation baseline audited by this gate: `10019fdb87bb0050e1041f7fd9cb9d05de18e175`.

That baseline had already passed CI #425 plus W6A release packaging, W6B install/update integrity and W6C real packaged desktop smoke before this final authority was written.

This document is intentionally written once as the final branch artifact. It does not self-record its own future commit SHA or workflow run numbers, because editing those values afterward would create a new unvalidated head. The exact commit containing this file becomes G03.8 PASS only when the gate conditions in section 12 are satisfied and PR #19 records that tested SHA in metadata.

## 1. Final scope decision

G03 as a whole is the Project / Knowledge desktop core model behind AppRuntime. It establishes bounded Store-backed reads and a typed Store-backed write plane without creating a GUI workspace, a second project database, a second engine, or an alternate AI execution path.

The final boundary remains:

```text
Desktop UI / future desktop transport
        |
        v
AppRuntime
  Snapshot / Query / Dispatch / Subscribe / Close
        |
        v
narrow Host seams
        |
        v
existing canonical Store / domain owners
```

No G03 change grants the frontend direct Host, Store, filesystem or WebAI access.

## 2. Fresh G03.1 → G03.7 audit

The final audit re-read the parent authorities and the complete PR #19 changed-file set rather than relying only on prior child-gate conclusions.

| Child gate | Final-audit result | Locked outcome |
| --- | --- | --- |
| G03.1 Project/Knowledge domain + Store seam authority | PASS | Existing Store/domain remain canonical; no duplicate Project/Chapter/Outline/Context/Canon/Character/Timeline persistence. |
| G03.2 AppRuntime query contract + DTO skeleton | PASS | Stable eleven-query catalog, strict typed JSON, bounded requests, serialization-safe result DTOs. |
| G03.3 Project/chapter/outline read handlers | PASS | Reads use narrow Host seams; plan/draft/workspace/accepted record remain distinct; flat/layered outline semantics preserved. |
| G03.4 Document catalog/get | PASS | Supported-artifact view only; logical IDs, containment checks and fixed discovery scope; no DocumentStore or arbitrary filesystem browser. |
| G03.5 Knowledge read handlers | PASS | Context bounded/ephemeral; Canon provenance-bearing; core/cast origins distinct; World/Timeline remain canonical; historical leakage protections retained. |
| G03.6 Mutation command contract + DTO skeleton | PASS | Exactly fourteen typed mutation kinds; canonical logical resource IDs; strict payload validation and conflict taxonomy. |
| G03.7 Store-backed mutation handlers | PASS | Exactly those fourteen kinds execute through narrow Host seams and existing Store/domain owners with chapter/outline/knowledge consistency guards. |

No child authority is weakened or superseded by this final gate.

## 3. `ainovel.desktop.v1` compatibility audit

G03 preserves:

```text
ContractVersion = "ainovel.desktop.v1"
SchemaVersion   = 1
```

The five-method AppRuntime facade remains unchanged:

```go
Snapshot(ctx, ...)
Query(ctx, ...)
Dispatch(ctx, ...)
Subscribe(ctx, ...)
Close(ctx, ...)
```

G03 changes are additive behind that facade:

- Query gained stable typed Project/Knowledge query kinds and result DTOs.
- CommandResult gained optional `resource` and `data` fields.
- No required existing field was removed or renamed.
- Existing lifecycle command kinds and lifecycle execution remain on the G02 path.
- Query and mutation payload decoding rejects unknown fields and malformed/trailing JSON.
- DTOs crossing the contract do not contain `domain.*`, `store.*`, Host pointers, file handles, raw Go errors, cookies, browser storage or provider objects.

## 4. Read-plane audit

The frozen read catalog remains exactly:

```text
project.overview
chapters.list
chapters.get
outline.get
documents.list
documents.get
knowledge.context
knowledge.canon
knowledge.characters
knowledge.world
knowledge.timeline
```

All eleven kinds route through `AppRuntime.Query` and are read-only.

Large/broad facts remain bounded with paging/range/window semantics. Chapter prose is opt-in on chapter detail rather than injected into global snapshot. Layered `Progress.TotalChapters` is not exposed as a fixed final chapter count.

## 5. Write-plane audit

The executable G03 mutation catalog remains exactly:

```text
project.metadata.update
project.premise.update
chapter.plan.save
chapter.draft.save
chapter.workspace.save
outline.tail.revise
outline.arc.expand
outline.volume.append
outline.compass.update
knowledge.characters.core.replace
knowledge.world.rules.replace
knowledge.timeline.append
knowledge.relationships.update
knowledge.foreshadow.update
```

All fourteen kinds route through `AppRuntime.Dispatch` -> strict G03 mutation decode -> narrow Host mutation seam -> existing canonical Store/domain owner.

There is no fifteenth G03 mutation and no generic fallback mutation route.

## 6. Persistence / filesystem audit

The complete PR #19 changed-file set is limited to:

- G03 authority documentation;
- `internal/appruntime/*` facade/contracts/read-write handlers/tests;
- narrow `internal/host/desktop_*` seams/tests.

G03 adds no changed file under `internal/store`, no database package, no migration and no new project format.

No G03 mutation accepts a filesystem path. There is no generic `writeFile(path, bytes)`, recursive editor, JSON patch to arbitrary project files, `DocumentStore`, `ContextStore`, `CanonStore`, second Character store, second Timeline store or desktop project database.

The document read seam performs filesystem reads only after resolving a supported logical catalog entry. Dynamic discovery is limited to fixed Store-owned directories and canonical filename patterns; symlink containment prevents supported-looking paths from escaping project root. That read-only catalog does not confer write authority.

## 7. Host / Store / GUI leakage audit

The public desktop contract exposes only AppRuntime DTOs and `AppError`.

`Runtime.core` remains private. The Host seams added by G03 return detached domain values internally to AppRuntime or accept detached domain values internally from AppRuntime; they do not cross the desktop-facing interface.

No GUI files are modified by PR #19, so G03 does not introduce a direct GUI -> Host/Store/WebAI dependency. Future G04 UI must still consume only AppRuntime.

## 8. Chapter continuity / revision audit

Plan, draft, editable workspace and accepted `ChapterRecord` remain separate facts.

`chapter.draft.save` and `chapter.workspace.save` support compare-before-write SHA guards. Workspace mutation additionally supports accepted-record revision awareness. Missing guarded targets return `target_not_found`; mismatches return `stale_conflict`.

A workspace save never accepts, rewrites or silently updates the accepted ChapterRecord and never starts AI revision analysis. User/editor workspace divergence therefore remains visible to the existing revision workflow rather than corrupting accepted historical facts.

## 9. Outline / knowledge consistency audit

Structural outline writes continue through existing Store coordination seams:

- `Store.ReviseOutline`
- `Store.ExpandArc`
- `Store.AppendVolume`

Completed/in-progress outline history stays protected. Layered/flat projections and Progress capacity coordination remain core-owned. Same-payload arc/volume recovery paths retain idempotent/replay behavior.

Core-character replacement writes only CharacterStore and does not rewrite CastStore. Timeline remains append-only under WorldStore with stable-key deduplication. Relationship and foreshadow read-modify-write paths fail closed on unreadable canonical state. Context remains computed read state and Canon remains a provenance-bearing projection rather than writable duplicate persistence.

Historical knowledge reads continue to bound accepted records/supporting-cast/world projections so chapter-scoped reads do not expose future supporting-cast or chapter-derived facts.

## 10. Error / fail-closed audit

Desktop error messages remain sanitized through AppError. G03 keeps distinct stable categories for:

- validation / malformed request;
- unsupported mutation;
- missing typed target;
- precondition conflict;
- stale optimistic conflict;
- canonical Store read failure;
- canonical Store write failure.

Raw filesystem paths, Store implementation errors and provider/browser internals are retained only in internal Go cause chains and are not serialized into AppError messages.

Mutations are also rejected while the physical engine is active or AppRuntime is in a running/transitional lifecycle state. This is a consistency guard only; it does not prematurely implement G05 scheduling/resource locks.

## 11. Fresh regression state before final authority

Immediately before this authority was created, the audited implementation baseline `10019fdb87bb0050e1041f7fd9cb9d05de18e175` was verified with:

- CI #425 — PASS;
- Linux format — PASS;
- Linux vet — PASS;
- Linux test — PASS;
- Linux critical race tests — PASS;
- Windows format — PASS;
- Windows vet — PASS;
- Windows test — PASS;
- WEB-only / NO-API audit — PASS;
- W6A Release Packaging Gate — PASS;
- W6B Install Update Integrity Gate — PASS;
- W6C Release Artifact Desktop Smoke — PASS.

PR #19 had no unresolved inline review threads at final-audit time.

These baseline results are supporting evidence only. The commit containing this final authority must pass its own exact-head checks below before merge.

## 12. Exact-head G03.8 acceptance rule

The exact commit containing this file is accepted as `G03.8 — PASS / FINAL AUTHORITY LOCKED` only when all of the following are true for that same SHA:

1. CI concludes SUCCESS.
2. WEB-only / NO-API residue audit concludes SUCCESS.
3. Linux format, vet, tests and critical race tests conclude SUCCESS.
4. Windows format, vet and tests conclude SUCCESS; workflow-defined Windows race skip remains acceptable.
5. W6A Release Packaging Gate concludes SUCCESS.
6. W6B Install Update Integrity Gate concludes SUCCESS.
7. W6C real packaged desktop smoke concludes SUCCESS; it must not be bypassed if the workflow requires an interactive runner.
8. PR #19 remains mergeable and has no unresolved blocking review thread.
9. No branch commit is added after those exact-head results before merge.

W6D release-publication workflows are not a G03 feature merge requirement: they publish releases rather than validate this Project/Knowledge feature branch. G03.8 therefore requires W6A/W6B/W6C, not a release publication.

After conditions 1-9 pass, PR metadata may record the exact tested SHA without changing the branch head.

## 13. Merge rule

Only after section 12 passes may the following sequence occur:

```text
G03.8 PASS
  -> update PR #19 final checklist/authority metadata
  -> mark PR #19 Ready for review
  -> merge PR #19 with expected exact head SHA
  -> fetch PR/main state
  -> verify merged = true
  -> verify resulting main HEAD contains the G03 final authority
  -> declare G03 = 100%
  -> open G04
```

If the head moves before merge, G03.8 reopens and the new head must be revalidated.

## 14. Final inherited locks

G03 final acceptance preserves all of the following:

- Store/project files are canonical source of truth.
- Browser conversation is not project memory or persistence.
- Desktop UI accesses core only through AppRuntime.
- Documents are a supported catalog/view, not a filesystem browser/store.
- Context is ephemeral and bounded.
- Canon is a provenance-bearing projection, not duplicate persistence.
- Core Characters and supporting Cast remain distinct authorities.
- Timeline remains a WorldStore subdomain.
- Accepted ChapterRecord remains distinct from editable workspace text.
- Project format remains v2.
- WEB-only Gemini remains the sole AI execution path.
- No direct AI API/provider fallback is permitted.
- G05 resource locking/scheduling is not implemented early by G03.

Once the exact-head acceptance rule passes and PR #19 is merged/verified on main, G03 is complete and G04 may be opened.