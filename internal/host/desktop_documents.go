package host

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	apperrs "github.com/voocel/ainovel-cli/internal/errs"
)

var ErrDesktopDocumentNotFound = errors.New("desktop document not found")

type DesktopDocumentArtifact struct {
	ID          string
	Kind        string
	Path        string
	Title       string
	ContentType string
	SizeBytes   int64
}

type desktopDocumentSpec struct {
	ID          string
	Kind        string
	Path        string
	Title       string
	ContentType string
}

var desktopStaticDocumentSpecs = []desktopDocumentSpec{
	{ID: "project.format", Kind: "project", Path: "meta/format.json", Title: "Project format", ContentType: "application/json"},
	{ID: "project.book", Kind: "project", Path: "meta/book.json", Title: "Book metadata", ContentType: "application/json"},
	{ID: "project.book-markdown", Kind: "project", Path: "book.md", Title: "Book", ContentType: "text/markdown; charset=utf-8"},
	{ID: "project.progress", Kind: "project", Path: "meta/progress.json", Title: "Writing progress", ContentType: "application/json"},
	{ID: "project.premise", Kind: "project", Path: "premise.md", Title: "Premise", ContentType: "text/markdown; charset=utf-8"},
	{ID: "outline.flat", Kind: "outline", Path: "outline.json", Title: "Flat outline", ContentType: "application/json"},
	{ID: "outline.flat-markdown", Kind: "outline", Path: "outline.md", Title: "Flat outline", ContentType: "text/markdown; charset=utf-8"},
	{ID: "outline.layered", Kind: "outline", Path: "layered_outline.json", Title: "Layered outline", ContentType: "application/json"},
	{ID: "outline.layered-markdown", Kind: "outline", Path: "layered_outline.md", Title: "Layered outline", ContentType: "text/markdown; charset=utf-8"},
	{ID: "outline.compass", Kind: "outline", Path: "meta/compass.json", Title: "Story compass", ContentType: "application/json"},
	{ID: "knowledge.characters", Kind: "knowledge", Path: "characters.json", Title: "Characters", ContentType: "application/json"},
	{ID: "knowledge.characters-markdown", Kind: "knowledge", Path: "characters.md", Title: "Characters", ContentType: "text/markdown; charset=utf-8"},
	{ID: "knowledge.cast", Kind: "knowledge", Path: "meta/cast_ledger.json", Title: "Supporting cast ledger", ContentType: "application/json"},
	{ID: "knowledge.world-rules", Kind: "knowledge", Path: "world_rules.json", Title: "World rules", ContentType: "application/json"},
	{ID: "knowledge.world-rules-markdown", Kind: "knowledge", Path: "world_rules.md", Title: "World rules", ContentType: "text/markdown; charset=utf-8"},
	{ID: "knowledge.timeline-log", Kind: "knowledge", Path: "timeline.jsonl", Title: "Timeline fact log", ContentType: "application/x-ndjson"},
	{ID: "knowledge.timeline", Kind: "knowledge", Path: "timeline.json", Title: "Timeline projection", ContentType: "application/json"},
	{ID: "knowledge.timeline-markdown", Kind: "knowledge", Path: "timeline.md", Title: "Timeline", ContentType: "text/markdown; charset=utf-8"},
	{ID: "knowledge.foreshadow", Kind: "knowledge", Path: "foreshadow_ledger.json", Title: "Foreshadow ledger", ContentType: "application/json"},
	{ID: "knowledge.foreshadow-markdown", Kind: "knowledge", Path: "foreshadow_ledger.md", Title: "Foreshadow ledger", ContentType: "text/markdown; charset=utf-8"},
	{ID: "knowledge.relationships", Kind: "knowledge", Path: "relationship_state.json", Title: "Relationship state", ContentType: "application/json"},
	{ID: "knowledge.relationships-markdown", Kind: "knowledge", Path: "relationship_state.md", Title: "Relationship state", ContentType: "text/markdown; charset=utf-8"},
	{ID: "knowledge.state-changes-log", Kind: "knowledge", Path: "meta/state_changes.jsonl", Title: "State change fact log", ContentType: "application/x-ndjson"},
	{ID: "knowledge.state-changes", Kind: "knowledge", Path: "meta/state_changes.json", Title: "State change projection", ContentType: "application/json"},
}

var (
	chapterFinalNameRE   = regexp.MustCompile(`^([0-9]+)\.md$`)
	chapterPlanNameRE    = regexp.MustCompile(`^([0-9]+)\.plan\.json$`)
	chapterDraftNameRE   = regexp.MustCompile(`^([0-9]+)\.draft\.md$`)
	chapterRecordNameRE  = regexp.MustCompile(`^([0-9]+)\.json$`)
	chapterSummaryNameRE = regexp.MustCompile(`^([0-9]+)\.json$`)
	arcSummaryNameRE     = regexp.MustCompile(`^arc-v([0-9]+)a([0-9]+)\.json$`)
	volumeSummaryNameRE  = regexp.MustCompile(`^vol-v([0-9]+)\.json$`)
)

