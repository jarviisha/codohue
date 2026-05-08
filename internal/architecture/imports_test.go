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
	root := filepath.Join("..")

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
