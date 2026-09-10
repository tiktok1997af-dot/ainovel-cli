# AINOVEL Desktop — G03.7 Store-backed Project / Knowledge Mutation Handlers v1

Status: `G03.7 — PASS / STORE-BACKED MUTATION HANDLERS LOCKED`

Forward authority: `docs/desktop-g03-roadmap-successor-authority-v1.md`.

Mutation-contract authority: `docs/desktop-g03-mutation-contract-v1.md`.

Implementation candidate validated before this authority:

- exact implementation head: `0ee011a4be3c3db535c504925cc6eb4a3ca290a5`
- CI #424: PASS on Linux/Windows plus WEB-only / NO-API audit

This authority freezes the executable Store-backed implementation of the fourteen mutation commands locked by G03.6. It does not add a fifteenth mutation, a generic write surface, a new persistence authority, browser/AI execution, or G05 resource locking.

## 1. Executable Dispatch boundary

`AppRuntime.Dispatch` now preserves two explicitly separate command planes:

```text
lifecycle kind
  -> existing G02 dispatchLifecycle

locked G03 mutation kind
  -> strict G03.6 decodeMutationContract
  -> canonical resource derivation
  -> G03.7 dispatchMutation
  -> narrow Host mutation seam
  -> existing canonical Store/domain owner
```

The six G02 lifecycle commands remain `start`, `pause`, `resume`, `stop`, `cancel`, and `retry`. All non-lifecycle commands enter the typed mutation decoder; commands outside the G03.6 catalog fail closed as `unsupported_operation`.

The desktop bridge still exposes no `Host`, Store pointer, sub-store pointer, raw filesystem handle, raw domain object or generic write callback.

## 2. Locked executable catalog

G03.7 implements exactly the fourteen commands frozen by G03.6:

1. `project.metadata.update`
2. `project.premise.update`
3. `chapter.plan.save`
4. `chapter.draft.save`
5. `chapter.workspace.save`
6. `outline.tail.revise`
7. `outline.arc.expand`
8. `outline.volume.append`
9. `outline.compass.update`
10. `knowledge.characters.core.replace`
11. `knowledge.world.rules.replace`
12. `knowledge.timeline.append`
13. `knowledge.relationships.update`
14. `knowledge.foreshadow.update`

No G03.7 handler exists outside this list.

## 3. Canonical Host / Store ownership

The AppRuntime layer converts serialization-safe DTOs into detached domain values. The new Host seams are intentionally narrow and map directly to the already-existing canonical Store authorities:

| Command | Host seam | Canonical owner |
| --- | --- | --- |
| `project.metadata.update` | `DesktopSaveBookMetadata` | `BookStore.Save` |
| `project.premise.update` | `DesktopSavePremise` | `OutlineStore.SavePremise` |
| `chapter.plan.save` | `DesktopSaveChapterPlan` | `DraftStore.SaveChapterPlan` |
| `chapter.draft.save` | `DesktopSaveChapterDraft` | `DraftStore.SaveDraft` |
| `chapter.workspace.save` | `DesktopSaveChapterWorkspace` | `DraftStore.SaveFinalChapter` workspace artifact only |
| `outline.tail.revise` | `DesktopReviseOutline` | `Store.ReviseOutline` |
| `outline.arc.expand` | `DesktopExpandArc` | `Store.ExpandArc` |
| `outline.volume.append` | `DesktopAppendVolume` | `Store.AppendVolume` |
| `outline.compass.update` | `DesktopSaveCompass` | `OutlineStore.SaveCompass` |
| `knowledge.characters.core.replace` | `DesktopReplaceCoreCharacters` | `CharacterStore.Save` |
| `knowledge.world.rules.replace` | `DesktopReplaceWorldRules` | `WorldStore.SaveWorldRules` |
| `knowledge.timeline.append` | `DesktopAppendTimelineEvents` | `WorldStore.AppendTimelineEvents` |
| `knowledge.relationships.update` | `DesktopUpdateRelationships` | `WorldStore.UpdateRelationships` |
| `knowledge.foreshadow.update` | `DesktopUpdateForeshadow` | `WorldStore.UpdateForeshadow` |

