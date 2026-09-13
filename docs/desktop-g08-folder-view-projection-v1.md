# G08.8 — Folder View projection / workspace state v1

Status: IMPLEMENTATION CANDIDATE — NOT AUTHOR-CONFIRMED / NOT LOCKED.

Locked parent authority: G08.7 `PASS / AUTHOR-CONFIRMED / LOCKED` at `4957c0f90dc2781d76fcbd9f51b7d52dc9404337`.

## Scope

G08.8 adds only a read-only Folder View projection over the existing Project workspace `documents.list` state.

The projection:
- consumes only `appruntime.DocumentsListResultDTO` already admitted through the locked AppRuntime query plane;
- creates deterministic logical folder/document nodes from sanitized catalog metadata;
- preserves paging metadata and the existing opaque `SelectedDocumentID` presentation state;
- keeps directories before documents and uses stable case-insensitive labels with deterministic tie-breaking;
- treats document IDs as opaque values and never derives filesystem authority from them;
- accepts logical relative catalog paths only and omits absolute, traversal, empty, mutable, or ID-less entries fail-closed;
- performs no filesystem read, stat, walk, open, mutation, or process/network operation;
- exposes no write/move/rename/delete/create action.

## Existing authority reused

Folder View is derived on demand from `ProjectWorkspaceState.Documents` through `ProjectWorkspaceState.FolderView()`.

No new AppRuntime query or command is added. The existing paths remain authoritative:
- `documents.list` supplies the bounded catalog page;
- `documents.get` reads one supported document by opaque ID;
- `ProjectWorkspaceQueryLimit = 100` remains the desktop page bound.

## Boundary

G08.8 does not add or modify:
- Host, Engine, scheduler/resource ownership, browser lanes, Store/DB, SessionManager, provider/API paths;
- direct WebAI access;
- direct filesystem/network packages in `internal/desktopui`;
- primary shell routes;
- AppRuntime transport contracts or document-catalog semantics;
- CoCreate semantics locked by G08.2–G08.7.

`G08.9+` remain CLOSED. This gate does not authorize PR merge.

## Validation intent

Regression coverage verifies:
- deterministic hierarchy and selection projection;
- fail-closed handling of absolute/traversal/mutable/ID-less catalog entries;
- paging metadata retention;
- integration with the existing `LoadDocumentsPage` / `LoadDocument` read plane;
- zero command dispatch for Folder View projection;
- existing desktop boundary tests continue to reject direct filesystem/network/core dependencies.
