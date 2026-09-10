# AINOVEL Desktop — G03.6 Project / Knowledge Mutation Contract v1

Status: `G03.6 — PASS / CONTRACT LOCKED`

Forward authority: `docs/desktop-g03-roadmap-successor-authority-v1.md`.

Implementation candidate validated before this authority:

- exact head: `a0fac1d62675f3243d2915f501c0b0f9e199f153`
- CI #415: PASS on Linux/Windows plus WEB-only / NO-API audit

This authority freezes the typed Project / Knowledge mutation contract for the desktop bridge. It does not enable the new mutation handlers; those remain G03.7 work.

## 1. Boundary decision

All supported desktop story/project writes enter through:

```text
Desktop UI
  -> AppRuntime.Dispatch(CommandRequest)
  -> typed G03 command contract
  -> G03.7 narrow Host / Store coordination seam
  -> existing canonical Store/domain owner
```

The frontend never receives Store/domain implementation types and never chooses a filesystem path as write authority.

`Runtime.Dispatch` remains lifecycle-only in G03.6. The new G03 mutation contract is deliberately staged and validation-testable but not executable until G03.7.

## 2. Mutation seam audit

The following existing owning seams are safe enough to expose behind typed AppRuntime commands in G03.7, subject to the guards frozen here.

| Command kind | Canonical owner / intended G03.7 seam | Canonical resource | G03.6 decision |
| --- | --- | --- | --- |
| `project.metadata.update` | `BookStore.Save` | `project:metadata` | ALLOW CONTRACT |
| `project.premise.update` | `OutlineStore.SavePremise` | `project:premise` | ALLOW CONTRACT |
| `chapter.plan.save` | `DraftStore.SaveChapterPlan` | `chapter:%06d:plan` | ALLOW CONTRACT |
| `chapter.draft.save` | `DraftStore.SaveDraft` | `chapter:%06d:draft` | ALLOW CONTRACT |
| `chapter.workspace.save` | guarded `DraftStore.SaveFinalChapter` working-artifact seam | `chapter:%06d:workspace` | ALLOW CONTRACT WITH REVISION GUARD |
| `outline.tail.revise` | `Store.ReviseOutline` | `outline:structure` | ALLOW CONTRACT |
| `outline.arc.expand` | `Store.ExpandArc` | `outline:structure` | ALLOW CONTRACT |
| `outline.volume.append` | `Store.AppendVolume` | `outline:structure` | ALLOW CONTRACT |
| `outline.compass.update` | `OutlineStore.SaveCompass` | `outline:compass` | ALLOW CONTRACT |
| `knowledge.characters.core.replace` | `CharacterStore.Save` | `knowledge:characters:core` | ALLOW CORE ONLY |
| `knowledge.world.rules.replace` | `WorldStore.SaveWorldRules` | `knowledge:world:rules` | ALLOW CONTRACT |
| `knowledge.timeline.append` | `WorldStore.AppendTimelineEvents` | `knowledge:timeline` | ALLOW APPEND ONLY |
| `knowledge.relationships.update` | `WorldStore.UpdateRelationships` | `knowledge:relationships` | ALLOW CONTRACT |
| `knowledge.foreshadow.update` | `WorldStore.UpdateForeshadow` | `knowledge:foreshadow` | ALLOW CONTRACT |

The order returned by `G03MutationCommandKinds()` is stable and forms the G03.6 desktop mutation catalog.

## 3. Explicitly unsupported or deferred mutations

G03.6 does not expose the following:

- project create/open/switch: the current AppRuntime/Host instance is bound to one project root and no safe desktop project-switch seam is frozen yet;
- generic Document mutation: Documents remain a catalog/view, never `writeFile(path, bytes)`;
- Context mutation/persistence: Context remains computed read state;
- Canon mutation/persistence: Canon remains a semantic projection; edits must route to the owning fact domain rather than a `CanonStore`;
- supporting-cast direct replacement: Cast remains a derived/supporting-cast authority distinct from core Characters;
- direct state-change replacement/append from the GUI: state changes remain continuity facts owned by the existing story workflow;
- direct ChapterRecord save/accept from desktop text editing;
- AI-assisted revision sync through `revision.Service.Sync`: it invokes a ChatModel and therefore belongs outside G03's non-AI write plane;
- arbitrary absolute/relative filesystem path mutation;
- recursive folder editing;
- direct writes to projections such as `*.md` sidecars when a structured Store owns the truth;
- G05 run/resource locking or scheduling.

Unsupported wishes fail closed with a typed unsupported-operation error. They must not be emulated through raw filesystem access.

## 4. Chapter artifact semantics

Chapter plan, draft, workspace text and accepted ChapterRecord remain distinct facts.

### Plan

`chapter.plan.save` targets only the typed chapter plan artifact and does not mark a chapter accepted or completed.

### Draft

`chapter.draft.save` targets only the working draft. It does not promote content to `chapters/*.md` or ChapterRecord.

### Workspace

`chapter.workspace.save` targets the committed editable workspace text at `chapters/{chapter}.md`, but **workspace save is not acceptance**.

G03.7 must enforce stale/revision guards before writing. `ChapterTextSavePayload` provides:

- `expected_sha256` for compare-before-write against the current target artifact when supplied;
- `expected_record_revision` for accepted-record awareness when supplied.

For a chapter that already has an accepted ChapterRecord, changing workspace text must leave that accepted record unchanged. The subsequent revision workflow is responsible for reconciling the divergence. G03.7 must not silently call `ChapterRecordStore.Accept`, rewrite accepted facts, or invoke AI revision analysis as part of a workspace save.

## 5. Outline semantics

The desktop does not receive generic JSON-patch or arbitrary insert/delete operations.

