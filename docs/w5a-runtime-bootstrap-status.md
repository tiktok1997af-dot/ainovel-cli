# W5A — WEB-ONLY RUNTIME BOOTSTRAP MIGRATION

Status: IMPLEMENTED — awaiting full PR CI.

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

Guarded migration evidence:
- migration assertions: PASS
- `gofmt`: PASS
- `go test ./internal/bootstrap ./internal/webai ./internal/host`: PASS

Blocking gate before W5A LOCKED:
- full pull-request CI on Linux + Windows must PASS.
