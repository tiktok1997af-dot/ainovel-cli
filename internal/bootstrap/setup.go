package bootstrap

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/voocel/ainovel-cli/internal/rules"
	"github.com/voocel/ainovel-cli/internal/utils"
)

// exampleConfig là mẫu cấu hình có chú thích ghi vào ~/.ainovel/config.example.jsonc.
//
//go:embed config.example.jsonc
var exampleConfig string

// NeedsSetup kiểm tra xem có cần chạy trình hướng dẫn khởi tạo lần đầu hay không.
func NeedsSetup() bool {
	if p := DefaultConfigPath(); p != "" {
		if _, err := os.Stat(p); err == nil {
			return false
		}
	}
	if _, err := os.Stat(projectConfigPath()); err == nil {
		return false
	}
	return true
}

type setupOption struct {
	name  string
	label string
}

type setupLanguageOption struct {
	code  string
	label string
}

var languageOptions = []setupLanguageOption{
	{code: "vi", label: "Tiếng Việt (Mặc định - Sáng tác văn phong Việt tự nhiên)"},
	{code: "zh", label: "Tiếng Trung (Nguyên bản - Sáng tác bằng Tiếng Trung)"},
}

// NewWebSetupConfig creates the only first-run AI configuration produced in
// W5B. It stores browser metadata only; Google login credentials stay in Chrome.
func NewWebSetupConfig(language, browserPath, profileName string) Config {
	cfg := Config{
		Web: WebAIConfig{
			Enabled:     true,
			Site:        "gemini-web",
			BrowserPath: strings.TrimSpace(browserPath),
			ProfileName: strings.TrimSpace(profileName),
		},
		Roles:    map[string]RoleConfig{},
		Style:    "default",
		Language: strings.TrimSpace(language),
	}
	cfg.FillDefaults()
	return cfg
}

// RunSetup runs the W5B WEB-only first-run wizard.
// There is deliberately no provider/API-key/Base-URL/model-ID step.
func RunSetup() (Config, error) {
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99")).
		Render("Chưa tìm thấy cấu hình — thiết lập Gemini Web (không dùng AI API)"))
	fmt.Fprintf(os.Stderr, "  Tệp cấu hình: %s\n", lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(DefaultConfigPath()))
	fmt.Fprintln(os.Stderr, "  ainovel sẽ mở Chrome hiển thị; bạn đăng nhập Google/Gemini trực tiếp trong Chrome.")
	fmt.Fprintln(os.Stderr, "  ainovel không hỏi, không lưu và không đọc mật khẩu Google của bạn.")
	fmt.Fprintln(os.Stderr)

	selectedLang, err := runLanguageSelect()
	if err != nil {
		return Config{}, err
	}
	printStepDone("Ngôn ngữ sáng tác", selectedLang.label)
	printStepDone("AI", "Gemini Web · WEB-only · không API key")

	browserPath, err := runOptionalTextInput(
		"[2/3] Đường dẫn Chrome (tùy chọn)",
		"Để trống để ainovel tự tìm Google Chrome",
	)
	if err != nil {
		return Config{}, err
	}
	if browserPath == "" {
		printStepDone("Chrome", "Tự động phát hiện")
	} else {
		printStepDone("Chrome", browserPath)
	}

	profileName, err := runTextInputWithDefault(
		"[3/3] Tên hồ sơ đăng nhập Chrome dành cho ainovel",
		"default",
		"default",
	)
	if err != nil {
		return Config{}, err
	}
	printStepDone("Hồ sơ Chrome", profileName)

	cfg := NewWebSetupConfig(selectedLang.code, browserPath, profileName)
	if err := cfg.ValidateBase(); err != nil {
		return cfg, fmt.Errorf("cấu hình WEB-only không hợp lệ: %w", err)
	}
	path := DefaultConfigPath()
	if err := SaveConfig(path, cfg); err != nil {
		return cfg, fmt.Errorf("lỗi lưu cấu hình: %w", err)
	}
	saveExampleConfig()

	fmt.Fprintln(os.Stderr)
	fmt.Fprintf(os.Stderr, "%s Cấu hình WEB-only đã lưu: %s\n",
		lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("✓"), path)
	fmt.Fprintln(os.Stderr, "  Site: Gemini Web")
	fmt.Fprintf(os.Stderr, "  Hồ sơ Chrome: %s\n", cfg.Web.ProfileName)
	fmt.Fprintln(os.Stderr, "  Bước tiếp theo tự động: Chrome mở → đăng nhập Gemini nếu cần → trạng thái READY.")
	fmt.Fprintln(os.Stderr, "  Trong TUI: /model xem trạng thái Gemini Web; /config chỉnh Chrome/profile cho lần khởi động kế tiếp.")
	if rulesDir := rules.DefaultHomeRulesDir(); rulesDir != "" {
		fmt.Fprintf(os.Stderr, "  Quy tắc/phong cách cá nhân: %s\n", rulesDir)
	}
	fmt.Fprintln(os.Stderr)
	return cfg, nil
}

