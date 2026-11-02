package worldgpuchunk

import (
	"bytes"
	"encoding/binary"
	"slices"
	"testing"
)

func catalogFixture() Catalog {
	return Catalog{
		GeometrySHA256:       [32]byte{1},
		MaterialLayoutSHA256: [32]byte{2},
		CellSize:             4,
		Cells: []Cell{
			{
				ID:            0,
				GridSpan:      1,
				Bounds:        [6]float64{0, 0, 0, 1, 1, 0},
				Sectors:       []uint32{0},
				PhysicalIDs:   []uint32{0},
				ContentSHA256: [32]byte{3},
				Size:          2048,
				GPUBytes:      3*VertexStride + 3*AddressStride + 3*IndexStride + RecordStride,
				RNMBytes:      192,
			},
		},
	}
}
func TestCatalogRoundTripAndTopologyReferences(t *testing.T) {
	t.Parallel()
	c := catalogFixture()
	encoded, err := EncodeCatalog(c)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeCatalog(encoded)
	if err != nil {
		t.Fatal(err)
	}
	again, err := EncodeCatalog(decoded)
	if err != nil || !bytes.Equal(encoded, again) {
		t.Fatal("noncanonical grid catalog", err)
	}
	if err := decoded.ValidateReferences([]uint32{0}, 1); err != nil {
		t.Fatal(err)
	}
	if decoded.ValidateReferences([]uint32{1}, 1) == nil {
		t.Fatal("wrong coordinate space admitted")
	}
	if decoded.ValidateReferences([]uint32{0}, 0) == nil {
		t.Fatal("foreign physical owner admitted")
	}
	decoded.Cells[0].PhysicalIDs = nil
	if err := decoded.ValidateReferences([]uint32{0}, 0); err != nil {
		t.Fatal("whole-world physics profile rejected", err)
	}
}
func TestCatalogSeparatesOverlappingNonEuclideanSpaces(t *testing.T) {
	t.Parallel()
	c := catalogFixture()
	second := c.Cells[0]
	second.ID = 1
	second.SpaceID = 1
	second.Sectors = []uint32{1}
	second.ContentSHA256 = [32]byte{4}
	c.Cells = append(c.Cells, second)
	if err := c.ValidateReferences([]uint32{0, 1}, 1); err != nil {
		t.Fatal("distinct overlapping portal spaces rejected", err)
	}
	c.Cells[1].SpaceID = 0
	c.Cells[1].Sectors = []uint32{0}
	if c.Validate() == nil {
		t.Fatal("duplicate grid cell in one coordinate space admitted")
	}
	c.Cells[1].SpaceID = 1
	c.Cells[1].Sectors = []uint32{0}
	if c.Validate() == nil {
		t.Fatal("same sector admitted in unrelated coordinate spaces")
	}
}
func TestCatalogRejectsBoundsOrderAndHostileCounts(t *testing.T) {
	t.Parallel()
	c := catalogFixture()
	c.Cells[0].Bounds[3] = 5
	if c.Validate() == nil {
		t.Fatal("cell exceeds its world-position partition")
	}
	c = catalogFixture()
	c.Cells[0].PhysicalIDs = []uint32{1, 1}
	if c.Validate() == nil {
		t.Fatal("duplicate physical IDs admitted")
	}
	encoded, err := EncodeCatalog(catalogFixture())
	if err != nil {
		t.Fatal(err)
	}
	binary.LittleEndian.PutUint32(encoded[104:], ^uint32(0))
	if _, err := DecodeCatalog(encoded); err == nil {
		t.Fatal("unbounded cell allocation admitted")
	}
}

func TestCatalogPreservesAuthoredCoarseAndFineExtents(t *testing.T) {
	t.Parallel()
	c := catalogFixture()
	c.Cells[0].GridSpan = 8
	c.Cells[0].Bounds[3] = 32
	if err := c.Validate(); err != nil {
		t.Fatal("coarse authored extent rejected", err)
	}
	encoded, err := EncodeCatalog(c)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeCatalog(encoded)
	if err != nil || decoded.Cells[0].GridSpan != 8 {
		t.Fatal("gridspan changed", err)
	}
	c.Cells[0].GridSpan = 3
	if c.Validate() == nil {
		t.Fatal("noncanonical grid extent admitted")
	}
	c.Cells[0].GridSpan = 0
	if c.Validate() == nil {
		t.Fatal("missing grid extent admitted")
	}
}

func TestCatalogPreservesSeparateAuthoredGeometryGroup(t *testing.T) {
	t.Parallel()
	c := catalogFixture()
	other := c.Cells[0]
	other.ID = 1
	other.Group = 1
	other.ContentSHA256 = [32]byte{7}
	c.Cells = append(c.Cells, other)
	if err := c.Validate(); err != nil {
		t.Fatal("authored geometry group rejected", err)
	}
	c.Cells[1].Group = 2
	if c.Validate() == nil {
		t.Fatal("unknown ownership group admitted")
	}
}

func TestCatalogBakedLightsRetainSelectedSourcePartition(t *testing.T) {
	t.Parallel()
	c := catalogFixture()
	c.BakedLights = []uint32{1, 3}
	if err := c.ValidateLights(4); err != nil {
		t.Fatal(err)
	}
	encoded, err := EncodeCatalog(c)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeCatalog(encoded)
	if err != nil || !slices.Equal(decoded.BakedLights, c.BakedLights) {
		t.Fatal("baked source union changed", err)
	}
	if decoded.ValidateLights(3) == nil {
		t.Fatal("foreign root light admitted")
	}
	c.BakedLights = []uint32{1, 1}
	if c.Validate() == nil {
		t.Fatal("duplicate baked light admitted")
	}
}

func TestCatalogWholeRoomsShareEnclosingGridAndRoundTrip(t *testing.T) {
	t.Parallel()
	catalog := catalogFixture()
	second := catalog.Cells[0]
	second.ID = 1
	second.Sectors = []uint32{1}
	second.Bounds = [6]float64{2, 0, 0, 3, 1, 1}
	second.ContentSHA256 = [32]byte{4}
	catalog.Cells = append(catalog.Cells, second)
	if err := catalog.ValidateReferences([]uint32{0, 0}, 1); err != nil {
		t.Fatal("whole rooms sharing an enclosing grid lost distinct owners", err)
	}
	encoded, err := EncodeCatalog(catalog)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeCatalog(encoded)
	if err != nil {
		t.Fatal(err)
	}
	again, err := EncodeCatalog(decoded)
	if err != nil || !bytes.Equal(encoded, again) || !slices.Equal(decoded.Cells[1].Sectors, second.Sectors) ||
		decoded.Cells[1].Bounds != second.Bounds {
		t.Fatal("whole room extent or owner changed during roundtrip", err)
	}
	catalog.Cells[0].Sectors, catalog.Cells[1].Sectors = []uint32{1}, []uint32{0}
	if catalog.Validate() == nil {
		t.Fatal("noncanonical same-grid owner order admitted")
	}
	catalog.Cells[0].Sectors, catalog.Cells[1].Sectors = []uint32{0}, []uint32{0, 1}
	if catalog.Validate() == nil {
		t.Fatal("duplicate owner/grid key admitted with a different secondary sector list")
	}
}

func TestCellGridOwnerComparisonRejectsMissingOwnerWithoutPanic(t *testing.T) {
	t.Parallel()
	cell := catalogFixture().Cells[0]
	missing := cell
	missing.Sectors = nil
	if cellBefore(missing, cell) || cellBefore(cell, missing) || cellBefore(missing, missing) {
		t.Fatal("incomplete owner identity establishes a same-grid canonical order")
	}
}