// DesktopDocumentCatalog exposes only supported existing project artifacts.
// It is a read-only view over the canonical Store root, not a generic filesystem
// browser. Dynamic discovery is limited to fixed Store-owned directories and
// canonical Store filenames.
func (h *Host) DesktopDocumentCatalog() ([]DesktopDocumentArtifact, error) {
	if h == nil || h.store == nil {
		return nil, fmt.Errorf("desktop project store unavailable: %w", apperrs.ErrStoreRead)
	}

	var docs []DesktopDocumentArtifact
	for _, spec := range desktopStaticDocumentSpecs {
		artifact, exists, err := desktopDocumentArtifact(h.store.Dir(), spec)
		if err != nil {
			return nil, desktopProjectReadError("document catalog", err)
		}
		if exists {
			docs = append(docs, artifact)
		}
	}

	for _, dir := range []string{"chapters", "drafts", "summaries", "meta/chapter_records"} {
		specs, err := desktopDynamicDocumentSpecs(h.store.Dir(), dir)
		if err != nil {
			return nil, desktopProjectReadError("document catalog", err)
		}
		for _, spec := range specs {
			artifact, exists, err := desktopDocumentArtifact(h.store.Dir(), spec)
			if err != nil {
				return nil, desktopProjectReadError("document catalog", err)
			}
			if exists {
				docs = append(docs, artifact)
			}
		}
	}

	sort.Slice(docs, func(i, j int) bool {
		if docs[i].Path == docs[j].Path {
			return docs[i].ID < docs[j].ID
		}
		return docs[i].Path < docs[j].Path
	})
	return docs, nil
}

// DesktopDocumentRead resolves an ID through the supported catalog, then reads
// only that catalogued UTF-8 artifact. Callers cannot supply a filesystem path.
func (h *Host) DesktopDocumentRead(id string) (DesktopDocumentArtifact, string, error) {
	docs, err := h.DesktopDocumentCatalog()
	if err != nil {
		return DesktopDocumentArtifact{}, "", err
	}
	for _, doc := range docs {
		if doc.ID != id {
			continue
		}
		resolved, err := resolveDesktopProjectPath(h.store.Dir(), doc.Path)
		if err != nil {
			return DesktopDocumentArtifact{}, "", desktopProjectReadError("document content", err)
		}
		data, err := os.ReadFile(resolved)
		if err != nil {
			return DesktopDocumentArtifact{}, "", desktopProjectReadError("document content", err)
		}
		if !utf8.Valid(data) {
			return DesktopDocumentArtifact{}, "", desktopProjectReadError("document content", fmt.Errorf("artifact is not valid UTF-8"))
		}
		doc.SizeBytes = int64(len(data))
		return doc, string(data), nil
	}
	return DesktopDocumentArtifact{}, "", ErrDesktopDocumentNotFound
}

func desktopDocumentArtifact(root string, spec desktopDocumentSpec) (DesktopDocumentArtifact, bool, error) {
	resolved, err := resolveDesktopProjectPath(root, spec.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return DesktopDocumentArtifact{}, false, nil
		}
		return DesktopDocumentArtifact{}, false, err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		if os.IsNotExist(err) {
			return DesktopDocumentArtifact{}, false, nil
		}
		return DesktopDocumentArtifact{}, false, err
	}
	if !info.Mode().IsRegular() {
		return DesktopDocumentArtifact{}, false, fmt.Errorf("supported artifact is not a regular file")
	}
	return DesktopDocumentArtifact{
		ID:          spec.ID,
		Kind:        spec.Kind,
		Path:        spec.Path,
		Title:       spec.Title,
		ContentType: spec.ContentType,
		SizeBytes:   info.Size(),
	}, true, nil
}

func desktopDynamicDocumentSpecs(root, dir string) ([]desktopDocumentSpec, error) {
	resolvedDir, err := resolveDesktopProjectPath(root, dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	info, err := os.Stat(resolvedDir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("supported artifact directory is not a directory")
	}
	entries, err := os.ReadDir(resolvedDir)
	if err != nil {
		return nil, err
	}

	var specs []desktopDocumentSpec
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if spec, ok := dynamicDesktopDocumentSpec(dir, entry.Name()); ok {
			specs = append(specs, spec)
		}
	}
	return specs, nil
}

