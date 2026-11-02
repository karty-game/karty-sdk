package worldchunk

import (
	"bytes"
	"testing"
)

func TestPhysicalCatalogKCC6CarriesNoRenderOrImageReservation(t *testing.T) {
	t.Parallel()
	document, catalog := binaryCatalogFixture(t)
	catalog.Schema = PhysicalSchema
	for index := range catalog.Chunks {
		chunk := &catalog.Chunks[index]
		chunk.CollisionBytes = chunk.MeshBytes
		chunk.MeshBytes = 0
		chunk.LightmapBytes = 0
		chunk.LightingBytes = 4096
	}
	encoded, err := EncodeCatalog(catalog, &document)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded[:4]) != "KCC6" {
		t.Fatal("physical-only catalog reused existing wire magic")
	}
	decoded, err := DecodeCatalog(encoded, &document)
	if err != nil {
		t.Fatal(err)
	}
	again, err := EncodeCatalog(decoded, &document)
	if err != nil || !bytes.Equal(encoded, again) {
		t.Fatal("noncanonical physical catalog", err)
	}
	for _, mutate := range []func(*Chunk){func(c *Chunk) { c.MeshBytes = 484 }, func(c *Chunk) { c.LightmapBytes = 512 * 512 * 12 }, func(c *Chunk) { c.CollisionBytes = 0 }, func(c *Chunk) { c.CollisionBytes++ }} {
		changed := catalog
		changed.Chunks = append([]Chunk(nil), catalog.Chunks...)
		mutate(&changed.Chunks[0])
		if ValidateCatalog(&changed, &document) == nil {
			t.Fatal("physical-only catalog accepted render/image data or invalid collision budget")
		}
	}
	decoded.Schema = FixedSchema
	if ValidateCatalog(&decoded, &document) == nil {
		t.Fatal("historical fixed schema accepted physical-only descriptors")
	}
}
