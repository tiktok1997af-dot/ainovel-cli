package desktopui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/voocel/ainovel-cli/internal/appruntime"
)

type WritePlaneState struct {
	Pending   bool
	CommandID string
	Kind      appruntime.CommandKind
	Accepted  bool
	Status    string
	Resource  string
	Error     *ErrorView
}

type writeControlPlane struct {
	mu     sync.Mutex
	nextID uint64
}

func (c *Controller) WriteState() WritePlaneState {
	c.write.mu.Lock()
	defer c.write.mu.Unlock()
	out := c.shell.Write
	if out.Error != nil {
		copyErr := *out.Error
		out.Error = &copyErr
	}
	return out
}

func (c *Controller) SaveProjectMetadata(ctx context.Context, payload appruntime.ProjectMetadataUpdatePayload) (appruntime.CommandResult, error) {
	return c.executeWrite(ctx, appruntime.CommandProjectMetadataUpdate, payload, func() error {
		if err := c.Refresh(ctx); err != nil {
			return err
		}
		if c.shell.Route == RouteProject {
			return c.LoadProjectOverview(ctx)
		}
		return nil
	})
}

func (c *Controller) SaveProjectPremise(ctx context.Context, payload appruntime.ProjectPremiseUpdatePayload) (appruntime.CommandResult, error) {
	return c.executeWrite(ctx, appruntime.CommandProjectPremiseUpdate, payload, func() error {
		if err := c.Refresh(ctx); err != nil {
			return err
		}
		if c.shell.Route == RouteProject {
			return c.LoadProjectOverview(ctx)
		}
		return nil
	})
}

func (c *Controller) SaveChapterPlan(ctx context.Context, payload appruntime.ChapterPlanSavePayload) (appruntime.CommandResult, error) {
	chapter := payload.Chapter
	return c.executeWrite(ctx, appruntime.CommandChapterPlanSave, payload, func() error {
		if c.shell.Route == RouteCreative {
			return c.reconcileCreativeChapter(ctx, chapter, true)
		}
		if err := c.Refresh(ctx); err != nil {
			return err
		}
		if c.shell.Route == RouteProject {
			return c.LoadChapter(ctx, chapter)
		}
		return nil
	})
}

func (c *Controller) SaveChapterDraft(ctx context.Context, payload appruntime.ChapterTextSavePayload) (appruntime.CommandResult, error) {
	chapter := payload.Chapter
	return c.executeWrite(ctx, appruntime.CommandChapterDraftSave, payload, func() error {
		if c.shell.Route == RouteCreative {
			return c.reconcileCreativeChapter(ctx, chapter, true)
		}
		if err := c.Refresh(ctx); err != nil {
			return err
		}
		if c.shell.Route == RouteProject {
			return c.LoadChapter(ctx, chapter)
		}
		return nil
	})
}

func (c *Controller) SaveChapterWorkspace(ctx context.Context, payload appruntime.ChapterTextSavePayload) (appruntime.CommandResult, error) {
	chapter := payload.Chapter
	return c.executeWrite(ctx, appruntime.CommandChapterWorkspaceSave, payload, func() error {
		if c.shell.Route == RouteCreative {
			return c.reconcileCreativeChapter(ctx, chapter, true)
		}
		if err := c.Refresh(ctx); err != nil {
			return err
		}
		if c.shell.Route == RouteProject {
			return c.LoadChapter(ctx, chapter)
		}
		return nil
	})
}

func (c *Controller) ReviseOutlineTail(ctx context.Context, payload appruntime.OutlineTailRevisePayload) (appruntime.CommandResult, error) {
	return c.executeWrite(ctx, appruntime.CommandOutlineTailRevise, payload, func() error {
		if err := c.Refresh(ctx); err != nil {
			return err
		}
		if c.shell.Route == RouteProject {
			return c.LoadOutlineWindow(ctx, payload.FromChapter)
		}
		return nil
	})
}

func (c *Controller) ExpandOutlineArc(ctx context.Context, payload appruntime.OutlineArcExpandPayload) (appruntime.CommandResult, error) {
	return c.executeWrite(ctx, appruntime.CommandOutlineArcExpand, payload, func() error {
		if err := c.Refresh(ctx); err != nil {
			return err
		}
		if c.shell.Route == RouteProject {
			return c.LoadOutlineWindow(ctx, 1)
		}
		return nil
	})
}

func (c *Controller) AppendOutlineVolume(ctx context.Context, payload appruntime.OutlineVolumeAppendPayload) (appruntime.CommandResult, error) {
	return c.executeWrite(ctx, appruntime.CommandOutlineVolumeAppend, payload, func() error {
		if err := c.Refresh(ctx); err != nil {
			return err
		}
		if c.shell.Route == RouteProject {
			return c.LoadOutlineWindow(ctx, 1)
		}
		return nil
	})
}

