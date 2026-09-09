package appruntime

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/host"
)

func TestProjectRuntimeQueueItemPreservesDurableCursor(t *testing.T) {
	now := time.Date(2026, 9, 9, 13, 20, 0, 0, time.FixedZone("UTC+7", 7*60*60))
	item := domain.RuntimeQueueItem{
		Seq:      42,
		Time:     now,
		TaskID:   "task-42",
		Category: "REVIEW",
		Summary:  "review complete",
		Payload: host.Event{
			ID:       "e42",
			Time:     now,
			Category: "REVIEW",
			Agent:    "reviewer",
			Summary:  "review complete",
			Level:    "success",
		},
	}

	got := projectRuntimeQueueItem(item)
	if got.Seq != 42 || got.Type != eventTypeDurable || got.TaskID != "task-42" {
		t.Fatalf("durable envelope mismatch: %+v", got)
	}
	if got.Category != "REVIEW" || got.Level != "success" || got.Summary != "review complete" {
		t.Fatalf("durable event projection mismatch: %+v", got)
	}
	if got.Time.Location() != time.UTC {
		t.Fatalf("durable time must be UTC: %v", got.Time)
	}
}

func TestProjectRuntimeQueueItemDecodesPayloadAfterJSONRoundTrip(t *testing.T) {
	original := domain.RuntimeQueueItem{
		Seq:      7,
		Time:     time.Now(),
		Category: "SYSTEM",
		Summary:  "checkpoint",
		Payload: host.Event{
			ID: "e7", Time: time.Now(), Category: "SYSTEM", Summary: "checkpoint", Level: "info",
		},
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal runtime queue item: %v", err)
	}
	var loaded domain.RuntimeQueueItem
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("unmarshal runtime queue item: %v", err)
	}
	got := projectRuntimeQueueItem(loaded)
	if got.Seq != 7 || got.Category != "SYSTEM" || got.Level != "info" || got.Summary != "checkpoint" {
		t.Fatalf("round-trip durable projection mismatch: %+v", got)
	}
}

func TestDesktopSubscriptionDropsOldestWithoutBlocking(t *testing.T) {
	sub := newDesktopSubscription(desktopEventBuffer, nil)
	defer sub.Close()

	for i := 1; i <= desktopEventBuffer+25; i++ {
		sub.offer(DesktopEvent{Seq: int64(i), Type: eventTypeDurable})
	}

	var first, last int64
	count := 0
	for count < desktopEventBuffer {
		select {
		case ev := <-sub.Events():
			if count == 0 {
				first = ev.Seq
			}
			last = ev.Seq
			count++
		default:
			t.Fatalf("subscription buffer unexpectedly short after %d events", count)
		}
	}
	if first != 26 {
		t.Fatalf("oldest buffered seq = %d, want 26", first)
	}
	if last != desktopEventBuffer+25 {
		t.Fatalf("newest buffered seq = %d, want %d", last, desktopEventBuffer+25)
	}
}

func TestDesktopSubscriptionCloseIsIdempotent(t *testing.T) {
	calls := 0
	sub := newDesktopSubscription(desktopEventBuffer, func() { calls++ })
	if err := sub.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := sub.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
	if calls != 1 {
		t.Fatalf("onClose calls = %d, want 1", calls)
	}
	if _, ok := <-sub.Events(); ok {
		t.Fatal("events channel remains open after Close")
	}
}
