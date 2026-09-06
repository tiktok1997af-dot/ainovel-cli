from pathlib import Path

# Patch config.go: WEB-only validation is the only valid runtime.
p = Path("internal/bootstrap/config.go")
s = p.read_text()
start = s.index("// ValidateBase 校验基础配置。")
end = s.index("func (c *Config) validateWebOnly() error {")
replacement = '''// LegacyAPIMigrationHint is intentionally stable and user-facing. Old API-era
// configuration is never interpreted as an alternate runtime path.
const LegacyAPIMigrationHint = "legacy AI provider/API configuration is no longer supported; set web.enabled=true and web.site=gemini-web, then remove provider/providers/api_key/base_url and role provider/model/fallback routing"

// ValidateBase validates the only supported product runtime: visible browser WEB-only.
func (c *Config) ValidateBase() error {
\tif !c.Web.Enabled {
\t\tif c.hasLegacyAPIRouting() {
\t\t\treturn fmt.Errorf("%s: %w", LegacyAPIMigrationHint, errs.ErrConfig)
\t\t}
\t\treturn fmt.Errorf("WEB-only runtime requires web.enabled=true and web.site=gemini-web; login is completed manually in the visible Chrome session: %w", errs.ErrConfig)
\t}
\treturn c.validateWebOnly()
}

func (c Config) hasLegacyAPIRouting() bool {
\tif strings.TrimSpace(c.Provider) != "" || strings.TrimSpace(c.ModelName) != "" || len(c.Providers) != 0 {
\t\treturn true
\t}
\tfor _, rc := range c.Roles {
\t\tif strings.TrimSpace(rc.Provider) != "" || strings.TrimSpace(rc.Model) != "" || len(rc.Fallbacks) != 0 {
\t\t\treturn true
\t\t}
\t}
\treturn false
}

'''
s = s[:start] + replacement + s[end:]
p.write_text(s)

# Patch configfile.go: merge browser settings, remove API credential writer,
# and refuse/sanitize legacy fields during persistence.
p = Path("internal/bootstrap/configfile.go")
s = p.read_text()
needle = '''\tif overlay.ModelName != "" {
\t\tbase.ModelName = overlay.ModelName
\t}
'''
insert = needle + '''\tif overlay.Web != (WebAIConfig{}) {
\t\tif overlay.Web.Enabled {
\t\t\tbase.Web.Enabled = true
\t\t}
\t\tif overlay.Web.Site != "" {
\t\t\tbase.Web.Site = overlay.Web.Site
\t\t}
\t\tif overlay.Web.BrowserPath != "" {
\t\t\tbase.Web.BrowserPath = overlay.Web.BrowserPath
\t\t}
\t\tif overlay.Web.ProfileName != "" {
\t\t\tbase.Web.ProfileName = overlay.Web.ProfileName
\t\t}
\t\tif overlay.Web.StartURL != "" {
\t\t\tbase.Web.StartURL = overlay.Web.StartURL
\t\t}
\t}
'''
if needle not in s:
    raise SystemExit("merge ModelName anchor missing")
s = s.replace(needle, insert, 1)

needle = '''\tif overlay.Style != "" {
\t\tbase.Style = overlay.Style
\t}
'''
insert = needle + '''\tif overlay.Language != "" {
\t\tbase.Language = overlay.Language
\t}
'''
if needle not in s:
    raise SystemExit("merge Style anchor missing")
s = s.replace(needle, insert, 1)

start = s.index("// SaveProviderConfig 补丁式更新目标配置层里单个 provider 的凭证与模型库。")
end = s.index("// stripJSONComments 去除 JSON 中的 // 行注释")
s = s[:start] + s[end:]

old = '''func SaveConfig(path string, cfg Config) error {
\tif err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
\t\treturn err
\t}
\tdata, err := json.MarshalIndent(cfg, "", "  ")
'''
new = '''func SaveConfig(path string, cfg Config) error {
\tpersist := CloneConfig(cfg)
\tpersist.FillDefaults()
\tif err := persist.ValidateBase(); err != nil {
\t\treturn fmt.Errorf("refusing to persist non-WEB configuration: %w", err)
\t}
\t// Provider/model are runtime compatibility aliases only. Never persist them,
\t// and never persist an API provider credential map.
\tpersist.Provider = ""
\tpersist.ModelName = ""
\tpersist.Providers = nil

\tif err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
\t\treturn err
\t}
\tdata, err := json.MarshalIndent(persist, "", "  ")
'''
if old not in s:
    raise SystemExit("SaveConfig anchor missing")
s = s.replace(old, new, 1)
p.write_text(s)
