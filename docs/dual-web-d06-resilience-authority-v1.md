# D06 — Dual-Web Resilience Layer Authority v1

Status: OPEN / AUTHOR-AUTHORIZED

Parent authority: D05 `PASS / AUTHOR-CONFIRMED / LOCKED` at exact `43a8de3e9a1facb9231d0f3e45b494c66b3a98af`.

D06 extends the locked Dual-Web Parallel v1 runtime with provider-health, bounded recovery, and circuit-breaking only. It does not reopen D01–D05 and it does not authorize D07 Balanced/Turbo pipeline behavior.

## Ownership

D06 owns only provider execution health at the existing dual-Web lane-admission boundary:

- Gemini Web lane health and bounded recovery through the existing Run Center `BrowserLanePool` authority;
- ChatGPT Web lane health and bounded stop/start recovery through the existing isolated ChatGPT lane authority;
- provider-local circuit state that prevents repeated browser churn after consecutive readiness/recovery failures;
- fail-closed admission while a provider circuit is open.

Existing authorities remain unchanged:

- Run Center remains the sole scheduler/queue/lifecycle authority;
- existing Resource Locks remain the sole story-write serialization authority;
- D04 limits remain exact: Gemini lane ×1, ChatGPT lane ×1, local workers ×2, max AI concurrency 2, max active jobs 4;
- D05 model selection/preflight remains mandatory before desktop START;
- Store/project files remain canonical story truth;
- WEB-only / NO-API remains mandatory.

## Frozen bounded policy

D06 uses a provider-local circuit breaker with these bounded values:

- consecutive final provider-admission failures to open circuit: `2`;
- open-circuit cooldown: `30s`;
- half-open probes: exactly `1` in flight per provider;
- provider recovery attempts per failed admission: at most `1`;
- successful readiness/recovery closes the circuit and clears the consecutive-failure count.

The circuit is isolated per provider. Gemini failures must not open ChatGPT, and ChatGPT failures must not open Gemini.

Circuit state is ephemeral runtime health only. It is not persisted into Store/project/Canon/Context/Official truth and is reset by process restart.

## Failure classification

Only final provider lane/readiness failures after the bounded recovery attempt count toward the circuit.

The following are not provider-health failures and must not trip the circuit:

- caller cancellation or cancelled/deadline context before provider recovery can complete;
- Run Center/browser-lane capacity contention such as `ErrNoBrowserLaneAvailable`;
- explicit authentication-required state, which requires user action;
- model-selection/preflight mismatch owned by D05;
- resource-lock contention;
- core/Store/business-domain failures outside provider lane admission.

## Recovery semantics

### Gemini Web

D06 reuses the existing allocated `BrowserLanePool` lane and its existing `Recover` operation. Recovery performs the existing bounded stop/restart while preserving run ownership, lane identity, and the persistent isolated profile. D06 does not create a second browser pool or profile authority.

### ChatGPT Web

D06 performs at most one stop/start recovery on the existing isolated ChatGPT lane when readiness is degraded/failed/stopped for a recoverable provider-health reason. The persistent ChatGPT profile remains the same; there is no cross-provider fallback.

## Hard exclusions

D06 must not:

- fall back Gemini → ChatGPT or ChatGPT → Gemini automatically;
- introduce an AI API/provider fallback;
- replay an accepted/ambiguous provider turn or duplicate Store side effects;
- change D04 concurrency/resource limits;
- change D05 explicit model-selection/preflight semantics;
- add Balanced/Turbo role routing, Auto Review/QA/Repair pipeline behavior, or checkpoint policy execution (D07);
- perform cumulative packaged dual-provider final smoke (D08);
- merge the extension roadmap into `main` (D09).

## D06 acceptance

D06 becomes `PASS CANDIDATE` only when the exact candidate head proves:

1. provider-local two-failure circuit opening;
2. open-circuit fail-closed admission with no browser touch;
3. cooldown → single half-open probe → success closes circuit;
4. failed half-open probe reopens circuit;
5. Gemini bounded recovery reuses `BrowserLanePool.Recover` and preserves lane/run ownership;
6. ChatGPT bounded recovery performs at most one stop/start attempt;
7. auth-required/cancellation/capacity contention do not poison provider health;
8. provider isolation and D04 concurrency/resource authority remain intact;
9. full Linux/Windows CI and critical race remain green;
10. WEB-only / NO-API audit remains green and blocking review threads = 0.

D07 remains CLOSED until D06 is separately AUTHOR-confirmed and locked.
