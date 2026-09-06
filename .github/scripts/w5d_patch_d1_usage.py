from pathlib import Path
import json
import re


def read(path):
    return Path(path).read_text(encoding="utf-8")


def write(path, text):
    Path(path).write_text(text, encoding="utf-8")


def must_replace(text, old, new, label, count=1):
    got = text.count(old)
    if got != count:
        raise SystemExit(f"{label}: expected {count} matches, got {got}")
    return text.replace(old, new, count)


def find_block_end(text, start):
    brace = text.find("{", start)
    if brace < 0:
        raise SystemExit("opening brace not found")
    depth = 0
    in_str = False
    esc = False
    i = brace
    while i < len(text):
        ch = text[i]
        if in_str:
            if esc:
                esc = False
            elif ch == "\\":
                esc = True
            elif ch == '"':
                in_str = False
        else:
            if ch == '"':
                in_str = True
            elif ch == "{":
                depth += 1
            elif ch == "}":
                depth -= 1
                if depth == 0:
                    return i + 1
        i += 1
    raise SystemExit("unterminated brace block")


def remove_func(text, name, required=True):
    m = re.search(r"(?m)^func(?:\s+\([^\n)]*\))?\s+" + re.escape(name) + r"\s*\(", text)
    if not m:
        if required:
            raise SystemExit(f"function {name} not found")
        return text
    start = m.start()
    end = find_block_end(text, m.start())
    while end < len(text) and text[end] == "\n":
        end += 1
    return text[:start] + text[end:]


def replace_func(text, name, replacement):
    m = re.search(r"(?m)^func(?:\s+\([^\n)]*\))?\s+" + re.escape(name) + r"\s*\(", text)
    if not m:
        raise SystemExit(f"function {name} not found")
    start = m.start()
    end = find_block_end(text, m.start())
    while end < len(text) and text[end] == "\n":
        end += 1
    return text[:start] + replacement.rstrip() + "\n\n" + text[end:]


def remove_struct_type(text, name, required=True):
    m = re.search(r"(?m)^type " + re.escape(name) + r"\s+struct\s*\{", text)
    if not m:
        if required:
            raise SystemExit(f"struct {name} not found")
        return text
    start = m.start()
    end = find_block_end(text, m.start())
    while end < len(text) and text[end] == "\n":
        end += 1
    return text[:start] + text[end:]


# --- Host: remove API-era usage accounting and expose truthful WEB-only telemetry status.
host_path = "internal/host/host.go"
host = read(host_path)
host = must_replace(host, "\tusage           *UsageTracker\n\tusageCancel     context.CancelFunc  // 停掉 autoSaveLoop 并触发最后一次 flush\n", "", "host usage fields")
start = host.find("\n\tusage := NewUsageTracker(models, store)\n")
end = host.find("\n\t// onGuardBlock 前置声明", start)
if start < 0 or end < 0:
    raise SystemExit("host usage startup block markers not found")
host = host[:start] + host[end:]
host = must_replace(host, "models, bundle, usage.Record,", "models, bundle, nil,", "worker usage callback")
host = host.replace("\t\tusage:           usage,\n", "").replace("\t\tusageCancel:     usageCancel,\n", "")
host = must_replace(host, 'arbiterModel:    newUsageTrackedModel(models.Default, "arbiter", usage.Record),', 'arbiterModel:    models.Default,', "engine arbiter usage wrapper")
host = replace_func(host, "arbiterModel", '''func (h *Host) arbiterModel() agentcore.ChatModel {
\treturn h.models.Default
}''')

# Remove shutdown persistence block.
host, n = re.subn(r'''\n\t\tif h\.usageCancel != nil \{\n\t\t\th\.usageCancel\(\)\n\t\t\th\.usageCancel = nil\n\t\t\}\n\t\th\.usage\.WaitAutoSave\(\)\n\t\tif err := h\.usage\.SaveNow\(\); err != nil \{\n\t\t\tslog\.Warn\("usage 退出前落盘失败", "module", "usage", "err", err\)\n\t\t\}\n''', "\n", host)
if n != 1:
    raise SystemExit(f"host shutdown usage block expected 1, got {n}")

