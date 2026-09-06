# W3 — Web Prompt Submit & Response Capture v0.1

Status: **OPEN — DETERMINISTIC IMPLEMENTATION / CI GATE**

Repository: `tiktok1997af-dot/ainovel-cli`  
Branch: `w3-web-prompt-capture`  
Base: W2 locked main commit `214e0a9f0697e775ff4ea4fe1eaa12a4c264dc0e`

## Locked product rule

W3 remains **WEB-ONLY / NO-API**.

The only AI execution path introduced here is the visible, already logged-in Gemini Web page controlled through the local loopback Chrome DevTools endpoint established in W2. W3 must not add API keys, Gemini/OpenAI/Anthropic provider HTTP calls, cloud-browser fallback, or credential extraction.

## W3 scope

Target lifecycle for one model request:

```text
READY
  -> submit one prompt through the Gemini web composer
  -> BUSY
  -> capture a new stable final model response
  -> READY
```

Failure/control semantics:

- caller cancellation -> best-effort click Gemini Stop -> return `context.Canceled`;
- response timeout -> best-effort click Stop -> `ErrorTimeout`;
- transient DevTools failure before submit may retry locally;
- capture connection may reattach after submit without resubmitting the prompt;
- once Send has been clicked, W3 must never blindly auto-resubmit the same prompt;
- auth/security challenge remains a user-action state; no bypass attempt.

## Gemini DOM interaction boundary

W3 adds a site interaction adapter that can:

1. read only conversation-generation state and assistant response text needed for the current round trip;
2. populate the visible Gemini composer;
3. click the visible Send control;
4. detect visible Stop/generation state;
5. click Stop for cancellation/timeout;
6. return the final assistant response to `WebChatModel`.

The adapter does not read cookies, tokens, localStorage, Google account identity, password fields or browser databases.

## Final-response rule

Before submission, W3 records a response baseline. A final response is accepted only when:

- the captured assistant response is new/changed relative to that baseline;
- it is non-empty;
- Gemini is no longer visibly generating; and
- the same response remains unchanged for a bounded stability window.

Oversized responses are rejected rather than silently truncated into the W1 protocol parser.

## Gate

W3 can be locked only after:

1. Linux and Windows CI pass format, vet and full tests; Linux critical race tests remain green.
2. Deterministic tests prove submit/capture, timeout, cancellation and reattach-without-resubmit behavior.
3. A Windows verifier uses the already logged-in W2 profile to submit one real protocol prompt through Gemini Web.
4. The real run demonstrates `READY -> BUSY -> READY` and returns a valid W1 response envelope through `WebChatModel`.

W4 tool-call E2E remains closed until W3 is locked.
