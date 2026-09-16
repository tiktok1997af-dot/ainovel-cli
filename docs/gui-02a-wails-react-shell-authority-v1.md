# GUI-02A — Wails/React Desktop Shell Foundation v0.1

Status: OPEN / AUTHORIZED / IMPLEMENTATION IN PROGRESS / NOT LOCKED

Exact base: `main@43e8de4d7dd11ee841bcf2c82a80373786a6687c`.

## Objective

Build the real AINOVEL Desktop executable with Wails + React. The AUTHOR-provided AINOVEL Desktop screenshot is a visual/product target only; it is not runtime evidence.

GUI-02A establishes the runnable desktop shell only. Backend/domain/runtime behavior already locked in v0.1.4 remains frozen.

## Required shell surfaces

- left navigation and project-area shell;
- project header / top bar;
- central workspace container;
- right contextual panel container;
- lower AI Bridge / Run Control / workflow-action region;
- S01–S10 navigation manifest;
- stable desktop resizing, scrolling, focus, and loading/empty/error shell states.

## Architecture boundaries

- React remains the frontend technology;
- Wails replaces the legacy desktop shell/integration layer;
- frontend reaches product authority only through explicit adapters/view-models and later GUI gateway work;
- no direct frontend filesystem, Host, Store, Engine, WebAI, browser-session, or canonical-state ownership;
- no backend refactor merely for UI convenience;
- no Tauri dependency in the new shell;
- GUI-02B Gateway remains CLOSED;
- GUI-02C Project lifecycle remains CLOSED;
- S01–S10 business-function implementation remains staged behind their authorized GUI milestones.

## Acceptance

GUI-02A is not PASS until:

1. Wails/React foundation builds successfully;
2. the actual desktop executable launches;
3. shell navigation/layout/resizing/scrolling are exercised in the running executable;
4. no prohibited direct-authority or Tauri dependency is present in the new shell;
5. evidence includes screenshots captured from the actual running executable;
6. Fresh Audit finds no blocker;
7. AUTHOR explicitly issues `CHỐT GUI-02A`.

Generated, mock, illustrative, or AI-created screenshots are not acceptable runtime evidence.

## Successor gate

Only after GUI-02A is PASS / AUTHOR-CONFIRMED / LOCKED may GUI-02B — Query/Command/Event Gateway Foundation be opened.
