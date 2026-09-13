package desktopui

import (
	"os"
	"strings"
	"testing"
)

func TestReviewWorkspaceDoesNotHardcodePassWarnFailStrings(t *testing.T) {
	data, err := os.ReadFile("review_workspace.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, forbidden := range []string{"== \"pass\"", "== \"warn\"", "== \"fail\""} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("Review UI contains local quality verdict policy %q", forbidden)
		}
	}
}
