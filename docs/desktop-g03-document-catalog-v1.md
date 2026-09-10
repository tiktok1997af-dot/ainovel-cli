# AINOVEL Desktop — G03.4 Store-backed Document Catalog / Document Get Authority v1

Status: `G03.4 — PASS / READ HANDLERS LOCKED`

Parent authorities:

- `docs/desktop-g03-project-knowledge-authority-v1.md`
- `docs/desktop-g03-query-contract-v1.md`
- `docs/desktop-g03-project-read-handlers-v1.md`

Branch: `feat/desktop-roadmap-v2-g03-project-knowledge`

Validation authority: implementation and authority state are accepted only when the repository's exact-head CI for this branch is green. Prior validation chain includes CI runs #393–#397, each covering WEB-only/NO-API audit, Ubuntu format/vet/test/race and Windows format/vet/test. Any later code or authority change reopens exact-head validation.

## 1. Scope

G03.4 converts the two frozen document queries into real read handlers:

```text
documents.list
documents.get
```

The implementation is a read-only catalog/view over existing canonical project artifacts. It does not add project mutation, GUI code, a persistence migration, a generic filesystem API, or a `DocumentStore`.

The desktop boundary remains:

```text
Desktop UI -> AppRuntime.Query -> typed document handler -> narrow Host read seam -> existing Store root artifacts
```

The GUI never receives a Store pointer, filesystem handle, absolute path, or arbitrary read primitive.

## 2. Non-negotiable persistence rule

There is no new document persistence domain.

G03.4 MUST NOT introduce:

- `DocumentStore`;
- document index/database persisted beside the project;
- generic `listDir`, `readFile(path)`, or filesystem browser methods at AppRuntime;
- absolute-path access from the desktop;
- any document mutation through Query.

The catalog is reconstructed from supported existing artifacts each time it is read.

## 3. Stable document kinds

The AppRuntime document kind vocabulary is frozen for this step as:

```text
project
outline
knowledge
chapter
summary
```

`documents.list.kind` is optional. When present it must be one of these values; unsupported kinds are rejected as a validation error rather than interpreted as a filesystem selector.

## 4. Stable logical IDs

Desktop document reads use logical IDs, never paths.

Representative fixed IDs include:

```text
project.format
project.book
project.book-markdown
project.progress
project.premise
outline.flat
outline.flat-markdown
outline.layered
outline.layered-markdown
outline.compass
knowledge.characters
knowledge.characters-markdown
knowledge.cast
knowledge.world-rules
knowledge.world-rules-markdown
knowledge.timeline-log
knowledge.timeline
knowledge.timeline-markdown
knowledge.foreshadow
knowledge.foreshadow-markdown
knowledge.relationships
knowledge.relationships-markdown
knowledge.state-changes-log
knowledge.state-changes
```

Dynamic Store-owned artifacts use deterministic IDs derived from their domain identity:

```text
chapter.plan:<chapter>
chapter.draft:<chapter>
chapter.final:<chapter>
chapter.record:<chapter>
summary.chapter:<chapter>
summary.arc:<volume>:<arc>
summary.volume:<volume>
```

IDs are serialization-safe selectors. A caller cannot substitute a relative or absolute filesystem path for an ID.

## 5. Supported artifact discovery

### 5.1 Fixed artifacts

Known project-level artifacts are represented by an in-code whitelist with an exact logical ID, kind, project-relative path, display title and content type. Missing optional artifacts are simply absent from the returned catalog.

### 5.2 Dynamic artifacts

Dynamic discovery is deliberately limited to these four existing Store-owned directories:

```text
chapters
drafts
summaries
meta/chapter_records
```

The implementation does not recursively walk the project and does not accept a directory from the caller.

Only canonical Store filenames are admitted:

```text
chapters/%02d.md
drafts/%02d.plan.json
drafts/%02d.draft.md
meta/chapter_records/%06d.json
summaries/%02d.json
summaries/arc-v%02da%02d.json
summaries/vol-v%02d.json
```

