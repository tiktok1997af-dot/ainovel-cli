# G08.9 — Folder tree + documents.list/get integration v1

Status: IMPLEMENTATION CANDIDATE — NOT AUTHOR-CONFIRMED / NOT LOCKED.

Locked parent authority: G08.8 `PASS / AUTHOR-CONFIRMED / LOCKED` at `b3e56c51a99f995ea49b42e6b3b554efeea1e323`.

## Scope

G08.9 integrates the G08.8 read-only Folder View tree with the already-locked G03 AppRuntime document catalog.

The integration:
- adds an explicit Project-workspace Folder View tab/state target without adding a second application shell;
- loads bounded catalog metadata only through `documents.list`;
- derives the tree only from the sanitized `DocumentSummaryDTO.Path` metadata returned by that query;
- selects content only by an opaque document ID that is present exactly once in the current safe Folder View projection;
- reads selected content only through `documents.get` using that opaque ID;
- verifies that `documents.get` returns the same canonical logical path/kind as the selected tree node and remains read-only;
- preserves existing paging/filter metadata and AppRuntime query/error/stale-ticket handling;
- dispatches no mutation command.

`Path` remains presentation/tree metadata only. It never becomes a filesystem selector or a `documents.get` selector.

## Preserved G03 / G08 authority

Folder View does not add a filesystem API. It inherits the locked document-catalog protections: supported canonical artifact whitelist, project-relative paths, symlink containment, UTF-8 regular-file reads, and invisibility of unsupported/noncanonical files.

There is no `listDir`, recursive filesystem scan, raw `readFile(path)`, `writeFile`, create, rename, delete, chmod, watcher, desktop-local file index/database, direct Host/Store/WebAI/Engine/SessionManager access, provider/API fallback, or new scheduler/resource/browser lane.

`meta/sessions/cocreate.jsonl`, browser profiles/storage, credentials, and raw runtime/session files remain outside Folder View.

## Successor boundary

G08.10 owns Folder View renderer/responsive/full-width integration and remains CLOSED. G08.9 adds no renderer or layout authority and does not authorize PR merge.
