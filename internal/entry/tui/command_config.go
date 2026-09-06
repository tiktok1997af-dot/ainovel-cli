package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/voocel/ainovel-cli/internal/host"
)

const (
	webConfigFieldChrome = iota
	webConfigFieldProfile
	webConfigFieldSave
	webConfigFieldCount
)

// modelConfigState keeps the historical name so the TUI overlay/event loop
// remains stable, but W5B turns the active surface into browser-only settings.
// No provider, API key, Base URL, protocol, token or Google credential is held.
type modelConfigState struct {
	web     host.WebConfigurationSnapshot
	cursor  int
	message string
	input   textinput.Model
	editing int
	saving  bool

	browserPath string
	profileName string
}

func newModelConfigState(rt *host.Host) *modelConfigState {
	web := rt.WebConfiguration()
	state := &modelConfigState{
		web:         web,
		editing:     -1,
		browserPath: strings.TrimSpace(web.BrowserPath),
		profileName: strings.TrimSpace(web.ProfileName),
	}
	if state.profileName == "" {
		state.profileName = "default"
	}
	if !web.Enabled {
		state.message = "Cấu hình runtime không ở trạng thái WEB-only hợp lệ; hãy khởi động lại sau khi sửa cấu hình browser."
	}
	return state
}

func (s *modelConfigState) fieldLabel(field int) string {
	switch field {
	case webConfigFieldChrome:
		return "Chrome"
	case webConfigFieldProfile:
		return "Hồ sơ Chrome"
	case webConfigFieldSave:
		return "Lưu cấu hình"
	default:
		return ""
	}
}

func (s *modelConfigState) fieldValue(field int) string {
	switch field {
	case webConfigFieldChrome:
		if strings.TrimSpace(s.browserPath) == "" {
			return "Tự động phát hiện"
		}
		return s.browserPath
	case webConfigFieldProfile:
		if strings.TrimSpace(s.profileName) == "" {
			return "default"
		}
		return s.profileName
	case webConfigFieldSave:
		return "Áp dụng ở lần khởi động kế tiếp"
	default:
		return ""
	}
}

func (s *modelConfigState) beginEdit(field int) tea.Cmd {
	if field != webConfigFieldChrome && field != webConfigFieldProfile {
		return nil
	}
	s.editing = field
	input := textinput.New()
	input.Prompt = ""
	input.CharLimit = 0
	input.Width = 46
	input.TextStyle = lipgloss.NewStyle().Foreground(bodyTextColor).Underline(true)
	input.PlaceholderStyle = lipgloss.NewStyle().Foreground(colorDim).Underline(true)
	input.Cursor.Style = lipgloss.NewStyle().Foreground(colorAccent)
	if field == webConfigFieldChrome {
		input.SetValue(s.browserPath)
		input.Placeholder = "Để trống để tự động phát hiện Google Chrome"
	} else {
		input.SetValue(s.profileName)
		input.Placeholder = "default"
	}
	input.CursorEnd()
	s.input = input
	s.message = ""
	return s.input.Focus()
}

func (s *modelConfigState) commitEdit() {
	value := strings.TrimSpace(s.input.Value())
	switch s.editing {
	case webConfigFieldChrome:
		s.browserPath = value
	case webConfigFieldProfile:
		if value == "" {
			value = "default"
		}
		s.profileName = value
	}
	s.input.Blur()
	s.editing = -1
	s.message = ""
}

func (s *modelConfigState) cancelEdit() {
	s.input.Blur()
	s.editing = -1
	s.message = ""
}

type modelConfigSavedMsg struct{ err error }

func saveModelConfiguration(rt *host.Host, browserPath, profileName string) tea.Cmd {
	return func() tea.Msg {
		return modelConfigSavedMsg{err: rt.SaveWebConfiguration(browserPath, profileName)}
	}
}

