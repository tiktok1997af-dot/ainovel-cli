package webai

import (
	"context"
	"errors"
	"testing"
	"time"
)

type scriptedWatchdogTransport struct {
	responses []string
	errs      []error
	wait      bool
	calls     int
}

func (f *scriptedWatchdogTransport) RoundTrip(ctx context.Context, _ string) (string, error) {
	f.calls++
	if f.wait {
		<-ctx.Done()
		return "", ctx.Err()
	}
	idx := f.calls - 1
	var raw string
	var err error
	if idx < len(f.responses) {
		raw = f.responses[idx]
	}
	if idx < len(f.errs) {
		err = f.errs[idx]
	}
	return raw, err
}

func TestAutoRecoveryRetriesExplicitGeminiUIError(t *testing.T) {
	inner := &scriptedWatchdogTransport{responses: []string{
		"Sorry, something went wrong. Please try your request again.",
		"final protocol response",
	}}
	watchdog, err := NewAutoRecoveryTransport(AutoRecoveryConfig{
		Inner:               inner,
		SoftRetryLimit:      1,
		BrowserRestartLimit: -1,
		RetryDelay:          -1,
		StallTimeout:        time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := watchdog.RoundTrip(context.Background(), "prompt")
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	if got != "final protocol response" {
		t.Fatalf("response = %q", got)
	}
	if inner.calls != 2 {
		t.Fatalf("calls = %d, want 2", inner.calls)
	}
}

func TestAutoRecoveryStallTimeoutIsBoundedAndNonRetryableAfterExhaustion(t *testing.T) {
	inner := &scriptedWatchdogTransport{wait: true}
	watchdog, err := NewAutoRecoveryTransport(AutoRecoveryConfig{
		Inner:               inner,
		StallTimeout:        5 * time.Millisecond,
		SoftRetryLimit:      1,
		BrowserRestartLimit: -1,
		RetryDelay:          -1,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = watchdog.RoundTrip(context.Background(), "prompt")
	if err == nil {
		t.Fatal("expected watchdog exhaustion")
	}
	if inner.calls != 2 {
		t.Fatalf("calls = %d, want 2", inner.calls)
	}
	var webErr *Error
	if !errors.As(err, &webErr) {
		t.Fatalf("err = %T %v, want *Error", err, err)
	}
	if webErr.Retryable() {
		t.Fatal("exhausted watchdog error must be non-retryable")
	}
}

func TestAutoRecoveryDoesNotReplayAmbiguousTransportFailure(t *testing.T) {
	inner := &scriptedWatchdogTransport{errs: []error{
		&Error{Kind: ErrorTransport, Op: "confirm Gemini web prompt submit", Cause: errors.New("SEND ACK missing"), Retry: false},
	}}
	watchdog, err := NewAutoRecoveryTransport(AutoRecoveryConfig{
		Inner:               inner,
		SoftRetryLimit:      2,
		BrowserRestartLimit: 2,
		RetryDelay:          -1,
		RestartDelay:        -1,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = watchdog.RoundTrip(context.Background(), "prompt")
	if err == nil {
		t.Fatal("expected transport failure")
	}
	if inner.calls != 1 {
		t.Fatalf("calls = %d, want exactly 1", inner.calls)
	}
}

func TestAutoRecoveryRestartsBrowserAfterSoftRetryBudget(t *testing.T) {
	inner := &scriptedWatchdogTransport{responses: []string{
		"Sorry, something went wrong. Please try your request again.",
		"Sorry, something went wrong. Please try your request again.",
		"ok",
	}}
	restarts := 0
	watchdog, err := NewAutoRecoveryTransport(AutoRecoveryConfig{
		Inner:               inner,
		SoftRetryLimit:      1,
		BrowserRestartLimit: 1,
		RetryDelay:          -1,
		RestartDelay:        -1,
		StallTimeout:        time.Second,
		restart: func(context.Context) error {
			restarts++
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := watchdog.RoundTrip(context.Background(), "prompt")
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	if got != "ok" {
		t.Fatalf("response = %q, want ok", got)
	}
	if restarts != 1 {
		t.Fatalf("restarts = %d, want 1", restarts)
	}
	if inner.calls != 3 {
		t.Fatalf("calls = %d, want 3", inner.calls)
	}
}

func TestLooksLikeGeminiTransientUIError(t *testing.T) {
	cases := []struct {
		text string
		want bool
	}{
		{"Sorry, something went wrong. Please try your request again.", true},
		{"Đã xảy ra lỗi. Vui lòng thử lại.", true},
		{"出了点问题，请重试。", true},
		{"A character says something went wrong in the ancient ritual.", false},
		{"normal final response", false},
	}
	for _, tc := range cases {
		if got := looksLikeGeminiTransientUIError(tc.text); got != tc.want {
			t.Fatalf("looksLikeGeminiTransientUIError(%q) = %v, want %v", tc.text, got, tc.want)
		}
	}
}
