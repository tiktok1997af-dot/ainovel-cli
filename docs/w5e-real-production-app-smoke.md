# W5E — REAL PRODUCTION APP SMOKE

> Status: **OPEN / AUTHORIZED — REAL DESKTOP EVIDENCE REQUIRED**  
> Authority base: `068372014f55887f0df1188b96c0f0bd6016a6d6`  
> W5A–W5.5 remain **PASS / LOCKED / INTEGRATED**.  
> FINAL W5 LOCK and merge of PR #5 to `main` remain **CLOSED** until W5E passes.

## 1. Purpose

W5E is the final production-path proof for the WEB-only migration. It does not accept a mock model, a fake browser adapter, a test-only Store, or a hosted-CI approximation as proof.

The required path is the actual product path:

```text
ainovel-cli production binary
  -> bootstrap.Config (WEB-only)
  -> Host / Engine / Workers
  -> WebChatModel
  -> owned visible Chrome + persistent profile
  -> logged-in Gemini Web
  -> deterministic response/tool protocol
  -> local Tools
  -> local Store
```

## 2. PASS criteria

W5E may be marked **PASS / LOCKED** only when one candidate head satisfies all of the following:

1. The real `cmd/ainovel-cli` production binary is built from the candidate head and its SHA-256 is recorded.
2. An already-authorized persistent Chrome profile reaches real Gemini Web `READY` through the production browser/session code.
3. The browser is stopped and started again with the same profile and reaches `READY` again without extracting, copying, or recording account credentials, cookies, tokens, browser storage, page HTML, or account identity.
4. The real production binary is launched in an isolated workspace with `--headless --prompt-file`; it must use normal `Host -> Engine -> Workers -> WebChatModel` execution rather than a verifier-only model path.
5. Chapter 1 is durably committed to the real local Store while the run is still resumable. The first production process is then terminated to create an actual restart boundary.
6. The same production binary is launched again from the same workspace with `--headless` and **without a prompt**. It must enter the real `Resume` path, preserve Chapter 1 byte-for-byte, and advance through Chapter 2.
7. Real Store evidence exists: `meta/progress.json`, chapter files, Worker session JSONL, and local tool-call evidence including at least `draft_chapter` and `commit_chapter`.
8. Runtime logs/evidence show no AI API credential, direct AI API endpoint, provider-fallback, or local Ollama execution path.
9. The durable W5.5 repository-wide NO-API audit still passes on the exact candidate head.
10. The final evidence JSON contains only sanitized state/provenance: commit/hash identifiers, READY states, profile **name** only, progress summaries, chapter digest, session-file count, tool names, and gate results. Story body, browser profile path, account identity, cookies, tokens, page content, and full session content must not be copied into the evidence JSON.

Any missing item is a W5E **FAIL / NOT LOCKED**, not a reason to infer success.

## 3. Why hosted CI is not W5E evidence

GitHub-hosted runners do not own the user's persistent, normally logged-in Google/Gemini browser profile. They therefore cannot honestly prove the real `Chrome/profile -> Gemini Web READY` boundary without introducing credential handling or a fake login state, both of which violate the locked architecture.

Hosted CI is allowed to:

- compile the exact production binary;
- compile the read-only W5E readiness verifier;
- syntax-check the desktop smoke runner;
- run normal tests and the W5.5 NO-API gate;
- package a Windows smoke bundle.

Hosted CI is **preflight only**. A green hosted job must never by itself change W5E to PASS.

## 4. W5E tools

### `cmd/ainovel-w5e-readiness`

A read-only browser-lifecycle verifier. It uses `bootstrap.LoadConfig` and the real `webai.SessionManager`. It requires an existing profile to reach `READY`, stops Chrome, starts the same profile again, and requires `READY` again. It never submits a model prompt and never reads credential/browser-storage material.

### `scripts/w5e-production-smoke.ps1`

The real Windows desktop runner. It:

1. builds the production binary and verifier from the current commit;
2. runs the W5.5 static NO-API gate;
3. proves `READY -> STOPPED -> READY` with the persistent profile;
4. creates an isolated temporary novel workspace;
5. starts the production app with an exact two-chapter smoke requirement;
6. waits for a durable Chapter-1 resumable boundary;
7. terminates only the smoke production process and only Chrome processes owned by the dedicated ainovel profile;
8. records the Chapter-1 SHA-256;
9. restarts the same production binary without a prompt;
10. requires real Resume evidence and Chapter-2 completion;
11. verifies the Chapter-1 digest did not change;
12. verifies Worker session files and required local tool calls;
13. scans runtime logs for forbidden API/local-provider markers;
14. writes sanitized `w5e-production-smoke-evidence.json`.

The runner deliberately fails if Chapter 2 finishes before it can create the required Chapter-1 restart boundary. Persistence is not inferred from a run that never actually restarted.

## 5. Evidence privacy boundary

W5E evidence must **not** contain or upload:

- Google email/account identifiers;
- passwords, cookies, tokens, browser storage, DevTools payloads;
- full Chrome profile paths;
- Gemini page HTML;
- story prose or full prompt/response transcripts;
- raw Worker session bodies.

Only the sanitized evidence JSON is authority evidence for the W5E lock record.

## 6. Lock procedure

After a real desktop smoke PASS:

1. verify the evidence `git_sha` equals the exact W5E candidate head;
2. verify candidate CI/preflight is green;
3. record evidence and exact-head CI on the W5E PR;
4. mark W5E **PASS / LOCKED**;
5. integrate the exact W5E lock head into `w5-web-only-integration-api-removal` with head protection;
6. require post-integration CI and repository-wide NO-API audit to pass;
7. only then open **FINAL W5 LOCK** and consider merging PR #5 into `main`.
