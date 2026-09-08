package host

import (
	"strings"
	"testing"
)

func TestLocalizeCoCreateSystemPromptVietnameseColdStart(t *testing.T) {
	got := localizeCoCreateSystemPrompt(coCreateSystemPrompt, "vi")

	for _, want := range []string{
		"trợ lý đồng sáng tác tiểu thuyết",
		"Phản hồi tự nhiên bằng tiếng Việt",
		"Bản nháp chỉ thị sáng tác hoàn chỉnh",
		"<reply>",
		"<draft>",
		"<ready>false</ready>",
		"<suggestions>",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Vietnamese cold-start prompt should contain %q\n%s", want, got)
		}
	}

	for _, forbidden := range []string{"中文自然回复", "中文创作指令"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("Vietnamese cold-start prompt must not force Chinese via %q", forbidden)
		}
	}
}

func TestLocalizeCoCreateSystemPromptVietnameseStagePreservesSummary(t *testing.T) {
	const suffix = "\n\n---\n## 当前故事状态\n- SENTINEL: dữ liệu đã viết"
	got := localizeCoCreateSystemPrompt(stageCoCreateSystemPrompt+suffix, "vi")

	for _, want := range []string{
		"đồng sáng tác theo giai đoạn",
		"Phản hồi tự nhiên bằng tiếng Việt",
		"brief định hướng tiếp theo",
		"SENTINEL: dữ liệu đã viết",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Vietnamese stage prompt should contain %q\n%s", want, got)
		}
	}
	if strings.Contains(got, "给用户看的中文自然回复") {
		t.Fatal("Vietnamese stage prompt must not retain the Chinese reply-language directive")
	}
}

func TestLocalizeCoCreateSystemPromptChineseUnchanged(t *testing.T) {
	cold := localizeCoCreateSystemPrompt(coCreateSystemPrompt, "zh")
	if cold != coCreateSystemPrompt {
		t.Fatal("Chinese cold-start prompt should remain unchanged")
	}

	stage := stageCoCreateSystemPrompt + "\n\nsummary"
	if got := localizeCoCreateSystemPrompt(stage, "zh"); got != stage {
		t.Fatal("Chinese stage prompt should remain unchanged")
	}
}

func TestLocalizeCoCreateSystemPromptDefaultsToVietnamese(t *testing.T) {
	for _, language := range []string{"", "vi", "VI", "en", "unknown"} {
		t.Run(language, func(t *testing.T) {
			got := localizeCoCreateSystemPrompt(coCreateSystemPrompt, language)
			if !strings.Contains(got, "Phản hồi tự nhiên bằng tiếng Việt") {
				t.Fatalf("language %q should follow Config's non-Chinese => Vietnamese default", language)
			}
		})
	}
}