# Run-end notifications must not invent dollar totals.
host = replace_func(host, "runEndBody", '''func (h *Host) runEndBody(title, summary string) string {
\tif name := strings.TrimSpace(title); name != "" {
\t\tsummary = "《" + name + "》" + summary
\t}
\treturn summary
}''')
host = host.replace("// runEndBody 组装 run_end 通知正文：书名 + 进度摘要 + 累计花费。", "// runEndBody 组装 run_end 通知正文；WEB-only 不伪造 token/美元账单。")

# Snapshot: remove every numeric usage/cache projection and replace with explicit unavailable status.
start = host.find("\n\t// 动态解析当前模型的上下文窗口")
end = host.find("\n\tsnap := UISnapshot{", start)
if start < 0 or end < 0:
    raise SystemExit("snapshot usage prelude markers not found")
host = host[:start] + "\n" + host[end:]
for field in [
    "TotalInputTokens", "TotalOutputTokens", "TotalCacheReadTokens", "TotalCacheWriteTokens",
    "TotalCostUSD", "TotalSavedUSD", "BudgetLimitUSD", "OverallCacheCapable",
    "OverallRecentCacheRead", "OverallRecentInput", "OverallRecentSamples", "TotalCacheBreaks",
    "CachePerAgent", "CachePerModel", "MissingAssistantUsage",
]:
    host, cnt = re.subn(r"(?m)^\t\t" + field + r":.*\n", "", host)
    if cnt != 1:
        raise SystemExit(f"snapshot field {field}: expected 1, got {cnt}")
anchor = "\t\tIsRunning:              state == lifecycleRunning,\n"
host = must_replace(host, anchor, anchor + "\t\tAITelemetryStatus:      WebAITelemetryUnavailable,\n", "snapshot telemetry status")
host = must_replace(host, '\tmodel = newUsageTrackedModel(model, "editor", h.usage.Record)\n', '', "revision usage wrapper")
host = must_replace(host, '\tmodel = newUsageTrackedModel(model, role, h.usage.Record)\n', '', "import usage wrapper")
if "h.usage" in host or "UsageTracker" in host or "newUsageTrackedModel" in host:
    raise SystemExit("host still references API-era usage tracker")
write(host_path, host)

# --- Host UI contract.
events_path = "internal/host/events.go"
events = read(events_path)
start = events.find("\n\t// 累计用量")
end = events.find("\n\t// 基础设定", start)
if start < 0 or end < 0:
    raise SystemExit("UISnapshot usage block markers not found")
events = events[:start] + "\n\t// Gemini Web does not expose authoritative billing/token/cache telemetry to the browser bridge.\n\tAITelemetryStatus string\n" + events[end:]
events = remove_struct_type(events, "AgentCacheStat")
events = re.sub(r'(?ms)^// AgentCacheStat .*?(?=// AgentContextSnapshot)', '', events)
const_anchor = "// UISnapshot 是 TUI 渲染所需的聚合状态快照。\n"
telemetry_const = '''// WebAITelemetryUnavailable states the WEB-only telemetry boundary explicitly.
// The browser bridge observes visible page interaction, not provider billing APIs.
const WebAITelemetryUnavailable = "Gemini Web không cung cấp số token, chi phí billing hoặc cache telemetry đáng tin cậy cho browser bridge."

'''
events = must_replace(events, const_anchor, telemetry_const + const_anchor, "telemetry constant")
for bad in ["TotalCostUSD", "BudgetLimitUSD", "AgentCacheStat", "MissingAssistantUsage", "stream_options.include_usage"]:
    if bad in events:
        raise SystemExit(f"events.go residue: {bad}")
write(events_path, events)

# --- Store/domain: remove durable API billing state.
store_path = "internal/store/store.go"
st = read(store_path)
st = must_replace(st, "\tUsage          *UsageStore\n", "", "Store.Usage field")
st = must_replace(st, "\t\tUsage:          NewUsageStore(newIO(dir)),\n", "", "Store.Usage init")
write(store_path, st)

