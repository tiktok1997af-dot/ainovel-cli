package sites

import (
	"strings"
	"testing"
)

func TestGeminiQLReadbackReconstructsSemanticBlockNewlines(t *testing.T) {
	for _, want := range []string{
		"composer.classList.contains('ql-editor')",
		"composer.children.length > 0",
		"Array.from(composer.children)",
		"String(block.textContent || '')",
		".join('\\n')",
		"composer_line_breaks",
		"expected_line_breaks",
	} {
		if !strings.Contains(geminiVerifyPromptExpressionTemplate, want) {
			t.Fatalf("Quill semantic readback missing %q", want)
		}
	}
	for _, forbidden := range []string{
		"replace(/\\n+/g",
		"replace(/\\s+/g",
		"split(/\\n+/",
	} {
		if strings.Contains(geminiVerifyPromptExpressionTemplate, forbidden) {
			t.Fatalf("Quill readback must not collapse intentional whitespace via %q", forbidden)
		}
	}
}

func TestGeminiQLReadbackRemainsReadOnly(t *testing.T) {
	for _, forbidden := range []string{
		"textContent =",
		"innerText =",
		".focus()",
		".click()",
		"dispatchEvent(",
		"execCommand(",
	} {
		if strings.Contains(geminiVerifyPromptExpressionTemplate, forbidden) {
			t.Fatalf("Quill readback contains forbidden side effect %q", forbidden)
		}
	}
}