func dynamicDesktopDocumentSpec(dir, name string) (desktopDocumentSpec, bool) {
	switch dir {
	case "chapters":
		if chapter, ok := canonicalNumberMatch(chapterFinalNameRE, name, func(n int) string { return fmt.Sprintf("%02d.md", n) }); ok {
			return desktopDocumentSpec{ID: fmt.Sprintf("chapter.final:%d", chapter), Kind: "chapter", Path: "chapters/" + name, Title: fmt.Sprintf("Chapter %d final", chapter), ContentType: "text/markdown; charset=utf-8"}, true
		}
	case "drafts":
		if chapter, ok := canonicalNumberMatch(chapterPlanNameRE, name, func(n int) string { return fmt.Sprintf("%02d.plan.json", n) }); ok {
			return desktopDocumentSpec{ID: fmt.Sprintf("chapter.plan:%d", chapter), Kind: "chapter", Path: "drafts/" + name, Title: fmt.Sprintf("Chapter %d plan", chapter), ContentType: "application/json"}, true
		}
		if chapter, ok := canonicalNumberMatch(chapterDraftNameRE, name, func(n int) string { return fmt.Sprintf("%02d.draft.md", n) }); ok {
			return desktopDocumentSpec{ID: fmt.Sprintf("chapter.draft:%d", chapter), Kind: "chapter", Path: "drafts/" + name, Title: fmt.Sprintf("Chapter %d draft", chapter), ContentType: "text/markdown; charset=utf-8"}, true
		}
	case "meta/chapter_records":
		if chapter, ok := canonicalNumberMatch(chapterRecordNameRE, name, func(n int) string { return fmt.Sprintf("%06d.json", n) }); ok {
			return desktopDocumentSpec{ID: fmt.Sprintf("chapter.record:%d", chapter), Kind: "chapter", Path: "meta/chapter_records/" + name, Title: fmt.Sprintf("Chapter %d accepted record", chapter), ContentType: "application/json"}, true
		}
	case "summaries":
		if chapter, ok := canonicalNumberMatch(chapterSummaryNameRE, name, func(n int) string { return fmt.Sprintf("%02d.json", n) }); ok {
			return desktopDocumentSpec{ID: fmt.Sprintf("summary.chapter:%d", chapter), Kind: "summary", Path: "summaries/" + name, Title: fmt.Sprintf("Chapter %d summary", chapter), ContentType: "application/json"}, true
		}
		if match := arcSummaryNameRE.FindStringSubmatch(name); len(match) == 3 {
			volume, errV := strconv.Atoi(match[1])
			arc, errA := strconv.Atoi(match[2])
			if errV == nil && errA == nil && volume > 0 && arc > 0 && name == fmt.Sprintf("arc-v%02da%02d.json", volume, arc) {
				return desktopDocumentSpec{ID: fmt.Sprintf("summary.arc:%d:%d", volume, arc), Kind: "summary", Path: "summaries/" + name, Title: fmt.Sprintf("Volume %d arc %d summary", volume, arc), ContentType: "application/json"}, true
			}
		}
		if volume, ok := canonicalNumberMatch(volumeSummaryNameRE, name, func(n int) string { return fmt.Sprintf("vol-v%02d.json", n) }); ok {
			return desktopDocumentSpec{ID: fmt.Sprintf("summary.volume:%d", volume), Kind: "summary", Path: "summaries/" + name, Title: fmt.Sprintf("Volume %d summary", volume), ContentType: "application/json"}, true
		}
	}
	return desktopDocumentSpec{}, false
}

func canonicalNumberMatch(re *regexp.Regexp, name string, canonical func(int) string) (int, bool) {
	match := re.FindStringSubmatch(name)
	if len(match) != 2 {
		return 0, false
	}
	n, err := strconv.Atoi(match[1])
	if err != nil || n <= 0 || canonical(n) != name {
		return 0, false
	}
	return n, true
}

func resolveDesktopProjectPath(root, rel string) (string, error) {
	if err := validateDesktopProjectRelativePath(rel); err != nil {
		return "", err
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	resolvedRoot, err = filepath.Abs(resolvedRoot)
	if err != nil {
		return "", err
	}
	candidate := filepath.Join(resolvedRoot, filepath.FromSlash(rel))
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", err
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return "", err
	}
	inside, err := filepath.Rel(resolvedRoot, resolved)
	if err != nil {
		return "", err
	}
	if inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) || filepath.IsAbs(inside) {
		return "", fmt.Errorf("supported artifact resolves outside project root")
	}
	return resolved, nil
}

func validateDesktopProjectRelativePath(rel string) error {
	if rel == "" || strings.ContainsRune(rel, '\x00') || strings.Contains(rel, "\\") || strings.Contains(rel, ":") || path.IsAbs(rel) {
		return fmt.Errorf("invalid project-relative artifact path")
	}
	clean := path.Clean(rel)
	if clean == "." || clean != rel || clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("invalid project-relative artifact path")
	}
	for _, segment := range strings.Split(clean, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return fmt.Errorf("invalid project-relative artifact path")
		}
	}
	return nil
}
