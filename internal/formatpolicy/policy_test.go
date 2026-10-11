package formatpolicy

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestRuntimeFormatsContainNoJSONOrReflection(t *testing.T) {
	root := filepath.Join("..", "..")
	for _, directory := range []string{"format", "codec", "internal/binwire", "bake/worldlightmap"} {
		err := filepath.WalkDir(filepath.Join(root, directory), func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ParseComments)
			if err != nil {
				return err
			}
			for _, reason := range formatViolations(file) {
				t.Errorf("runtime format %s: %s", path, reason)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	// The compiled fixture is a binary payload, with no JSON input fallback.
	if _, err := os.Stat(filepath.Join(root, "format/world/testdata/two-room.world.bin")); err != nil {
		t.Fatal(err)
	}
}

// Inspect static source, including escaped/quoted struct tags and custom JSON
// methods that could accidentally restore an implicit serialization contract.
func formatViolations(file *ast.File) []string {
	var violations []string
	for _, imported := range file.Imports {
		name, _ := strconv.Unquote(imported.Path.Value)
		if name == "encoding/json" || name == "reflect" {
			violations = append(violations, "imports "+name)
		}
	}
	for _, comment := range file.Comments {
		for _, line := range comment.List {
			if strings.HasPrefix(line.Text, "//go:embed") && strings.Contains(strings.ToLower(line.Text), ".json") {
				violations = append(violations, "embeds JSON")
			}
		}
	}
	ast.Inspect(file, func(node ast.Node) bool {
		switch node := node.(type) {
		case *ast.Field:
			if node.Tag != nil {
				tag, _ := strconv.Unquote(node.Tag.Value)
				for _, part := range strings.Fields(tag) {
					if strings.HasPrefix(part, "json:") {
						violations = append(violations, "declares JSON tag")
					}
				}
			}
		case *ast.FuncDecl:
			if node.Name.Name == "MarshalJSON" || node.Name.Name == "UnmarshalJSON" {
				violations = append(violations, "declares "+node.Name.Name)
			}
		}
		return true
	})
	return violations
}
func TestPolicyRejectsImplicitJSONContracts(t *testing.T) {
	for name, source := range map[string]string{
		"tag":              "package fixture; type T struct { Value int `json:\"value\"` }",
		"escaped tag":      `package fixture; type T struct { Value int "json:\"value\"" }`,
		"marshal method":   "package fixture; type T struct{}; func(T) MarshalJSON()([]byte,error){return nil,nil}",
		"unmarshal method": "package fixture; type T struct{}; func(*T) UnmarshalJSON([]byte)error{return nil}",
		"alias import":     "package fixture; import alternate \"encoding/json\"; var _=alternate.Marshal",
	} {
		t.Run(name, func(t *testing.T) {
			file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", source, parser.ParseComments)
			if err != nil {
				t.Fatal(err)
			}
			if len(formatViolations(file)) == 0 {
				t.Fatal("implicit JSON contract accepted")
			}
		})
	}
	file, err := parser.ParseFile(
		token.NewFileSet(),
		"fixture.go",
		"package fixture; type T struct { Value int `yaml:\"value\" toml:\"value\"` }",
		0,
	)
	if err != nil {
		t.Fatal(err)
	}
	if violations := formatViolations(file); len(violations) != 0 {
		t.Fatalf("authoring vocabulary rejected: %v", violations)
	}
}
