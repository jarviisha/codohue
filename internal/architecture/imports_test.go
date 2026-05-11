package architecture

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const modulePath = "github.com/jarviisha/codohue"

func TestInternalPackagesDoNotImportPeerDomains(t *testing.T) {
	root := ".."

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}

		importer, err := importPathForFile(root, path)
		if err != nil {
			return err
		}

		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, spec := range file.Imports {
			imported, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			if !strings.HasPrefix(imported, modulePath+"/internal/") {
				continue
			}
			if allowedInternalImport(importer, imported) {
				continue
			}
			t.Errorf("%s imports peer internal package %s", importer, imported)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func importPathForFile(root, path string) (string, error) {
	dir := filepath.Dir(path)
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return "", err
	}
	return modulePath + "/internal/" + filepath.ToSlash(rel), nil
}

func allowedInternalImport(importer, imported string) bool {
	if importer == imported {
		return true
	}

	rel := strings.TrimPrefix(imported, modulePath+"/internal/")
	switch {
	case rel == "config":
		return true
	case strings.HasPrefix(rel, "core/"):
		return true
	case strings.HasPrefix(rel, "infra/"):
		return true
	default:
		return false
	}
}

// TestCatalogEmbedderSeamIsolation directly asserts the contract the
// 004-catalog-embedder feature relies on: the catalog and embedder domains
// must communicate ONLY through internal/core/embedstrategy (the forward-compat
// seam). Cross-imports between the two would tie the data-plane and worker
// implementations together and break the constitution's import rule.
//
// This is a tighter, named assertion than the generic peer-domain test
// above — it survives even if the allowed-import rules are loosened later.
func TestCatalogEmbedderSeamIsolation(t *testing.T) {
	root := ".."

	type pair struct {
		importer, forbidden string
	}
	forbiddenPairs := []pair{
		{importer: modulePath + "/internal/catalog", forbidden: modulePath + "/internal/embedder"},
		{importer: modulePath + "/internal/embedder", forbidden: modulePath + "/internal/catalog"},
	}
	allowedPairs := []pair{
		{importer: modulePath + "/internal/catalog", forbidden: modulePath + "/internal/core/embedstrategy"},
		{importer: modulePath + "/internal/embedder", forbidden: modulePath + "/internal/core/embedstrategy"},
		{importer: modulePath + "/internal/admin", forbidden: modulePath + "/internal/core/embedstrategy"},
	}

	imports, err := collectImports(root)
	if err != nil {
		t.Fatal(err)
	}

	for _, p := range forbiddenPairs {
		for _, imp := range imports[p.importer] {
			if imp == p.forbidden {
				t.Errorf("%s must NOT import %s (use core/embedstrategy as the seam)",
					p.importer, p.forbidden)
			}
		}
	}

	// Sanity: the allowed pairs are only meaningful when the importer pkg
	// actually exists in the tree. The generic test above covers correctness;
	// here we just make sure listing them doesn't trip any test logic.
	_ = allowedPairs
}

// collectImports walks the tree rooted at root and returns a map from
// importer Go package path to the list of internal/* packages it imports.
// Files outside internal/ and non-Go files are skipped.
func collectImports(root string) (map[string][]string, error) {
	out := make(map[string][]string)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		importer, err := importPathForFile(root, path)
		if err != nil {
			return err
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, spec := range file.Imports {
			imported, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			if !strings.HasPrefix(imported, modulePath+"/internal/") {
				continue
			}
			out[importer] = append(out[importer], imported)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
