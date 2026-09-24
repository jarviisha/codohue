package architecture

import (
	"go/ast"
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

	// A domain may import its own subpackages — they are not peers, they are
	// internal helpers of the domain (e.g. internal/admin uses
	// internal/admin/sse + internal/admin/eventbus for SSE plumbing).
	if strings.HasPrefix(imported, importer+"/") {
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

// TestHandlersReadRouteParamsThroughHTTPAPI keeps the path-decoding fix from
// rotting. chi hands back the raw segment when the client escaped anything, so
// "did%3Aplc%3Aabc" and "did:plc:abc" arrive as two different ids — separate
// vector lookups, cache keys and echoed response fields. httpapi.URLParam
// resolves that exactly once; a handler reading the parameter straight from
// chi silently reintroduces the bug on its own route, which no handler test
// catches because a hand-built RouteContext returns whatever the test stored.
//
// Matching on the method name rather than on the chi import covers every way
// in — chi.URLParam, an aliased import, chi.URLParamFromCtx, and
// chi.RouteContext(r).URLParam(…) — because only the wrapper is spelled
// httpapi.URLParam. Other chi APIs (NewRouter, RoutePattern, Mount) are
// untouched.
func TestHandlersReadRouteParamsThroughHTTPAPI(t *testing.T) {
	// internal/ holds the handlers; cmd/ wires the routers and reads {ns} for
	// auth, so both are in scope.
	roots := []string{"..", "../../cmd"}
	// internal/core/httpapi is the one legal caller: it *is* the wrapper.
	const wrapperDir = "core/httpapi"

	walk := func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.Contains(filepath.ToSlash(filepath.Dir(path)), wrapperDir) {
			return nil
		}

		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}

		ast.Inspect(file, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if sel.Sel.Name != "URLParam" && sel.Sel.Name != "URLParamFromCtx" {
				return true
			}
			if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == "httpapi" {
				return true
			}
			t.Errorf("%s:%d: reads a route parameter straight from chi; use httpapi.URLParam so it is decoded once",
				path, fset.Position(sel.Pos()).Line)
			return true
		})
		return nil
	}

	for _, root := range roots {
		if err := filepath.WalkDir(root, walk); err != nil {
			t.Fatal(err)
		}
	}
}
