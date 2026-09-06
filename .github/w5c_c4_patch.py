from pathlib import Path

# setup.go: remove provider presets and provider-named setup option.
p = Path("internal/bootstrap/setup.go")
s = p.read_text()
start = s.index("// ProviderPreset is retained only for the legacy W5C removal path.")
end = s.index("type setupProvider struct {")
s = s[:start] + s[end:]
s = s.replace("type setupProvider struct {", "type setupOption struct {")
s = s.replace("[]setupProvider", "[]setupOption")
s = s.replace("setupProvider{", "setupOption{")
p.write_text(s)

# host.go: remove provider/model management surface and final revision failover.
p = Path("internal/host/host.go")
s = p.read_text()
start = s.index("// ── 模型管理 ──")
end = s.index("// concreteThinkingRoles 是可应用推理强度的具体角色")
replacement = '''// ── WEB-only model status / reasoning ──

func (h *Host) CurrentModelSelection(role string) (string, string, bool) {
\treturn h.models.CurrentSelection(role)
}

'''
s = s[:start] + replacement + s[end:]
old = '''\tmodel := h.models.ForRoleWithFailover("editor", func(ev bootstrap.FailoverEvent) {
\t\tslog.Warn("章节修订 provider 切换", "module", "revision", "role", ev.Role,
\t\t\t"reason", ev.Reason, "from", fmt.Sprintf("%s/%s", ev.FromProvider, ev.FromModel),
\t\t\t"to", fmt.Sprintf("%s/%s", ev.ToProvider, ev.ToModel), "err", ev.Err)
\t})
'''
if old not in s:
    raise SystemExit("revision failover block missing")
s = s.replace(old, '\tmodel := h.models.ForRole("editor")\n', 1)
p.write_text(s)

# imp/call.go: remove direct LiteLLM dependency; classify browser failures instead.
p = Path("internal/host/imp/call.go")
s = p.read_text()
s = s.replace('\t"github.com/voocel/litellm"\n', '\t"github.com/voocel/ainovel-cli/internal/webai"\n')
start = s.index("// errTypeLabels 把 litellm 错误分类翻成一眼可读的中文短标签。")
end = s.index("// callOptions 组装本次调用的 CallOption")
replacement = '''// modelErrDetail extracts browser-transport facts without depending on an API
// provider implementation. The web adapter preserves the actionable failure
// classes needed by import retry/error reporting.
func modelErrDetail(err error) string {
\tvar webErr *webai.Error
\tif !errors.As(err, &webErr) {
\t\treturn ""
\t}
\tparts := make([]string, 0, 2)
\tif webErr.Kind != "" {
\t\tparts = append(parts, string(webErr.Kind))
\t}
\tif webErr.Op != "" {
\t\tparts = append(parts, webErr.Op)
\t}
\treturn strings.Join(parts, "，")
}

'''
s = s[:start] + replacement + s[end:]
p.write_text(s)
