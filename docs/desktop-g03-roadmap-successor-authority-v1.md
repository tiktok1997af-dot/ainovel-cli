# AINOVEL Desktop — G03 Roadmap Successor Authority v1

Status: `AUTHOR-CONFIRMED / LOCKED SUCCESSOR AUTHORITY`

Roadmap parent: **AINOVEL Desktop Optimized v2 — 8 GATE / 60 steps**.

Branch: `feat/desktop-roadmap-v2-g03-project-knowledge`

Base authority chain:

- `docs/desktop-g01-foundation-authority-v1.md`
- `docs/desktop-g02-appruntime-contract-v1.md`
- `docs/desktop-g03-project-knowledge-authority-v1.md`
- `docs/desktop-g03-query-contract-v1.md`
- `docs/desktop-g03-project-read-handlers-v1.md`
- `docs/desktop-g03-document-catalog-v1.md`
- PR #19 G03.5 exact-head gate record

## 1. Why this successor exists

The original Optimized v2 roadmap authority is known to define eight parent gates and sixty child steps, but the original verbatim wording for the remaining G03 child-step lines could not be recovered from repository history, PR metadata, accessible conversation/library artifacts, or commit search.

This document does **not** pretend to recover or rewrite that missing text. It is a forward-only successor authority, explicitly reconstructed from the already-authoritative G01/G02/G03 contracts and the implementation state that has passed through G03.5.

The successor preserves all previously locked behavior and supplies an auditable, executable definition for G03.6 through G03.8 so the roadmap can continue without inventing hidden historical provenance.

## 2. Inherited G03 state

The following completed steps are inherited without reopening their implementation:

1. **G03.1 — Project / Knowledge domain + Store seam audit & contract freeze** — `PASS / INHERITED`.
2. **G03.2 — AppRuntime Project / Knowledge query contract + DTO skeleton** — `PASS / INHERITED`; exact-head `2276a40eccccbbb5768f512d0ee273ef777d7153`, CI #377 PASS.
3. **G03.3 — Store-backed Project read handlers: overview / chapters / outline** — `PASS / INHERITED`; exact-head `d38d9c328f26bb3969435499d8da426f375bf0e1`, CI #384 PASS.
4. **G03.4 — Store-backed Document catalog / document get** — `PASS / INHERITED`; exact-head `079c506ec8da6b14db90cb2feccb57a48bc36992`, CI #400 PASS.
5. **G03.5 — Knowledge read handlers** — `PASS / INHERITED`; exact-head `0c79141b01dc42d27ea65bac31c3e504e45e89c2`, CI #413 PASS.

Inherited PASS means their locked contracts remain authoritative. This successor may clarify forward sequencing but must not silently weaken, reinterpret, or invalidate those gates.

## 3. G03.6 — Project / Knowledge mutation command contract + DTO skeleton

Status after this successor is validated: `OPEN / ACTIVE`.

Purpose: establish the typed desktop **write-plane contract** behind `AppRuntime.Dispatch` before any new Project/Knowledge mutation is exposed to a GUI.

G03.6 MUST:

- audit the existing Host/Store/domain mutation seams that are safe to expose;
- freeze stable typed command identities for the supported Project/Chapter/Outline/Knowledge mutations;
- define serialization-safe request/result DTOs only — no `domain.*`, `store.*`, Host pointers, raw Go errors, or filesystem handles cross the desktop boundary;
- define logical `resource` identity for every mutating command so later orchestration can serialize conflicting writes without redesigning G03 contracts;
- preserve the G02 command acceptance, lifecycle, structured `AppError`, schema-version and compatibility rules;
- strictly validate payloads and reject unknown fields;
- distinguish unsupported mutation, missing target, precondition conflict, stale/revision conflict, validation failure, read failure and write failure;
- map each allowed command to an existing Store/Host/domain authority or to a narrowly additive core coordination seam when one is demonstrably required;
- keep all G03 read queries backward compatible and read-only.

G03.6 MUST NOT:

- perform the new Project/Knowledge mutations merely because the DTO exists;
- add `ContextStore`, `CanonStore`, `DocumentStore`, a second character database, a second timeline store or any desktop-only canonical database;
- add generic `writeFile(path, bytes)`, arbitrary path mutation, recursive filesystem editing or a GUI file-manager write escape hatch;
- permit clients to choose an absolute path as story authority;
- add AI/browser execution or any AI API/provider fallback;
- implement G05 resource locking early;
- bypass chapter accepted-record/revision semantics, protected outline history, core/cast ownership, WorldStore ownership or provenance rules.