for path in [
    "internal/domain/usage.go",
    "internal/store/usage.go",
    "internal/store/usage_test.go",
    "internal/host/usage.go",
    "internal/host/usage_replay.go",
    "internal/host/usage_test.go",
    "internal/host/arbiter_model.go",
    "internal/host/arbiter_model_test.go",
    "internal/models/gen_models.go",
    "internal/models/model_lookup.go",
    "internal/models/models_generated.go",
    "internal/models/pricing.go",
    "internal/models/registry.go",
]:
    p = Path(path)
    if not p.exists():
        raise SystemExit(f"expected legacy file missing before patch: {path}")
    p.unlink()

# --- TUI status bar: model/session identity only, no fake billing numbers.
status_path = "internal/entry/tui/statusbar.go"
status = read(status_path)
status = replace_func(status, "renderStatusBar", '''func renderStatusBar(snap host.UISnapshot, outputDir string, width int) string {
\tdim := lipgloss.NewStyle().Foreground(colorDim)
\tval := lipgloss.NewStyle().Foreground(colorMuted)

\tvar segs []string
\tif snap.ModelName != "" {
\t\ts := lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render("◆") + " "
\t\tif snap.Provider != "" {
\t\t\ts += dim.Render(snap.Provider) + " "
\t\t}
\t\ts += val.Render(snap.ModelName)
\t\tif suffix := modelInfoSuffix(snap); suffix != "" {
\t\t\ts += dim.Render("(" + suffix + ")")
\t\t}
\t\tsegs = append(segs, s)
\t}

\tleft := strings.Join(segs, dim.Render(" │ "))
\tvar right string
\tif outputDir != "" {
\t\tright = dim.Render("./" + filepath.Base(outputDir))
\t}
\tif left == "" && right == "" {
\t\treturn dim.Render("SẴN SÀNG")
\t}
\treturn joinInlineSides(left, right, width)
}''')
status = re.sub(r'(?s)// renderStatusBar 渲染屏幕最底部的用量状态栏.*?\nfunc renderStatusBar', '// renderStatusBar shows only truthful WEB-only runtime identity; Gemini Web does not expose authoritative token/billing telemetry.\nfunc renderStatusBar', status, count=1)
write(status_path, status)

# Sidebar: replace cost/cache panels with one explicit WEB telemetry boundary.
sidebar_path = "internal/entry/tui/panels_sidebar.go"
side = read(sidebar_path)
old = '''\tif body := renderUsageSidebar(snap, contentW); body != "" {
\t\tsections = append(sections, renderSidebarSection("Tài Nguyên Đã Dùng", body, contentW))
\t}

\tif body := renderCacheSidebar(snap, contentW); body != "" {
\t\tsections = append(sections, renderSidebarSection("Bộ Nhớ Đệm (Cache)", body, contentW))
\t}
'''
new = '''\tif body := renderWebTelemetrySidebar(snap, contentW); body != "" {
\t\tsections = append(sections, renderSidebarSection("AI Web", body, contentW))
\t}
'''
side = must_replace(side, old, new, "sidebar usage/cache sections")
for fn in ["renderUsageSidebar", "usageStatsByCost", "renderUsageGroupHeader", "renderUsageLine", "renderCacheSidebar", "renderCacheCategory", "renderCacheAgent", "renderCacheAgentLine", "colorPercent", "formatTokensCompact", "cacheHitRate", "formatPercent", "cacheHitColor"]:
    side = remove_func(side, fn, required=False)
side += '''\nfunc renderWebTelemetrySidebar(snap host.UISnapshot, width int) string {
\tstatus := strings.TrimSpace(snap.AITelemetryStatus)
\tif status == "" {
\t\treturn ""
\t}
\treturn lipgloss.NewStyle().Foreground(colorDim).Width(max(8, width-2)).Render(status)
}
'''
if "AgentCacheStat" in side or "TotalCostUSD" in side or "BudgetLimitUSD" in side or "MissingAssistantUsage" in side:
    raise SystemExit("sidebar still contains API usage contract")
