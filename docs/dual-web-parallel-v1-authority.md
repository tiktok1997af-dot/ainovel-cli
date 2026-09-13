# AINOVEL Dual-Web Parallel v1 — Authority Freeze

Status: D01 IMPLEMENTATION CANDIDATE — NOT AUTHOR-CONFIRMED / NOT LOCKED.

Exact baseline: `main@54b9ceb194108193579ef647490830db30937919` (AINOVEL Desktop Optimized v2 G01–G08 COMPLETE, 60/60).

This extension is a new roadmap. It is not G09 and does not reopen or weaken any G01–G08 authority.

## 1. Locked target architecture

The target runtime architecture is:

- Gemini Web lane: exactly 1 persistent provider lane, at most 1 active AI turn;
- ChatGPT Web lane: exactly 1 persistent provider lane, at most 1 active AI turn;
- local workers: exactly 2 concurrent local worker slots;
- maximum AI concurrency: 2;
- maximum total active jobs: 4;
- browser startup: lazy by default;
- one provider = one isolated browser profile = one persistent working tab;
- Run Center remains the single scheduler / queue / pause / resume / stop authority;
- resource locks remain core-owned and serialize conflicting project/chapter/write/official/store resources;
- Resilience Layer owns provider health, auth/readiness, DOM/UI-version detection, response-boundary detection, stream completion, timeout, cancel, retry, session recovery, degraded state and circuit breaking;
- Canon, Context, Store, project state and Official truth remain local AINOVEL authority. Web AI providers never own canonical state;
- WEB-only / NO-API remains mandatory. No Gemini API, OpenAI API or provider fallback is authorized.

## 2. Production pipeline defaults

Default mode is `balanced`:

- pipeline: ON;
- Gemini role: primary writer / fast generation / ordinary CoCreate;
- ChatGPT role: specialist reviewer / continuity / architecture / bounded repair / QA;
- Strict Role: ON by default;
- Auto Review: ON;
- Auto QA: ON;
- Auto Repair: BOUNDED;
- checkpoint interval: 10 chapters by default, configurable only within 5..10;
- no competing two-provider generation by default;
- independent reads and non-conflicting jobs may execute concurrently;
- two providers must never concurrently write the same authority resource;
- Official promotion remains separately authorized and is never implied by parallel execution.

`turbo` may increase draft/review pipeline throughput, but may not weaken resource locking, model verification, WEB-only authority or Official promotion rules.

## 3. Provider model selection gate — mandatory before run

Before `START`, AINOVEL must expose a Model Selection Gate for every provider required by the planned run.

Provider vocabulary is exactly:

- `gemini-web`
- `chatgpt-web`

Each required provider must expose a sanitized model catalog discovered from the current authenticated web session. Core contracts must not permanently hard-code volatile marketing model names.

For every provider, the pre-run gate tracks:

- requested model ID;
- requested display label;
- observed active model ID;
- provider authentication state;
- provider readiness state;
- model availability;
- exact model activation verification;
- verification timestamp / revision suitable for invalidating stale verification.

`START` is disabled until every provider required by the run satisfies all of:

1. authenticated;
2. provider READY;
3. requested model exists in the currently discovered catalog;
4. requested model is activated in that provider's browser lane;
5. observed active model exactly matches the resolved requested model;
6. verification is current for this pre-run attempt.

If a model disappears, is renamed, cannot be selected, cannot be observed, or the UI changes so verification is ambiguous, the provider fails closed with `MODEL_UNAVAILABLE` or an equivalent typed preflight failure. There is no silent fallback and no silent model substitution.

An optional `Recommended/Auto` presentation choice may exist later, but it must resolve to an exact concrete model ID, show that resolved model to the user, and verify it before `START`.

Saved model choices are preferences only. They are revalidated on every run.

## 4. Browser and concurrency authority

Normal maximum web footprint is two AI tabs total:

- Gemini browser profile / working tab: 1;
- ChatGPT browser profile / working tab: 1.

Authentication popups/tabs may be temporary but do not create new execution lanes and must not become a third AI turn lane.

Provider lane resource keys are reserved conceptually as:

- `browser:gemini-web`;
- `browser:chatgpt-web`.

Exact resource-key encoding remains owned by the existing G05 scheduler/domain contracts and must be integrated rather than duplicated.

## 5. Resilience fail-closed rules

A provider may be `READY`, `DEGRADED`, `AUTH_REQUIRED`, `MODEL_UNAVAILABLE`, `BUSY`, `RECOVERING`, or an equivalent typed state projected through existing authority boundaries.

When a lane is not verified-ready:

- no new model turn is dispatched into that lane;
- in Strict Role mode, compatible work waits rather than silently switching provider;
- a provider failure must not corrupt the other provider lane or local state;
- retry and recovery remain bounded;
- repeated provider failure opens a circuit breaker rather than creating uncontrolled retries/tabs.

## 6. New extension roadmap

- D01 — authority audit + Dual-Web / concurrency / model-selection contract freeze;
- D02 — provider/lane typed contracts + dynamic model catalog and preflight DTOs;
- D03 — ChatGPT Web adapter + isolated authenticated browser profile/lane;
- D04 — existing Run Center integration for two AI lanes + two local workers + resource-lock coexistence;
- D05 — desktop pre-run Model Selection Gate and START fail-closed wiring;
- D06 — Resilience Layer: health/degraded/recovery/circuit-breaker semantics;
- D07 — Balanced/Turbo parallel pipeline + bounded repair + 5..10 chapter checkpoints;
- D08 — cumulative regression + packaged dual-provider WEB-only / NO-API smoke;
- D09 — final exact-head authority / merge / post-merge main verification.

D02+ remain CLOSED until the preceding step is AUTHOR-confirmed / locked, unless a later explicit AUTHOR instruction opens the successor.

## 7. D01 implementation boundary

D01 may add only:

- this authority document;
- typed finite provider/model-selection contract substrate and deterministic validation tests needed to freeze the boundary;
- no ChatGPT browser execution yet;
- no second live browser lane yet;
- no scheduler concurrency change yet;
- no desktop START behavior change yet;
- no merge.

Acceptance requires exact-head tests/CI and explicit AUTHOR confirmation before D02 opens.
