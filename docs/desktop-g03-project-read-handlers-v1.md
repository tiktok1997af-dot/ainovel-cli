# AINOVEL Desktop — G03.3 Store-backed Project Read Handlers v1

Status: `G03.3 — PASS / HANDLERS LOCKED`

Parent authorities:
- `docs/desktop-g03-project-knowledge-authority-v1.md`
- `docs/desktop-g03-query-contract-v1.md`

Implementation validation evidence: exact implementation/test head `63a8d5fd363b378ff695c882eca748596a4439b6` passed CI run #383, including WEB-only/NO-API audit, Ubuntu format/vet/test/race and Windows format/vet/test. This authority commit is docs-only and is also subject to exact-head CI before G03.3 acceptance.

## 1. Scope

G03.3 converts the first four locked Project / Knowledge queries from staged routing into real canonical reads:

- `project.overview`
- `chapters.list`
- `chapters.get`
- `outline.get`

No mutation command, GUI workspace, database, file format, browser behavior, AI execution path, or persistence layer is introduced by this step. The remaining Documents / Knowledge queries stay explicitly staged until later G03 steps.

## 2. Boundary

The desktop path is:

```text
Desktop UI -> AppRuntime.Query -> typed G03 router
           -> narrow Host desktop read seam
           -> existing canonical Store/sub-stores
           -> AppRuntime serialization-safe DTO
```

`internal/host/desktop_project_read.go` is a read-only seam. It does not return `store.Store`, sub-store pointers, file handles, mutation functions, Host channels, or browser/session implementation objects.

The GUI still never receives Host/domain/store implementation types. AppRuntime projects all returned domain values into the DTOs locked by G03.2.

## 3. Canonical sources

G03.3 reads only existing canonical artifacts through Store methods:

- project format: `meta/format.json` through `Store.LoadProjectFormatVersion`;
- title/synopsis: `meta/book.json` through `BookStore.Load`;
- premise: `premise.md` through `OutlineStore.LoadPremise`;
- progress: `meta/progress.json` through `ProgressStore.Load`;
- flat outline: `outline.json` through `OutlineStore.LoadOutline`;
- layered structure: `layered_outline.json` through `OutlineStore.LoadLayeredOutline`;
- story compass: `meta/compass.json` through `OutlineStore.LoadCompass`;
- chapter plan: `drafts/*.plan.json` through `DraftStore.LoadChapterPlan`;
- working draft: `drafts/*.draft.md` through `DraftStore.LoadDraft`;
- published/final chapter: `chapters/*.md` through `DraftStore.LoadChapterText`;
- accepted revision authority: `meta/chapter_records/*.json` through `ChapterRecordStore.Load`.

No directory scan or duplicate index/database is added.

## 4. Chapter authority semantics

Chapter artifacts remain separate facts. Status precedence is stable:

```text
accepted record > final > draft > plan > not_started
```

The meanings are deliberately distinct:

- `plan` = chapter planning artifact;
- `draft` = current working draft;
- `final` = published/editable `chapters/*.md` artifact;
- `accepted record` = accepted revision baseline with origin/revision/content hash/accepted time.

A final chapter is never synthesized from an accepted record, and an accepted record is never synthesized from a final chapter.

`chapters.get` returns draft/final presence and word counts even when `include_content=false`; text content is included only when explicitly requested.

Word counts use the existing `domain.WordCount` semantics. For list projection, published final text has word-count precedence, then draft, then accepted-record content; persisted per-chapter progress count is a fallback when no readable text artifact supplies a count.

## 5. Chapter catalog and paging

`chapters.list` uses a numeric detailed-chapter catalog derived from canonical facts:

- detailed outline chapter numbers;
- completed chapter facts;
- current in-progress chapter;
- flat-mode `Progress.TotalChapters` when it represents the detailed flat outline count.

In layered mode, `Progress.TotalChapters` is an internal capacity estimate that may include unexpanded skeleton estimates. G03.3 MUST NOT expose that value as a desktop chapter total. Layered chapter totals therefore use detailed/completed/in-progress facts only.

Paging remains bounded by G03.2 `MaxQueryPageSize = 500`. A zero request limit uses a stable G03.3 default page size of 100. Offset past the current catalog returns an empty page rather than inventing chapters.

## 6. Flat / layered outline semantics

When `layered_outline.json` exists, it is the structural authority and AppRuntime derives the detailed flat chapter sequence with `domain.FlattenOutline`; `outline.json` remains the Store-maintained compatibility view.

`outline.get` applies `from_chapter` and `limit` to detailed chapters. In layered mode:

- the selected detailed chapter window is returned as flat `chapters` DTOs;
- volume/arc metadata is preserved;
- chapter arrays inside arcs contain only chapters from the selected window;
- unexpanded arc metadata and `estimated_chapters` remain visible as structure;
- the story compass is projected when present.

Individual skeleton arc estimates are structure metadata; they are not summed or exposed as a fixed total chapter count.

## 7. Error and safety behavior

Store read failures are wrapped with the existing `errs.ErrStoreRead` category at the Host seam. G02.6 AppError normalization therefore emits the stable Store read error shape rather than leaking raw paths/internal errors to desktop JSON.

All four handlers are read-only. Missing optional artifacts retain the existing Store semantics (nil/empty); missing data is not silently manufactured from another artifact.

## 8. Test authority

G03.3 tests lock:

- canonical Store seam reads for project metadata/progress/outline/compass;
- plan/draft/final/accepted-record separation;
- status precedence;
- `include_content` behavior;
- flat-mode detailed total behavior;
- layered-mode protection against exposing estimated capacity as total chapters;
- chapter pagination bounds/default;
- layered chapter-window projection while preserving volume/arc/skeleton metadata;
- the rule that only the remaining Documents/Knowledge query families continue to return staged `ErrNotImplemented`.

G03.3 does not open the mutation plane or GUI implementation.