`outline.tail.revise` maps to `Store.ReviseOutline`, which already protects completed/in-progress history. The payload supplies a future replacement tail and the core owns chapter renumbering.

`outline.arc.expand` maps to `Store.ExpandArc`, preserving layered-outline + Progress coordination and retry behavior.

`outline.volume.append` maps to `Store.AppendVolume`, preserving layered-outline + Progress coordination and the existing idempotent-retry behavior for an already-written identical last volume.

`outline.compass.update` maps only to `OutlineStore.SaveCompass` and cannot mutate the layered outline implicitly.

All structural outline commands share `outline:structure` so later orchestration may serialize conflicting structural writes without changing this contract.

## 6. Knowledge ownership semantics

### Characters

`knowledge.characters.core.replace` owns only core/designed characters through `CharacterStore`. It must never write the Cast ledger. Core/cast origin and core precedence remain intact.

### World rules

`knowledge.world.rules.replace` replaces the typed world-rule authority through `WorldStore`, including its structured JSON and human-readable projection semantics.

### Timeline

`knowledge.timeline.append` is append-only at the desktop contract. G03.7 must call the existing stable-key deduplicating append seam rather than replacing the timeline or appending raw JSONL itself.

### Relationships

`knowledge.relationships.update` performs typed relationship updates through `WorldStore`; the GUI does not own the projection files.

### Foreshadow

`knowledge.foreshadow.update` supports only existing domain actions `plant`, `advance`, and `resolve`. `plant` requires a description. The owning WorldStore retains semantic validation and ledger/projection persistence.

## 7. DTO and serialization contract

All G03.6 payload/result structures are AppRuntime DTOs. No `domain.*`, `store.*`, Host pointer, filesystem handle, raw Go error or browser/provider object crosses the bridge.

The new DTOs are additive under `ainovel.desktop.v1`; `SchemaVersion` remains `1`.

`CommandResult` gains two additive optional fields:

```text
resource  canonical logical resource actually addressed
data      typed command-result JSON payload
```

Existing lifecycle fields and behavior remain compatible.

Mutation payload size is bounded by `MaxMutationPayloadBytes = 4 MiB` before typed decoding.

## 8. Strict JSON rules

Every G03 mutation payload:

1. is required and must be valid JSON;
2. must be no larger than the fixed mutation payload bound;
3. is decoded into the exact DTO for its `CommandKind`;
4. rejects unknown fields using `DisallowUnknownFields`;
5. rejects trailing JSON values;
6. validates required strings, positive chapter/volume/arc numbers, supported actions and basic collection invariants;
7. never accepts a filesystem path field.

The validation boundary must not echo raw client payloads or private paths into `AppError.Message`.

## 9. Canonical resource identity

`CommandRequest.Resource` is not client-defined authority.

The core derives the canonical resource from the command kind and typed payload. A client may omit `resource`. If supplied, it must exactly equal the derived value or the command is invalid.

Canonical identities are:

```text
project:metadata
project:premise
chapter:%06d:plan
chapter:%06d:draft
chapter:%06d:workspace
outline:structure
outline:compass
knowledge:characters:core
knowledge:world:rules
knowledge:timeline
knowledge:relationships
knowledge:foreshadow
```

These are logical conflict identities, not filesystem paths. G03.6 does not implement locks; it merely freezes identities so later G05 locking can reuse them without redesigning the desktop API.

## 10. Error and conflict contract

G03.6 adds stable sanitized error distinctions:

| Error code | Category | Meaning |
| --- | --- | --- |
| `invalid_argument` | `validation` | malformed JSON, wrong DTO, unknown field, invalid value or spoofed resource |
| `unsupported_operation` | `validation` | mutation is not in the locked G03 catalog |
| `target_not_found` | `conflict` | a typed target expected by an allowed mutation does not exist |
| `precondition_conflict` | `conflict` | current project state does not permit the mutation |
| `stale_conflict` | `conflict` | compare/revision guard shows data changed after the client read it |
| `store_read` | `store` | canonical project facts could not be read |
| `store_write` | `store` | canonical project facts could not be saved |

The new errors preserve Go cause chains internally while only sanitized messages cross JSON. `target_not_found`, `precondition_conflict` and `stale_conflict` are deliberately distinct so desktop UX can refresh, explain and retry safely without parsing prose errors.

## 11. G03.6 non-execution guarantee

The existence of `mutations_contract.go` does not make the commands executable.

G03.6 does **not** route the new kinds from `Runtime.Dispatch` to Store/Host. Existing lifecycle dispatch remains unchanged. Any actual handler wiring, Store conversion, compare-before-write logic and post-write typed result belongs exclusively to G03.7.

This prevents a partially tested write surface from becoming reachable before the contract is locked.

## 12. G03.6 tests

Contract tests lock:

- stable ordered command catalog;
- typed decoding for every allowed command;
- deterministic canonical resource derivation;
- rejection of caller resource spoofing;
- unknown-field/path injection rejection;
- malformed and semantically unsafe value rejection;
- explicit unsupported-operation mapping;
- distinct target/precondition/stale error codes;
- additive `CommandResult.resource` + typed `data` JSON round-trip.

Repository regression on implementation head `a0fac1d62675f3243d2915f501c0b0f9e199f153` passed CI #415 before this authority was committed.

## 13. G03.6 acceptance

G03.6 is locked only if the authority commit itself also passes exact-head CI on Linux and Windows plus WEB-only / NO-API audit.

After that exact-head validation:

- mark G03.6 `PASS / MUTATION CONTRACT LOCKED` in PR #19;
- open only G03.7;
- keep PR #19 Draft / NOT MERGED;
- keep G03.8 and G04 closed.
