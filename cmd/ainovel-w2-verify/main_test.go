package main

import "testing"

func TestRunRejectsPositionalArguments(t *testing.T) {
	if code := run([]string{"unexpected"}); code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
}

func TestRunRejectsBadDuration(t *testing.T) {
	if code := run([]string{"--poll", "not-a-duration"}); code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
}
