package store

import (
	"fmt"
	"sort"
	"time"

	"github.com/voocel/ainovel-cli/internal/domain"
)

type runScheduleReplay struct {
	tickets     map[domain.RunID]domain.RunScheduleTicket
	currentTurn int64
}

// LoadScheduledRuns reconstructs the active G05.4 scheduler queue entirely from
// append-only queue.jsonl facts. Legacy/non-scheduler queue records are ignored.
func (s *RuntimeStore) LoadScheduledRuns() ([]domain.RunScheduleTicket, error) {
	items, err := s.LoadQueue()
	if err != nil {
		return nil, err
	}
	state, err := replayRunSchedule(items)
	if err != nil {
		return nil, err
	}
	return sortedRunScheduleTickets(state), nil
}

// EnqueueScheduledRun persists one run intent. Empty priority means the frozen
// default "normal". Repeating the exact same enqueue is idempotent; callers must
// use SetScheduledRunPriority to change a queued run's priority.
func (s *RuntimeStore) EnqueueScheduledRun(runID domain.RunID, priority domain.RunSchedulerPriority) (domain.RunScheduleTicket, error) {
	if priority == "" {
		priority = domain.DefaultRunSchedulerPriority
	}
	if err := validateSchedulableRunIdentity(runID); err != nil {
		return domain.RunScheduleTicket{}, err
	}
	if !priority.Valid() {
		return domain.RunScheduleTicket{}, fmt.Errorf("unsupported run scheduler priority %q", priority)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	record, err := s.loadRunRecord(runID)
	if err != nil {
		return domain.RunScheduleTicket{}, err
	}
	if record == nil {
		return domain.RunScheduleTicket{}, fmt.Errorf("run %q is not registered", runID)
	}
	state, err := s.loadRunScheduleLocked()
	if err != nil {
		return domain.RunScheduleTicket{}, err
	}
	if existing, ok := state.tickets[runID]; ok {
		if existing.Priority == priority {
			return existing, nil
		}
		return domain.RunScheduleTicket{}, fmt.Errorf("run %q is already queued with priority %q; use SetScheduledRunPriority", runID, existing.Priority)
	}

	item, err := s.appendSchedulerQueueLocked(domain.RuntimeQueueItem{
		Priority:    domain.RuntimePriorityControl,
		RunID:       runID,
		RunPriority: priority,
		Category:    domain.RuntimeQueueCategoryRunEnqueued,
		Summary:     "run scheduled",
	})
	if err != nil {
		return domain.RunScheduleTicket{}, err
	}
	return domain.RunScheduleTicket{
		RunID:       runID,
		Priority:    priority,
		EnqueueSeq:  item.Seq,
		EnqueueTurn: state.currentTurn + 1,
	}, nil
}

// SetScheduledRunPriority appends a deterministic priority transition while
// preserving the original enqueue sequence/turn so aging cannot be reset by a
// reprioritization request.
func (s *RuntimeStore) SetScheduledRunPriority(runID domain.RunID, priority domain.RunSchedulerPriority) (domain.RunScheduleTicket, error) {
	if err := validateSchedulableRunIdentity(runID); err != nil {
		return domain.RunScheduleTicket{}, err
	}
	if !priority.Valid() {
		return domain.RunScheduleTicket{}, fmt.Errorf("unsupported run scheduler priority %q", priority)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	state, err := s.loadRunScheduleLocked()
	if err != nil {
		return domain.RunScheduleTicket{}, err
	}
	ticket, ok := state.tickets[runID]
	if !ok {
		return domain.RunScheduleTicket{}, fmt.Errorf("run %q is not queued", runID)
	}
	if ticket.Priority == priority {
		return ticket, nil
	}
	if _, err := s.appendSchedulerQueueLocked(domain.RuntimeQueueItem{
		Priority:    domain.RuntimePriorityControl,
		RunID:       runID,
		RunPriority: priority,
		Category:    domain.RuntimeQueueCategoryRunPriorityChanged,
		Summary:     "run priority changed",
	}); err != nil {
		return domain.RunScheduleTicket{}, err
	}
	ticket.Priority = priority
	return ticket, nil
}

// DequeueNextScheduledRun deterministically selects and removes one queued run.
// It does not start work, acquire a resource, allocate a browser lane, or dispatch
// an AppRuntime command; those responsibilities remain closed to later G05 gates.
func (s *RuntimeStore) DequeueNextScheduledRun() (*domain.RunScheduleTicket, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, err := s.loadRunScheduleLocked()
	if err != nil {
		return nil, err
	}
	tickets := sortedRunScheduleTickets(state)
	if len(tickets) == 0 {
		return nil, nil
	}
	selected := tickets[0]
	if _, err := s.appendSchedulerQueueLocked(domain.RuntimeQueueItem{
		Priority:    domain.RuntimePriorityControl,
		RunID:       selected.RunID,
		RunPriority: selected.Priority,
		Category:    domain.RuntimeQueueCategoryRunDequeued,
		Summary:     "run dequeued",
	}); err != nil {
		return nil, err
	}
	return &selected, nil
}

// RemoveScheduledRun removes a queued intent without interpreting why it was
// removed. Cancellation/retry lifecycle routing remains owned by later gates.
// Removing an already-absent run is idempotent.
func (s *RuntimeStore) RemoveScheduledRun(runID domain.RunID) error {
	if err := validateSchedulableRunIdentity(runID); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	state, err := s.loadRunScheduleLocked()
	if err != nil {
		return err
	}
	ticket, ok := state.tickets[runID]
	if !ok {
		return nil
	}
	_, err = s.appendSchedulerQueueLocked(domain.RuntimeQueueItem{
		Priority:    domain.RuntimePriorityControl,
		RunID:       runID,
		RunPriority: ticket.Priority,
		Category:    domain.RuntimeQueueCategoryRunRemoved,
		Summary:     "run removed from scheduler queue",
	})
	return err
}

func (s *RuntimeStore) loadRunScheduleLocked() (runScheduleReplay, error) {
	items, err := loadJSONLines[domain.RuntimeQueueItem](s.io, runtimeQueuePath)
	if err != nil {
		return runScheduleReplay{}, err
	}
	return replayRunSchedule(items)
}

func replayRunSchedule(items []domain.RuntimeQueueItem) (runScheduleReplay, error) {
	state := runScheduleReplay{tickets: make(map[domain.RunID]domain.RunScheduleTicket)}
	for _, item := range items {
		if !domain.IsRunSchedulerQueueCategory(item.Category) {
			continue
		}
		state.currentTurn++
		if item.Seq <= 0 {
			return runScheduleReplay{}, fmt.Errorf("scheduler queue record has invalid seq %d", item.Seq)
		}
		if item.Priority != domain.RuntimePriorityControl {
			return runScheduleReplay{}, fmt.Errorf("scheduler queue record %d must use control audit priority", item.Seq)
		}
		if err := validateSchedulableRunIdentity(item.RunID); err != nil {
			return runScheduleReplay{}, fmt.Errorf("scheduler queue record %d: %w", item.Seq, err)
		}

		switch item.Category {
		case domain.RuntimeQueueCategoryRunEnqueued:
			if !item.RunPriority.Valid() {
				return runScheduleReplay{}, fmt.Errorf("scheduler enqueue %d has unsupported priority %q", item.Seq, item.RunPriority)
			}
			if _, exists := state.tickets[item.RunID]; exists {
				return runScheduleReplay{}, fmt.Errorf("scheduler enqueue %d duplicates queued run %q", item.Seq, item.RunID)
			}
			state.tickets[item.RunID] = domain.RunScheduleTicket{
				RunID:       item.RunID,
				Priority:    item.RunPriority,
				EnqueueSeq:  item.Seq,
				EnqueueTurn: state.currentTurn,
			}
		case domain.RuntimeQueueCategoryRunPriorityChanged:
			if !item.RunPriority.Valid() {
				return runScheduleReplay{}, fmt.Errorf("scheduler priority change %d has unsupported priority %q", item.Seq, item.RunPriority)
			}
			ticket, exists := state.tickets[item.RunID]
			if !exists {
				return runScheduleReplay{}, fmt.Errorf("scheduler priority change %d references non-queued run %q", item.Seq, item.RunID)
			}
			ticket.Priority = item.RunPriority
			state.tickets[item.RunID] = ticket
		case domain.RuntimeQueueCategoryRunDequeued, domain.RuntimeQueueCategoryRunRemoved:
			if _, exists := state.tickets[item.RunID]; !exists {
				return runScheduleReplay{}, fmt.Errorf("scheduler removal %d references non-queued run %q", item.Seq, item.RunID)
			}
			delete(state.tickets, item.RunID)
		}
	}
	return state, nil
}

func sortedRunScheduleTickets(state runScheduleReplay) []domain.RunScheduleTicket {
	out := make([]domain.RunScheduleTicket, 0, len(state.tickets))
	for _, ticket := range state.tickets {
		out = append(out, ticket)
	}
	sort.Slice(out, func(i, j int) bool {
		return domain.CompareRunScheduleTickets(out[i], out[j], state.currentTurn) < 0
	})
	return out
}

func validateSchedulableRunIdentity(runID domain.RunID) error {
	if err := (domain.RunIdentity{RunID: runID}).Validate(); err != nil {
		return err
	}
	if runID == domain.LegacySingleRunID {
		return fmt.Errorf("run_id %q is a read-only legacy projection", runID)
	}
	return nil
}

func (s *RuntimeStore) appendSchedulerQueueLocked(item domain.RuntimeQueueItem) (domain.RuntimeQueueItem, error) {
	if err := s.ensureSeqLoadedLocked(); err != nil {
		return item, err
	}
	s.nextSeqNum++
	item.Seq = s.nextSeqNum
	if item.Time.IsZero() {
		item.Time = time.Now().UTC()
	}
	if err := s.appendJSONLine(runtimeQueuePath, item); err != nil {
		return item, err
	}
	return item, nil
}
