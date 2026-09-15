# D09-B — Desktop Stack Reconciliation & Forward-Port v0.1

Base authority: exact D08 `4b71a27ba901daf5a817115b4128c99e420a07d8`.

This gate reconciles historically verified desktop behavior with the current AppRuntime/backend stack. It does not authorize final E2E or packaged smoke. Those remain closed until this integration foundation passes.

## Ancestry finding

The historical Desktop Roadmap v2 G08 head `4043bf68235e5e985bc86374b074db223fe527e8` is an ancestor of D08. D08 is 137 commits ahead and 0 behind that G08 head. Therefore D09-B is a forward-port/reconciliation exercise, not a merge of a detached desktop lineage.

## Component inventory and current mapping

| Historical / desktop component | Verified behavior retained | Current API/runtime authority | D09-B disposition |
| --- | --- | --- | --- |
| G02 lifecycle control plane | START / PAUSE / RESUME / STOP / CANCEL / RETRY; authoritative snapshot/event reconciliation; structured protocol errors | `appruntime.CommandRequest`, `CommandResult`, lifecycle events/snapshots | RETAIN; START envelope required reconciliation |
| G04 shell/workspaces | shell routes, project/creative/knowledge/review/run-center/settings state, no hidden runtime work on presentation-only navigation | `desktopui.RuntimeClient` five-method facade | RETAIN |
| G05/G06 Run Center and review seams | canonical run/resource ownership, review flow, recovery/regression invariants | AppRuntime run backend + review contracts | RETAIN; runtime owns execution semantics |
| G08 product completeness | cross-workspace state preservation, responsive presentation-only rendering, complete primary navigation/lifecycle controls | `desktopui` projections over AppRuntime | RETAIN |
| D05 model picker | explicit Gemini/ChatGPT desktop selection | `DesktopStartCommandPayload` + AppRuntime D05 fail-closed model gate | FORWARD-PORT into generic lifecycle START |
| D04/D06 dual-provider resilience | provider/model verification, provider capacity, recovery/resilience | `dualWeb`, ChatGPT lane, provider resilience authority | RUNTIME-INTERNAL; no duplicate UI ownership |
| D07 run admission / pipeline policy | provider admission before core Resume; release admission before story resource lock; pipeline/checkpoint policy | `d07AdmissionRunBackend`, D07 policy | RUNTIME-INTERNAL; no new desktop contract required |
| D08 model graph / dual-web packaged evidence | Host-owned ChatGPT lane and strict-role model graph | Host + AppRuntime/WebAI | RETAIN; final packaged smoke remains CLOSED in D09-B foundation |

## Reconciliation seam 1 — lifecycle START -> D05 provider envelope

Before D09-B, generic `ExecuteLifecycle(LifecycleStart)` serialized the legacy `StartCommandPayload`. D05 AppRuntime requires `DesktopStartCommandPayload` with an explicit Gemini/ChatGPT provider and rejects missing/unknown/unready/unverified selections.

D09-B changes only the serialization seam: START now carries the current desktop model selection in `DesktopStartCommandPayload`, while the existing G02 lifecycle command ID, result validation, structured errors, refresh, event reconciliation and non-START payloads remain unchanged. An empty selection is preserved as an empty provider so the authoritative D05 runtime gate rejects it fail-closed rather than inventing a default provider.

Regression coverage: `internal/desktopui/d09b_reconciliation_test.go` verifies selected-provider propagation, the empty-provider fail-closed envelope, and unchanged non-START payload contracts. Existing G02/G04/G06/G08 desktop regressions remain the compatibility net.

## Dependency order for remaining D09-B foundation

1. Lifecycle/model-selection envelope reconciliation — implemented; targeted regression required.
2. AppRuntime snapshot/query/dispatch compatibility against desktop workspaces — regress existing desktop + AppRuntime suites.
3. Run Center -> D07 admission and review routing compatibility — regress runtime/run-center seams without packaging.
4. Settings/model graph projection compatibility — regress settings and provider/model configuration seams.
5. Integration-foundation aggregate regression — only after seams 1–4 are green.

Final E2E and packaged Windows smoke are explicitly outside this foundation stage and must not run until D09-B integration foundation is PASS.
