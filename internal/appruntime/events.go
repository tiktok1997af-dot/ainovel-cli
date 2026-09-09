package appruntime

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/host"
	"github.com/voocel/ainovel-cli/internal/utils"
)

const (
	desktopEventBuffer       = 256
	durableQueuePollInterval = 75 * time.Millisecond

	eventTypeHost        = "host_event"
	eventTypeDurable     = "durable_event"
	eventTypeStreamDelta = "stream_delta"
	eventTypeStreamClear = "stream_clear"
	eventTypeStreamMode  = "stream_mode"
)

type streamPayload struct {
	Text string `json:"text,omitempty"`
	Mode string `json:"mode"`
}

type hostEventPayload struct {
	ID         string        `json:"id,omitempty"`
	FinishedAt time.Time     `json:"finished_at,omitempty"`
	Failed     bool          `json:"failed"`
	Agent      string        `json:"agent,omitempty"`
	Kind       string        `json:"kind,omitempty"`
	Depth      int           `json:"depth,omitempty"`
	Duration   time.Duration `json:"duration,omitempty"`
	RetryAt    time.Time     `json:"retry_at,omitempty"`
}

type desktopSubscription struct {
	mu      sync.Mutex
	events  chan DesktopEvent
	closed  bool
	onClose func()
}

func newDesktopSubscription(capacity int, onClose func()) *desktopSubscription {
	if capacity < desktopEventBuffer {
		capacity = desktopEventBuffer
	}
	return &desktopSubscription{
		events:  make(chan DesktopEvent, capacity),
		onClose: onClose,
	}
}

func (s *desktopSubscription) Events() <-chan DesktopEvent { return s.events }

func (s *desktopSubscription) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	close(s.events)
	onClose := s.onClose
	s.onClose = nil
	s.mu.Unlock()
	if onClose != nil {
		onClose()
	}
	return nil
}

func (s *desktopSubscription) offer(ev DesktopEvent) {
	if s == nil {
		return
	}
	if ev.ContractVersion == "" {
		ev.ContractVersion = ContractVersion
	}
	if ev.Level == "error" {
		if ev.Error == nil {
			ev.Error = &AppError{
				Code:     ErrorCodeInternal,
				Category: ErrorCategoryInternal,
				Message:  safeErrorMessage(ErrorCodeInternal),
			}
		}
		ev.Summary = ev.Error.Message
		ev.Payload = nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	select {
	case s.events <- ev:
		return
	default:
	}
	select {
	case <-s.events:
	default:
	}
	select {
	case s.events <- ev:
	default:
	}
}

func (r *Runtime) startEventHub() {
	r.eventHubOnce.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		r.eventMu.Lock()
		if r.subscribers == nil {
			r.subscribers = make(map[uint64]*desktopSubscription)
		}
		r.eventHubCancel = cancel
		r.eventHubDone = make(chan struct{})
		done := r.eventHubDone
		r.eventMu.Unlock()
		go func() {
			defer close(done)
			r.runEventHub(ctx)
		}()
	})
}

func (r *Runtime) stopEventHub() {
	r.eventMu.Lock()
	cancel := r.eventHubCancel
	done := r.eventHubDone
	subs := make([]*desktopSubscription, 0, len(r.subscribers))
	for _, sub := range r.subscribers {
		subs = append(subs, sub)
	}
	r.subscribers = make(map[uint64]*desktopSubscription)
	r.eventMu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
	for _, sub := range subs {
		_ = sub.Close()
	}
}