write(sidebar_path, side)

# layout helper becomes dead once dollar UI is removed.
layout_path = "internal/entry/tui/layout.go"
layout = read(layout_path)
layout = remove_func(layout, "formatCostUSD", required=False)
write(layout_path, layout)

# TUI tests lock WEB-only truth.
panels_test_path = "internal/entry/tui/panels_test.go"
pt = read(panels_test_path)
pt = pt.replace('Provider:  "openrouter",\n\t\tModelName: "test-model",', 'Provider:  "web",\n\t\tModelName: "gemini-web",')
pt = replace_func(pt, "TestRenderStatusBar", '''func TestRenderStatusBar(t *testing.T) {
\tout := ansi.Strip(renderStatusBar(host.UISnapshot{
\t\tProvider:           "web",
\t\tModelName:          "gemini-web",
\t\tModelContextWindow: 200000,
\t\tThinkingLevel:      "medium",
\t}, "/tmp/output", 120))
\tfor _, want := range []string{"web", "gemini-web(200K,med)", "./output"} {
\t\tif !strings.Contains(out, want) {
\t\t\tt.Fatalf("WEB-only status bar missing %q: %q", want, out)
\t\t}
\t}
\tfor _, forbidden := range []string{"$", "↑", "↓", "tiết kiệm"} {
\t\tif strings.Contains(out, forbidden) {
\t\t\tt.Fatalf("WEB-only status bar must not fabricate billing/usage %q: %q", forbidden, out)
\t\t}
\t}
}''')
pt = remove_func(pt, "TestRenderUsageLineSeparatesFullWidthNameAndTokens")
pt += '''\nfunc TestRenderWebTelemetrySidebarIsExplicit(t *testing.T) {
\tout := ansi.Strip(renderWebTelemetrySidebar(host.UISnapshot{AITelemetryStatus: host.WebAITelemetryUnavailable}, 48))
\tif !strings.Contains(out, "không cung cấp") || !strings.Contains(out, "billing") {
\t\tt.Fatalf("WEB telemetry boundary missing: %q", out)
\t}
\tfor _, forbidden := range []string{"$0", "0%"} {
\t\tif strings.Contains(out, forbidden) {
\t\t\tt.Fatalf("WEB telemetry notice must not imply measured zero %q: %q", forbidden, out)
\t\t}
\t}
}
'''
write(panels_test_path, pt)

# --- Eval: keep deterministic quality/tool/duration facts, remove API billing/token gates.
collect_path = "internal/eval/collect.go"
col = read(collect_path)
col = col.replace("\tUsage       UsageMetrics\n", "")
col = remove_struct_type(col, "UsageMetrics")
col = must_replace(col, "\tusage := collectUsage(s, check)\n", "", "collect usage call")
col = must_replace(col, "\t\tUsage:       usage,\n", "", "collected usage field")
col = remove_func(col, "collectUsage")
write(collect_path, col)

grade_path = "internal/eval/grade.go"
grade = read(grade_path)
grade = grade.replace("\tUsage             UsageMetrics     `json:\"usage\"`\n", "")
for line in [
    '\tCostDeltaRatio        float64     `json:"cost_delta_ratio,omitempty"`\n',
    '\tInputTokenDeltaRatio  float64     `json:"input_token_delta_ratio,omitempty"`\n',
    '\tOutputTokenDeltaRatio float64     `json:"output_token_delta_ratio,omitempty"`\n',
]:
    grade = must_replace(grade, line, "", f"grade field {line.strip()}")
