# AINOVEL Desktop — G06.7 End-to-End Review Regression / Final Authority v1

Status: `G06.7 — FINAL EXACT-HEAD CANDIDATE`

Roadmap parent: **AINOVEL Desktop Optimized v2 — 8 GATE / 60 steps**.

Branch: `feat/desktop-roadmap-v2-g06-7-final-regression`.

Accepted parent: G06.6 exact head `5b4933225d9a85dedfc1edd214bce81260ed1006` (`G06.6 PASS / AUTHOR-CONFIRMED / LOCKED`).

Baseline remains `main@a2e722528b9c729a3a1232ce9775fb5c8cfae3b6` until final G06 merge authority is satisfied.

## 1. Mission

G06.7 is the final cumulative Review regression and merge-authority gate for G06. It does not open a new Review feature family. It may only:

1. add integrated regression coverage over the already-frozen G06.2–G06.6 contracts;
2. make a bounded defect repair only when a final regression proves violation of an already-locked invariant;
3. verify G05 orchestration/recovery compatibility and WEB-only / NO-API boundaries remain intact;
4. establish one exact-head cumulative G06 candidate for CI/W6A/W6B/W6C acceptance;
5. only after exact-head acceptance, authorize PR #27 to leave Draft and merge;
6. after merge, re-read `main` and verify accepted G06 authority/content before declaring G06 COMPLETE.

No G07 scope opens inside this gate.

## 2. Frozen cumulative Review authority

G06.7 treats the following as immutable acceptance authority:

- ordered 12-gate catalog: `contract_fulfillment`, `consistency`, `character`, `pacing`, `continuity`, `foreshadow`, `hook`, `aesthetic`, `style_regression`, `flow_integrity`, `planning_integrity`, `context_integrity`;
- gate-state vocabulary: `not_run`, `running`, `pass`, `warn`, `fail`, `stale`, `unavailable`;
- AppRuntime Review queries: `review.catalog`, `review.status`, `review.history`;
- AppRuntime Review commands: `review.run`, `review.repair`, `review.rerun`, `review.promote_official`;
- Review events: `review_state`, `review_gate`, `review_action`;
- canonical Store/project facts remain story truth; Review/UI/browser state never becomes a second authority.

## 3. End-to-end regression invariants

The final integrated regression must preserve/prove, as applicable:

1. catalog order and command/query/event vocabulary remain exact;
2. Review projection carries deterministic freshness fingerprint plus exact chapter/revision/content-SHA refs;
3. stale semantic evidence cannot be silently promoted as current;
4. `review.repair` remains bounded to canonical affected chapters and actionable gates with expected fingerprint CAS;
5. `review.rerun` uses the expected current fingerprint and cannot silently overwrite changed evidence;
6. `review.promote_official` requires exact current fingerprint + revision/SHA set, rejects stale/conflicting state, blocks critical/major or non-ready required gates, permits only the frozen bounded style-unavailable exception, and remains idempotent;
7. promotion never mutates chapter content/revision or Review evidence/history;
8. Review orchestration remains behind the existing G05 scheduler/resource/lane/recovery authority;
9. Review workspace reaches core only through AppRuntime and does not claim `RunID`, `TaskID`, or `Resource` ownership;
10. Review/Review-owned Run events are notification-only and trigger fresh AppRuntime queries rather than GUI-owned truth;
11. UI does not invent PASS/WARN/FAIL thresholds or a durable local Official badge;
12. no second Review engine, scheduler, Store/DB, browser lane, filesystem/network UI surface, or direct AI API/provider fallback is introduced.

## 4. Fail-closed authority

Review failure paths stay explicit and typed. Invalid targets, stale fingerprints, changed revision refs, unavailable required gates, unresolved blocking semantic issues, mutation conflicts, malformed payloads and runtime precondition failures must reject rather than guess or overwrite.

Warnings alone do not create hidden UI policy. Promotion eligibility remains an AppRuntime/core authority decision even when the UI presents command availability from the runtime catalog.

## 5. UI / Official-state boundary

The Review workspace is projection/control only:

- target selection is presentation state;
- current status/history/catalog come from fresh AppRuntime queries;
- event payloads never replace canonical reads;
- `review.promote_official` sends the exact currently projected fingerprint and revision refs;
- a successful promotion result may be displayed as transient operation feedback;
- because G06.6 exposes no durable Official-status query, the UI must not persist or reconstruct an Official badge across reloads.

## 6. Compatibility boundary

G06.7 does not change:

- G05 scheduler, resource-lock, Browser Lane Pool or restart/recovery policy;
- Store/project formats except the already-accepted additive G06 Review/Official metadata;
- AppRuntime `Snapshot / Query / Dispatch / Subscribe / Close` desktop facade;
- one Engine implementation per project Host;
- bounded visible Gemini Web execution;
- WEB-only / W5.5 NO-API authority;
- no credential/cookie extraction;
- no generic filesystem surface;
- legacy project/runtime compatibility.

## 7. Candidate boundary

The default G06.7 candidate is one commit on accepted G06.6 and is restricted to authority documentation and cumulative regression tests:

```text
docs/desktop-g06-final-authority-v1.md
internal/appruntime/g06_final_regression_test.go
internal/desktopui/g06_final_regression_test.go
```

No production-code change is authorized by default. If a regression exposes a concrete defect, any bounded production repair must be explicitly documented in the acceptance record and creates a replacement candidate SHA that resets all exact-head evidence.

## 8. Exact-head acceptance

G06.7 becomes `PASS / AUTHOR-CONFIRMED / LOCKED` only when the same exact candidate SHA satisfies all of the following:

1. CI SUCCESS;
2. Linux format/vet/full tests SUCCESS;
3. required critical race tests SUCCESS;
4. Windows format/vet/full tests SUCCESS;
5. repository-wide WEB-only / W5.5 NO-API audit SUCCESS;
6. W6A Release Packaging Gate SUCCESS;
7. W6B Install Update Integrity Gate SUCCESS;
8. W6C Release Artifact Desktop Smoke SUCCESS, including real Windows packaged production WEB-only smoke;
9. branch/PR head remains the exact tested SHA;
10. `main` remains `a2e722528b9c729a3a1232ce9775fb5c8cfae3b6` before merge;
11. G06.6→G06.7 remains behind 0 and contains only authorized final-gate changes;
12. blocking review threads = 0 and blocking review submissions = 0;
13. cumulative G06 compare remains behind 0 from the locked G05 baseline.

Any corrective change creates a replacement candidate SHA and resets exact-head acceptance evidence.

## 9. PASS effect and final merge authority

Only after section 8 is satisfied may authority become:

```text
G06.7 = PASS / AUTHOR-CONFIRMED / LOCKED
```

At that point PR #27 may leave Draft and merge. Roadmap progress remains `45/60 = 75.0%` until merge completes and the resulting `main` is re-verified.

Only after successful post-merge verification may authority become:

```text
G06 = COMPLETE
roadmap progress = 46/60 = 76.7%
```

If the merge result or post-merge `main` verification differs from the accepted cumulative G06 authority, G06 remains incomplete and G07 remains closed.