func (c *Controller) UpdateOutlineCompass(ctx context.Context, payload appruntime.OutlineCompassUpdatePayload) (appruntime.CommandResult, error) {
	return c.executeWrite(ctx, appruntime.CommandOutlineCompassUpdate, payload, func() error {
		if err := c.Refresh(ctx); err != nil {
			return err
		}
		if c.shell.Route == RouteProject {
			return c.LoadOutlineWindow(ctx, 1)
		}
		return nil
	})
}

func (c *Controller) ReplaceCoreCharacters(ctx context.Context, payload appruntime.KnowledgeCharactersReplacePayload) (appruntime.CommandResult, error) {
	return c.executeWrite(ctx, appruntime.CommandKnowledgeCharactersReplace, payload, func() error {
		if err := c.Refresh(ctx); err != nil {
			return err
		}
		if c.shell.Route == RouteKnowledge {
			return c.LoadKnowledgeCharacters(ctx, 0, "core")
		}
		return nil
	})
}

func (c *Controller) ReplaceWorldRules(ctx context.Context, payload appruntime.KnowledgeWorldRulesReplacePayload) (appruntime.CommandResult, error) {
	return c.executeWrite(ctx, appruntime.CommandKnowledgeWorldRulesReplace, payload, func() error {
		if err := c.Refresh(ctx); err != nil {
			return err
		}
		if c.shell.Route == RouteKnowledge {
			return c.LoadKnowledgeWorld(ctx, []string{"rules"})
		}
		return nil
	})
}

func (c *Controller) AppendTimeline(ctx context.Context, payload appruntime.KnowledgeTimelineAppendPayload) (appruntime.CommandResult, error) {
	return c.executeWrite(ctx, appruntime.CommandKnowledgeTimelineAppend, payload, func() error {
		if err := c.Refresh(ctx); err != nil {
			return err
		}
		if c.shell.Route == RouteKnowledge {
			return c.LoadKnowledgeTimeline(ctx, 0, 0)
		}
		return nil
	})
}

func (c *Controller) UpdateRelationships(ctx context.Context, payload appruntime.KnowledgeRelationshipsUpdatePayload) (appruntime.CommandResult, error) {
	return c.executeWrite(ctx, appruntime.CommandKnowledgeRelationshipsUpdate, payload, func() error {
		if err := c.Refresh(ctx); err != nil {
			return err
		}
		if c.shell.Route == RouteKnowledge {
			return c.LoadKnowledgeWorld(ctx, []string{"relationships"})
		}
		return nil
	})
}

func (c *Controller) UpdateForeshadow(ctx context.Context, payload appruntime.KnowledgeForeshadowUpdatePayload) (appruntime.CommandResult, error) {
	return c.executeWrite(ctx, appruntime.CommandKnowledgeForeshadowUpdate, payload, func() error {
		if err := c.Refresh(ctx); err != nil {
			return err
		}
		if c.shell.Route == RouteKnowledge {
			return c.LoadKnowledgeWorld(ctx, []string{"foreshadow"})
		}
		return nil
	})
}

func (c *Controller) executeWrite(ctx context.Context, kind appruntime.CommandKind, payload any, reconcile func() error) (appruntime.CommandResult, error) {
	if c.runtime == nil {
		return appruntime.CommandResult{}, ErrNilRuntime
	}
	if !isG03WriteKind(kind) {
		return appruntime.CommandResult{}, writeAppError(appruntime.ErrorCodeUnsupportedOperation, appruntime.ErrorCategoryValidation, "This project action is not supported.", false)
	}
	if !c.shell.HasSnapshot {
		return appruntime.CommandResult{}, writeAppError(appruntime.ErrorCodeRuntimeUnavailable, appruntime.ErrorCategoryRuntime, "The application runtime is unavailable.", true)
	}
	if !mutationPresentationAllowed(c.shell.Runtime.State) {
		return appruntime.CommandResult{}, writeAppError(appruntime.ErrorCodePreconditionConflict, appruntime.ErrorCategoryConflict, "The project changed state before this action could be applied.", false)
	}
	raw, err := json.Marshal(payload)
	if err != nil || len(raw) == 0 || len(raw) > appruntime.MaxMutationPayloadBytes {
		return appruntime.CommandResult{}, writeAppError(appruntime.ErrorCodeInvalidArgument, appruntime.ErrorCategoryValidation, "The request is invalid.", false)
	}

	commandID, err := c.beginWrite(kind)
	if err != nil {
		return appruntime.CommandResult{}, err
	}
	request := appruntime.CommandRequest{
		ContractVersion: appruntime.ContractVersion,
		ID:              commandID,
		Kind:            kind,
		Payload:         raw,
	}
	result, dispatchErr := c.runtime.Dispatch(ctx, request)

	var operationErr error
	switch {
	case dispatchErr != nil:
		operationErr = dispatchErr
	case result.Error != nil:
		operationErr = result.Error
	case result.ContractVersion != appruntime.ContractVersion:
		operationErr = writeAppError(appruntime.ErrorCodeContractMismatch, appruntime.ErrorCategoryValidation, "The desktop and core contract versions are incompatible.", false)
	case result.CommandID != commandID:
		operationErr = writeAppError(appruntime.ErrorCodeInternal, appruntime.ErrorCategoryInternal, "An internal runtime error occurred.", false)
	case !result.Accepted:
		operationErr = writeAppError(appruntime.ErrorCodeCommandRejected, appruntime.ErrorCategoryRuntime, "The runtime rejected this action.", false)
	case strings.TrimSpace(result.Resource) == "":
		operationErr = writeAppError(appruntime.ErrorCodeInternal, appruntime.ErrorCategoryInternal, "An internal runtime error occurred.", false)
	case len(result.Data) == 0 || !json.Valid(result.Data):
		operationErr = writeAppError(appruntime.ErrorCodeInternal, appruntime.ErrorCategoryInternal, "An internal runtime error occurred.", false)
	}

	var reconcileErr error
	if reconcile != nil {
		reconcileErr = reconcile()
	}
	if operationErr != nil {
		c.finishWrite(commandID, result, false, viewErrorFromError(operationErr))
	} else {
		c.finishWrite(commandID, result, true, nil)
	}
	if operationErr != nil {
		return result, errors.Join(operationErr, reconcileErr)
	}
	if reconcileErr != nil {
		return result, reconcileErr
	}
	return result, nil
}