for pattern, label in [
    (r'''\tif deltaGateEnabled\(c\.Gate\.MaxCostDeltaRatio\) && d\.Metrics\.CostDeltaRatio > \*c\.Gate\.MaxCostDeltaRatio \{\n\t\twarn\("delta:cost", fmt\.Sprintf\("成本增幅 %.1f%% 超过阈值 %.1f%%",\n\t\t\td\.Metrics\.CostDeltaRatio\*100, \*c\.Gate\.MaxCostDeltaRatio\*100\)\)\n\t\}\n''', "cost gate"),
    (r'''\tif deltaGateEnabled\(c\.Gate\.MaxCostDeltaRatio\) && d\.Metrics\.InputTokenDeltaRatio > \*c\.Gate\.MaxCostDeltaRatio \{\n\t\twarn\("delta:input_tokens", fmt\.Sprintf\("输入 token 增幅 %.1f%% 超过阈值 %.1f%%",\n\t\t\td\.Metrics\.InputTokenDeltaRatio\*100, \*c\.Gate\.MaxCostDeltaRatio\*100\)\)\n\t\}\n''', "input token gate"),
    (r'''\tif deltaGateEnabled\(c\.Gate\.MaxCostDeltaRatio\) && d\.Metrics\.OutputTokenDeltaRatio > \*c\.Gate\.MaxCostDeltaRatio \{\n\t\twarn\("delta:output_tokens", fmt\.Sprintf\("输出 token 增幅 %.1f%% 超过阈值 %.1f%%",\n\t\t\td\.Metrics\.OutputTokenDeltaRatio\*100, \*c\.Gate\.MaxCostDeltaRatio\*100\)\)\n\t\}\n''', "output token gate"),
]:
    grade, cnt = re.subn(pattern, "", grade)
    if cnt != 1:
        raise SystemExit(f"{label}: expected 1, got {cnt}")
for line in [
    "\t\tCostDeltaRatio:        deltaRatioFloat(vm.Usage.CostUSD, bm.Usage.CostUSD),\n",
    "\t\tInputTokenDeltaRatio:  deltaRatio(vm.Usage.Input, bm.Usage.Input),\n",
    "\t\tOutputTokenDeltaRatio: deltaRatio(vm.Usage.Output, bm.Usage.Output),\n",
]:
    grade = must_replace(grade, line, "", "delta metrics usage line")
grade = remove_func(grade, "deltaRatioFloat")
write(grade_path, grade)

case_path = "internal/eval/case.go"
case = read(case_path)
case = must_replace(case, '\tMaxCostDeltaRatio     *float64 `json:"max_cost_delta_ratio,omitempty"`\n', "", "case cost gate field")
case = must_replace(case, "\tif c.Gate.MaxCostDeltaRatio == nil {\n\t\tc.Gate.MaxCostDeltaRatio = float64Ptr(defaultDeltaRatio)\n\t}\n", "", "case cost gate default")
write(case_path, case)

report_path = "internal/eval/report.go"
rep = read(report_path)
rep = must_replace(rep, '\tCostUSD         *RangeSummary `json:"cost_usd,omitempty"`\n', "", "repeat cost summary")
rep = must_replace(rep, "\t\tvar costs, toolCalls []float64\n", "\t\tvar toolCalls []float64\n", "report range vars")
rep, cnt = re.subn(r'''\t\t\tif run\.Result\.Metrics\.Usage\.UsageRecorded \{\n\t\t\t\tcosts = append\(costs, run\.Result\.Metrics\.Usage\.CostUSD\)\n\t\t\t\}\n''', "", rep)
if cnt != 1:
    raise SystemExit(f"report usage accumulation expected 1 got {cnt}")
rep = must_replace(rep, "\t\ts.CostUSD = summarizeRange(costs)\n", "", "report cost summary assignment")
rep, cnt = re.subn(r'''\t\tif c\.Summary\.CostUSD != nil \{\n\t\t\tr := c\.Summary\.CostUSD\n\t\t\tfmt\.Fprintf\(&b, "- cost_usd: min=%.2f avg=%.2f max=%.2f\\n", r\.Min, r\.Avg, r\.Max\)\n\t\t\}\n''', "", rep)
if cnt != 1:
    raise SystemExit(f"markdown cost block expected 1 got {cnt}")
rep, cnt = re.subn(r'''\tif m\.Usage\.UsageRecorded \{\n\t\tfmt\.Fprintf\(b, " cost=\$%.4f tokens\(in=%d out=%d\)", m\.Usage\.CostUSD, m\.Usage\.Input, m\.Usage\.Output\)\n\t\}\n''', "", rep)
if cnt != 1:
    raise SystemExit(f"writeRun usage block expected 1 got {cnt}")
