// Package world defines the bounded compiled sector format shared by level
// compilers and Karty hosts. It contains data contracts and validation only.
package world

import (
	"errors"

	"github.com/karty-game/karty-sdk/format/cartridge"
)

const (
	Version            = uint16(1)
	EntryName          = "@world/main"
	Feature            = cartridge.FeatureWorldSectorsV1
	MaxEncodedSize     = 8 * 1024 * 1024
	MaxSectors         = 256
	MaxWalls           = 8192
	MaxWallsPerSector  = 256
	MaxContents        = 4096
	MaxIdentifierBytes = 128
	MaxKindBytes       = 128
	MaxCoordinate      = 1_000_000.0
	MinEdgeLength      = 1e-6
	MinClearance       = 1e-6
	geometryEpsilon    = 1e-9
)

var (
	ErrSize      = errors.New("world payload size is invalid")
	ErrSyntax    = errors.New("world payload syntax is invalid")
	ErrVersion   = errors.New("world payload version is unsupported")
	ErrBounds    = errors.New("world payload exceeds a declared bound")
	ErrIdentity  = errors.New("world identity is invalid")
	ErrGeometry  = errors.New("world geometry is invalid")
	ErrPortal    = errors.New("world portal is invalid")
	ErrContent   = errors.New("world content is invalid")
	ErrCanonical = errors.New("world payload is not canonical")
)

type Vec2 struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type Vec3 struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

// Plane describes elevation z=A*x+B*y+C.
type Plane struct {
	A float64 `json:"a"`
	B float64 `json:"b"`
	C float64 `json:"c"`
}

// Document is one completely compiled world. Sector order is stable because
// portal references use sector indexes.
type Document struct {
	Version  uint16    `json:"version"`
	Sectors  []Sector  `json:"sectors"`
	Contents []Content `json:"contents"`
}

// Sector is a strictly convex CCW cell. SourceRoom and Instance retain the
// authoring identity after prefab expansion and decomposition.
type Sector struct {
	ID              string `json:"id"`
	SourceRoom      string `json:"source_room"`
	Instance        string `json:"instance"`
	Walls           []Wall `json:"walls"`
	Floor           Plane  `json:"floor"`
	Ceiling         Plane  `json:"ceiling"`
	FloorMaterial   uint32 `json:"floor_material"`
	CeilingMaterial uint32 `json:"ceiling_material"`
}

// Wall is one directed sector boundary. Portal is the adjacent sector index,
// or -1 for an opaque wall. SourceEdge is empty only for compiler-generated
// internal decomposition edges.
type Wall struct {
	Start      Vec2   `json:"start"`
	End        Vec2   `json:"end"`
	Portal     int32  `json:"portal"`
	Material   uint32 `json:"material"`
	SourceEdge string `json:"source_edge"`
}

// Content retains both its compiled identity and authored/prefab provenance.
// The component payload is deliberately deferred to its own versioned format.
type Content struct {
	ID       string `json:"id"`
	SourceID string `json:"source_id"`
	Instance string `json:"instance"`
	Kind     string `json:"kind"`
	Sector   uint32 `json:"sector"`
	Position Vec3   `json:"position"`
}
