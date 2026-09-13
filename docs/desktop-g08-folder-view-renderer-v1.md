# G08.10 — Folder View renderer / responsive / full-width integration v1

Status: IMPLEMENTATION CANDIDATE — NOT AUTHOR-CONFIRMED / NOT LOCKED.

Locked parent authority: G08.9 `PASS / AUTHOR-CONFIRMED / LOCKED` at `bbb7b5b789f2252de4faa5c144cd9df87e60e4be`.

## Scope

G08.10 adds only a deterministic presentation renderer model for the already-locked G08.8/G08.9 Folder View state. It does not create a new UI framework, shell, route, data source, persistence model or filesystem authority.

The renderer:
- activates only on the existing `RouteProject` + `ProjectTabFolder` surface;
- flattens the safe logical Folder View tree into bounded renderer rows while preserving depth, opaque document IDs and selection;
- renders selected content only when the already-loaded `documents.get` DTO still matches the selected safe tree node by opaque ID, canonical logical path, kind and read-only state;
- reuses the existing G01 `LayoutForWidth` authority and its desktop/tablet/mobile viewport classes;
- uses desktop split presentation for tree + selected content;
- uses stacked full-width tree/content presentation on tablet/mobile;
- uses tree full-width presentation when no safe selected document is available;
- subtracts only already-owned visible shell navigation/inspector widths when deriving workspace width;
- performs no query or command dispatch from the renderer.

## Preserved authority

Folder View rendering remains a projection over AppRuntime/RuntimeClient-owned state. `Path` remains display/tree metadata only. Selection/read authority remains G08.9 `documents.get` by opaque logical document ID.

No direct `os`, `io/fs`, `path/filepath`, filesystem stat/walk/open/write, shell, network, Host, Store, WebAI, Engine, SessionManager, provider/API, scheduler/resource/browser-lane access is added. No create/move/rename/delete/write semantics are added.

The renderer introduces no new AppRuntime command/query contract and no new breakpoint authority: responsive behavior inherits `LayoutForWidth` exactly.

## Bounded renderer state

Renderer rows are capped at `MaxFolderRenderRows = 512`. This is a presentation cap only; it does not change the canonical `documents.list` result, project state, paging metadata or document catalog authority.

## Successor boundary

G08.11 owns cross-workspace product-completeness regression and remains CLOSED. G08.10 does not authorize G08.11, PR merge, or any Folder View mutation semantics.