func (r *Runtime) runEventHub(ctx context.Context) {
	lastDurableSeq := int64(0)
	if items, err := r.core.DesktopRuntimeQueueAfter(0); err == nil && len(items) > 0 {
		lastDurableSeq = items[len(items)-1].Seq
	} else if err != nil {
		slog.Warn("desktop runtime queue baseline load failed", "module", "appruntime.events", "err", err)
	}

	eventsCh := r.core.Events()
	streamCh := r.core.Stream()
	ticker := time.NewTicker(durableQueuePollInterval)
	defer ticker.Stop()
	streamMode := "content"

	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-eventsCh:
			if !ok {
				eventsCh = nil
				continue
			}
			r.broadcast(projectHostEvent(ev, eventTypeHost, 0))
		case delta, ok := <-streamCh:
			if !ok {
				streamCh = nil
				continue
			}
			switch delta {
			case host.StreamClearSentinel:
				streamMode = "content"
				r.broadcast(DesktopEvent{Time: time.Now().UTC(), Category: "STREAM", Type: eventTypeStreamClear})
			case utils.ThinkingSep:
				if streamMode == "thinking" {
					streamMode = "content"
				} else {
					streamMode = "thinking"
				}
				payload, _ := json.Marshal(streamPayload{Mode: streamMode})
				r.broadcast(DesktopEvent{Time: time.Now().UTC(), Category: "STREAM", Type: eventTypeStreamMode, Summary: streamMode, Payload: payload})
			default:
				payload, _ := json.Marshal(streamPayload{Text: delta, Mode: streamMode})
				r.broadcast(DesktopEvent{Time: time.Now().UTC(), Category: "STREAM", Type: eventTypeStreamDelta, Payload: payload})
			}
		case <-ticker.C:
			items, err := r.core.DesktopRuntimeQueueAfter(lastDurableSeq)
			if err != nil {
				slog.Warn("desktop runtime queue tail failed", "module", "appruntime.events", "after_seq", lastDurableSeq, "err", err)
				continue
			}
			for _, item := range items {
				if item.Seq <= lastDurableSeq {
					continue
				}
				r.broadcast(projectRuntimeQueueItem(item))
				lastDurableSeq = item.Seq
			}
		}
	}
}

func (r *Runtime) broadcast(ev DesktopEvent) {
	r.eventMu.Lock()
	defer r.eventMu.Unlock()
	for _, sub := range r.subscribers {
		sub.offer(ev)
	}
}

func (r *Runtime) registerSubscription(sub *desktopSubscription) uint64 {
	id := r.nextSubscriberID.Add(1)
	r.eventMu.Lock()
	if r.subscribers == nil {
		r.subscribers = make(map[uint64]*desktopSubscription)
	}
	r.subscribers[id] = sub
	r.eventMu.Unlock()
	return id
}

func (r *Runtime) removeSubscription(id uint64) {
	r.eventMu.Lock()
	delete(r.subscribers, id)
	r.eventMu.Unlock()
}

func projectHostEvent(ev host.Event, typ string, seq int64) DesktopEvent {
	payload, _ := json.Marshal(hostEventPayload{
		ID:         ev.ID,
		FinishedAt: utcTime(ev.FinishedAt),
		Failed:     ev.Failed,
		Agent:      ev.Agent,
		Kind:       ev.Kind,
		Depth:      ev.Depth,
		Duration:   ev.Duration,
		RetryAt:    utcTime(ev.RetryAt),
	})
	return DesktopEvent{
		ContractVersion: ContractVersion,
		Seq:             seq,
		Time:            utcTime(ev.Time),
		Category:        ev.Category,
		Type:            typ,
		Level:           ev.Level,
		Summary:         ev.Summary,
		Payload:         payload,
	}
}

func projectRuntimeQueueItem(item domain.RuntimeQueueItem) DesktopEvent {
	if ev, ok := decodeRuntimeHostEvent(item.Payload); ok {
		out := projectHostEvent(ev, eventTypeDurable, item.Seq)
		if out.Time.IsZero() {
			out.Time = utcTime(item.Time)
		}
		if out.Category == "" {
			out.Category = item.Category
		}
		if out.Summary == "" {
			out.Summary = item.Summary
		}
		out.TaskID = item.TaskID
		return out
	}
	payload, _ := json.Marshal(item.Payload)
	return DesktopEvent{
		ContractVersion: ContractVersion,
		Seq:             item.Seq,
		Time:            utcTime(item.Time),
		Category:        item.Category,
		Type:            eventTypeDurable,
		TaskID:          item.TaskID,
		Summary:         item.Summary,
		Payload:         payload,
	}
}

func decodeRuntimeHostEvent(payload any) (host.Event, bool) {
	if payload == nil {
		return host.Event{}, false
	}
	if ev, ok := payload.(host.Event); ok {
		return ev, true
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return host.Event{}, false
	}
	var ev host.Event
	if err := json.Unmarshal(data, &ev); err != nil {
		return host.Event{}, false
	}
	if ev.Category == "" && ev.Summary == "" && ev.ID == "" {
		return host.Event{}, false
	}
	return ev, true
}
