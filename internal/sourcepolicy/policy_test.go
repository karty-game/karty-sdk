package sourcepolicy

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/karty-game/karty-sdk/internal/cleanup"
)

// Revive covers the current build. This guard also sees platform-specific,
// opt-in source and handwritten templates that native lint does not load.
func TestHandwrittenProductionFilesStayWithin500Lines(t *testing.T) {
	t.Parallel()
	root, err := os.OpenRoot(moduleRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup.Close(root)
	err = fs.WalkDir(root.FS(), ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		name := entry.Name()
		if entry.IsDir() {
			if path != "." && skipDirectory(name) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(name, "_test.go") || (!strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, ".go.tmpl")) {
			return nil
		}
		return checkSourceFile(t, root, path)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	path, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
			return path
		}
		parent := filepath.Dir(path)
		if parent == path {
			t.Fatal("module root not found")
		}
		path = parent
	}
}

func skipDirectory(name string) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	switch name {
	case "dist", "tmp", "node_modules", "vendor", "testdata":
		return true
	default:
		return false
	}
}

func checkSourceFile(t *testing.T, root *os.Root, path string) error {
	t.Helper()
	raw, err := root.ReadFile(path)
	if err != nil {
		return err
	}
	lines := bytes.Count(raw, []byte{'\n'})
	if len(raw) > 0 && raw[len(raw)-1] != '\n' {
		lines++
	}
	if lines <= 500 {
		return nil
	}
	if strings.HasSuffix(path, ".go") {
		file, err := parser.ParseFile(token.NewFileSet(), path, raw, parser.ParseComments|parser.ImportsOnly)
		if err != nil {
			return err
		}
		if ast.IsGenerated(file) {
			return nil
		}
	}
	t.Errorf("%s has %d lines; split handwritten production files at 500 lines", path, lines)
	return nil
}
