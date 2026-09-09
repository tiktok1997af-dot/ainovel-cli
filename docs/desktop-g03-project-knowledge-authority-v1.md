# AINOVEL Desktop — G03.1 Project & Knowledge Domain / Store Seam Authority v1

Status: `G03.1 — CONTRACT FREEZE CANDIDATE`

Parent authorities:

- `docs/desktop-g01-foundation-authority-v1.md`
- `docs/desktop-g02-appruntime-contract-v1.md`

Baseline: `main@443368d3f1cb2400984de28e4919e7fb505d96aa` after G02 AppRuntime merge. The existing WEB-only Gemini execution invariant, Store source-of-truth rule, and AppRuntime-only desktop boundary remain mandatory.

This authority audits and freezes the seams for the G03 Project / Knowledge product surfaces before any GUI workspace or new query implementation is added.

## 1. G03.1 decision

G03 MUST reuse the existing file-backed Store and domain contracts. The desktop product MUST NOT introduce a parallel project database, a second chapter store, a second outline, or duplicate Context/Canon persistence.

Desktop reads and writes remain behind the G02 facade:

```text
Desktop UI
   |
   v
AppRuntime.Query / AppRuntime.Dispatch
   |
   v
existing Host / Store / domain
```

The GUI MUST NOT import Store/domain implementation structs or read/write project files directly.

## 2. Source-of-truth matrix

| Product surface | Existing authority | G03 decision | Notes |
| --- | --- | --- | --- |
| Project | `store.Store`, `BookStore`, project format | KEEP + WRAP | Store root remains project root; `meta/book.json` is book metadata truth. |
| Chapters | `DraftStore`, `ChapterRecordStore`, `ProgressStore`, `SummaryStore` | KEEP + COMPOSE | No second chapter database. Workspace text, accepted baseline/facts, progress and summaries are separate existing facts. |
| Arc / Volume | `OutlineStore`, `domain.VolumeOutline`, `domain.ArcOutline` | KEEP + WRAP | Layered outline is authoritative in layered mode; flat outline is its derived view. |
| Documents | known project artifacts | ADD VIEW ONLY | No generic `DocumentStore`. Desktop document catalog indexes supported existing artifacts and bounded previews. |
| Context | existing project facts + summaries + memory policy | ADD COMPOSED READ MODEL | Context is computed for a task/window; it is not a new persisted truth object. |
| Canon | characters/world/timeline/state/relationships/foreshadow/chapter facts | ADD COMPOSED READ MODEL | No `CanonStore` and no duplicate `canon.json`. Canon view carries provenance to real sources. |
| Characters | `CharacterStore` + `CastStore` + snapshots | KEEP + COMPOSE | Core characters and supporting-cast ledger remain distinct authorities. |
| World | `WorldStore` | KEEP + WRAP | World rules, relationships, foreshadow, state changes and related story facts stay in the existing store. |
| Timeline | `WorldStore` timeline append log | KEEP + WRAP | Timeline remains part of WorldStore; no separate desktop timeline database. |

## 3. Project structure authority

`store.Store` is the composition root. Current project format remains `CurrentProjectFormatVersion = 2`; G03.1 introduces no format bump and no migration.

Important current authorities include:

```text
meta/format.json                project format
meta/book.json                  book metadata
meta/progress.json              writing/progress state
premise.md                      premise
outline.json / outline.md       flat outline
layered_outline.json / .md      layered outline
meta/compass.json               long-form story compass
characters.json / .md           core character profiles
meta/cast_ledger.json           supporting cast ledger
world_rules.json / .md          world rules
timeline.jsonl                  timeline fact log
timeline.json / timeline.md     timeline projections
foreshadow_ledger.json / .md    foreshadow ledger
relationship_state.json / .md   relationship state
meta/state_changes.jsonl        state-change facts
chapters/*.md                   editable committed chapter workspace
drafts/*.plan.json              chapter plans
drafts/*.draft.md               working drafts
meta/chapter_records/*.json     accepted chapter baseline + structured facts
summaries/...                   chapter/arc/volume summaries
meta/runtime/...                runtime queue/task logs
meta/checkpoints...             recovery facts
```

This list is a product-facing catalog, not permission for the GUI to bypass Store. Physical paths are implementation/provenance metadata; AppRuntime remains the read/write boundary.

Human-readable `.md` sidecars produced from structured stores are projections unless the existing Store explicitly treats the Markdown itself as the working artifact (for example chapter workspace text and premise). G03 must preserve each existing Store method's source-of-truth semantics instead of applying one blanket file rule.

## 4. Project workspace contract

### 4.1 Project overview

The Project workspace composes:

