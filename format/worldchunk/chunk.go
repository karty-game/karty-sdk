// Package worldchunk defines build-generated render assets and conservative
// streaming visibility for one logical world. Geometry stays in authored space;
// a host may show one immutable asset through several transformed portal paths.
package worldchunk

import (
	"errors"

	"github.com/karty-game/karty-sdk/format/cartridge"
	"github.com/karty-game/karty-sdk/format/level"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
)

const (
	Schema           = "karty.world-chunks@1"
	MeshSchema       = "karty.world-chunk-mesh@1"
	Feature          = cartridge.FeatureWorldChunksV1
	CatalogEntryName = "@world/chunks/catalog"
	MeshEntryName    = "@world/chunks/mesh"
	MetadataKey      = "kartyWorldChunks"
	ChunkMetadataKey = "kartyWorldChunk"
	MaxEncodedSize   = level.MaxEntrySize
	MaxEncodedBytes  = level.MaxEnvelopeSize * 2
	MaxVertices      = 196608
	MaxTriangles     = MaxVertices / 3
)

var ErrChunk = errors.New("world chunk is invalid")

type Catalog struct {
	Schema         string
	GeometrySHA256 string
	Chunks         []Chunk
	SectorChunks   []int
	// Candidates is directed graph reachability, a worst-case visibility
	// superset. It is deduplicated by immutable chunk ID, not by portal path.
	Candidates [][]int
	// BakedLights is the sorted unique union of authored light indexes carried
	// by completed child prebakes. Hosts partition these sources before loading
	// children; they never consume runtime direct-light slots.
	BakedLights []int
}

type Chunk struct {
	ID            int
	Sectors       []int
	Solids        []int
	ContentSHA256 string
	Size          uint64
	MeshBytes     uint64
	LightmapBytes uint64
	Bounds        [6]float64
}

type Mesh struct {
	Schema         string
	GeometrySHA256 string
	SurfaceSHA256  string
	Sectors        []int
	Solids         []int
	Vertices       []Vertex
	Indices        []uint32
	Triangles      []Triangle
	Bounds         [6]float64
}

type Vertex struct {
	Position    [3]float32
	UV          [3][2]float32
	BandUV      [2]float32
	SecondaryUV [3][2]float32
	LightmapUV  [2]float32
	WaveUV      [2]float32
}

type Basis struct {
	Tangent   [3]float32
	Bitangent [3]float32
}

type Triangle struct {
	Sector          int
	Solid           int
	Surface         string
	SurfaceEdge     int
	Normal          [3]float32
	Tangent         [3]float32
	Bitangent       [3]float32
	Material        uint32
	MappingCount    uint8
	Weights         [3]float32
	ProjectionBases [3]Basis
	Band            Band
	Secondary       Secondary
	Sky             bool
	Outdoor         bool
	Chart           int
}

type Band struct {
	Material uint32
	Coverage uint8
	RepeatU  bool
	RepeatV  bool
	Basis    Basis
}

type Secondary struct {
	Material     uint32
	Strength     float32
	MappingCount uint8
	Weights      [3]float32
}

type Options struct {
	MaxMeshBytes    uint64
	MaxEncodedBytes uint64
	MaxSectors      int
	// Lightmap.PageSize is a maximum: 1024 (or zero) first tries 512, then
	// retries 1024 only when the receiver charts exceed page capacity.
	Lightmap   worldlightmap.Options
	NoLightmap bool
	// SectorGroups is an optional build-only authored-area preference. Nil keeps
	// budget-only grouping. Otherwise every global sector has an ID in [0,256).
	// Ordinary connectivity and hard budgets can still split an authored area.
	SectorGroups []int
}

type PlannedChunk struct {
	ID              int
	Sectors, Solids []int
	Mesh            Mesh
	Layout          worldlightmap.Layout
	Bounds          [6]float64
	MeshBytes       uint64
	LightmapBytes   uint64
}

type PlanResult struct {
	Chunks       []PlannedChunk
	SectorChunks []int
	Candidates   [][]int
}

// DecodedMeshBytes is the packed scalar array size, independent of Go padding:
// 21 float32 attributes per vertex, uint32 indices and 55 scalar words per
// triangle (including fixed identity/flags slots). It excludes the binary
// header and sector/solid ownership lists.
func DecodedMeshBytes(mesh Mesh) uint64 {
	return uint64(len(mesh.Vertices))*84 + uint64(len(mesh.Indices))*4 + uint64(len(mesh.Triangles))*220
}
