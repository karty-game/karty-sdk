// Package worldgpuchunk defines portal-selected spatial cells containing final
// GPU buffers and independently resident, offline prepared RNM images.
package worldgpuchunk

import (
	"errors"

	"github.com/karty-game/karty-sdk/format/level"
)

const (
	Feature                = "world/gpu-chunks@2"
	Schema                 = "karty.world-gpu-chunks@2"
	MetadataKey            = "kartyWorldGPUChunks"
	CellMetadataKey        = "kartyWorldGPUCell"
	CatalogEntry           = "@world/gpu-chunks/catalog"
	PayloadEntry           = "@world/gpu-chunks/cell"
	RNMEntry               = "@world/gpu-chunks/rnm.ktx2"
	ABI                    = "karty.webgpu-cell@2"
	VertexStride           = 96
	AddressStride          = 8
	IndexStride            = 4
	RecordStride           = 368
	MaxCells               = 4096
	MaxRecords             = 8192
	MaxDraws               = 1024
	MaxVertices            = 262144
	MaxIndices             = 1572864
	MaxDimension           = 2048
	MaxEncodedSize         = level.MaxEntrySize
	OwnerSector     uint32 = 0
	OwnerSolid      uint32 = 1
)

var ErrChunk = errors.New("GPU-ready spatial chunk is invalid")

// Catalog groups whole room/geometry extents inside enclosing XY grid bounds.
// Portal topology, rather than grid adjacency, determines loading and visibility.
type Catalog struct {
	BakedLights                          []uint32
	GeometrySHA256, MaterialLayoutSHA256 [32]byte
	Origin                               [2]float64
	CellSize                             float64
	Cells                                []Cell
}

// Cell is a content-addressed independent render asset. Sectors are explicit
// topological ownership, including for solids; PhysicalIDs never own rendering.
// Grid/GridSpan enclose the extent without requiring room cuts. The first sector
// ID distinguishes canonical room owners sharing the same enclosing grid key.
type Cell struct {
	ID, SpaceID              uint32
	Group                    uint32
	GridSpan                 uint32
	Grid                     [2]int32
	Bounds                   [6]float64
	Sectors, PhysicalIDs     []uint32
	ContentSHA256            [32]byte
	Size, GPUBytes, RNMBytes uint64
}

// RecordRef binds a packed immutable GPU record to canonical authored owners
// and a material ID in the root's bound material layout.
type RecordRef struct{ MaterialID, OwnerKind, OwnerID, SurfaceKind uint32 }

// Draw refers only to cell-local address/index and same-owner record buffers.
// Sectors retain build-time ownership. Renderers submit complete prebuilt ranges
// without subdividing them by internal sector boundaries.
type Draw struct {
	FirstIndex, IndexCount, Record, AlphaMode uint32
	Bounds                                    [6]float64
	Sectors                                   []uint32
}

// GPUData contains final shader ABI bytes, already ordered and indexed offline.
// Decoders borrow buffer bytes; callers must retain immutable source storage.
type GPUData struct {
	Vertices, Addresses, Indices, Records []byte
	RecordRefs                            []RecordRef
	Draws                                 []Draw
}

// Payload is paired with one cell-local, three-layer lossless RNM KTX2 image.
// All buffers and lighting use authored coordinates; portal contexts are dynamic
// draw metadata and never modify this asset.
type Payload struct {
	CellID               uint32
	Bounds               [6]float64
	MaterialLayoutSHA256 [32]byte
	RGBMRange            float64
	GPUData
}
