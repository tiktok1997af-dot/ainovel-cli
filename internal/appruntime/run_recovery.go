package appruntime

import (
	"context"
	"fmt"
	"sort"

	"github.com/voocel/ainovel-cli/internal/domain"
)

// runRestartRecoveryBackend is deliberately narrower than runBackend so G05.8
// can consume the crash-safe Store primitive without widening normal run-control
// authority. Production Host implements this optional restart-only seam.
type runRestartRecoveryBackend interface {
	DesktopRecoverResourceLocks() ([]domain.ResourceLockState, error)
}

// recoverRestart reconciles persisted G05 run facts before the coordinator loop
// is allowed to schedule new work. Browser lane ownership is process-local and
// therefore never trusted across restart; Store scheduler/resource/run facts are
// the only recovery authority.
func (c *runCoordinator) recoverRestart(ctx context.Context) error {
	if c == nil || c.backend == nil || c.lanes == nil {
		return fmt.Errorf("run coordinator recovery is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.recoverRestartLocked(ctx)
}

func (c *runCoordinator) recoverRestartLocked(ctx context.Context) error {
	recoverer, ok := c.backend.(runRestartRecoveryBackend)
	if !ok {
		return fmt.Errorf("run backend does not support restart resource recovery")
	}
	if _, err := recoverer.DesktopRecoverResourceLocks(); err != nil {
		return fmt.Errorf("recover stale resource ownership: %w", err)
	}

	runs, err := c.backend.DesktopRunsList()
	if err != nil {
		return fmt.Errorf("load persisted runs for recovery: %w", err)
	}
	tickets, err := c.backend.DesktopScheduledRuns()
	if err != nil {
		return fmt.Errorf("load persisted scheduler for recovery: %w", err)
	}
	ticketByRun := make(map[domain.RunID]domain.RunScheduleTicket, len(tickets))
	for _, ticket := range tickets {
		ticketByRun[ticket.RunID] = ticket
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].RunID < runs[j].RunID })

	for i := range runs {
		if err := ctx.Err(); err != nil {
			return err
		}
		record := runs[i]
		if record.Source != domain.RunRecordSourceRegistry {
			continue
		}

		switch RunLifecycleState(record.State) {
		case "":
			if _, queued := ticketByRun[record.RunID]; queued {
				if err := c.cleanupRestartClaimsLocked(record.RunID, true); err != nil {
					return err
				}
				if err := c.setRunStateLocked(&record, RunStateFailed, "restart found scheduler intent without lifecycle state"); err != nil {
					return err
				}
				delete(ticketByRun, record.RunID)
				continue
			}
			if err := c.cleanupRestartClaimsLocked(record.RunID, false); err != nil {
				return err
			}

		case RunStateQueued, RunStateBlockedResource, RunStateBlockedLane:
			if _, queued := ticketByRun[record.RunID]; queued {
				// Existing scheduler intent (and a blocked-resource FIFO waiter when
				// present) is already the durable restart authority. Preserve it.
				continue
			}
			if err := c.backend.DesktopReleaseRunResourceClaims(record.RunID); err != nil {
				return fmt.Errorf("clear interrupted claims for run %q: %w", record.RunID, err)
			}
			if err := c.lanes.Release(record.RunID); err != nil {
				return fmt.Errorf("clear interrupted lane for run %q: %w", record.RunID, err)
			}
			if err := c.setRunStateLocked(&record, RunStateRecovering, "restart recovered interrupted scheduler boundary"); err != nil {
				return err
			}
			if err := c.ensureRestartTicketLocked(&record, ticketByRun); err != nil {
				return err
			}
			if err := c.setRunStateLocked(&record, RunStateQueued, "restart recovery waiting for deterministic scheduler"); err != nil {
				return err
			}

		case RunStateRecovering:
			if err := c.backend.DesktopReleaseRunResourceClaims(record.RunID); err != nil {
				return fmt.Errorf("clear recovering claims for run %q: %w", record.RunID, err)
			}
			if err := c.lanes.Release(record.RunID); err != nil {
				return fmt.Errorf("clear recovering lane for run %q: %w", record.RunID, err)
			}
			if err := c.ensureRestartTicketLocked(&record, ticketByRun); err != nil {
				return err
			}
			if err := c.setRunStateLocked(&record, RunStateQueued, "restart recovery waiting for deterministic scheduler"); err != nil {
				return err
			}

		case RunStateStarting, RunStateRunning:
			// No Engine goroutine or browser-lane ownership survives the desktop
			// process. Release stale claims, mark the orphan explicitly, then
			// requeue Resume() against canonical durable project facts.
			if err := c.backend.DesktopReleaseRunResourceClaims(record.RunID); err != nil {
				return fmt.Errorf("clear orphan resource claims for run %q: %w", record.RunID, err)
			}
			if err := c.lanes.Release(record.RunID); err != nil {
				return fmt.Errorf("clear orphan browser lane for run %q: %w", record.RunID, err)
			}
			if err := c.setRunStateLocked(&record, RunStateRecovering, "restart recovered interrupted active execution"); err != nil {
				return err
			}
			if err := c.ensureRestartTicketLocked(&record, ticketByRun); err != nil {
				return err
			}
			if err := c.setRunStateLocked(&record, RunStateQueued, "restart recovery queued durable resume"); err != nil {
				return err
			}

		case RunStatePausing:
			if err := c.cleanupRestartClaimsLocked(record.RunID, true); err != nil {
				return err
			}
			delete(ticketByRun, record.RunID)
			if err := c.setRunStateLocked(&record, RunStatePaused, "restart completed accepted pause intent"); err != nil {
				return err
			}

		case RunStateStopping:
			if err := c.cleanupRestartClaimsLocked(record.RunID, true); err != nil {
				return err
			}
			delete(ticketByRun, record.RunID)
			if err := c.setRunStateLocked(&record, RunStateStopped, "restart completed accepted stop intent"); err != nil {
				return err
			}

		case RunStateCancelling:
			if err := c.cleanupRestartClaimsLocked(record.RunID, true); err != nil {
				return err
			}
			delete(ticketByRun, record.RunID)
			if err := c.setRunStateLocked(&record, RunStateCancelled, "restart completed accepted cancel intent"); err != nil {
				return err
			}

		case RunStatePaused, RunStateStopped, RunStateCancelled, RunStateFailed, RunStateCompleted:
			// Stable non-running states never auto-resume. Remove any impossible
			// leftover scheduler/resource/lane authority fail-closed.
			if err := c.cleanupRestartClaimsLocked(record.RunID, true); err != nil {
				return err
			}
			delete(ticketByRun, record.RunID)

		default:
			if err := c.cleanupRestartClaimsLocked(record.RunID, true); err != nil {
				return err
			}
			delete(ticketByRun, record.RunID)
			if err := c.setRunStateLocked(&record, RunStateFailed, "unsupported persisted run state during restart recovery"); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *runCoordinator) ensureRestartTicketLocked(record *domain.RunRegistryRecord, ticketByRun map[domain.RunID]domain.RunScheduleTicket) error {
	if record == nil {
		return fmt.Errorf("run record is required for restart scheduling")
	}
	priority := domain.DefaultRunSchedulerPriority
	if existing, ok := ticketByRun[record.RunID]; ok {
		priority = existing.Priority
	}
	ticket, err := c.backend.DesktopEnqueueRun(record.RunID, priority)
	if err != nil {
		return fmt.Errorf("restore scheduler intent for run %q: %w", record.RunID, err)
	}
	ticketByRun[record.RunID] = ticket
	return nil
}

func (c *runCoordinator) cleanupRestartClaimsLocked(runID domain.RunID, removeSchedule bool) error {
	if removeSchedule {
		if err := c.backend.DesktopRemoveScheduledRun(runID); err != nil {
			return fmt.Errorf("remove stale scheduler intent for run %q: %w", runID, err)
		}
	}
	if err := c.backend.DesktopReleaseRunResourceClaims(runID); err != nil {
		return fmt.Errorf("release stale resource claims for run %q: %w", runID, err)
	}
	if err := c.lanes.Release(runID); err != nil {
		return fmt.Errorf("release stale browser lane for run %q: %w", runID, err)
	}
	return nil
}
