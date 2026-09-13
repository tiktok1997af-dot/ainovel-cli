package desktopui

import "testing"

func TestReviewNavigationIsEnabledByG066(t *testing.T) {
	for _, item := range PrimaryNavigation() {
		if item.ID == RouteReview {
			if !item.Enabled || item.Availability != "G06.6" {
				t.Fatalf("Review navigation=%+v, want enabled G06.6", item)
			}
			return
		}
	}
	t.Fatal("Review navigation item missing")
}
