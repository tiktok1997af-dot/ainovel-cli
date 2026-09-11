package store

import (
	"fmt"

	"github.com/voocel/ainovel-cli/internal/domain"
)

// DequeueScheduledRun removes one specific runnable ticket after G05.7 has
// applied resource/lane constraints. Scheduler priority still determines the
// candidate whenever no older resource waiter constrains runnable work.
func (s *RuntimeStore) DequeueScheduledRun(runID domain.RunID) (*domain.RunScheduleTicket, error) {
	if err := validateSchedulableRunIdentity(runID); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	state, err := s.loadRunScheduleLocked()
	if err != nil {
		return nil, err
	}
	ticket, ok := state.tickets[runID]
	if !ok {
		return nil, fmt.Errorf("run %q is not queued", runID)
	}
	if _, err := s.appendSchedulerQueueLocked(domain.RuntimeQueueItem{
		Priority:    domain.RuntimePriorityControl,
		RunID:       runID,
		RunPriority: ticket.Priority,
		Category:    domain.RuntimeQueueCategoryRunDequeued,
		Summary:     "runnable run dequeued by orchestrator",
	}); err != nil {
		return nil, err
	}
	return &ticket, nil
}
