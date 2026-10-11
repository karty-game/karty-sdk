package worldchunk

import (
	"bytes"
	"encoding/binary"
	"slices"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
)

func bakedCatalogFixture(t testing.TB) (world.Document, Catalog, []worldlightmap.Layout) {
	t.Helper()
	document := fixture(t)
	document.Lighting = &world.Lighting{Version: 1, Lights: []world.PointLight{
		{ID: "first", Position: world.Vec3{X: 2, Y: 2, Z: 2}, Radius: 10, Color: world.Vec3{X: 1}},
		{ID: "second", Position: world.Vec3{X: 6, Y: 2, Z: 2}, Radius: 10, Color: world.Vec3{Z: 1}},
	}}
	catalog := catalogForDocument(t, document)
	catalog.BakedLights = []int{0, 1}
	layouts := make([]worldlightmap.Layout, len(catalog.Chunks))
	for i := range catalog.Chunks {
		catalog.Chunks[i].LightmapBytes = 512 * 512 * 12
		chunk := catalog.Chunks[i]
		var err error
		layouts[i], err = worldlightmap.CompileReceivers(document,
			worldlightmap.Options{AllStatic: true, PageSize: 512}, chunk.Sectors, chunk.Solids)
		if err != nil {
			t.Fatal(err)
		}
	}
	return document, catalog, layouts
}

func TestCatalogBakedSourcesBoundColdPartitionAndExactChildMembership(t *testing.T) {
	document, catalog, layouts := bakedCatalogFixture(t)
	encoded, err := EncodeCatalog(catalog, &document)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeCatalog(encoded, &document)
	if err != nil || !slices.Equal(decoded.BakedLights, []int{0, 1}) || binary.LittleEndian.Uint32(encoded[20:24]) != 2 {
		t.Fatal("catalog source partition lost", err)
	}
	for i := range layouts {
		if err := ValidateBakedSelection(&decoded, i, &document, &layouts[i]); err != nil {
			t.Fatal("complete child binding rejected", err)
		}
		changed := layouts[i]
		recipe := *changed.RuntimeBake
		changed.RuntimeBake = &recipe
		recipe.LightIDs = []string{"second"}
		if err := ValidateBakedSelection(&decoded, i, &document, &changed); err == nil {
			t.Fatal("missing first source accepted")
		}
		recipe.LightIDs = []string{"second", "first"}
		if err := ValidateBakedSelection(&decoded, i, &document, &changed); err != nil {
			t.Fatal("recipe ordering confused with source membership", err)
		}
	}
	for name, mutate := range map[string]func(*Catalog){
		"duplicate": func(c *Catalog) { c.BakedLights = []int{0, 0} },
		"unsorted":  func(c *Catalog) { c.BakedLights = []int{1, 0} },
		"unknown":   func(c *Catalog) { c.BakedLights = []int{0, 2} },
		"negative":  func(c *Catalog) { c.BakedLights = []int{-1, 0} },
		"unbaked neighbor": func(c *Catalog) {
			c.Chunks = slices.Clone(c.Chunks)
			c.Chunks[1].LightmapBytes = 0
		},
	} {
		t.Run(name, func(t *testing.T) {
			changed := catalog
			mutate(&changed)
			if err := ValidateCatalog(&changed, &document); err == nil {
				t.Fatal("invalid parent source partition accepted")
			}
		})
	}
	document.Lighting.Lights[1].Motion = &world.LightMotion{Version: 1, PeriodSeconds: 1}
	if err := ValidateCatalog(&catalog, &document); err == nil {
		t.Fatal("moving source removed from runtime partition")
	}
}

func TestCatalogBakedSourcePreflightRejectsFinalMalformedIndexBeforeAllocation(t *testing.T) {
	document, catalog, _ := bakedCatalogFixture(t)
	encoded, err := EncodeCatalog(catalog, &document)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range [][]byte{
		changedWord(encoded, len(encoded)-4, world.MaxLights),
		changedWord(encoded, len(encoded)-4, 0),
		changedWord(encoded, len(encoded)-4, ^uint32(0)),
		changedWord(encoded, 20, world.MaxLights+1),
		encoded[:len(encoded)-4],
	} {
		var result Catalog
		if decodeCatalogBinary(raw, &result) == nil || result.Chunks != nil || result.BakedLights != nil || result.Candidates != nil {
			t.Fatal("malformed final source index allocated or published catalog arrays")
		}
	}
	old := bytes.Clone(encoded)
	copy(old[:4], "KCC1")
	binary.LittleEndian.PutUint16(old[4:6], 1)
	if _, err := DecodeCatalog(old, &document); err == nil {
		t.Fatal("old catalog silently reinterpreted as a cold source partition")
	}
}

func TestCatalogBakedMembershipFiltersTransformedPhysicalSpaces(t *testing.T) {
	document, _, _ := bakedCatalogFixture(t)
	for i := range document.Sectors[1].Walls {
		document.Sectors[1].Walls[i].Start.X += 100
		document.Sectors[1].Walls[i].End.X += 100
	}
	document.Sectors[1].Floor.C -= document.Sectors[1].Floor.A * 100
	document.Sectors[1].Ceiling.C -= document.Sectors[1].Ceiling.A * 100
	document.Contents[0].Position.X += 100
	document.Lighting.Lights[1].Position.X += 100
	catalog := catalogForDocument(t, document)
	catalog.BakedLights = []int{0, 1}
	for i := range catalog.Chunks {
		catalog.Chunks[i].LightmapBytes = 512 * 512 * 12
	}
	for i := range catalog.Chunks {
		chunk := catalog.Chunks[i]
		layout, err := worldlightmap.CompileReceivers(
			document,
			worldlightmap.Options{AllStatic: true, PageSize: 512},
			chunk.Sectors,
			chunk.Solids,
		)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(layout.RuntimeBake.LightIDs, []string{document.Lighting.Lights[i].ID}) {
			t.Fatal("source crossed transformed portal")
		}
		if err := ValidateBakedSelection(&catalog, i, &document, &layout); err != nil {
			t.Fatal("correct physical source partition rejected", err)
		}
	}
	// A runtime-lit second physical space does not invalidate a completed
	// first component whose sources are the only removed sources.
	catalog.BakedLights = []int{0}
	catalog.Chunks[1].LightmapBytes = 0
	if err := ValidateCatalog(&catalog, &document); err != nil {
		t.Fatal("unrelated runtime component invalidated source partition", err)
	}
}
