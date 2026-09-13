# G08.11 — Cross-workspace product-completeness regression v1

Status: IMPLEMENTATION CANDIDATE — NOT AUTHOR-CONFIRMED / NOT LOCKED.

Locked parent authority: G08.10 `PASS / AUTHOR-CONFIRMED / LOCKED` at `b257c479e127fc186e2406ddfc3f1fa46ef70cfa`.

## Scope

G08.11 adds regression/authority coverage only. It introduces no new product feature or production runtime semantics.

The regression verifies cumulative coexistence of the already-locked desktop product surfaces:
- the original primary shell remains the sole route/navigation authority;
- Project / Creative / Knowledge / Review / Run Center / Settings remain selectable through the existing shell;
- CoCreate remains transient controller presentation state and does not become a second primary route;
- Folder View remains a Project workspace tab backed only by the locked `documents.list/get` read plane;
- switching between sibling workspaces does not overwrite already-loaded Folder View, Creative or CoCreate presentation state;
- responsive Folder View rendering uses existing `LayoutForWidth` state only and does not issue hidden runtime queries or dispatches;
- Start / Pause / Resume / Stop lifecycle controls remain present under the pre-existing lifecycle authority;
- Folder View rendering remains inactive outside `RouteProject` + `ProjectTabFolder` and resumes from the preserved state when returning to Project.

## Preserved authority

No production file is modified by G08.11. No new AppRuntime command/query contract, Host, Engine, Store/DB, scheduler/resource/browser lane, SessionManager, WebAI/provider/API path, filesystem authority, route core, layout core or persistence model is introduced.

WEB-only / NO-API and all G03/G05/G06/G07/G08 locks remain authoritative.

## Successor boundary

G08.12 owns the cumulative G01–G08 packaged WEB-only end-to-end regression and remains CLOSED. G08.13 owns final exact-head authority / merge / post-merge main verification and remains CLOSED.