//go:build js && wasm

package worldchunk

import (
	"syscall/js"
	"testing"
	"time"
)

func TestChunkDecodingAndValidationServiceJavaScriptTimers(t *testing.T) {
	document := fixture(t)
	plan, err := Plan(document, Options{NoLightmap: true, MaxSectors: 1})
	if err != nil {
		t.Fatal(err)
	}
	source := plan.Chunks[0].Mesh
	mesh := source
	mesh.Vertices, mesh.Indices, mesh.Triangles = nil, nil, nil
	// Refine existing authored triangles without changing complete coverage.
	// The resulting payload is large enough to require several browser slices.
	const factor = 8
	for index, triangle := range source.Triangles {
		vertices := source.Vertices[index*3 : index*3+3]
		for u := range factor {
			for v := range factor - u {
				emitLiquidTriangle(&mesh, triangle, vertices, factor, [3][2]int{{u, v}, {u + 1, v}, {u, v + 1}})
				if u+v+1 < factor {
					emitLiquidTriangle(&mesh, triangle, vertices, factor, [3][2]int{{u + 1, v}, {u + 1, v + 1}, {u, v + 1}})
				}
			}
		}
	}
	encoded, err := EncodeMesh(mesh, &document)
	if err != nil {
		t.Fatal(err)
	}
	phase := "decode"
	callbacks := make(map[string]int)
	timerCallback := js.FuncOf(func(js.Value, []js.Value) any {
		callbacks[phase]++
		return nil
	})
	timer := js.Global().Call("setInterval", timerCallback, 1)
	defer func() {
		js.Global().Call("clearInterval", timer)
		timerCallback.Release()
	}()
	started := time.Now()
	var decoded Mesh
	if err := decodeMeshBinary(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	decodedAt := time.Now()
	phase = "validation"
	if err := ValidateMesh(&decoded, &document); err != nil {
		t.Fatal(err)
	}
	validatedAt := time.Now()
	phase = "binding"
	if err := ValidateMeshBinding(&decoded, nil); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"decode", "validation", "binding"} {
		if callbacks[name] == 0 {
			t.Fatalf("%s monopolized JavaScript without servicing timers", name)
		}
	}
	t.Logf("%d bytes, %d vertices: decode=%s validation=%s JavaScript callbacks=%v", len(encoded), len(decoded.Vertices),
		decodedAt.Sub(started), validatedAt.Sub(decodedAt), callbacks)
}