The Store remains the sole project/story source of truth. G03.7 adds no parallel project, chapter, outline, character, timeline, Context, Canon or Document database.

## 4. Mutation execution state guard

Story/project mutations fail closed while the physical engine is active or AppRuntime is in a transitional/running lifecycle state.

The guard rejects mutation during:

- `starting`
- `running`
- `pausing`
- `resuming`
- `stopping`
- `cancelling`
- `recovering`
- any state where `DesktopEngineRunning()` is still true

This prevents desktop writes racing the existing writer/editor engine. It is a G03 consistency guard, not G05 resource scheduling/locking.

Paused/stopped/failed/completed are not globally converted into a universal write ban: the owning Store/domain preconditions remain authoritative for each individual command. For example, `Store.ReviseOutline` still rejects outline changes against completed or in-progress history and rejects modification of a completed book.

## 5. Chapter artifact and optimistic-conflict semantics

Plan, draft, workspace text and accepted `ChapterRecord` remain separate authorities.

### Plan

`chapter.plan.save` writes only `DraftStore.SaveChapterPlan`. It does not start, complete, commit or accept a chapter.

### Draft

`chapter.draft.save` writes only the chapter working draft.

When `expected_sha256` is present, AppRuntime first reads the current draft and compares its normalized `domain.ChapterContentSHA256`:

- missing current target -> `target_not_found`
- hash mismatch -> `stale_conflict`
- exact match -> write may proceed

### Workspace

`chapter.workspace.save` writes only the editable `chapters/{chapter}.md` workspace artifact. It never calls `ChapterRecordStore.Accept` or `Save`.

The same `expected_sha256` compare-before-write rule applies to the current workspace text.

When `expected_record_revision > 0` is supplied:

- no accepted record -> `target_not_found`
- revision mismatch -> `stale_conflict`
- exact revision -> workspace save may proceed

A successful workspace edit therefore may intentionally diverge from the accepted `ChapterRecord`. The record remains unchanged until the existing revision workflow later reconciles it. G03.7 never invokes AI-assisted revision analysis implicitly.

Returned chapter text results contain only the stable desktop facts `chapter`, `artifact`, normalized `content_sha256`, and `word_count`.

## 6. Outline consistency and replay safety

### Tail revision

`outline.tail.revise` delegates to `Store.ReviseOutline`, retaining the existing cross-domain lock, flat/layered handling, Progress coordination, phase checks and completed/in-progress history protection. The client cannot bypass those checks with JSON Patch, insert/delete commands or direct file writes.

### Arc expansion

Before `Store.ExpandArc`, AppRuntime checks the current layered target:

- missing volume/arc -> `target_not_found`
- unexpanded target -> allowed
- already expanded with exactly the same expansion -> allowed as an idempotent recovery replay
- already expanded with different content -> `precondition_conflict`

The Store then remains responsible for rebuilding layered/Markdown/flat outline projections and updating Progress capacity.

### Volume append

Before `Store.AppendVolume`:

- no current layered authority -> `precondition_conflict`
- exact same last volume -> allowed as recovery replay
- non-increasing volume index -> `precondition_conflict`
- valid later volume -> Store append path

The Store's existing retry semantics and Progress coordination remain authoritative.

### Compass

`outline.compass.update` only saves `StoryCompass`; it does not rewrite structural outline data.

All structural outline mutations retain canonical resource `outline:structure`, while compass remains `outline:compass`.

## 7. Character core / cast ownership

`knowledge.characters.core.replace` writes only `CharacterStore`.

`CastStore` is neither replaced nor merged by this handler. Supporting-cast appearance history remains unchanged, and the read model continues to preserve core/cast origins and core-character precedence.

There is no direct supporting-cast replacement command in G03.7.

## 8. WorldStore consistency semantics

### World rules

`knowledge.world.rules.replace` writes the typed WorldStore rule authority and lets WorldStore maintain its structured and Markdown projections.

### Timeline

`knowledge.timeline.append` performs a canonical read check before append, then calls `WorldStore.AppendTimelineEvents`. It never writes `timeline.jsonl` or `timeline.md` directly.

