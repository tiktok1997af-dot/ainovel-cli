package appruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/webai"
)

const defaultRunQueryPageSize = 100

func isRunCenterQueryKind(kind QueryKind) bool {
	switch kind {
	case QueryRunsList, QueryRunsGet, QueryRunsActivity:
		return true
	default:
		return false
	}
}

func (r *Runtime) routeRunCenterQuery(ctx context.Context, req QueryRequest) (json.RawMessage, error) {
	if r.run == nil {
		return nil, ErrNotImplemented
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	switch req.Kind {
	case QueryRunsList:
		var payload RunsListQuery
		if err := decodeQueryPayload(req.Payload, &payload); err != nil {
			return nil, err
		}
		if err := validatePage(payload.PageQuery); err != nil {
			return nil, err
		}
		payload.State = strings.TrimSpace(payload.State)
		return r.queryRunsList(payload)
	case QueryRunsGet:
		var payload RunsGetQuery
		if err := decodeQueryPayload(req.Payload, &payload); err != nil {
			return nil, err
		}
		if err := validateRunQueryID(payload.RunID); err != nil {
			return nil, err
		}
		return r.queryRunsGet(payload)
	case QueryRunsActivity:
		var payload RunsActivityQuery
		if err := decodeQueryPayload(req.Payload, &payload); err != nil {
			return nil, err
		}
		if err := validatePage(payload.PageQuery); err != nil {
			return nil, err
		}
		if err := validateRunQueryID(payload.RunID); err != nil {
			return nil, err
		}
		return r.queryRunsActivity(payload)
	default:
		return nil, ErrNotImplemented
	}
}

func (r *Runtime) queryRunsList(req RunsListQuery) (json.RawMessage, error) {
	records, err := r.run.backend.DesktopRunsList()
	if err != nil {
		return nil, err
	}
	summaries, err := r.projectRunSummaries(records)
	if err != nil {
		return nil, err
	}
	if req.State != "" {
		filtered := summaries[:0]
		for _, item := range summaries {
			if item.State == req.State {
				filtered = append(filtered, item)
			}
		}
		summaries = append([]RunSummaryDTO(nil), filtered...)
	}
	total := len(summaries)
	limit := effectiveRunQueryLimit(req.Limit)
	start, end := runPageBounds(req.Offset, limit, total)
	lanes := projectBrowserLanes(r.run.lanes.List())
	return marshalQueryData(RunsListResultDTO{
		Items:  append([]RunSummaryDTO(nil), summaries[start:end]...),
		Lanes:  lanes,
		Offset: req.Offset,
		Limit:  limit,
		Total:  total,
	})
}

func (r *Runtime) queryRunsGet(req RunsGetQuery) (json.RawMessage, error) {
	records, err := r.run.backend.DesktopRunsList()
	if err != nil {
		return nil, err
	}
	var selected *domain.RunRegistryRecord
	for i := range records {
		if records[i].RunID == req.RunID {
			copy := records[i]
			selected = &copy
			break
		}
	}
	if selected == nil {
		return nil, fmt.Errorf("%w: run not found", ErrInvalidQuery)
	}
	summaries, err := r.projectRunSummaries([]domain.RunRegistryRecord{*selected})
	if err != nil {
		return nil, err
	}
	result := RunsGetResultDTO{Run: summaries[0]}
	if selected.TaskID != "" {
		result.Tasks = []RunTaskDTO{{
			TaskID:  selected.TaskID,
			State:   selected.State,
			Summary: selected.Status,
		}}
	}
	return marshalQueryData(result)
}

func (r *Runtime) queryRunsActivity(req RunsActivityQuery) (json.RawMessage, error) {
	if req.RunID == domain.LegacySingleRunID {
		return marshalQueryData(RunsActivityResultDTO{
			Offset: req.Offset,
			Limit:  effectiveRunQueryLimit(req.Limit),
		})
	}
	record, err := r.run.backend.DesktopRunLoad(req.RunID)
	if err != nil {
		return nil, err
	}
	if record == nil {
		return nil, fmt.Errorf("%w: run not found", ErrInvalidQuery)
	}
	history, err := r.run.backend.DesktopRunHistory(req.RunID)
	if err != nil {
		return nil, err
	}
	total := len(history)
	limit := effectiveRunQueryLimit(req.Limit)
	start, end := runPageBounds(req.Offset, limit, total)
	items := make([]RunActivityDTO, 0, end-start)
	for _, entry := range history[start:end] {
		items = append(items, RunActivityDTO{
			Seq:      entry.Seq,
			Time:     entry.Time.UTC(),
			Category: entry.Category,
			Type:     entry.Type,
			Level:    entry.Level,
			TaskID:   entry.TaskID,
			Summary:  entry.Summary,
		})
	}
	return marshalQueryData(RunsActivityResultDTO{
		Items:  items,
		Offset: req.Offset,
		Limit:  limit,
		Total:  total,
	})
}

func (r *Runtime) projectRunSummaries(records []domain.RunRegistryRecord) ([]RunSummaryDTO, error) {
	tickets, err := r.run.backend.DesktopScheduledRuns()
	if err != nil {
		return nil, err
	}
	priorityByRun := make(map[domain.RunID]string, len(tickets))
	for _, ticket := range tickets {
		priorityByRun[ticket.RunID] = string(ticket.Priority)
	}
	locks, err := r.run.backend.DesktopResourceLocks()
	if err != nil {
		return nil, err
	}
	resourceByRun := make(map[domain.RunID]domain.ResourceKey)
	for _, lock := range locks {
		if lock.OwnerRunID != "" {
			resourceByRun[lock.OwnerRunID] = lock.Resource
		}
		for _, waiter := range lock.Waiters {
			resourceByRun[waiter.RunID] = lock.Resource
		}
	}
	laneByRun := make(map[domain.RunID]domain.BrowserLaneID)
	for _, lane := range r.run.lanes.List() {
		if lane.RunID != "" {
			laneByRun[lane.RunID] = lane.LaneID
		}
	}

	out := make([]RunSummaryDTO, 0, len(records))
	for _, record := range records {
		state := record.State
		status := record.Status
		if record.Source == domain.RunRecordSourceLegacySingleRun {
			if state == "" {
				state = "legacy"
			}
			if status == "" {
				status = string(record.LegacyPhase)
			}
		}
		out = append(out, RunSummaryDTO{
			RunID:      record.RunID,
			State:      state,
			Status:     status,
			TaskID:     record.TaskID,
			Priority:   priorityByRun[record.RunID],
			WaitReason: runWaitReason(state),
			Resource:   resourceByRun[record.RunID],
			LaneID:     laneByRun[record.RunID],
			CreatedAt:  record.CreatedAt.UTC(),
			UpdatedAt:  record.UpdatedAt.UTC(),
			StartedAt:  record.StartedAt.UTC(),
			FinishedAt: record.FinishedAt.UTC(),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if runStateSortRank(out[i].State) != runStateSortRank(out[j].State) {
			return runStateSortRank(out[i].State) < runStateSortRank(out[j].State)
		}
		return out[i].RunID < out[j].RunID
	})
	return out, nil
}

func projectBrowserLanes(lanes []webai.BrowserLaneProjection) []BrowserLaneDTO {
	out := make([]BrowserLaneDTO, 0, len(lanes))
	for _, lane := range lanes {
		out = append(out, BrowserLaneDTO{
			LaneID:        lane.LaneID,
			State:         string(lane.State),
			RunID:         lane.RunID,
			RecoveryCount: lane.RecoveryCount,
			ChangedAt:     lane.ChangedAt.UTC(),
		})
	}
	return out
}

func validateRunQueryID(runID domain.RunID) error {
	if runID == "" {
		return invalidQuery("run id is required")
	}
	if runID == domain.LegacySingleRunID {
		return nil
	}
	if err := (domain.RunIdentity{RunID: runID}).Validate(); err != nil {
		return invalidQuery("run id is invalid")
	}
	return nil
}

func effectiveRunQueryLimit(limit int) int {
	if limit <= 0 {
		return defaultRunQueryPageSize
	}
	return limit
}

func runPageBounds(offset, limit, total int) (int, int) {
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return offset, end
}

func runStateSortRank(state string) int {
	switch RunLifecycleState(state) {
	case RunStateRunning, RunStateStarting, RunStatePausing, RunStateStopping, RunStateCancelling:
		return 0
	case RunStateBlockedResource, RunStateBlockedLane, RunStateQueued, RunStateRecovering:
		return 1
	case RunStatePaused:
		return 2
	case RunStateFailed:
		return 3
	case RunStateCompleted, RunStateCancelled, RunStateStopped:
		return 4
	default:
		return 5
	}
}