The exact command catalog is frozen by G03.6 only after seam audit. Unsupported product wishes remain explicitly unsupported rather than being emulated with raw filesystem writes.

## 4. G03.7 — Store-backed Project / Knowledge mutation handlers + consistency semantics

Status: `CLOSED` until G03.6 is PASS.

Purpose: implement only the typed mutation catalog locked by G03.6 against the canonical Store/Host/domain seams.

G03.7 MUST:

- route every implemented write through `AppRuntime.Dispatch` and canonical core seams;
- preserve Store as source of truth and existing file/record projection rules;
- enforce chapter plan/draft/final/accepted-record and revision boundaries instead of treating a chapter as one interchangeable text file;
- protect completed/in-progress history when outline edits target future plans;
- preserve flat/layered outline semantics and existing cross-domain Store coordination methods;
- preserve core-character versus supporting-cast ownership and same-name precedence;
- keep Timeline under WorldStore and preserve its existing deduplication/projection semantics;
- route semantic Canon edits to the owning persisted authority rather than persisting a duplicate Canon projection;
- treat Context as computed read state and never persist it as canonical truth;
- return typed post-mutation results suitable for deterministic Query/Snapshot refresh;
- include contract, conflict, idempotency/replay-safety where applicable, corruption/error and regression tests.

G03.7 MUST NOT invent broad mutation capabilities when no safe owning seam exists. Such operations remain unsupported until a later explicit authority adds the required core contract.

## 5. G03.8 — G03 final regression / authority / merge gate

Status: `CLOSED` until G03.7 is PASS.

Purpose: prove that G03 as a whole is internally consistent and does not regress the locked desktop/runtime invariants before merging PR #19.

G03.8 requires:

1. fresh audit of G03.1 through G03.7 against this successor and all parent authorities;
2. exact-head CI PASS on Linux and Windows, including format/vet/tests and Linux critical race tests;
3. WEB-only / NO-API residue audit PASS;
4. release/package regression gates required by the repository remain green; real packaged desktop smoke must not be bypassed if the active workflow requires it;
5. no duplicate persistence domain, raw filesystem mutation surface, Host/Store exposure to GUI DTOs, or historical continuity leakage;
6. query and dispatch serialization compatibility under `ainovel.desktop.v1`;
7. PR #19 final body/checklist and authority record match the exact tested head;
8. only after all final checks pass may PR #19 be marked ready and merged;
9. after merge, `main` HEAD must be fetched and verified before declaring `G03 = 100%` or opening G04.

No documentation-only edit may be added after the final exact-head gate unless the changed head is revalidated.

## 6. Locked sequence

The successor child-step sequence for the rest of G03 is therefore:

```text
G03.1  Domain / Store seam audit & contract freeze                 PASS / inherited
G03.2  AppRuntime read query contract + DTO skeleton               PASS / inherited
G03.3  Store-backed Project read handlers                          PASS / inherited
G03.4  Store-backed Document catalog / get                         PASS / inherited
G03.5  Store-backed Knowledge read handlers                        PASS / inherited
G03.6  Project / Knowledge mutation command contract + DTO skeleton OPEN after successor gate
G03.7  Store-backed Project / Knowledge mutation handlers           CLOSED until G03.6 PASS
G03.8  G03 final regression / authority / merge gate                CLOSED until G03.7 PASS
```

G04 remains **CLOSED** until G03.8 passes, PR #19 is merged, and the resulting `main` HEAD is verified.

## 7. Non-negotiable inherited invariants

- Store/project files remain canonical story truth.
- Browser conversation is not persistent memory.
- Desktop UI accesses core only through AppRuntime.
- Queries remain bounded and read-only.
- Documents remain a catalog/view, not a generic store or file browser.
- Context remains an ephemeral bounded projection.
- Canon remains a provenance-bearing semantic projection, not a duplicate persistence file.
- Character core/cast origins remain distinct.
- Timeline remains a WorldStore subdomain.
- Existing project format remains v2 unless a later explicit migration authority proves a format change is necessary.
- WEB-only Gemini remains the sole AI execution path; no direct AI API or provider fallback may re-enter production.
- PR #19 stays Draft until G03.8 final gate.

## 8. Successor acceptance

This successor is authoritative when:

- the user explicitly authorizes the reconstructed successor approach;
- this file is committed to the G03 branch;
- the authority itself passes exact-head branch CI / WEB-only regression;
- PR #19 records this file as the controlling forward authority.

Once those conditions are met, G03.6 may be marked `OPEN / ACTIVE` and implemented. G03.7 and G03.8 remain closed until their predecessor gate passes.