func (c *Controller) beginWrite(kind appruntime.CommandKind) (string, error) {
	c.write.mu.Lock()
	defer c.write.mu.Unlock()
	if c.shell.Write.Pending {
		return "", writeAppError(appruntime.ErrorCodePreconditionConflict, appruntime.ErrorCategoryConflict, "The project changed state before this action could be applied.", false)
	}
	c.write.nextID++
	commandID := fmt.Sprintf("desktop-write-%d", c.write.nextID)
	c.shell.Write = WritePlaneState{Pending: true, CommandID: commandID, Kind: kind}
	return commandID, nil
}

func (c *Controller) finishWrite(commandID string, result appruntime.CommandResult, accepted bool, viewErr *ErrorView) {
	c.write.mu.Lock()
	defer c.write.mu.Unlock()
	if c.shell.Write.CommandID != commandID {
		return
	}
	c.shell.Write.Pending = false
	c.shell.Write.Accepted = accepted
	c.shell.Write.Status = result.Status
	c.shell.Write.Resource = result.Resource
	c.shell.Write.Error = viewErr
	if viewErr != nil {
		switch c.shell.Route {
		case RouteCreative:
			c.creative.Error = viewErr
			c.creative.Load = loadStateForWriteError(viewErr)
		case RouteProject:
			c.shell.Project.Error = viewErr
			c.shell.Project.Load = loadStateForWriteError(viewErr)
		case RouteKnowledge:
			c.shell.Knowledge.Error = viewErr
			c.shell.Knowledge.Load = loadStateForWriteError(viewErr)
		}
	}
}

func loadStateForWriteError(viewErr *ErrorView) LoadState {
	if viewErr == nil {
		return LoadRuntimeError
	}
	switch viewErr.Category {
	case string(appruntime.ErrorCategoryValidation):
		return LoadValidationError
	case string(appruntime.ErrorCategoryConflict):
		return LoadConflict
	default:
		return LoadRuntimeError
	}
}

func mutationPresentationAllowed(state string) bool {
	switch appruntime.DesktopLifecycleState(strings.ToLower(strings.TrimSpace(state))) {
	case appruntime.LifecycleStarting, appruntime.LifecycleRunning, appruntime.LifecyclePausing,
		appruntime.LifecycleResuming, appruntime.LifecycleStopping, appruntime.LifecycleCancelling,
		appruntime.LifecycleRecovering:
		return false
	default:
		return true
	}
}

func isG03WriteKind(kind appruntime.CommandKind) bool {
	switch kind {
	case appruntime.CommandProjectMetadataUpdate,
		appruntime.CommandProjectPremiseUpdate,
		appruntime.CommandChapterPlanSave,
		appruntime.CommandChapterDraftSave,
		appruntime.CommandChapterWorkspaceSave,
		appruntime.CommandOutlineTailRevise,
		appruntime.CommandOutlineArcExpand,
		appruntime.CommandOutlineVolumeAppend,
		appruntime.CommandOutlineCompassUpdate,
		appruntime.CommandKnowledgeCharactersReplace,
		appruntime.CommandKnowledgeWorldRulesReplace,
		appruntime.CommandKnowledgeTimelineAppend,
		appruntime.CommandKnowledgeRelationshipsUpdate,
		appruntime.CommandKnowledgeForeshadowUpdate:
		return true
	default:
		return false
	}
}

func writeAppError(code appruntime.ErrorCode, category appruntime.ErrorCategory, message string, retryable bool) *appruntime.AppError {
	return &appruntime.AppError{Code: code, Category: category, Message: message, Retryable: retryable}
}
