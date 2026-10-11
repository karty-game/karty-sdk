package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestGeneratedTypedCodecsAreCurrent(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	temporary := t.TempDir()
	for _, pkg := range []string{"world", "worldlightmap", "worldmaterial", "worldparticles"} {
		directory := filepath.Join(temporary, "format", pkg)
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
		paths, err := filepath.Glob(filepath.Join(root, "format", pkg, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range paths {
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(directory, filepath.Base(path)), content, 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	generate(temporary)
	for _, pkg := range []string{"world", "worldlightmap", "worldmaterial", "worldparticles"} {
		relative := filepath.Join("format", pkg, "binary.generated.go")
		actual, err := os.ReadFile(filepath.Join(temporary, relative))
		if err != nil {
			t.Fatal(err)
		}
		expected, err := os.ReadFile(filepath.Join(root, relative))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(actual, expected) {
			t.Fatalf("regenerate %s using go generate ./format/world", relative)
		}
	}
}
