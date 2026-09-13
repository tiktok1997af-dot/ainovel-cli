package desktopui

import (
	"os"
	"strings"
	"testing"
)

func TestReviewWorkspaceDoesNotPersistOfficialBadgeTruth(t *testing.T) {
	data, err := os.ReadFile("review_workspace.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, forbidden := range []string{"Official bool", "IsOfficial", "OfficialStatus"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("Review UI introduced local durable Official truth marker %q", forbidden)
		}
	}
	if !strings.Contains(text, "PromotionReceipt") {
		t.Fatal("expected transient promotion receipt projection")
	}
}
