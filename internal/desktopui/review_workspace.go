package desktopui

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/voocel/ainovel-cli/internal/appruntime"
)

// ReviewWorkspaceState is a presentation projection only. Canonical quality
// evidence, freshness, repair eligibility, and Official promotion authority
// remain owned by AppRuntime. PromotionReceipt is intentionally transient: the
// parent contract has no durable Official-status query, so the UI must never
// turn this receipt into persistent truth.
type ReviewWorkspaceState struct {
	Load             LoadState
	Target           appruntime.ReviewTargetDTO
	Catalog          appruntime.ReviewContractCatalog
	Status           appruntime.ReviewStatusResultDTO
	History          []appruntime.ReviewHistoryItemDTO
	HistoryTotal     int
	LastCommand      *appruntime.ReviewCommandResultDTO
	PromotionReceipt *appruntime.ReviewPromoteOfficialResultDTO
	Error            *ErrorView
}

func NewReviewWorkspaceState() ReviewWorkspaceState {
	return ReviewWorkspaceState{Load: LoadInitial}
}

func (c *Controller) Review() *ReviewWorkspaceState {
	if c == nil {
		return nil
	}
	return &c.review
}

// Supports reports only what the authoritative Review contract advertises. It
// deliberately does not derive quality thresholds or promotion eligibility.
func (s *ReviewWorkspaceState) Supports(kind appruntime.CommandKind) bool {
	if s == nil {
		return false
	}
	return slices.Contains(s.Catalog.CommandKinds, kind)
}

func (c *Controller) OpenReview(ctx context.Context, target appruntime.ReviewTargetDTO) error {
	if c == nil || c.shell == nil {
		return fmt.Errorf("desktopui: Review route is unavailable")
	}
	if !c.shell.SelectRoute(RouteReview) {
		return fmt.Errorf("desktopui: Review route is unavailable")
	}
	if c.review.Target != target {
		c.review = NewReviewWorkspaceState()
	}
	c.review.Target = target
	return c.RefreshReview(ctx)
}

func (c *Controller) RefreshReview(ctx context.Context) error {
	if c == nil || c.runtime == nil {
		return ErrNilRuntime
	}
	state := &c.review
	if state.Target.Scope == "" {
		return fmt.Errorf("desktopui: Review target is not selected")
	}
	if len(state.Catalog.Gates) == 0 {
		state.Load = LoadLoading
	} else {
		state.Load = LoadRefreshing
	}
	state.Error = nil

	var catalog appruntime.ReviewCatalogResultDTO
	if err := c.queryReview(ctx, appruntime.QueryReviewCatalog, nil, &catalog); err != nil {
		return c.failReview(err)
	}

	statusPayload, err := json.Marshal(appruntime.ReviewStatusQuery{Target: state.Target})
	if err != nil {
		return c.failReview(err)
	}
	var status appruntime.ReviewStatusResultDTO
	if err := c.queryReview(ctx, appruntime.QueryReviewStatus, statusPayload, &status); err != nil {
		return c.failReview(err)
	}

	historyPayload, err := json.Marshal(appruntime.ReviewHistoryQuery{
		PageQuery: appruntime.PageQuery{Limit: 100},
		Target:    state.Target,
	})
	if err != nil {
		return c.failReview(err)
	}
	var history appruntime.ReviewHistoryResultDTO
	if err := c.queryReview(ctx, appruntime.QueryReviewHistory, historyPayload, &history); err != nil {
		return c.failReview(err)
	}

	state.Catalog = catalog.Contract
	state.Status = status
	state.History = append([]appruntime.ReviewHistoryItemDTO(nil), history.Items...)
	state.HistoryTotal = history.Total
	state.Load = LoadReady
	state.Error = nil
	return nil
}

func (c *Controller) RunReview(ctx context.Context) error {
	state, err := c.reviewReadyForCommand(appruntime.CommandReviewRun)
	if err != nil {
		return err
	}
	payload := appruntime.ReviewRunCommandPayload{
		Target:              state.Target,
		ExpectedFingerprint: state.Status.Freshness.Fingerprint,
	}
	return c.dispatchReviewAction(ctx, appruntime.CommandReviewRun, payload, false)
}

func (c *Controller) RepairReview(ctx context.Context, gateIDs []appruntime.ReviewGateID, chapters []int) error {
	state, err := c.reviewReadyForCommand(appruntime.CommandReviewRepair)
	if err != nil {
		return err
	}
	payload := appruntime.ReviewRepairCommandPayload{
		Target:              state.Target,
		GateIDs:             slices.Clone(gateIDs),
		Chapters:            slices.Clone(chapters),
		ExpectedFingerprint: state.Status.Freshness.Fingerprint,
	}
	return c.dispatchReviewAction(ctx, appruntime.CommandReviewRepair, payload, false)
}

