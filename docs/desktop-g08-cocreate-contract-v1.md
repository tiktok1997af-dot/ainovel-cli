# AINOVEL Desktop — G08.2 AppRuntime CoCreate Contract v1

Status: `G08.2 — IMPLEMENTATION CANDIDATE / CONTRACT FROZEN`

Parent authority: PR #32 comment `5653689913` (`G08.1 — FORWARD-ONLY CONTRACT FREEZE`).

Exact baseline: `main@f010ded328d711ba8deeace55762ab4f82d084d0`.

## 1. Mission

G08.2 freezes the serialization-safe AppRuntime CoCreate vocabulary, DTOs and finite transport bounds only.

This step does **not** call Host CoCreate methods, does not allocate a browser lane, does not implement the desktop workspace, and does not implement Folder View.

## 2. Command vocabulary

The exact desktop CoCreate command catalog is:

- `cocreate.turn`
- `cocreate.stage.begin`
- `cocreate.stage.finish`
- `cocreate.stage.cancel`

`cocreate.turn` mode vocabulary is exactly:

- `cold_start`
- `stage`

History roles are exactly:

- `user`
- `assistant`

No command accepts caller-owned `RunID`, `TaskID` or `Resource`. Those fields are rejected rather than interpreted as scheduler, task, story-resource or browser-lane authority.

## 3. Finite transport bounds

G08.2 owns these exact transport limits:

- maximum encoded CoCreate payload: `1 MiB` (`1 << 20` bytes);
- maximum history items: `24`;
- maximum one user/assistant presentation message: `16 KiB`;
- maximum cumulative draft: `32 KiB`;
- maximum suggestions: `3`;
- maximum one suggestion: `2 KiB`.

Bounds are validated before any later Host/WebAI execution.

History must start with `user`, alternate `user/assistant`, and end with a non-empty `user` message for a new model turn. User entries cannot carry assistant-only `draft`, `ready` or `suggestions` fields.

Unknown JSON fields, malformed JSON, trailing JSON values, unknown modes/roles, empty required text, oversized fields/history and authority-spoofing envelope values fail closed as AppRuntime validation errors.

## 4. Sanitized continuity DTO

Desktop history may carry only:

- role;
- visible message;
- cumulative draft for an assistant entry;
- ready state;
- up to three sanitized suggestions.

Later AppRuntime integration may reconstruct Host protocol continuity internally from these typed fields. The GUI is never granted Host raw XML response authority.

Desktop-facing turn result contains only:

- typed session metadata (`mode`, `history_count`, `stage_active`);
- assistant `message`;
- cumulative `draft`;
- `ready`;
- bounded suggestions.

It contains no reasoning/thinking, raw provider payload, browser/profile path, PID/process handle, cookie, credential, token, browser storage, Store/Host pointer or filesystem handle.

## 5. Persistence and authority boundary

CoCreate history in this contract is transient presentation/session transport state. It is not Canon, Context, project memory or story truth.

This step adds no CoCreate database and no event stream.

Existing internal Host diagnostics such as `meta/sessions/cocreate.jsonl` remain internal and are not exposed by this contract.

## 6. Explicitly closed after G08.2

G08.2 does not authorize:

- Host adapter execution;
- G05 managed-work ownership integration;
- cold-start Start/new handoff;
- stage begin/finish/cancel Host wrapping;
- desktop CoCreate workspace/controller;
- Folder View;
- direct WebAI/SessionManager access;
- browser-lane allocation from a CoCreate command;
- second Host/Engine/scheduler/resource lock;
- AI API/provider fallback;
- raw reasoning/provider/browser/session data exposure.

Those remain owned by later G08 child steps.

## 7. Acceptance boundary

The G08.2 delta must remain bounded to AppRuntime CoCreate contract/validation/tests plus this authority document.

Acceptance requires exact-head validation and a no-drift recheck before G08.2 can be marked PASS / AUTHOR-CONFIRMED / LOCKED. G08.3 must not open automatically.
