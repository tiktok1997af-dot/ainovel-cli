#!/usr/bin/env bash
set -euo pipefail

# W5.5 repository-wide WEB-only / NO-API gate.
# Historical provenance may describe removed API-era behavior, but active code,
# scripts, workflows and current-product docs must not expose an executable or
# supported AI API/provider/Ollama path.

failed=0

fail() {
  printf '::error::%s\n' "$*"
  failed=1
}

require_file() {
  local path="$1"
  [ -f "$path" ] || fail "required WEB-only file missing: $path"
}

require_text() {
  local path="$1" needle="$2"
  grep -Fq -- "$needle" "$path" || fail "$path missing required invariant: $needle"
}

reject_git_grep() {
  local label="$1" pattern="$2"
  shift 2
  local out
  if out="$(git grep -nEI -- "$pattern" "$@" 2>/dev/null)"; then
    printf '%s\n' "$out"
    fail "$label"
  fi
}

printf 'W5.5 NO-API audit: active executable surfaces\n'

# Unsupported/tactical artefacts must not return to the product tree.
for path in \
  Dockerfile docker-compose.yml .dockerignore \
  .github/workflows/docker.yml \
  .github/workflows/w2-verifier.yml .github/workflows/w3-verifier.yml .github/workflows/w4-verifier.yml \
  cmd/ainovel-w2-verify cmd/ainovel-w3-verify cmd/ainovel-w4-verify \
  scripts/w2-verify-windows.cmd scripts/w3-verify-windows.cmd scripts/w4-verify-windows.cmd; do
  [ ! -e "$path" ] || fail "unsupported/tactical artefact still exists: $path"
done

# Direct AI API construction, credentials/endpoints, failover and Ollama execution.
reject_git_grep 'direct llm.NewModel constructor remains in active Go' 'llm\.NewModel[[:space:]]*\(' -- 'cmd/**/*.go' 'internal/**/*.go' ':!*_test.go'
reject_git_grep 'API key/base URL model options remain in active Go' 'With(APIKey|BaseURL)[[:space:]]*\(' -- 'cmd/**/*.go' 'internal/**/*.go' ':!*_test.go'
reject_git_grep 'provider failover runtime remains in active Go' 'ForRoleWithFailover|failoverModel|FallbackModels|ProviderFallback' -- 'cmd/**/*.go' 'internal/**/*.go' ':!*_test.go'
reject_git_grep 'application code directly imports litellm' 'github\.com/voocel/litellm' -- 'cmd/**/*.go' 'internal/**/*.go' ':!*_test.go'
reject_git_grep 'AI API credential environment variable remains executable' 'OPENAI_API_KEY|ANTHROPIC_API_KEY|GEMINI_API_KEY|OPENROUTER_API_KEY' -- 'cmd/**' 'internal/**' 'scripts/**' '.github/scripts/**' '.github/workflows/release.yml' ':!scripts/w55-no-api-audit.sh'
reject_git_grep 'AI API endpoint remains executable' 'api\.openai\.com|api\.anthropic\.com|openrouter\.ai/api|generativelanguage\.googleapis\.com' -- 'cmd/**' 'internal/**' 'scripts/**' '.github/scripts/**' '.github/workflows/release.yml' ':!scripts/w55-no-api-audit.sh'
reject_git_grep 'Ollama execution path remains executable' 'ollama[[:space:]]+(serve|run|pull|create)|localhost:11434|/api/(chat|generate)' -- 'cmd/**' 'internal/**' 'scripts/**' '.github/**' ':!scripts/w55-no-api-audit.sh'
reject_git_grep 'upstream release/image path remains executable' 'github\.com/voocel/ainovel-cli/releases|ghcr\.io/voocel/ainovel-cli' -- 'cmd/**' 'internal/**' 'scripts/**' '.github/**' ':!scripts/w55-no-api-audit.sh'

