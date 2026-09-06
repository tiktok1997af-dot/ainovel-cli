# W5A — WEB-ONLY RUNTIME BOOTSTRAP MIGRATION

Status: PASS / LOCKED

Production WEB path:

`Config(web.enabled=true) -> SessionManager -> GeminiWebTransport -> WebChatModel -> ModelSet -> Architect/Writer/Editor/Arbiter`

Locked properties:
- WEB-only config requires no AI API credential.
- Legacy `providers/api_key/base_url` entries are rejected when `web.enabled=true`.
- All roles share the one browser-backed model graph; provider/model failover is disabled in WEB mode.
- OpenRouter pricing refresh is not started in WEB mode.
- Host owns the browser session and stops it during `Host.Close()`.
- Browser readiness may transiently be DEGRADED while Chrome starts; an alive owned process remains available for later readiness convergence.
- No Gemini/OpenAI/Anthropic/OpenRouter AI API is introduced by W5A.

Verification evidence:
- guarded migration assertions: PASS
- `gofmt`: PASS
- guarded package tests (`bootstrap + webai + host`): PASS
- full PR CI run #53: PASS
  - Ubuntu: format, installer syntax, `go vet`, full `go test`, critical race tests
  - Windows: format, `go vet`, full `go test`

W5A is locked. W5B may proceed; W5 overall remains OPEN until W5B–W5E and W5.5 pass.
