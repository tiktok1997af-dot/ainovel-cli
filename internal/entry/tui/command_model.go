package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/voocel/ainovel-cli/internal/host"
)

// webStatusRuntime is the complete /model dependency in the WEB-only product.
// The command is intentionally read-only: it observes the owned browser
// session and has no provider/model mutation surface.
type webStatusRuntime interface {
	WebConfiguration() host.WebConfigurationSnapshot
}

type modelSwitchState struct {
	web host.WebConfigurationSnapshot
}

func newModelSwitchState(rt webStatusRuntime, _ string) *modelSwitchState {
	if rt == nil {
		return &modelSwitchState{}
	}
	return &modelSwitchState{web: rt.WebConfiguration()}
}

// normalizeRoleKey remains temporarily for command parsing compatibility. Role
// hints no longer change model selection; every role uses the same Gemini Web
// session.
func normalizeRoleKey(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "", "default":
		return "default"
	case "architect", "writer", "editor":
		return strings.ToLower(strings.TrimSpace(role))
	default:
		return ""
	}
}

func (m Model) handleModelSwitchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.modelSwitch == nil {
		return m, nil
	}
	switch msg.Type {
	case tea.KeyEsc, tea.KeyEnter:
		m.modelSwitch = nil
		return m, m.textarea.Focus()
	default:
		return m, nil
	}
}

func renderModelSwitchBar(width int, state *modelSwitchState) string {
	if state == nil || width <= 0 {
		return ""
	}
	return renderWebModelStatusBar(width, state.web)
}

func renderWebModelStatusBar(width int, web host.WebConfigurationSnapshot) string {
	status := "STOPPED"
	reason := ""
	pid := 0
	if web.HasSession {
		status = string(web.Session.State)
		reason = strings.TrimSpace(web.Session.Reason)
		pid = web.Session.PID
	}
	profile := strings.TrimSpace(web.ProfileName)
	if profile == "" {
		profile = "default"
	}
	model := strings.TrimSpace(web.Model)
	if model == "" {
		model = "gemini-web"
	}

	title := lipgloss.NewStyle().Foreground(colorMuted).Bold(true).Render("/model Gemini Web · WEB-only")
	lines := []string{
		renderWebStatusField("Kết nối", "WEB-only · Chrome hiển thị"),
		renderWebStatusField("AI", "Gemini Web · "+model),
		renderWebStatusField("Trạng thái", status),
		renderWebStatusField("Hồ sơ Chrome", profile),
	}
	if pid > 0 {
		lines = append(lines, renderWebStatusField("Chrome PID", fmt.Sprintf("%d", pid)))
	}
	if reason != "" {
		lines = append(lines, lipgloss.NewStyle().Foreground(colorDim).Render(truncate("Chi tiết: "+reason, max(20, width-10))))
	}
	if status == "AUTH_REQUIRED" {
		lines = append(lines, lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render("Hãy hoàn tất đăng nhập Gemini trong cửa sổ Chrome đang mở."))
	}
	lines = append(lines, lipgloss.NewStyle().Foreground(colorDim).Italic(true).Render("Enter/Esc Đóng · Không có chuyển Provider/Model trong WEB-only"))
	return renderModelBox(width, title, lines)
}

func renderWebStatusField(label, value string) string {
	labelText := lipgloss.NewStyle().Foreground(colorMuted).Width(16).Render(label + ":")
	return labelText + lipgloss.NewStyle().Foreground(bodyTextColor).Render(value)
}

func renderModelBox(width int, title string, lines []string) string {
	content := strings.Join(lines, "\n")
	boxW := lipgloss.Width(content) + 8
	maxW := width - 2
	if maxW > 76 {
		maxW = 76
	}
	if boxW > maxW {
		boxW = maxW
	}
	if boxW < 56 {
		boxW = 56
	}
	innerW := boxW - 2
	if innerW < 16 {
		innerW = 16
	}
	sepW := innerW - lipgloss.Width(title) - 3
	if sepW < 0 {
		sepW = 0
	}
	lineStyle := lipgloss.NewStyle().Foreground(colorDim)
	topBorder := lineStyle.Render("┌─ ") + title + lineStyle.Render(" "+strings.Repeat("─", sepW)+"┐")
	bottomBorder := lineStyle.Render("└" + strings.Repeat("─", innerW) + "┘")
	body := make([]string, 0, len(lines))
	for _, line := range lines {
		padding := innerW - lipgloss.Width(line)
		if padding < 0 {
			padding = 0
		}
		body = append(body, lineStyle.Render("│")+line+strings.Repeat(" ", padding)+lineStyle.Render("│"))
	}
	return strings.Join(append(append([]string{topBorder}, body...), bottomBorder), "\n")
}
