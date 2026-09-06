from pathlib import Path
import re


def load(path):
    p = Path(path)
    return p, p.read_text(encoding="utf-8")


def save(p, text):
    p.write_text(text, encoding="utf-8")


def replace_once(text, old, new, label):
    if old not in text:
        raise SystemExit(f"missing expected block: {label}")
    return text.replace(old, new, 1)


# 1) Remove the dead API-era /config connection-test compatibility path.
p, s = load("internal/entry/tui/command_config.go")
s = s.replace('\t"context"\n', '', 1)
s = replace_once(
    s,
    '\tsaving     bool\n\ttesting    bool\n\ttestCancel context.CancelFunc // compatibility with the generic TUI event loop; unused in WEB-only /config\n',
    '\tsaving     bool\n',
    "model config testing fields",
)
s = re.sub(
    r'\n// modelConfigConnectionMsg is retained until W5C because model_update\.go has a\n// generic compatibility branch for the deleted API connection tester\. WEB-only\n// /config never emits this message\.\ntype modelConfigConnectionMsg struct \{\n\tmodel string\n\terr   error\n\}\n',
    '\n',
    s,
    count=1,
)
s = replace_once(s, 'if state.saving || state.testing {', 'if state.saving {', "saving/testing guard")
s = s.replace(
    'Cấu hình hiện tại chưa ở WEB-only. Provider/API cũ sẽ được xử lý ở W5C; /config không còn cho phép tạo hoặc sửa API credential.',
    'Cấu hình runtime không ở trạng thái WEB-only hợp lệ; hãy khởi động lại sau khi sửa cấu hình browser.',
)
s = s.replace(
    'Không thể lưu browser settings lên cấu hình legacy. Hãy hoàn tất migration WEB-only ở W5C.',
    'Không thể lưu browser settings khi WEB-only runtime chưa được bật.',
)
save(p, s)

p, s = load("internal/entry/tui/model_update.go")
block = '''\tcase modelConfigConnectionMsg:\n\t\tif m.modelConfig == nil {\n\t\t\treturn m, nil, true\n\t\t}\n\t\tm.modelConfig.testing = false\n\t\tm.modelConfig.testCancel = nil\n\t\tif errors.Is(msg.err, context.Canceled) {\n\t\t\tm.modelConfig.message = "Kiểm tra kết nối đã bị hủy"\n\t\t} else if msg.err != nil {\n\t\t\tm.modelConfig.message = msg.err.Error()\n\t\t} else {\n\t\t\tm.modelConfig.message = "Kiểm tra kết nối thành công: " + msg.model\n\t\t}\n\t\treturn m, nil, true\n'''
s = replace_once(s, block, '', "modelConfigConnectionMsg handler")
if 'context.' not in s:
    s = s.replace('\t"context"\n', '', 1)
if 'errors.' not in s:
    s = s.replace('\t"errors"\n', '', 1)
save(p, s)

# 2) Remove API-provider usage/prompt-cache semantics from active worker assembly.
p, s = load("internal/agents/build.go")
s = s.replace('\t"crypto/sha256"\n', '', 1)
s = s.replace('\t"encoding/hex"\n', '', 1)
s, n = re.subn(
    r'// promptCacheBase .*?\nfunc promptCacheBase\(bookDir string\) string \{\n.*?\n\}\n\n',
    '',
    s,
    count=1,
    flags=re.S,
)
if n != 1:
    raise SystemExit("promptCacheBase block not found")
s, n = re.subn(
    r'// UsageRecorder .*?\ntype UsageRecorder func\(agentName, task string, msg agentcore\.AgentMessage\)\n\n',
    '',
    s,
    count=1,
    flags=re.S,
)
if n != 1:
    raise SystemExit("UsageRecorder block not found")
s = replace_once(s, '\trecordUsage UsageRecorder,\n', '', "BuildWorkers usage parameter")
old = '''\tbaseOnMsg := store.Sessions.SubAgentLogger(modelLookup)\n\tonMsg := func(agentName, task string, msg agentcore.AgentMessage) {\n\t\tbaseOnMsg(agentName, task, msg)\n\t\tif recordUsage != nil {\n\t\t\trecordUsage(agentName, task, msg)\n\t\t}\n\t}\n\n\t// 提示词缓存：一书一基、一角色一名、一会话一键（subagent spawn 追加 #seq）。\n\t// OpenAI 系用 prompt_cache_key 做路由亲和；Claude 系用 cache_control 滚动断点\n\t//（system 地板 + 末消息尖端）。provider 不支持时由 agentcore 按能力静默丢弃，\n\t// 多轮会话下读缓存收益恒为正，故不设开关。\n\tcacheBase := promptCacheBase(store.Dir())\n\n'''
new = '''\tonMsg := store.Sessions.SubAgentLogger(modelLookup)\n\n'''
s = replace_once(s, old, new, "worker usage/cache setup")
s = re.sub(r'^\s*CacheLastMessage:\s*"ephemeral",\n', '', s, flags=re.M)
s = re.sub(r'^\s*PromptCacheKey:\s*cacheBase \+ "-[^"]+",\n', '', s, flags=re.M)
s = s.replace(
    '// Writer 的 ContextManager 由工厂每次调用重建，窗口随模型 swap 动态跟随（见下方工厂）。',
    '// Writer 的 ContextManager 由工厂每次调用重建；窗口来自 WEB-only 本地配置。',
)
s = s.replace(
    '// modelLookup 写入 session 时给每条 assistant 消息附 _meta:{provider,model}，\n\t// 让 replay 不再依赖"当前 ModelSet"来反推历史 cost，运行中切换模型也能精确算。',
    '// modelLookup 只给 session 写固定 WEB runtime identity，便于离线 provenance/replay。',
)
save(p, s)