This preserves WorldStore's stable-key deduplication and recovery-aware projection behavior. Replaying the same timeline event does not duplicate the fact.

A corrupt canonical timeline log fails closed as `store_read`; G03.7 does not append on top of unreadable truth or replace it with an empty model.

### Relationships

`knowledge.relationships.update` first verifies the current relationship authority is readable, then calls `WorldStore.UpdateRelationships`. Corrupt current state therefore fails closed before read-modify-write.

### Foreshadow

`knowledge.foreshadow.update` first reads the current ledger, validates typed semantic targets, then calls `WorldStore.UpdateForeshadow`.

`plant`, `advance`, and `resolve` remain the only actions. A payload may plant then advance/resolve the same ID sequentially in one typed command. An `advance` or `resolve` with no current/prior-in-payload plant returns `target_not_found` rather than relying on Store error prose.

Unreadable ledger state fails closed as `store_read` before mutation.

## 9. Typed result and resource contract

Every successful G03.7 mutation returns:

```text
accepted = true
status = "completed"
resource = canonical G03.6 logical resource
data = command-specific typed result JSON
```

The canonical resource is still derived by core code from kind + typed payload. A caller cannot redirect a handler by supplying a different resource.

Lifecycle result semantics remain unchanged.

## 10. Error mapping

G03.7 preserves the G03.6 desktop error taxonomy:

- malformed/invalid typed command -> `invalid_argument`
- command outside catalog -> `unsupported_operation`
- missing typed target -> `target_not_found`
- Store/domain state conflict -> `precondition_conflict`
- optimistic hash/revision mismatch -> `stale_conflict`
- canonical read failure -> `store_read`
- canonical write failure -> `store_write`

Existing `ErrToolPrecondition`, `ErrToolConflict`, phase-transition and flow-transition failures are translated to the stable mutation precondition sentinel while retaining the internal Go cause chain. Raw Store/path/provider error details remain excluded from the JSON `AppError.Message`.

## 11. Explicit non-goals preserved

G03.7 does not add or expose:

- project create/open/switch;
- generic Document writes;
- generic `writeFile(path, bytes)` or arbitrary filesystem mutation;
- Context persistence or `ContextStore`;
- Canon persistence or `CanonStore`;
- `DocumentStore`;
- direct Cast replacement;
- direct StateChange mutation;
- direct accepted `ChapterRecord` mutation;
- AI-assisted revision sync;
- browser/AI execution from the mutation handlers;
- direct AI API/provider fallback;
- G05 resource locks, queues or schedulers;
- a project format change beyond v2.

## 12. Regression evidence

The implementation candidate `0ee011a4be3c3db535c504925cc6eb4a3ca290a5` passed CI #424 before this authority was added.

The tests cover, among other things:

- lifecycle/mutation plane separation;
- strict G03.6 catalog reuse;
- chapter SHA and accepted-record revision guards;
- accepted-record immutability during workspace save;
- arc and volume target/precondition/replay behavior;
- Store-coordinated outline/Progress updates;
- successful and protected tail revisions;
- CharacterStore writes preserving CastStore data;
- Timeline replay deduplication;
- corrupt Timeline fail-closed behavior;
- Book, premise, plan, draft, compass, world-rule, relationship and foreshadow canonical seams;
- sequential foreshadow plant/advance/resolve validation;
- stable Store/domain-to-AppError conflict mapping;
- DTO normalization and detached conversion.

## 13. G03.7 acceptance and successor gate

This document records the G03.7 implementation contract, but G03.7 is accepted only after the commit containing this authority itself passes exact-head repository CI on Linux and Windows plus WEB-only / NO-API audit.

After that exact-head validation:

- mark G03.7 `PASS / STORE-BACKED MUTATION HANDLERS LOCKED` in PR #19;
- open G03.8 — G03 final regression / authority / merge gate;
- keep PR #19 `Draft / NOT MERGED` until G03.8 itself passes;
- keep G04 closed until G03.8 passes, PR #19 is merged and the resulting `main` head is verified.

Any source or documentation commit added after the exact-head G03.7 gate becomes a new candidate and must be revalidated.