# API-era compiled compatibility semantics are forbidden even when currently unreachable.
reject_git_grep 'legacy model connection-test message remains compiled' 'modelConfigConnectionMsg|Kiểm tra kết nối thành công' -- 'internal/**/*.go' ':!*_test.go'
reject_git_grep 'API-era usage/prompt-cache worker semantics remain compiled' 'UsageRecorder|PromptCacheKey|CacheLastMessage' -- 'internal/**/*.go' ':!*_test.go'

printf 'W5.5 NO-API audit: positive WEB-only invariants\n'
require_file internal/bootstrap/models.go
require_file internal/host/web_runtime.go
require_file internal/webai/model.go
require_file internal/webai/devtools.go
require_text internal/bootstrap/models.go 'webai.NewGeminiWebTransport'
require_text internal/bootstrap/models.go 'webai.NewModel'
require_text internal/host/host.go 'legacy AI provider/API runtime has been removed'
require_text internal/bootstrap/config.go 'WebProviderName      = "web"'
require_text internal/bootstrap/config.go 'WebModelName         = "gemini-web"'
require_text internal/bootstrap/configfile.go 'LegacyAPIMigrationHint'
require_text internal/version/repository.go 'const ProductRepository = "tiktok1997af-dot/ainovel-cli"'
require_text scripts/install.sh 'REPO="tiktok1997af-dot/ainovel-cli"'
require_text .github/workflows/release.yml "github.repository == 'tiktok1997af-dot/ainovel-cli'"

printf 'W5.5 NO-API audit: current-product docs\n'
current_docs=(
  README.md
  HUONG_DAN_SU_DUNG.md
  docs/architecture.md
  docs/context-management.md
  docs/import-pipeline.md
  docs/observability.md
  docs/evaluation-system.md
  docs/chapter-advance-gate.md
  docs/user-rules-runtime.md
  docs/voice-layer.md
)
for path in "${current_docs[@]}"; do
  require_file "$path"
done

# Current docs may explain that API support was removed, but must not contain
# runnable credential/provider/Ollama/Docker setup or claim authoritative WEB billing telemetry.
for pattern in \
  '"api_key"[[:space:]]*:' \
  'OPENAI_API_KEY|ANTHROPIC_API_KEY|GEMINI_API_KEY|OPENROUTER_API_KEY' \
  'ollama[[:space:]]+(serve|run|pull|create)' \
  'docker[[:space:]]+compose[[:space:]]+(build|run|up|pull|start)([[:space:]]|$)' \
  'meta/usage\.json' \
  'UsageTracker|usageTrackedModel' \
; do
  if grep -nEi -- "$pattern" "${current_docs[@]}"; then
    fail "stale current-product documentation matched: $pattern"
  fi
done

# Old design records are allowed only when clearly marked superseded.
for path in docs/engine-rfc.md docs/engine-arbiter.md docs/refactor-flow-driven.md; do
  require_file "$path"
  head -n 12 "$path" | grep -Eqi 'HISTORICAL|历史|SUPERSEDED|废弃' || fail "$path is legacy design material but lacks a historical/superseded banner"
done

# D1 truth: Gemini Web does not expose authoritative provider billing/cache telemetry.
if grep -nEi 'authoritative.*(cost|billing|token|cache)|精确.*(成本|token|缓存)|可靠成本/token' docs/evaluation-system.md; then
  fail 'evaluation-system still claims authoritative API-era cost/token/cache telemetry'
fi

printf 'W5.5 NO-API audit: dependency classification\n'
# litellm may remain only as a transitive dependency of agentcore. Direct app imports are forbidden above.
if grep -Eq '^\s*github\.com/voocel/litellm\s+.*// indirect\s*$' go.mod; then
  printf 'INFO: litellm is present only as an indirect module dependency; no direct application import matched.\n'
elif grep -Eq 'github\.com/voocel/litellm' go.mod; then
  fail 'litellm is present in go.mod but is not classified as indirect'
fi

if [ "$failed" -ne 0 ]; then
  printf 'W5.5 NO-API audit: FAIL\n'
  exit 1
fi
printf 'W5.5 NO-API audit: PASS\n'
