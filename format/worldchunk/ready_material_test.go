package worldchunk

import (
	"slices"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
)

func TestReadyLiquidRetainsWaveInputsTransparencyAndDisplacementBounds(t *testing.T) {
	for _, opacity := range []float64{.4, 0} {
		document := liquidFixture(t)
		document.Animations.Presets[0].Opacity = &opacity
		plan, err := Plan(document, Options{NoLightmap: true, MaxSectors: 1})
		if err != nil {
			t.Fatal(err)
		}
		checked := false
		for _, chunk := range plan.Chunks {
			mesh := chunk.Mesh
			encodedMesh, err := EncodeMesh(mesh, &document)
			if err != nil {
				t.Fatal(err)
			}
			ready, err := CompileReady(&mesh, encodedMesh, &document, nil, ReadyOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if err := ValidateReady(&ready, &mesh, encodedMesh, &document, nil, ReadyOptions{}); err != nil {
				t.Fatal(err)
			}
			for _, batch := range ready.Batches {
				for index, source := range batch.Sources {
					triangle := mesh.Triangles[source]
					if triangle.Material != 1 {
						continue
					}
					checked = true
					if batch.StaticMaterial || !batch.Transparent || batch.Invisible != (opacity == 0) || batch.Winding != 0 {
						t.Fatal("liquid lost generic animation or transparency classification")
					}
					for corner := range 3 {
						vertex := mesh.Vertices[int(source)*3+corner]
						at := (index*3 + corner) * 2
						if !slices.Equal(batch.WaveUV[at:at+2], vertex.WaveUV[:]) {
							t.Fatal("offline preparation changed shader wave input")
						}
					}
				}
			}
		}
		if !checked {
			t.Fatal("fixture did not exercise animated material")
		}
	}
}

func TestReadyMixedProjectionRemainsGenericAndMaskedOptionsStayBound(t *testing.T) {
	document := liquidFixture(t)
	document.Animations = nil
	for index := range document.Sectors {
		document.Sectors[index].FloorUV = &world.SurfaceUV{
			Projections: []world.UVProjection{
				{U: world.UVPlane{X: .25}, V: world.UVPlane{Y: .25}},
				{U: world.UVPlane{X: .125, Offset: .13}, V: world.UVPlane{Y: .125, Offset: .17}},
				{U: world.UVPlane{X: .5}, V: world.UVPlane{Y: .5}},
			}, Weights: []float64{.4, .6, 0},
		}
	}
	plan, err := Plan(document, Options{NoLightmap: true})
	if err != nil {
		t.Fatal(err)
	}
	mesh := plan.Chunks[0].Mesh
	encodedMesh, err := EncodeMesh(mesh, &document)
	if err != nil {
		t.Fatal(err)
	}
	options := ReadyOptions{Masked: make([]bool, len(world.MaterialIDs(&document)))}
	for index := range options.Masked {
		options.Masked[index] = true
	}
	ready, err := CompileReady(&mesh, encodedMesh, &document, nil, options)
	if err != nil {
		t.Fatal(err)
	}
	checkedGeneric, checkedMasked := false, false
	for _, batch := range ready.Batches {
		checkedMasked = checkedMasked || batch.Masked
		for index, source := range batch.Sources {
			triangle := mesh.Triangles[source]
			if triangle.MappingCount <= 1 {
				continue
			}
			checkedGeneric = true
			if batch.StaticMaterial {
				t.Fatal("mixed projections were incorrectly simplified")
			}
			for corner := range 3 {
				at := (index*3 + corner) * ReadyStride
				vertex := mesh.Vertices[int(source)*3+corner]
				for projection := range int(triangle.MappingCount) {
					if !slices.Equal(batch.Vertices[at+6+projection*2:at+8+projection*2], vertex.UV[projection][:]) {
						t.Fatal("generic projection UV changed")
					}
				}
				if !slices.Equal(batch.Vertices[at+18:at+21], triangle.Weights[:]) {
					t.Fatal("generic projection weights changed")
				}
			}
		}
	}
	if !checkedGeneric || !checkedMasked {
		t.Fatal("fixture omitted generic projections or static masked batches")
	}
	if err := ValidateReady(&ready, &mesh, encodedMesh, &document, nil, options); err != nil {
		t.Fatal(err)
	}
	if err := ValidateReady(&ready, &mesh, encodedMesh, &document, nil, ReadyOptions{}); err == nil {
		t.Fatal("changed material alpha classification retained ready publication")
	}
	if _, err := CompileReady(&mesh, encodedMesh, &document, nil, ReadyOptions{Masked: []bool{true}}); err == nil {
		t.Fatal("partial alpha policy table accepted")
	}
}