func saveExampleConfig() {
	dir, err := configDir()
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(dir, "config.example.jsonc"), []byte(exampleConfig), 0o644)
}

func printStepDone(label, value string) {
	fmt.Fprintf(os.Stderr, "  %s %s: %s\n",
		lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("✓"),
		label,
		lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(value))
}

// ---------- TUI Components ----------

func runLanguageSelect() (setupLanguageOption, error) {
	items := make([]setupOption, len(languageOptions))
	for i, opt := range languageOptions {
		items[i] = setupOption{name: opt.code, label: opt.label}
	}
	m := setupSelectModel{
		title: "[1/3] Chọn Ngôn Ngữ Sáng Tác Nội Dung Truyện (giao diện luôn là Tiếng Việt)",
		items: items,
	}
	p := tea.NewProgram(m, tea.WithOutput(os.Stderr))
	final, err := p.Run()
	if err != nil {
		return setupLanguageOption{}, err
	}
	result := final.(setupSelectModel)
	if result.cancelled {
		return setupLanguageOption{}, fmt.Errorf("đã hủy khởi tạo")
	}
	return languageOptions[result.cursor], nil
}

func runTextInput(label, placeholder string) (string, error) {
	return runTextInputWithDefault(label, placeholder, "")
}

func runOptionalTextInput(label, placeholder string) (string, error) {
	m := setupInputModel{label: label, placeholder: placeholder, allowEmpty: true}
	p := tea.NewProgram(m, tea.WithOutput(os.Stderr))
	final, err := p.Run()
	if err != nil {
		return "", err
	}
	result := final.(setupInputModel)
	if result.cancelled {
		return "", fmt.Errorf("đã hủy khởi tạo")
	}
	return utils.CleanInputLine(result.value), nil
}

func runTextInputWithDefault(label, placeholder, defaultValue string) (string, error) {
	m := setupInputModel{label: label, placeholder: placeholder, defaultValue: defaultValue}
	p := tea.NewProgram(m, tea.WithOutput(os.Stderr))
	final, err := p.Run()
	if err != nil {
		return "", err
	}
	result := final.(setupInputModel)
	if result.cancelled {
		return "", fmt.Errorf("đã hủy khởi tạo")
	}
	if result.value == "" && result.defaultValue != "" {
		return result.defaultValue, nil
	}
	return utils.CleanInputLine(result.value), nil
}

var (
	setupCursorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("212"))
	setupDimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	setupHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99"))
	setupInputStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("212"))
)

type setupSelectModel struct {
	title     string
	items     []setupOption
	cursor    int
	cancelled bool
}

func (m setupSelectModel) Init() tea.Cmd { return nil }

func (m setupSelectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if msg, ok := msg.(tea.KeyMsg); ok {
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		case "enter":
			return m, tea.Quit
		case "q", "esc", "ctrl+c":
			m.cancelled = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m setupSelectModel) View() string {
	var b strings.Builder
	b.WriteString(setupHeaderStyle.Render(m.title))
	b.WriteString("\n\n")
	for i, item := range m.items {
		cursor := "  "
		label := item.label
		if i == m.cursor {
			cursor = setupCursorStyle.Render("❯ ")
			label = setupCursorStyle.Render(label)
		}
		b.WriteString(cursor + label + "\n")
	}
	b.WriteString(setupDimStyle.Render("\n  ↑↓ Chọn  Enter Xác nhận  Esc Hủy"))
	return b.String()
}

type setupInputModel struct {
	label        string
	placeholder  string
	defaultValue string
	allowEmpty   bool
	value        string
	cancelled    bool
}

func (m setupInputModel) Init() tea.Cmd { return nil }

func (m setupInputModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if msg, ok := msg.(tea.KeyMsg); ok {
		switch msg.String() {
		case "enter":
			if utils.CleanInputLine(m.value) != "" || m.defaultValue != "" || m.allowEmpty {
				return m, tea.Quit
			}
		case "ctrl+c", "esc":
			m.cancelled = true
			return m, tea.Quit
		case "backspace":
			if len(m.value) > 0 {
				runes := []rune(m.value)
				m.value = string(runes[:len(runes)-1])
			}
		default:
			if msg.Type == tea.KeyRunes {
				m.value += utils.CleanInputRunes(msg.Runes)
			} else if msg.Type == tea.KeySpace {
				m.value += " "
			}
		}
	}
	return m, nil
}

func (m setupInputModel) View() string {
	var b strings.Builder
	b.WriteString(setupHeaderStyle.Render(m.label))
	b.WriteString("\n\n")
	b.WriteString(setupInputStyle.Render("❯ "))
	if m.value == "" {
		b.WriteString(setupCursorStyle.Render("▌"))
		b.WriteString(setupDimStyle.Render(m.placeholder))
	} else {
		b.WriteString(m.value)
		b.WriteString(setupCursorStyle.Render("▌"))
	}
	b.WriteString(setupDimStyle.Render("  (Enter Xác nhận, Esc Hủy)"))
	b.WriteString("\n")
	return b.String()
}