- `BookStore.Load()` for title/synopsis;
- `ProgressStore` for phase/current/completed/word counts;
- `OutlineStore` for structure;
- Store consistency/foundation readiness where relevant.

It does not persist a new `project.json` summary.

### 4.2 Open/create/change project

Project selection and project mutation are commands through `AppRuntime.Dispatch`; they are never raw filesystem operations initiated by the frontend.

Folder View may expose project-relative path, type, size, modified metadata and bounded preview for supported artifacts. It is not an unrestricted file-manager write escape hatch.

## 5. Chapter contract

The chapter desktop model is a composition, not a single-file assumption:

```text
Chapter row/detail
├── outline entry / V-A location
├── progress state
├── chapter plan (optional)
├── draft text (optional)
├── chapters/*.md workspace text (optional)
├── ChapterRecord baseline/facts/revision (optional)
├── summary (optional)
└── review/rewrite state (optional)
```

Key rules:

1. `chapters/*.md` is the editable committed workspace used by the existing revision flow.
2. `meta/chapter_records/*.json` is the accepted baseline + structured facts used to detect and reconcile external/user revisions; it must not be replaced by a GUI-only revision model.
3. Missing optional plan/draft/summary/record is represented explicitly; corrupted/unreadable data is an error, not silently converted to "missing".
4. Chapter list/detail queries must be bounded; full-book prose is never stuffed into `DesktopSnapshot`.
5. Chapter writes/accept/revision actions later in G03 use `Dispatch` and existing Store/Host semantics.

## 6. Arc / Volume contract

`OutlineStore` remains authoritative.

For non-layered projects:

- `outline.json` is the structured outline authority.

For layered projects:

- `layered_outline.json` is the structural authority;
- `outline.json` / `outline.md` are synchronized flattened projections;
- `VolumeOutline` / `ArcOutline` define the hierarchy;
- global chapter numbers are obtained through the existing flatten/location logic.

The desktop MUST NOT independently renumber chapters or maintain a separate tree.

`EstimatedChapterCapacity` is an internal planning/context capacity estimate and MUST NOT be presented as a fixed final chapter count to the user.

## 7. Documents contract

There is currently no generic document persistence domain. G03 therefore freezes **Document Catalog as a view**, not a new store.

A document entry is a serialization-safe desktop DTO with at minimum:

- stable logical ID;
- supported artifact kind;
- display name;
- project-relative path when useful;
- format (`json`, `jsonl`, `markdown`, etc.);
- editable/read-only capability;
- size/modified metadata when available;
- optional bounded preview.

The catalog is limited to supported project artifacts. Arbitrary absolute-path browsing/writing is outside this seam.

Editing a supported document later must resolve to a typed domain command/store operation; generic `writeFile(path, bytes)` is forbidden at the desktop boundary.

## 8. Context contract

There is no canonical `ContextStore`, and none should be added for G03.

Context is a **task-scoped read model** assembled from real authorities such as:

- book + premise;
- current outline / Volume / Arc;
- progress;
- chapter summaries and bounded chapter text;
- characters + cast;
- world rules;
- recent timeline/state/relationship/foreshadow facts;
- story compass;
- existing memory/context policy.

Every context section should carry enough source/provenance identity for the desktop to explain where a fact came from. The browser conversation is never context authority.

Context queries are bounded by chapter/range/window and must not mutate any source.

## 9. Canon contract

There is no canonical `CanonStore`. Canon is a **semantic projection over persisted story facts**.

The initial desktop Canon view may classify items from:

- core character profiles;
- world rules/boundaries;
- accepted chapter structured facts;
- timeline events;
- state changes;
- relationships;
- foreshadow ledger;
- supporting cast where relevant.

Each projected canon item must retain provenance (`source kind`, logical key/path/chapter as applicable) so a future edit can be routed to the correct domain operation.

A Canon query MUST NOT copy these facts into a second persistent `canon.json`. A future canon mutation MUST target the owning Store/domain contract through `Dispatch` and must respect historical/revision semantics.

## 10. Characters contract

The Characters workspace composes two intentionally distinct domains:

1. **Core / designed characters** — `characters.json` via `CharacterStore`, using `domain.Character` and arc-boundary `CharacterSnapshot` records.
2. **Supporting cast ledger** — `meta/cast_ledger.json` via `CastStore`, using `domain.CastEntry` generated from committed appearances.

Same-name precedence remains the current core rule: core Character authority wins and supporting cast does not duplicate it.

The GUI may present a unified visual list, but the DTO must preserve origin (`core` / `cast`) so updates never write to the wrong store.

## 11. World contract

`WorldStore` remains the owner of world/story continuity facts, including:

- world rules;
- timeline;
- foreshadow ledger;
- relationship state;
- state changes;
- other existing WorldStore projections such as style/review/handoff data where exposed later.