A project file that happens to exist but does not match one of the supported fixed artifacts or canonical dynamic patterns is invisible to this catalog.

## 6. Safe relative paths

`DocumentSummaryDTO.Path` is metadata only and is always project-relative.

Path validation rejects:

- empty paths;
- NUL bytes;
- absolute paths;
- Windows drive/colon selectors;
- backslash-based traversal selectors;
- `.` / `..` segments;
- path-cleaning drift such as repeated separators or embedded traversal.

Returned paths do not grant write permission and are not accepted by `documents.get` as read selectors.

## 7. Symlink containment

Before a supported artifact is catalogued or read, the Host resolves the project root and candidate through the operating system symlink resolver and verifies that the resolved candidate is still inside the resolved project root.

A supported-looking symlink that escapes the project root is rejected as a Store read error. This prevents the fixed whitelist from becoming an indirect arbitrary-file read channel.

## 8. Read-only content retrieval

`documents.get`:

1. validates a stable logical ID;
2. resolves that ID through the supported catalog;
3. resolves and containment-checks its fixed project-relative artifact path;
4. reads only that resolved regular file;
5. requires valid UTF-8 content;
6. returns `DocumentSummaryDTO` plus content.

The DTO is always `read_only=true` in G03.4.

Unknown or absent IDs are not reinterpreted as paths. They become a safe validation failure at the AppRuntime boundary.

No write, rename, delete, create, chmod, or filesystem mutation is exposed.

## 9. Content types and metadata

Supported catalog entries expose content type according to their existing artifact format:

```text
application/json
application/x-ndjson
text/markdown; charset=utf-8
```

They also expose stable ID, kind, safe relative path, display title, read-only capability and current byte size.

No credentials, browser data, absolute filesystem locations, file descriptors or raw Go implementation objects cross the desktop boundary.

## 10. Filtering and pagination

`documents.list` supports:

- exact document kind filtering;
- safe project-relative prefix filtering over catalogued paths only;
- offset pagination;
- default page size `100`;
- maximum requested page size `500` via the existing G03 query contract.

Filtering occurs after the safe catalog is built. Prefix is not used to choose a directory to scan and therefore cannot widen the discovery surface.

## 11. Error semantics

Invalid ID syntax, path-like IDs, unsafe prefixes and unsupported kinds are rejected by the existing AppRuntime validation error contract.

Filesystem/read/corruption failures preserve the Store read error chain and are normalized by the existing G02 `AppError` boundary without exposing internal paths to the desktop.

Missing optional supported files are omitted from the catalog. An unknown requested logical ID is distinct from a Store read failure.

## 12. Test authority

G03.4 tests lock the following behavior:

- supported existing static/dynamic artifacts are catalogued;
- logical IDs are unique and deterministic;
- noncanonical and unrelated project files do not leak into the catalog;
- only safe project-relative paths are exposed;
- document reads use stable IDs rather than caller paths;
- traversal/absolute/backslash/unsupported selectors are rejected;
- symlink escape outside project root is rejected;
- kind/prefix filtering and pagination are deterministic;
- default page size remains 100;
- metadata/content DTOs remain read-only and serialization-safe;
- after G03.4 only the Knowledge query family remains intentionally staged.

## 13. Compatibility and invariants

G03.4 is additive and does not change existing CLI/TUI Store behavior.

It does not change project format version and requires no migration.

WEB-only Gemini execution and the NO-API/no-fallback invariant are untouched.

Store/domain remain the source of truth. Browser conversation data is not part of the document catalog.

## 14. Gate rule

G03.4 is locked only when CI succeeds on the exact final commit containing this authority status plus the implementation/tests. Any later code or authority change reopens exact-head validation.

PR #19 remains Draft after G03.4. It MUST NOT be merged until the final G03 gate is completed.