func (m Model) handleModelConfigKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	state := m.modelConfig
	if state == nil {
		return m, nil
	}

	if msg.Type == tea.KeyEsc {
		if state.editing >= 0 {
			state.cancelEdit()
			return m, nil
		}
		m.modelConfig = nil
		return m, m.textarea.Focus()
	}
	if state.saving {
		return m, nil
	}
	if state.editing >= 0 {
		if msg.Type == tea.KeyEnter {
			state.commitEdit()
			return m, nil
		}
		var cmd tea.Cmd
		state.input, cmd = state.input.Update(msg)
		return m, cmd
	}

	switch msg.Type {
	case tea.KeyUp:
		state.cursor = (state.cursor - 1 + webConfigFieldCount) % webConfigFieldCount
	case tea.KeyDown, tea.KeyTab:
		state.cursor = (state.cursor + 1) % webConfigFieldCount
	case tea.KeyEnter:
		switch state.cursor {
		case webConfigFieldChrome, webConfigFieldProfile:
			return m, state.beginEdit(state.cursor)
		case webConfigFieldSave:
			if !state.web.Enabled {
				state.message = "Không thể lưu browser settings khi WEB-only runtime chưa được bật."
				return m, nil
			}
			state.saving = true
			state.message = "Đang lưu Chrome/profile cho lần khởi động kế tiếp..."
			return m, saveModelConfiguration(m.runtime, state.browserPath, state.profileName)
		}
	}
	return m, nil
}

func renderModelConfigModal(width int, state *modelConfigState) string {
	if state == nil {
		return ""
	}
	boxW := min(max(60, width*3/5), 82, width-4)
	contentW := paddedModalContentWidth(boxW)

	site := strings.TrimSpace(state.web.Site)
	if site == "" {
		site = "gemini-web"
	}
	status := "STOPPED"
	reason := ""
	if state.web.HasSession {
		status = string(state.web.Session.State)
		reason = strings.TrimSpace(state.web.Session.Reason)
	}

	lines := []string{
		configWebHeading("Gemini Web · WEB-only"),
		configWebStatic("Site", site),
		configWebStatic("Trạng thái", status),
	}
	if reason != "" {
		lines = append(lines, lipgloss.NewStyle().Foreground(colorDim).Render(truncate("Chi tiết: "+reason, contentW)))
	}
	if status == "AUTH_REQUIRED" {
		lines = append(lines, lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render(
			truncate("Hãy hoàn tất đăng nhập Gemini trong cửa sổ Chrome đang mở.", contentW)))
	}
	lines = append(lines, "")

	for field := 0; field < webConfigFieldCount; field++ {
		selected := state.cursor == field
		marker := "  "
		if selected {
			marker = lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render("› ")
		}
		label := state.fieldLabel(field)
		value := state.fieldValue(field)
		if state.editing == field {
			value = state.input.View()
		}
		labelStyle := lipgloss.NewStyle().Foreground(colorMuted).Width(17)
		valueStyle := lipgloss.NewStyle().Foreground(bodyTextColor)
		if selected {
			valueStyle = valueStyle.Foreground(colorAccent).Bold(true)
		}
		row := marker + labelStyle.Render(label+":") + valueStyle.Render(truncate(value, max(18, contentW-21)))
		lines = append(lines, row)
	}

	lines = append(lines, "")
	lines = append(lines, lipgloss.NewStyle().Foreground(colorDim).Render(
		truncate("Không lưu mật khẩu Google, cookie, token hay credential AI. Chrome giữ phiên đăng nhập trong profile riêng.", contentW)))
	if state.web.ConfigPath != "" {
		lines = append(lines, lipgloss.NewStyle().Foreground(colorDim).Render(truncate("Tệp cấu hình: "+state.web.ConfigPath, contentW)))
	}
	if state.message != "" {
		style := lipgloss.NewStyle().Foreground(colorError)
		if state.saving || strings.HasPrefix(state.message, "Đang") {
			style = lipgloss.NewStyle().Foreground(colorAccent)
		}
		lines = append(lines, "", style.Render(truncate(state.message, contentW)))
	}

	hint := "↑↓/Tab Chọn · Enter Sửa/Lưu · Esc Đóng"
	if state.editing >= 0 {
		hint = "Nhập dữ liệu · Enter Xác nhận · Esc Hủy"
	}
	return renderPaddedModalFrame(boxW, len(lines)+2, "/config Gemini Web & Chrome", hint, lines)
}

func configWebHeading(text string) string {
	return lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render(text)
}

func configWebStatic(label, value string) string {
	return lipgloss.NewStyle().Foreground(colorMuted).Width(17).Render("  "+label+":") +
		lipgloss.NewStyle().Foreground(bodyTextColor).Render(value)
}