func (c *Controller) RerunReview(ctx context.Context) error {
	state, err := c.reviewReadyForCommand(appruntime.CommandReviewRerun)
	if err != nil {
		return err
	}
	payload := appruntime.ReviewRerunCommandPayload{
		Target:              state.Target,
		ExpectedFingerprint: state.Status.Freshness.Fingerprint,
	}
	return c.dispatchReviewAction(ctx, appruntime.CommandReviewRerun, payload, false)
}

func (c *Controller) PromoteReviewOfficial(ctx context.Context) error {
	state, err := c.reviewReadyForCommand(appruntime.CommandReviewPromoteOfficial)
	if err != nil {
		return err
	}
	payload := appruntime.ReviewPromoteOfficialCommandPayload{
		Target:              state.Target,
		ExpectedFingerprint: state.Status.Freshness.Fingerprint,
		ExpectedRevisions:   append([]appruntime.ReviewRevisionRefDTO(nil), state.Status.Freshness.Revisions...),
	}
	return c.dispatchReviewAction(ctx, appruntime.CommandReviewPromoteOfficial, payload, true)
}

func (c *Controller) reviewReadyForCommand(kind appruntime.CommandKind) (*ReviewWorkspaceState, error) {
	if c == nil || c.runtime == nil {
		return nil, ErrNilRuntime
	}
	state := &c.review
	if state.Target.Scope == "" || len(state.Catalog.Gates) == 0 {
		return nil, fmt.Errorf("desktopui: Review workspace is not loaded")
	}
	if !state.Supports(kind) {
		return nil, fmt.Errorf("desktopui: Review command %q is not advertised by AppRuntime", kind)
	}
	return state, nil
}

func (c *Controller) dispatchReviewAction(ctx context.Context, kind appruntime.CommandKind, payload any, promotion bool) error {
	state := &c.review
	raw, err := json.Marshal(payload)
	if err != nil {
		return c.failReview(err)
	}
	state.Load = LoadCommandPending
	state.Error = nil

	result, err := c.runtime.Dispatch(ctx, appruntime.CommandRequest{
		ContractVersion: appruntime.ContractVersion,
		ID:              c.nextRequestID(),
		Kind:            kind,
		Payload:         raw,
	})
	if err != nil {
		return c.failReview(err)
	}
	if result.Error != nil {
		return c.failReview(result.Error)
	}
	if result.ContractVersion != appruntime.ContractVersion {
		state.Load = LoadRuntimeError
		state.Error = &ErrorView{
			Code:     string(appruntime.ErrorCodeContractMismatch),
			Category: string(appruntime.ErrorCategoryValidation),
			Message:  "The desktop and core contract versions are incompatible.",
		}
		return fmt.Errorf("desktopui: Review command contract mismatch")
	}
	if !result.Accepted {
		return c.failReview(fmt.Errorf("desktopui: Review command %q was not accepted", kind))
	}

	if promotion {
		var receipt appruntime.ReviewPromoteOfficialResultDTO
		if err := json.Unmarshal(result.Data, &receipt); err != nil {
			return c.failReview(err)
		}
		state.PromotionReceipt = &receipt
		state.LastCommand = nil
	} else {
		var commandResult appruntime.ReviewCommandResultDTO
		if len(result.Data) != 0 {
			if err := json.Unmarshal(result.Data, &commandResult); err != nil {
				return c.failReview(err)
			}
		}
		state.LastCommand = &commandResult
	}
	return c.RefreshReview(ctx)
}

func (c *Controller) queryReview(ctx context.Context, kind appruntime.QueryKind, payload json.RawMessage, dst any) error {
	result, err := c.runtime.Query(ctx, appruntime.QueryRequest{
		ContractVersion: appruntime.ContractVersion,
		Kind:            kind,
		Payload:         payload,
	})
	if err != nil {
		return err
	}
	if result.Error != nil {
		return result.Error
	}
	if result.ContractVersion != appruntime.ContractVersion {
		return fmt.Errorf("desktopui: Review query contract mismatch")
	}
	if err := json.Unmarshal(result.Data, dst); err != nil {
		return err
	}
	return nil
}

func (c *Controller) failReview(err error) error {
	c.review.Load = LoadRuntimeError
	c.review.Error = viewErrorFromError(err)
	return err
}

func shouldRefreshReviewFromEvent(event appruntime.DesktopEvent) bool {
	if event.Category == appruntime.ReviewEventCategory {
		switch event.Type {
		case appruntime.EventTypeReviewState, appruntime.EventTypeReviewGate, appruntime.EventTypeReviewAction:
			return true
		}
	}
	if event.Category != appruntime.RunEventCategory {
		return false
	}
	switch appruntime.CommandKind(event.TaskID) {
	case appruntime.CommandReviewRun, appruntime.CommandReviewRepair, appruntime.CommandReviewRerun:
		return true
	default:
		return false
	}
}
