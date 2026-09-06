package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/voocel/ainovel-cli/internal/host"
)

func TestRenderTopBarShowsVersion(t *testing.T) {
	out := renderTopBar(host.UISnapshot{
		Provider:  "web",
		ModelName: "gemini-web",
		BookTitle: "测试小说",
	}, 120, "", "v1.2.3")
	if !strings.Contains(out, "ainovel-cli v1.2.3") {
		t.Fatalf("top bar missing version: %q", out)
	}
}

func TestRenderDetailContentShowsSynopsis(t *testing.T) {
	out := ansi.Strip(renderDetailContent(host.UISnapshot{Synopsis: "少年在永夜中寻找黎明。"}, 40))
	if !strings.Contains(out, "Tóm Tắt") || !strings.Contains(out, "少年在永夜中寻找黎明。") {
		t.Fatalf("detail panel missing synopsis: %q", out)
	}
}

func TestSameDetailSnapshotDetectsOutlineStateChanges(t *testing.T) {
	base := host.UISnapshot{Outline: []host.OutlineSnapshot{{Chapter: 1, Title: "第一章"}}}
	if !sameDetailSnapshot(base, base) {
		t.Fatal("相同详情不应触发重建")
	}
	changed := base
	changed.InProgressChapter = 1
	if sameDetailSnapshot(base, changed) {
		t.Fatal("章节状态变化必须触发详情重建")
	}
}

func TestRenderErrorEventKeepsOneLineSummary(t *testing.T) {
	out := ansi.Strip(renderEventLine(host.Event{
		Time:     time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC),
		Category: "ERROR",
		Summary:  "commit_chapter 参数错误：" + strings.Repeat("秦越在材料中发现线索", 20),
	}, 60, 0))
	if strings.Contains(out, "\n") {
		t.Fatalf("ERROR 事件应保持单行摘要，got %q", out)
	}
	if !strings.HasSuffix(out, "...") {
		t.Fatalf("超宽 ERROR 摘要应在 TUI 截断，got %q", out)
	}
}

// TestRenderStatusBar 守护底部状态栏的信息契约：模型身份（窗口+思考）、会话令牌、
// 花费/预算、书目录都必须在（样式剥离后按纯文本断言）。
func TestRenderStatusBar(t *testing.T) {
	out := ansi.Strip(renderStatusBar(host.UISnapshot{
		Provider:           "web",
		ModelName:          "gemini-web",
		ModelContextWindow: 200000,
		ThinkingLevel:      "medium",
	}, "/tmp/output", 120))
	for _, want := range []string{"web", "gemini-web(200K,med)", "./output"} {
		if !strings.Contains(out, want) {
			t.Fatalf("WEB-only status bar missing %q: %q", want, out)
		}
	}
	for _, forbidden := range []string{"$", "↑", "↓", "tiết kiệm"} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("WEB-only status bar must not fabricate billing/usage %q: %q", forbidden, out)
		}
	}
}

func TestRenderStatusBarAutoThinkingAndEmpty(t *testing.T) {
	out := ansi.Strip(renderStatusBar(host.UISnapshot{
		ModelName:          "test-model",
		ModelContextWindow: 128000,
	}, "", 120))
	if !strings.Contains(out, "test-model(128K,auto)") {
		t.Fatalf("缺思考等级 auto 括注：%q", out)
	}
	if out := ansi.Strip(renderStatusBar(host.UISnapshot{}, "", 120)); out != "SẴN SÀNG" {
		t.Fatalf("空快照应回退 READY，得 %q", out)
	}
}

func TestTruncateByDisplayWidth(t *testing.T) {
	// 纯中文按视觉宽度截：10 列预算 = 3 个汉字(6列) + "..."(3列)，按 rune 截会溢出到 17 列
	got := truncate("临港市公共算法伦理审计员", 10)
	if w := lipgloss.Width(got); w > 10 {
		t.Errorf("truncate 溢出列宽: %d > 10 (%q)", w, got)
	}
	if !strings.HasSuffix(got, "...") {
		t.Errorf("超宽截断应带省略号: %q", got)
	}
	// ASCII 行为与旧实现一致
	if got := truncate("abcdef", 6); got != "abcdef" {
		t.Errorf("未超宽不应截断: %q", got)
	}
	if got := truncate("abcdefgh", 6); got != "abc..." {
		t.Errorf("ASCII 截断: got %q want %q", got, "abc...")
	}
}

func TestRenderDetailContentWrapsCJK(t *testing.T) {
	long := "沈砚（主角；临港市公共算法伦理审计员，台风夜事故的调查负责人，坚持程序正义）"
	const contentW = 40
	out := renderDetailContent(host.UISnapshot{
		Characters:       []string{long},
		SupportingCount:  1,
		RecentSupporting: []string{long},
		RecentSummaries:  []string{"第6章：" + long},
	}, contentW)
	for line := range strings.SplitSeq(out, "\n") {
		if w := lipgloss.Width(line); w > contentW {
			t.Errorf("行溢出面板宽度: %d > %d (%q)", w, contentW, line)
		}
	}
	// 长描述应折成多行（悬挂缩进续行），而不是截断丢信息
	joined := strings.ReplaceAll(strings.ReplaceAll(out, "\n", ""), " ", "")
	if !strings.Contains(joined, "坚持程序正义") {
		t.Errorf("折行后应保留完整描述，实际输出:\n%s", out)
	}
}

func TestRenderWebTelemetrySidebarIsExplicit(t *testing.T) {
	out := ansi.Strip(renderWebTelemetrySidebar(host.UISnapshot{AITelemetryStatus: host.WebAITelemetryUnavailable}, 48))
	if !strings.Contains(out, "không cung cấp") || !strings.Contains(out, "billing") {
		t.Fatalf("WEB telemetry boundary missing: %q", out)
	}
	for _, forbidden := range []string{"$0", "0%"} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("WEB telemetry notice must not imply measured zero %q: %q", forbidden, out)
		}
	}
}
