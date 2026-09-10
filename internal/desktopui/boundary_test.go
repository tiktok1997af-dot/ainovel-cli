package desktopui

import (
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestProductionFilesDoNotImportForbiddenCorePackages(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	forbidden := []string{
		"github.com/voocel/ainovel-cli/internal/host",
		"github.com/voocel/ainovel-cli/internal/store",
		"github.com/voocel/ainovel-cli/internal/webai",
		"github.com/voocel/ainovel-cli/internal/engine",
		"github.com/voocel/ainovel-cli/internal/workers",
		"github.com/voocel/ainovel-cli/internal/arbiter",
		"github.com/voocel/ainovel-cli/internal/tools",
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
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
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	const internalPrefix = "github.com/voocel/ainovel-cli/internal/"
	const allowed = internalPrefix + "appruntime"
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
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
