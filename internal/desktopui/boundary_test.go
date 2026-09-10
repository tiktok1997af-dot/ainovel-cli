package desktopui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"
)

func productionGoFiles(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		files = append(files, name)
	}
	return files
}

func parseProductionFile(t *testing.T, name string, mode parser.Mode) *ast.File {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), name, nil, mode)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return file
}

func TestProductionFilesDoNotImportForbiddenCorePackages(t *testing.T) {
	forbidden := []string{
		"github.com/voocel/ainovel-cli/internal/host",
		"github.com/voocel/ainovel-cli/internal/store",
		"github.com/voocel/ainovel-cli/internal/webai",
		"github.com/voocel/ainovel-cli/internal/engine",
		"github.com/voocel/ainovel-cli/internal/workers",
		"github.com/voocel/ainovel-cli/internal/arbiter",
		"github.com/voocel/ainovel-cli/internal/tools",
	}
	for _, name := range productionGoFiles(t) {
		file := parseProductionFile(t, name, parser.ImportsOnly)
		for _, spec := range file.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatalf("unquote import in %s: %v", name, err)
			}
			for _, prefix := range forbidden {
				if path == prefix || strings.HasPrefix(path, prefix+"/") {
					t.Errorf("%s imports forbidden core package %q", name, path)
				}
			}
		}
	}
}

func TestProductionBoundaryOnlyUsesAppRuntimeAsAINovelInternalDependency(t *testing.T) {
	const internalPrefix = "github.com/voocel/ainovel-cli/internal/"
	const allowed = internalPrefix + "appruntime"
	for _, name := range productionGoFiles(t) {
		file := parseProductionFile(t, name, parser.ImportsOnly)
		for _, spec := range file.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				continue
			}
			if strings.HasPrefix(path, internalPrefix) && path != allowed {
				t.Errorf("%s crosses desktop boundary via %q", name, path)
			}
		}
	}
}

func TestProductionBoundaryRejectsDirectFilesystemAndNetworkTransport(t *testing.T) {
	forbidden := map[string]bool{
		"io/fs":         true,
		"net":           true,
		"net/http":      true,
		"os":            true,
		"os/exec":       true,
		"path/filepath": true,
	}
	for _, name := range productionGoFiles(t) {
		file := parseProductionFile(t, name, parser.ImportsOnly)
		for _, spec := range file.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				continue
			}
			if forbidden[path] {
				t.Errorf("%s imports direct I/O or transport package %q", name, path)
			}
		}
	}
}

func TestCommandRequestsDoNotClaimG05OwnershipFields(t *testing.T) {
	for _, name := range productionGoFiles(t) {
		file := parseProductionFile(t, name, 0)
		ast.Inspect(file, func(node ast.Node) bool {
			lit, ok := node.(*ast.CompositeLit)
			if !ok {
				return true
			}
			sel, ok := lit.Type.(*ast.SelectorExpr)
			if !ok || sel.Sel == nil || sel.Sel.Name != "CommandRequest" {
				return true
			}
			for _, element := range lit.Elts {
				kv, ok := element.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				key, ok := kv.Key.(*ast.Ident)
				if !ok {
					continue
				}
				switch key.Name {
				case "RunID", "TaskID", "Resource":
					t.Errorf("%s claims closed G05 ownership field %s in CommandRequest", name, key.Name)
				}
			}
			return true
		})
	}
}