rep = replace_func(rep, "writeDelta", '''func writeDelta(b *strings.Builder, idx int, d Delta) {
\tm := d.Metrics
\tfmt.Fprintf(b, "- delta#%d: %s completed_delta=%+d crit_delta=%+d warn_delta=%+d words_ratio=%.2f tool_calls_delta=%.2f\\n",
\t\tidx, d.Outcome, m.CompletedChapters, m.CriticalFindings, m.WarningFindings,
\t\tm.TotalWordsRatio, m.ToolCallDeltaRatio)
\tif m.Stylestat != nil {
\t\tsd := m.Stylestat
\t\tfmt.Fprintf(b, "  - stylestat: %s pattern_top=%+0.1f ending_short=%+0.2f repeated=%+d title_mixed=%+d\\n",
\t\t\tsd.Status, sd.PatternTopPerChapter, sd.EndingShortRatio, sd.RepeatedSentences, sd.TitleMixedDelta)
\t}
\twriteIssues(b, "Delta Hard Fail", d.HardFails)
\twriteIssues(b, "Delta Warnings", d.Warnings)
\twriteIssues(b, "Delta Notes", d.Notes)
}''')
write(report_path, rep)

# Eval tests.
ct_path = "internal/eval/collect_test.go"
ct = read(ct_path)
ct, cnt = re.subn(r'''\tif err := s\.Usage\.Save\(domain\.UsageState\{\n\t\tOverall:\s+domain\.AgentUsageTotals\{Input: 100, Output: 40, Cost: 0\.12\},\n\t\tMissingUsage: 1,\n\t\}\); err != nil \{\n\t\tt\.Fatalf\("save usage: %v", err\)\n\t\}\n''', "", ct)
if cnt != 1:
    raise SystemExit(f"collect test usage save expected 1 got {cnt}")
ct, cnt = re.subn(r'''\tif !col\.Usage\.UsageRecorded \|\| col\.Usage\.Input != 100 \|\| col\.Usage\.Output != 40 \|\| col\.Usage\.CostUSD != 0\.12 \{\n\t\tt\.Fatalf\("usage 读取不正确: %\+v", col\.Usage\)\n\t\}\n''', "", ct)
if cnt != 1:
    raise SystemExit(f"collect test usage assert expected 1 got {cnt}")
ct = ct.replace("TestCollectReadsStyleUsageAndToolCalls", "TestCollectReadsStyleAndToolCalls")
write(ct_path, ct)

gt_path = "internal/eval/grade_test.go"
gt = read(gt_path)
gt = replace_func(gt, "TestGradeDeltaCostAndToolCallThresholds", '''func TestGradeDeltaToolCallThreshold(t *testing.T) {
\tbase := cleanResult()
\tbase.Metrics.ToolCalls = 10
\tvariant := cleanResult()
\tvariant.Metrics.ToolCalls = 14

\tc := writerSmokeCase()
\tc.Gate.MaxToolCallDeltaRatio = float64Ptr(0.3)
\td := GradeDelta(c, base, variant)
\tif d.Outcome != Warn {
\t\tt.Fatalf("tool_calls 超阈值应 WARN，得到 %s", d.Outcome)
\t}
\tif !hasIssue(d.Warnings, "delta:tool_calls", "超过阈值") {
\t\tt.Fatalf("应报告 tool_calls 回归，实际 %+v", d.Warnings)
\t}
}''')
write(gt_path, gt)