p, s = load("internal/host/host.go")
s = replace_once(
    s,
    'workers, restore, applyThinking := agents.BuildWorkers(cfg, store, styleStats, models, bundle, nil,\n',
    'workers, restore, applyThinking := agents.BuildWorkers(cfg, store, styleStats, models, bundle,\n',
    "Host BuildWorkers call",
)
s = s.replace(
    '// exclusiveCancel 是当前独占作业的取消函数：预算硬停/手动暂停须能停掉正在烧钱的\n\t// 导入，而不仅是 Engine——abortWithEvent 在 Engine 未运行时取消它（预算哨兵的\n\t// abort 回调与手动 Abort 共用同一停机机制）。releaseExclusive 一并清空。',
    '// exclusiveCancel 是当前独占作业的取消函数：手动暂停/退出须能停止导入等后台作业，\n\t// 而不仅是 Engine。releaseExclusive 会一并清空。',
)
save(p, s)

p, s = load("internal/eval/collect.go")
s = s.replace('// UsageMetrics 是 meta/usage.json 中已有的可靠成本/token 事实。\n', '')
save(p, s)

# 3) Clearly classify legacy design records as historical; current authority stays WEB-only.
for path, title in [
    ("docs/engine-rfc.md", "Engine RFC"),
    ("docs/engine-arbiter.md", "Engine + Arbiter design record"),
]:
    p, s = load(path)
    marker = '> **HISTORICAL / SUPERSEDED BY W5 WEB-ONLY (2026-09-06).**'
    if marker not in s:
        first, rest = s.split('\n', 1)
        banner = (
            f"{first}\n\n{marker}\n"
            f"> {title} is retained as provenance only. Current product authority is `README.md` + `docs/architecture.md`. "
            "Provider failover, API prompt-cache/usage-budget semantics and runtime model switching described below are not current product behavior.\n"
        )
        s = banner + rest
    s = s.replace('当前架构见 README 架构节与 docs/engine-rfc.md。', '当前架构见 README 架构节与 docs/architecture.md。')
    save(p, s)

p, s = load("docs/refactor-flow-driven.md")
s = s.replace('当前设计见 `docs/engine-rfc.md`', '当前设计见 `docs/architecture.md`')
save(p, s)

# 4) Rewrite current evaluation truth: fixed WEB transport, no authoritative cost/token telemetry.
p, s = load("docs/evaluation-system.md")
s = s.replace(
    'A/B 的硬约束：同需求、同配置、同模型/provider、同风格、隔离输出目录。',
    'A/B 的硬约束：同需求、同配置、同一 `web/gemini-web` 浏览器运行时、同风格、隔离输出目录。',
)
s = s.replace(
    '   ├── usage / cost / token      → 从 meta/usage.json 读\n',
    '   ├── WEB runtime identity      → 固定 `web/gemini-web`（不生成账单/token/cache 估算）\n',
)
old = '> **当前实现覆盖确定性主线**：无 `--variant` 时为 `mode=single`；传 `--variant` 时为 `mode=ab`，同一 case 隔离运行 baseline 与 variant，并生成 delta。Collectors 已接 `diag.Diagnose`、case 契约、`stylestat.Compute`、`meta/usage.json`、session tool call 计数；Graders 已接确定性门禁、baseline/variant diag delta、cost/token/tool call delta、stylestat delta。Runner 直接 `host.New` 装配并自带章数上限截停，**不复用无章数上限的 `headless.Run`**。LLM Judge 与 Human 仍是后续可选层，不参与当前确定性门禁。'
new = '> **当前实现覆盖确定性主线**：无 `--variant` 时为 `mode=single`；传 `--variant` 时为 `mode=ab`，同一 case 隔离运行 baseline 与 variant，并生成 delta。Collectors 已接 `diag.Diagnose`、case 契约、`stylestat.Compute`、session tool call 计数；Graders 已接确定性门禁、baseline/variant diag delta、tool-call delta、stylestat delta。Runner 直接 `host.New` 装配并自带章数上限截停，**不复用无章数上限的 `headless.Run`**。Gemini Web 不向本地桥提供权威账单、token 或 provider-cache 遥测，因此评测系统不采集、不推算、也不以这些数值做门禁。LLM Judge 与 Human 仍是后续可选层，不参与当前确定性门禁。'
s = replace_once(s, old, new, "evaluation current implementation paragraph")
save(p, s)

# 5) Fix two audit-classification false positives discovered by CI #165.
p, s = load("scripts/w55-no-api-audit.sh")
s = s.replace('require_file internal/webai/gemini_transport.go\n', 'require_file internal/webai/devtools.go\n')
s = s.replace("  'role[ -]model.*failover|provider.*failover'; do\n", "; do\n")
save(p, s)

print('W5.5 bounded cleanup patch applied')
