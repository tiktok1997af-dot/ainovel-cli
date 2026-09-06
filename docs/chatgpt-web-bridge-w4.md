# W4 — Web Tool-Call E2E v0.1

Status: **OPEN — DETERMINISTIC IMPLEMENTATION / LIVE GATE**

Repository: `tiktok1997af-dot/ainovel-cli`  
Branch: `w4-web-toolcall-e2e`  
Base: W3 locked main commit `700cb288f33eb64879e3e364d8197361414a29df`

## Locked product rule

W4 remains **WEB-ONLY / NO-API**.

The website is an AI transport only. Gemini Web can request a local tool by returning the W1 protocol envelope, but it never receives authority to execute local code, write project files, read arbitrary files, or call shell commands directly. The local `agentcore` runtime validates the tool name/schema and is the only component that executes the tool.

## W4 target lifecycle

```text
Gemini Web
  -> W1 tool_calls envelope
  -> WebChatModel parses registry-bound call
  -> agentcore validates arguments
  -> LOCAL Tool executes
  -> local Tool result enters transcript
  -> WebChatModel sends the transcript back through Gemini Web
  -> Gemini produces final text
```

## Live proof design

The W4 verifier uses a deliberately harmless in-memory tool named `w4_local_proof`.

1. The verifier creates a random receipt locally before the run.
2. Gemini is told to call `w4_local_proof` exactly once with a fixed challenge.
3. The receipt is returned only by the local Go callback after schema validation and execution.
4. The receipt is not present in the initial browser prompt.
5. Gemini must read the local tool result on the second model turn and return `W4_TOOL_OK:<receipt>` exactly.
6. PASS additionally requires the callback execution count to equal one and the supplied challenge to match exactly.

This prevents a false PASS where the web model merely claims that a tool ran.

## Security boundary

- no AI API, API key or provider HTTP fallback;
- no browser-side local command execution;
- no direct web filesystem authority;
- no credential/cookie/token extraction;
- no security challenge bypass;
- tool execution remains registry-bound and schema-validated by `agentcore`;
- live proof tool has no filesystem or shell side effects;
- evidence stores only state/count/hash metadata, not conversation text or the raw random receipt.

## Gate

W4 can be locked only after:

1. Linux + Windows CI pass format, vet and full tests; Linux critical race tests remain green.
2. The Windows verifier build succeeds from the verified W4 head.
3. The real Windows/Gemini run shows at least two `BUSY` rounds (tool request and post-tool final response).
4. The local callback executes exactly once with the exact challenge.
5. Gemini returns the exact random receipt that only the local callback produced.
6. Final browser state returns to `READY`.

W5 remains closed until this live W4 gate passes.