case_test_path = "internal/eval/case_test.go"
cst = read(case_test_path)
cst = cst.replace('if c.Gate.MaxCostDeltaRatio == nil || *c.Gate.MaxCostDeltaRatio != 0.3 ||\n\t\t\tc.Gate.MaxToolCallDeltaRatio == nil || *c.Gate.MaxToolCallDeltaRatio != 0.3 {\n\t\t\tt.Errorf("%s: Validate 应填充默认 delta ratio，得到 cost=%v tool=%v",\n\t\t\t\tc.ID, c.Gate.MaxCostDeltaRatio, c.Gate.MaxToolCallDeltaRatio)\n\t\t}', 'if c.Gate.MaxToolCallDeltaRatio == nil || *c.Gate.MaxToolCallDeltaRatio != 0.3 {\n\t\t\tt.Errorf("%s: Validate 应填充默认 tool delta ratio，得到 %v", c.ID, c.Gate.MaxToolCallDeltaRatio)\n\t\t}')
cst = replace_func(cst, "TestCaseRejectsInvalidGate", '''func TestCaseRejectsInvalidGate(t *testing.T) {
\tc := Case{ID: "bad_gate", Prompt: "x", Gate: Gate{StylestatRegression: "maybe"}}
\tif err := c.Validate(); err == nil {
\t\tt.Fatal("非法 stylestat_regression 应被拒")
\t}
\tc = Case{ID: "disabled_ratio", Prompt: "x", Gate: Gate{MaxToolCallDeltaRatio: float64Ptr(-1)}}
\tif err := c.Validate(); err != nil {
\t\tt.Fatalf("负数 tool delta ratio 应作为显式关闭被接受: %v", err)
\t}
\tif *c.Gate.MaxToolCallDeltaRatio != -1 {
\t\tt.Fatalf("显式关闭的 tool delta ratio 不应被默认值覆盖: %+v", c.Gate)
\t}
\tc = Case{ID: "strict_ratio", Prompt: "x", Gate: Gate{MaxToolCallDeltaRatio: float64Ptr(0)}}
\tif err := c.Validate(); err != nil {
\t\tt.Fatalf("显式 0 tool delta ratio 应作为严格阈值被接受: %v", err)
\t}
\tif *c.Gate.MaxToolCallDeltaRatio != 0 {
\t\tt.Fatalf("显式 0 tool delta ratio 不应被默认值覆盖: %+v", c.Gate)
\t}
}''')
if "MaxCostDeltaRatio" in cst:
    raise SystemExit("case_test still references cost gate")
write(case_test_path, cst)

rt_path = "internal/eval/report_test.go"
rt = read(rt_path)
rt = rt.replace("\tpass.Metrics.Usage = UsageMetrics{UsageRecorded: true, CostUSD: 0.1}\n", "")
rt = rt.replace("\tif cr.Summary.CostUSD == nil || cr.Summary.CostUSD.Avg != 0.1 ||\n\t\tcr.Summary.ToolCalls == nil || cr.Summary.ToolCalls.Avg != 10 {", "\tif cr.Summary.ToolCalls == nil || cr.Summary.ToolCalls.Avg != 10 {")
write(rt_path, rt)

# Smoke cases: cost gate is invalid in WEB-only evaluation.
for p in Path("evals/cases").rglob("*.json"):
    data = json.loads(p.read_text(encoding="utf-8"))
    gate = data.get("gate")
    if isinstance(gate, dict):
        gate.pop("max_cost_delta_ratio", None)
    p.write_text(json.dumps(data, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")

# Active-source residue must be gone before the guarded workflow is allowed to commit.
residue_terms = [
    "github.com/voocel/ainovel-cli/internal/models",
    "TotalCostUSD", "TotalSavedUSD", "BudgetLimitUSD", "MissingAssistantUsage",
    "CachePerAgent", "CachePerModel", "stream_options.include_usage",
    "cost_usd", "saved_usd", "max_cost_delta_ratio", "CostDeltaRatio",
    "InputTokenDeltaRatio", "OutputTokenDeltaRatio",
]
for root in [Path("internal"), Path("evals")]:
    for p in root.rglob("*"):
        if not p.is_file() or p.suffix not in {".go", ".json"}:
            continue
        text = p.read_text(encoding="utf-8")
        for term in residue_terms:
            if term in text:
                raise SystemExit(f"WEB-only D1 residue {term!r} in {p}")

print("W5D-D1 WEB-only usage/pricing patch applied; ready for gofmt/vet/tests")