G03 may split these into separate tabs/views, but that is presentation only. It must not split persistence into competing desktop stores.

## 12. Timeline contract

Timeline is a first-class desktop view but remains a WorldStore subdomain.

`domain.TimelineEvent` contains chapter, story-time label, event, and optional characters. Timeline append/update behavior continues through `WorldStore` so its existing stable-key deduplication and projection recovery are preserved.

Desktop timeline queries support bounded chapter/range/window reads. Timeline writes later use typed commands; direct JSONL append from the GUI is forbidden.

## 13. AppRuntime query catalog freeze

G02 left `Query()` as the read-plane extension point. G03 will implement project/knowledge reads through stable query kinds. The initial logical catalog is frozen as:

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

Implementation may add optional filters/ranges/pagination, but these semantic query identities must not be replaced by UI-specific method proliferation.

Rules:

1. All requests/results are AppRuntime DTOs; no `domain.*` or `store.*` object crosses to the frontend.
2. Queries are read-only.
3. Missing optional artifact and read/corruption failure are distinct outcomes.
4. Large text is bounded or fetched by detail/range query.
5. Query DTOs remain compatible with `ainovel.desktop.v1` additive serialization rules.
6. Project-relative paths may be returned only as metadata/provenance; they never confer direct write permission.

## 14. Mutation boundary freeze

G03.1 does not implement mutations. It freezes these rules for later G03 steps:

- all writes enter through `AppRuntime.Dispatch`;
- commands map to existing Store/Host operations whenever such operations exist;
- no generic arbitrary filesystem-write command;
- no GUI-owned transaction/database;
- cross-domain changes use Store coordination methods or an additive core coordination method, never frontend sequencing that assumes multi-file atomicity;
- command acceptance/completion and structured AppError rules from G02 remain in force.

## 15. Concurrency / consistency

1. Read queries may execute concurrently when their underlying Store reads permit it.
2. Store remains responsible for its locks and atomic single-file writes.
3. Existing cross-domain Store operations retain their coordination locks.
4. G03 does not implement G05 multi-run resource locking early.
5. A future project/knowledge write command must identify its logical resource so G05 can serialize conflicting writes without redesigning G03 DTOs.
6. The GUI must refresh from Query/Snapshot/events after mutation; optimistic UI state never becomes story authority.

## 16. Error and compatibility rules

- Existing optional-file semantics (`not exists` => empty/nil where the Store already defines that behavior) are preserved.
- Parse, permission, validation and consistency errors must propagate through AppRuntime's structured error boundary.
- G03 must not turn corrupt/unreadable data into an empty successful view.
- Existing CLI/TUI may continue to use current Host/Store paths; G03 is additive for desktop.
- `CurrentProjectFormatVersion` stays at `2` until a later implementation step demonstrably needs a persistence change.
- WEB-only Gemini execution and no API fallback remain unchanged.

## 17. KEEP / EXTEND / WRAP / ADD summary

### KEEP

- `store.Store` and sub-stores as canonical persistence;
- current project layout/version;
- `DraftStore` + `ChapterRecordStore` revision model;
- flat/layered `OutlineStore` semantics;
- `CharacterStore`, `CastStore`, `WorldStore`;
- current timeline/state/foreshadow/relationship facts;
- existing JSON/JSONL + Markdown projection rules.

### EXTEND

- AppRuntime DTOs for bounded project/knowledge read models;
- provenance/capability metadata for desktop presentation;
- later typed project/knowledge commands where existing core mutation seams require desktop exposure.

### WRAP

- all Store reads behind `AppRuntime.Query`;
- all future Store mutations behind `AppRuntime.Dispatch`;
- Folder View and Documents as safe project-relative metadata/preview projections.

### ADD

- Document Catalog DTO/view;
- composed Context DTO/view;
- composed Canon DTO/view;
- typed query payload/result DTOs for the frozen query catalog.

### DO NOT ADD

- second project database;
- `ContextStore`;
- `CanonStore`;
- generic `DocumentStore` solely to mirror existing files;
- frontend filesystem writer;
- duplicate Arc/Volume/chapter/timeline authorities.

## 18. G03.1 acceptance criteria

G03.1 passes when:

1. the nine product domains are mapped to existing authorities;
2. duplicate persistence is explicitly prohibited;
3. AppRuntime query/mutation boundary is frozen;
4. source-of-truth ambiguity for chapter/outline/character/timeline is resolved;
5. Documents/Context/Canon are classified as views rather than duplicate stores;
6. project format remains unchanged;
7. G02 serialization/error/WEB-only boundaries remain intact;
8. branch regression CI remains green.

Only after these criteria pass may G03 proceed to implementation of the project/knowledge read plane.
