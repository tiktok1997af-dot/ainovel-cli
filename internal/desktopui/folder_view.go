package desktopui

import (
	"sort"
	"strings"

	"github.com/voocel/ainovel-cli/internal/appruntime"
)

type FolderNodeKind string

const (
	FolderNodeDirectory FolderNodeKind = "folder"
	FolderNodeDocument  FolderNodeKind = "document"
)

// FolderViewNode is a presentation-only node projected from the sanitized
// AppRuntime document catalog. Path is a logical catalog path; it is never
// resolved or opened by desktopui.
type FolderViewNode struct {
	Key          string
	Kind         FolderNodeKind
	Name         string
	Path         string
	DocumentID   string
	DocumentKind string
	Title        string
	ContentType  string
	ReadOnly     bool
	SizeBytes    int64
	Selected     bool
	Children     []FolderViewNode
}

// FolderViewState contains only the bounded page already returned by
// documents.list. Total preserves the catalog result total so the UI can show
// that the projection is paged without obtaining filesystem authority.
type FolderViewState struct {
	Roots              []FolderViewNode
	Offset             int
	Limit              int
	Total              int
	ProjectedDocuments int
	OmittedUnsafe      int
	SelectedDocumentID string
}

func NewFolderViewState() FolderViewState {
	return FolderViewState{Roots: []FolderViewNode{}}
}

func BuildFolderView(dto appruntime.DocumentsListResultDTO, selectedDocumentID string) FolderViewState {
	state := NewFolderViewState()
	state.Offset = dto.Offset
	state.Limit = dto.Limit
	state.Total = dto.Total
	state.SelectedDocumentID = strings.TrimSpace(selectedDocumentID)

	root := newFolderBuilder("", "")
	for _, item := range dto.Items {
		if strings.TrimSpace(item.ID) == "" || !item.ReadOnly {
			state.OmittedUnsafe++
			continue
		}
		segments, ok := safeLogicalDocumentSegments(item.Path)
		if !ok {
			state.OmittedUnsafe++
			continue
		}
		root.addDocument(segments, item, state.SelectedDocumentID)
		state.ProjectedDocuments++
	}
	state.Roots = root.projectChildren()
	return state
}

func (s *FolderViewState) SelectDocument(id string) {
	if s == nil {
		return
	}
	s.SelectedDocumentID = strings.TrimSpace(id)
	selectFolderNodes(s.Roots, s.SelectedDocumentID)
}

func selectFolderNodes(nodes []FolderViewNode, id string) {
	for i := range nodes {
		nodes[i].Selected = nodes[i].Kind == FolderNodeDocument && nodes[i].DocumentID == id && id != ""
		selectFolderNodes(nodes[i].Children, id)
	}
}

type folderBuilder struct {
	name    string
	path    string
	folders map[string]*folderBuilder
	docs    []FolderViewNode
}

func newFolderBuilder(name, path string) *folderBuilder {
	return &folderBuilder{name: name, path: path, folders: make(map[string]*folderBuilder)}
}

func (b *folderBuilder) addDocument(segments []string, item appruntime.DocumentSummaryDTO, selectedID string) {
	current := b
	for i, segment := range segments[:len(segments)-1] {
		path := strings.Join(segments[:i+1], "/")
		child := current.folders[segment]
		if child == nil {
			child = newFolderBuilder(segment, path)
			current.folders[segment] = child
		}
		current = child
	}
	logicalPath := strings.Join(segments, "/")
	name := segments[len(segments)-1]
	current.docs = append(current.docs, FolderViewNode{
		Key:          "document:" + item.ID,
		Kind:         FolderNodeDocument,
		Name:         name,
		Path:         logicalPath,
		DocumentID:   item.ID,
		DocumentKind: item.Kind,
		Title:        item.Title,
		ContentType:  item.ContentType,
		ReadOnly:     item.ReadOnly,
		SizeBytes:    item.SizeBytes,
		Selected:     item.ID != "" && item.ID == selectedID,
		Children:     []FolderViewNode{},
	})
}

func (b *folderBuilder) projectChildren() []FolderViewNode {
	folders := make([]*folderBuilder, 0, len(b.folders))
	for _, child := range b.folders {
		folders = append(folders, child)
	}
	sort.Slice(folders, func(i, j int) bool {
		return lessFolderLabel(folders[i].name, folders[i].path, folders[j].name, folders[j].path)
	})
	sort.Slice(b.docs, func(i, j int) bool {
		return lessFolderLabel(b.docs[i].Name, b.docs[i].Key, b.docs[j].Name, b.docs[j].Key)
	})

	nodes := make([]FolderViewNode, 0, len(folders)+len(b.docs))
	for _, child := range folders {
		nodes = append(nodes, FolderViewNode{
			Key:      "folder:" + child.path,
			Kind:     FolderNodeDirectory,
			Name:     child.name,
			Path:     child.path,
			Children: child.projectChildren(),
		})
	}
	nodes = append(nodes, b.docs...)
	return nodes
}

func lessFolderLabel(nameA, keyA, nameB, keyB string) bool {
	foldA := strings.ToLower(nameA)
	foldB := strings.ToLower(nameB)
	if foldA == foldB {
		return keyA < keyB
	}
	return foldA < foldB
}

func safeLogicalDocumentSegments(raw string) ([]string, bool) {
	path := strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	if path == "" || strings.HasPrefix(path, "/") || isWindowsAbsoluteLogicalPath(path) {
		return nil, false
	}
	parts := strings.Split(path, "/")
	segments := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == "." || part == ".." {
			return nil, false
		}
		segments = append(segments, part)
	}
	if len(segments) == 0 {
		return nil, false
	}
	return segments, true
}

func isWindowsAbsoluteLogicalPath(path string) bool {
	return len(path) >= 2 && path[1] == ':' && ((path[0] >= 'a' && path[0] <= 'z') || (path[0] >= 'A' && path[0] <= 'Z'))
}
