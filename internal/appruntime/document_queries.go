package appruntime

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/voocel/ainovel-cli/internal/host"
)

const (
	DocumentKindProject   = "project"
	DocumentKindOutline   = "outline"
	DocumentKindKnowledge = "knowledge"
	DocumentKindChapter   = "chapter"
	DocumentKindSummary   = "summary"
)

func (r *Runtime) queryDocumentsList(req DocumentsListQuery) (json.RawMessage, error) {
	artifacts, err := r.core.DesktopDocumentCatalog()
	if err != nil {
		return nil, err
	}

	filtered := make([]DocumentSummaryDTO, 0, len(artifacts))
	for _, artifact := range artifacts {
		if req.Kind != "" && artifact.Kind != req.Kind {
			continue
		}
		if req.Prefix != "" && !strings.HasPrefix(artifact.Path, req.Prefix) {
			continue
		}
		filtered = append(filtered, documentSummaryDTO(artifact))
	}

	limit := effectiveProjectQueryLimit(req.Limit)
	start, end := pageBounds(req.Offset, limit, len(filtered))
	items := append([]DocumentSummaryDTO(nil), filtered[start:end]...)
	return marshalQueryData(DocumentsListResultDTO{
		Items:  items,
		Offset: req.Offset,
		Limit:  limit,
		Total:  len(filtered),
	})
}

func (r *Runtime) queryDocumentsGet(req DocumentsGetQuery) (json.RawMessage, error) {
	artifact, content, err := r.core.DesktopDocumentRead(req.ID)
	if err != nil {
		if errors.Is(err, host.ErrDesktopDocumentNotFound) {
			return nil, invalidQuery("document id is not present in the supported catalog")
		}
		return nil, err
	}
	return marshalQueryData(DocumentsGetResultDTO{
		Document: documentSummaryDTO(artifact),
		Content:  content,
	})
}

func documentSummaryDTO(artifact host.DesktopDocumentArtifact) DocumentSummaryDTO {
	return DocumentSummaryDTO{
		ID:          artifact.ID,
		Kind:        artifact.Kind,
		Path:        artifact.Path,
		Title:       artifact.Title,
		ContentType: artifact.ContentType,
		ReadOnly:    true,
		SizeBytes:   artifact.SizeBytes,
	}
}
