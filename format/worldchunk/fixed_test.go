package worldchunk

import (
	"bytes"
	"testing"
)

func TestFixedCatalogKCC3PreservesLightingReservation(t *testing.T) {
	document, catalog := binaryCatalogFixture(t)
	catalog.Schema = FixedSchema
	for index := range catalog.Chunks {
		catalog.Chunks[index].LightmapBytes = 512 * 512 * 12
		catalog.Chunks[index].LightingBytes = 4096
	}
	encoded, err := EncodeCatalog(catalog, &document)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded[:4]) != "KCC3" {
		t.Fatal("fixed catalog lacks version gate")
	}
	decoded, err := DecodeCatalog(encoded, &document)
	if err != nil {
		t.Fatal(err)
	}
	again, err := EncodeCatalog(decoded, &document)
	if err != nil || !bytes.Equal(encoded, again) {
		t.Fatal("noncanonical fixed catalog", err)
	}
	decoded.Schema = Schema
	if _, err := EncodeCatalog(decoded, &document); err == nil {
		t.Fatal("legacy catalog accepted lighting scalar reservation")
	}
}
