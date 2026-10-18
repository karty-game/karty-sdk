package worldterrain

import (
	"encoding/binary"
	"errors"
	"math"
	"reflect"
	"testing"
)

func TestRejectCollapsedLattice(t *testing.T) {
	for _, origin := range []float32{999999, -999999} {
		for axis := range 2 {
			d := fixture()
			d.Origin[axis], d.CellSize = origin, .001
			if err := Validate(&d); !errors.Is(err, ErrBounds) {
				t.Fatalf("axis %d, origin %g: %v", axis, origin, err)
			}
			if _, err := Encode(d); !errors.Is(err, ErrBounds) {
				t.Fatalf("encode accepted collapsed lattice: %v", err)
			}
			if _, err := CompilePatches(&d); !errors.Is(err, ErrBounds) {
				t.Fatalf("compile accepted collapsed lattice: %v", err)
			}
		}
	}
	d := fixture()
	raw, err := Encode(d)
	if err != nil {
		t.Fatal(err)
	}
	// KWT1 header (16), semantic version (2), then origin (8) and cell size.
	binary.LittleEndian.PutUint32(raw[18:22], math.Float32bits(999999))
	binary.LittleEndian.PutUint32(raw[26:30], math.Float32bits(.001))
	if _, err := Decode(raw); !errors.Is(err, ErrBounds) {
		t.Fatalf("decode accepted collapsed lattice: %v", err)
	}
}

func TestSamplingUsesRenderedLattice(t *testing.T) {
	d := fixture()
	d.Origin, d.CellSize = [2]float32{100000, 100000}, .1
	mesh, err := CompilePatch(&d, 0)
	if err != nil {
		t.Fatal(err)
	}
	a, b := mesh.Vertices[0].Position, mesh.Vertices[1].Position
	x, y := (float64(a[0])+float64(b[0]))/2, float64(a[1])
	height, ok := HeightAt(&d, x, y)
	if !ok || height != (float64(a[2])+float64(b[2]))/2 {
		t.Fatalf("sampling disagrees with the rendered edge: %g, %t", height, ok)
	}
}

func TestCompilePatchesPreservesGeometryAndValidation(t *testing.T) {
	d := fixture()
	d.Patches = []Patch{
		{Columns: 1, Rows: 2, Weights: d.Patches[0].Weights},
		{X: 1, Columns: 1, Rows: 2, Weights: d.Patches[0].Weights},
	}
	meshes, err := CompilePatches(&d)
	if err != nil {
		t.Fatal(err)
	}
	for index, mesh := range meshes {
		single, err := CompilePatch(&d, index)
		if err != nil || !reflect.DeepEqual(mesh, single) {
			t.Fatalf("patch %d differs: %v", index, err)
		}
	}
	d.Materials[0].Normal.Pixels[3] = 0
	if _, err := CompilePatches(&d); err == nil {
		t.Fatal("batch compilation skipped complete material validation")
	}
}
