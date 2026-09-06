# W5A NO-API RUNTIME PROOF

This record is limited to the new WEB production path introduced in W5A.

When `web.enabled=true`:

1. `Config.ValidateBase` accepts the browser config without any AI API credential.
2. Legacy API provider maps are rejected rather than used.
3. `Host.New` starts/owns the browser session and calls `bootstrap.NewWebModelSet`.
4. `NewWebModelSet` constructs `GeminiWebTransport` + `webai.Model` only.
5. The web `ModelSet` has no role provider fallbacks and rejects provider/model swapping.
6. OpenRouter pricing refresh is skipped for WEB mode.
7. `Host.Close` stops the owned browser session.

Legacy API code still exists for migration compatibility at W5A and is scheduled for deletion/unreachability in W5B/W5C. W5A does not claim repository-wide NO-API PASS; that is the blocking W5.5 gate.
