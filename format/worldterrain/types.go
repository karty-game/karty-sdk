// Package worldterrain defines compiled, immutable heightfield ground resources.
package worldterrain

import (
	"errors"

	"github.com/karty-game/karty-sdk/format/cartridge"
)

const (
	Version            = uint16(1)
	EntryName          = "@world/terrain"
	MetadataKey        = "kartyWorldTerrain"
	Schema             = "karty.world-terrain@1"
	Feature            = cartridge.FeatureWorldTerrainV1
	MaxEncodedSize     = 8 * 1024 * 1024
	MaxDecodedBytes    = 32 * 1024 * 1024
	MaxDimension       = 257
	MaxVertices        = MaxDimension * MaxDimension
	MaxPatches         = 64
	MaxTriangles       = 65536
	MaxMaterials       = 16
	MaterialSize       = 256
	MaxImageDimension  = 1024
	MaxExteriorSectors = 256
)

var (
	ErrTerrain = errors.New("terrain resource is invalid")
	ErrBounds  = errors.New("terrain resource exceeds a bound")
	ErrVersion = errors.New("terrain version is unsupported")
)

// Image contains straight RGBA8 pixels in row-major order. Albedo/far colour
// are sRGB; normal images contain linear tangent normals (XYZ in RGB).
type Image struct {
	Width, Height uint16
	Pixels        []byte
}

type Material struct {
	Albedo         Image
	Normal         Image
	Scale          float32 // World metres per repeat.
	NormalStrength float32
	SideProjection bool // World-aligned cliff projections in addition to XY.
}

// Patch covers a cell rectangle. Columns/Rows are cell counts (not vertices).
// Palette indices address Materials; repeated entries permit fewer than four
// distinct materials. Weight channels are endpoint sampled and sum to 255.
type Patch struct {
	X, Y, Columns, Rows uint16
	Palette             [4]uint16
	Weights             Image
}

// Document contains one regular lattice. Columns/Rows count vertices. Holes
// has one byte per cell: 1 suppresses both render and collision triangles.
// ExteriorSectors lists ordinary sector floors replaced by this terrain.
type Document struct {
	Version         uint16
	Origin          [2]float32
	CellSize        float32
	Columns, Rows   uint16
	Heights         []float32
	Normals         [][3]float32
	ExteriorSectors []uint16
	Holes           []byte
	Materials       []Material
	Patches         []Patch
	FarColour       Image
}

type Vertex struct{ Position, Normal [3]float32 }
type Mesh struct {
	Vertices []Vertex
	Indices  []uint32
	Bounds   [6]float32
}
