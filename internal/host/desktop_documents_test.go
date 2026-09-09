package host

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/voocel/ainovel-cli/internal/domain"
	apperrs "github.com/voocel/ainovel-cli/internal/errs"
	storepkg "github.com/voocel/ainovel-cli/internal/store"
)

func TestDesktopDocumentCatalogWhitelistsExistingArtifacts(t *testing.T) {
	root := t.TempDir()
	store := storepkg.NewStore(root)
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveProjectFormatVersion(storepkg.CurrentProjectFormatVersion); err != nil {
		t.Fatal(err)
	}
	if err := store.Book.Save(domain.BookMetadata{Title: "Book", Synopsis: "Synopsis"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Outline.SavePremise("Premise"); err != nil {
		t.Fatal(err)
	}
	if err := store.Outline.SaveOutline([]domain.OutlineEntry{{Chapter: 1, Title: "One", CoreEvent: "Event"}}); err != nil {
		t.Fatal(err)
	}
	if err := store.Drafts.SaveChapterPlan(domain.ChapterPlan{Chapter: 1, Title: "Plan"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Drafts.SaveDraft(1, "draft text"); err != nil {
		t.Fatal(err)
	}
	if err := store.Drafts.SaveFinalChapter(1, "final text"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ChapterRecords.Accept(1, domain.ChapterOriginGenerated, "accepted text", domain.ChapterFacts{Title: "One"}, domain.StyleDelta{}); err != nil {
		t.Fatal(err)
	}
	if err := store.Summaries.SaveSummary(domain.ChapterSummary{Chapter: 1, Title: "One", Summary: "Summary"}); err != nil {
		t.Fatal(err)
	}
	if err := store.World.SaveWorldRules([]domain.WorldRule{{Category: "magic", Rule: "Rule", Boundary: "Boundary"}}); err != nil {
		t.Fatal(err)
	}

	// These files are inside the project but are not supported catalog artifacts.
	if err := os.WriteFile(filepath.Join(root, "secret.txt"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "chapters", "1.md"), []byte("non-canonical duplicate"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "drafts", "notes.txt"), []byte("notes"), 0o600); err != nil {
		t.Fatal(err)
	}

	h := &Host{store: store}
	docs, err := h.DesktopDocumentCatalog()
	if err != nil {
		t.Fatal(err)
	}
	byID := make(map[string]DesktopDocumentArtifact, len(docs))
	for _, doc := range docs {
		if _, duplicate := byID[doc.ID]; duplicate {
			t.Fatalf("duplicate stable document id %q", doc.ID)
		}
		byID[doc.ID] = doc
		if filepath.IsAbs(doc.Path) || strings.Contains(doc.Path, "..") || strings.Contains(doc.Path, "\\") {
			t.Fatalf("unsafe desktop document path %q", doc.Path)
		}
	}

	want := map[string]string{
		"project.format":        "meta/format.json",
		"project.book":          "meta/book.json",
		"project.premise":       "premise.md",
		"outline.flat":          "outline.json",
		"chapter.plan:1":        "drafts/01.plan.json",
		"chapter.draft:1":       "drafts/01.draft.md",
		"chapter.final:1":       "chapters/01.md",
		"chapter.record:1":      "meta/chapter_records/000001.json",
		"summary.chapter:1":     "summaries/01.json",
		"knowledge.world-rules": "world_rules.json",
	}
	for id, path := range want {
		doc, ok := byID[id]
		if !ok {
			t.Fatalf("missing catalog document %q; got %#v", id, byID)
		}
		if doc.Path != path {
			t.Fatalf("document %q path = %q, want %q", id, doc.Path, path)
		}
		if doc.ContentType == "" || doc.SizeBytes <= 0 {
			t.Fatalf("document %q metadata = %+v", id, doc)
		}
	}
	for _, doc := range docs {
		if doc.Path == "secret.txt" || doc.Path == "chapters/1.md" || doc.Path == "drafts/notes.txt" {
			t.Fatalf("unsupported project file leaked into catalog: %+v", doc)
		}
	}
}

func TestDesktopDocumentReadUsesStableIDNotPath(t *testing.T) {
	root := t.TempDir()
	store := storepkg.NewStore(root)
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}
	if err := store.Drafts.SaveFinalChapter(7, "chapter seven"); err != nil {
		t.Fatal(err)
	}

	h := &Host{store: store}
	doc, content, err := h.DesktopDocumentRead("chapter.final:7")
	if err != nil {
		t.Fatal(err)
	}
	if doc.Path != "chapters/07.md" || doc.Kind != "chapter" || content != "chapter seven" {
		t.Fatalf("document read = %+v content=%q", doc, content)
	}
	if doc.SizeBytes != int64(len(content)) {
		t.Fatalf("size = %d, want %d", doc.SizeBytes, len(content))
	}

	if _, _, err := h.DesktopDocumentRead("../chapters/07.md"); !errors.Is(err, ErrDesktopDocumentNotFound) {
		t.Fatalf("path-like id error = %v, want ErrDesktopDocumentNotFound", err)
	}
}

func TestValidateDesktopProjectRelativePathRejectsTraversal(t *testing.T) {
	valid := []string{"meta/book.json", "chapters/01.md", "meta/chapter_records/000001.json"}
	for _, rel := range valid {
		if err := validateDesktopProjectRelativePath(rel); err != nil {
			t.Fatalf("valid path %q rejected: %v", rel, err)
		}
	}
	invalid := []string{"", ".", "../outside", "meta/../outside", "/absolute", `C:\\outside`, `meta\\book.json`, "meta//book.json"}
	for _, rel := range invalid {
		if err := validateDesktopProjectRelativePath(rel); err == nil {
			t.Fatalf("unsafe path %q accepted", rel)
		}
	}
}

func TestDesktopDocumentCatalogRejectsSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation is not reliably available on Windows hosted runners")
	}
	root := t.TempDir()
	store := storepkg.NewStore(root)
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "premise.md")); err != nil {
		t.Fatal(err)
	}

	h := &Host{store: store}
	_, err := h.DesktopDocumentCatalog()
	if err == nil || !errors.Is(err, apperrs.ErrStoreRead) {
		t.Fatalf("symlink escape error = %v, want store read error", err)
	}
}
