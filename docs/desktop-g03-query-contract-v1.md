# AINOVEL Desktop — G03.2 Project / Knowledge Query Contract v1

Status: `G03.2 — PASS / CONTRACT LOCKED`

Parent authority: `docs/desktop-g03-project-knowledge-authority-v1.md`

Baseline: G03 branch based on `main@443368d3f1cb2400984de28e4919e7fb505d96aa`.

Validated implementation head: `b66f26bf5d10d58113ee267182a9c1aa41d2b5a9` (CI run 375: WEB-only/NO-API audit, Ubuntu format/vet/test/race, Windows format/vet/test PASS).

## 1. Scope

G03.2 freezes the desktop read-plane catalog and serialization-safe DTO skeleton. It does not add GUI workspaces, Store mutations, a second persistence layer, or final Store-backed query handlers.

The only desktop entry remains:

```text
Desktop UI -> AppRuntime.Query -> typed query router -> later Store-backed handlers
```

## 2. Stable query catalog

The G03 Project / Knowledge read catalog is:

- `project.overview`
- `chapters.list`
- `chapters.get`
- `outline.get`
- `documents.list`
- `documents.get`
- `knowledge.context`
- `knowledge.canon`
- `knowledge.characters`
- `knowledge.world`
- `knowledge.timeline`

`SupportedQueryKinds()` returns the catalog in stable order.

## 3. DTO boundary

Desktop query DTOs live in `internal/appruntime/queries_contract.go` and contain only JSON-safe values. They do not expose:

- `store.Store` or sub-store pointers;
- `domain` implementation structs;
- raw Host channels;
- file handles;
- browser/session implementation objects;
- raw Go errors.

Knowledge DTOs that represent composed facts include explicit provenance fields instead of pretending Context/Canon are independent databases.

## 4. Request validation

The router in `internal/appruntime/queries.go`:

- validates `QueryKind` against the locked catalog;
- decodes each payload into its typed request DTO;
- rejects malformed JSON;
- rejects unknown JSON fields;
- rejects invalid chapter/range/scope values;
- bounds paging/query limits at `MaxQueryPageSize = 500`;
- maps invalid requests to the stable G02 validation `AppError` shape without echoing raw payloads or paths.

## 5. Staged handler rule

G03.2 changes `Runtime.Query` from a single global stub into a typed router. After a request is recognized and validated, Store-backed business handlers are still intentionally staged for later G03 steps and currently return the existing `ErrNotImplemented` sentinel.

This is deliberate: successful empty DTOs MUST NOT be returned before canonical Store data is actually read, because that would make an unimplemented query indistinguishable from a genuinely empty project.

## 6. Read-only invariant

G03.2 introduces no mutation path. Query routing must remain side-effect-free. Project/knowledge writes, when opened by a later locked step, must use typed `AppRuntime.Dispatch` commands; Query MUST NOT become a hidden write channel.

## 7. Test authority

`internal/appruntime/queries_test.go` locks:

- exact stable query catalog;
- typed payload decoding for every query kind;
- rejection of unknown kinds, malformed payloads, unknown fields and invalid bounds;
- staged router behavior;
- JSON safety of all project/knowledge result DTO families;
- QueryRequest typed-payload round-trip compatibility.

G03.2 is locked only after exact-head CI passes WEB-only/NO-API audit, gofmt, vet, tests and required platform regression.
