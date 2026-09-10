# AINOVEL Desktop — G04.7 Creative Editor + Safe Write-Plane UI v1

Status: `G04.7 — IMPLEMENTATION CANDIDATE / EXACT-HEAD VALIDATION REQUIRED`

Parent authority: `docs/desktop-g04-creative-studio-authority-v1.md`

Accepted parent: `d5e3877db3dec9e9b635c2b7bbef5748feed0564` (`G04.6 — PASS / LOCKED`).

## 1. Decision

G04.7 activates the renderer-neutral `Sáng tác` workspace and the safe UI write plane already authorized by G03. It does not add a new AppRuntime command, Store seam, project format, AI transport, scheduler, resource lock, or browser lane.

All canonical writes still follow:

```text
Creative Studio renderer
  -> typed desktopui controller method
  -> RuntimeClient.Dispatch
  -> AppRuntime.Dispatch
  -> existing G03 typed mutation handler
  -> existing canonical Store/domain owner
```

There is intentionally no generic renderer-facing `Dispatch(kind, payload)` escape hatch.

## 2. Creative chapter artifact model

The editor keeps four visibly distinct surfaces:

1. **Plan** — typed `ChapterPlanSavePayload`; save targets only `chapter.plan.save`.
2. **Draft** — editable working draft; save targets only `chapter.draft.save`.
3. **Workspace** — committed editable chapter workspace; save targets only `chapter.workspace.save`.
4. **Accepted record** — read-only `ChapterRecordViewDTO` metadata projected by `chapters.get`; G04.7 has no accept/promote command.

`chapters.get` is called with `include_content=true` only inside the Creative editor. The Project workspace keeps its G04.4 metadata-only read.

Workspace save is never presented as acceptance. An existing accepted record remains immutable through the G04.7 UI because the frozen G03 mutation catalog contains no direct ChapterRecord save/accept command.

## 3. Existing-plan round-trip safety

The locked `ChapterPlanViewDTO` read surface does not expose every field carried by `ChapterContractMutationDTO` (`evaluation_focus`, `emotion_target`, `payoff_points`, `hook_goal` are write-only in the current G03 contract).

G04.7 therefore fails closed for an existing plan: a plan loaded from `chapters.get` is not silently considered a complete replacement. A renderer must explicitly provide a complete typed replacement before `SaveCreativePlan` can dispatch. This prevents an edit to visible fields from accidentally erasing contract fields that the read DTO cannot round-trip.

No additive AppRuntime contract is introduced because the user-authorized G04.7 boundary requires using only the locked G03 Query/Mutation contracts.

## 4. Draft/workspace optimistic guards

`chapter.draft.save` and `chapter.workspace.save` preserve the G03 guards exactly:

- `expected_record_revision` is populated from the current `ChapterRecordViewDTO.Revision` when an accepted record exists;
- `expected_sha256` is preserved after a successful typed text-save result provides a canonical target hash;
- G04.7 does not invent a target SHA from unnormalized frontend text when the read DTO does not expose the target artifact hash.

On `stale_conflict`, `precondition_conflict`, or `target_not_found`, local dirty editor text remains intact while the controller refreshes the AppRuntime snapshot and re-reads the chapter projection. Canonical data and unsaved local text therefore remain separately visible rather than one silently overwriting the other.

## 5. Frozen typed write catalog

G04.7 exposes typed controller bindings for exactly the G03 catalog:

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

Each public binding takes its exact AppRuntime payload DTO. The common executor is private package implementation only.

The UI omits `CommandRequest.Resource`; AppRuntime derives and validates the canonical logical resource. The UI also leaves `RunID`, `TaskID`, and all G05 scheduling/resource ownership empty.

## 6. Write command reconciliation

A safe write iteration is:

```text
validate UI preconditions
  -> create stable desktop-write-N command id
  -> Dispatch exact typed G03 command
  -> validate CommandResult contract/id/accepted/resource/data
  -> fresh AppRuntime Snapshot
  -> fresh owning Query when that workspace is visible
  -> render authoritative canonical projection
```

A Dispatch failure follows the same fresh reconciliation path before the structured error state is rendered.

The controller does not infer canonical success from button click or from a local dirty flag. `Accepted=true` means the AppRuntime mutation command completed; the subsequent read remains the UI's canonical projection.

## 7. Structured UI states

Write errors are mapped without parsing prose:

- `validation` -> `validation_error`;
- `conflict` -> `conflict`;
- Store/runtime/internal failure -> `runtime_error`.

The existing safe `AppError` messages remain the only error prose crossing the bridge.

A single transient write-pending state prevents duplicate renderer submissions. This is a local interaction guard, not G05 resource locking or multi-run scheduling.

## 8. Runtime consistency guard

The UI derives write availability from the authoritative lifecycle snapshot and disables write presentation while lifecycle state is starting/running/pausing/resuming/stopping/cancelling/recovering. AppRuntime remains authoritative and re-validates mutation legality on Dispatch.

This mirrors the existing G03 consistency rule and does not create a second lifecycle state machine.

## 9. Navigation effect

G04.7 enables `Sáng tác` in the preserved G01 navigation. The enabled G04 routes are now:

```text
Tổng quan
Dự án
Sáng tác
Tri thức
```

Review, Run Center and Settings remain disabled successor placeholders. G05 remains closed.

## 10. Explicitly unsupported

G04.7 does not add or emulate:

- ChapterRecord accept/promote/rewrite;
- Context or Canon mutation;
- generic Document/file write;
- supporting-cast replacement;
- direct state-change ledger mutation;
- project create/open/switch;
- raw Host/Store/WebAI/Engine/filesystem access;
- frontend canonical database;
- project-format migration;
- direct AI API/provider fallback;
- G05 scheduler, priority, resource locks, browser-lane pool or multi-run ownership.

## 11. Candidate test authority

The G04.7 tests must prove at minimum:

- Creative route is enabled while successor routes remain closed;
- `chapters.get(include_content=true)` is used by the Creative editor;
- plan/draft/workspace/accepted metadata remain distinct;
- an existing incomplete plan projection cannot be silently replace-saved;
- draft/workspace saves preserve accepted-record revision awareness;
- all 14 frozen mutation kinds have typed bindings and no extra kind is reachable through a generic renderer surface;
- `Resource`, `RunID`, and `TaskID` remain unset by the UI;
- stale conflict keeps local dirty text while fresh canonical data is re-read;
- active runtime write presentation rejects before Dispatch;
- malformed/mismatched CommandResult fails closed;
- desktopui production imports remain AppRuntime-only.

## 12. Exact-head acceptance

This candidate becomes `G04.7 — PASS / LOCKED` only if the same exact SHA passes:

1. CI including Linux regression/critical race and Windows regression;
2. WEB-only / NO-API audit;
3. W6A Release Packaging Gate;
4. W6B Install Update Integrity Gate;
5. W6C Release Artifact Desktop Smoke, including real packaged Windows WEB-only smoke when triggered;
6. final branch-head recheck with no drift;
7. no blocking review thread;
8. parent-to-candidate diff remains wholly inside the documented G04.7 desktopui/test/authority boundary.

Only after all eight conditions pass may G04.7 be locked and G04.8 opened. G05 remains CLOSED.
