// Package world defines the bounded compiled sector format shared by level
// compilers and Karty hosts. It contains data contracts and validation only.
package world

import (
	"errors"

	"github.com/karty-game/karty-sdk/format/cartridge"
)

const (
	LegacyVersion      = uint16(1)
	ActorVersion       = uint16(2)
	Version            = uint16(3)
	LightingVersion    = uint32(1)
	EntryName          = "@world/main"
	Feature            = cartridge.FeatureWorldSectorsV1
	FeatureLighting    = cartridge.FeatureWorldLightingV1
	MaxEncodedSize     = 8 * 1024 * 1024
	MaxSectors         = 256
	MaxWalls           = 8192
	MaxWallsPerSector  = 256
	MaxContents        = 4096
	MaxLights          = 50
	MinLightRadius     = 0.001
	MaxIdentifierBytes = 128
	MaxKindBytes       = 128
	MaxTags            = 8
	MaxTagBytes        = 64
	MaxCoordinate      = 1_000_000.0
	MinEdgeLength      = 1e-6
	MinClearance       = 1e-6
	geometryEpsilon    = 1e-9
)

var (
	ErrSize            = errors.New("world payload size is invalid")
	ErrSyntax          = errors.New("world payload syntax is invalid")
	ErrVersion         = errors.New("world payload version is unsupported")
	ErrBounds          = errors.New("world payload exceeds a declared bound")
	ErrIdentity        = errors.New("world identity is invalid")
	ErrGeometry        = errors.New("world geometry is invalid")
	ErrPortal          = errors.New("world portal is invalid")
	ErrContent         = errors.New("world content is invalid")
	ErrMaterialMapping = errors.New("world material mapping is invalid")
	ErrLighting        = errors.New("world lighting is invalid")
	ErrStaticSolids    = errors.New("world static solids are invalid")
	ErrCanonical       = errors.New("world payload is not canonical")
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
	// Lighting is opt-in in compiled v3 and requires FeatureLighting in addition
	// to Feature. Nil preserves the legacy wire encoding and runtime behavior.
	Lighting        *Lighting        `json:"lighting,omitempty"`
	MaterialMapping *MaterialMapping `json:"material_mapping,omitempty"`
	StaticSolids    *StaticSolids    `json:"static_solids,omitempty"`
}

// Sector is a strictly convex CCW cell. SourceRoom and Instance retain the
// authoring identity after prefab expansion and decomposition.
type Sector struct {
	ID              string     `json:"id"`
	SourceRoom      string     `json:"source_room"`
	Instance        string     `json:"instance"`
	Walls           []Wall     `json:"walls"`
	Floor           Plane      `json:"floor"`
	Ceiling         Plane      `json:"ceiling"`
	FloorMaterial   uint32     `json:"floor_material"`
	CeilingMaterial uint32     `json:"ceiling_material"`
	FloorUV         *SurfaceUV `json:"floor_uv,omitempty"`
	CeilingUV       *SurfaceUV `json:"ceiling_uv,omitempty"`
}

// Wall is one directed sector boundary. Portal is the adjacent sector index,
// or -1 for an opaque wall. PortalWall is one plus the reciprocal wall index
// in compiled-world v3; zero is reserved for v1/v2 coincident portals and
// opaque walls. The paired directed edges define a rigid destination-to-source
// portal transform. SourceEdge is empty only for compiler-generated internal
// decomposition edges.
type Wall struct {
	Start      Vec2       `json:"start"`
	End        Vec2       `json:"end"`
	Portal     int32      `json:"portal"`
	PortalWall uint16     `json:"portal_wall,omitempty"`
	Material   uint32     `json:"material"`
	SourceEdge string     `json:"source_edge"`
	UV         *SurfaceUV `json:"uv,omitempty"`
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
	Actor    *Actor `json:"actor,omitempty"`
}

// Actor is the optional typed ECS payload introduced by compiled-world v2.
// Position remains on Content so actor and non-rendered gameplay contents share
// the same stable identity and sector assignment.
type Actor struct {
	Yaw    float64  `json:"yaw"`
	Pitch  float64  `json:"pitch"`
	Roll   float64  `json:"roll"`
	Scale  Vec3     `json:"scale"`
	Sprite *Sprite  `json:"sprite,omitempty"`
	Tags   []string `json:"tags,omitempty"`
}

type SpriteFacing string

const (
	SpriteCameraFacing SpriteFacing = "camera-facing"
	SpriteUpright      SpriteFacing = "upright"
	SpriteCross        SpriteFacing = "cross"
	SpriteFixed        SpriteFacing = "fixed"
)

type SpriteAlpha string

const (
	SpriteCutout SpriteAlpha = "cutout"
	SpriteBlend  SpriteAlpha = "blend"
)

type Sprite struct {
	AssetID uint32       `json:"asset_id"`
	Facing  SpriteFacing `json:"facing"`
	Alpha   SpriteAlpha  `json:"alpha"`
	Width   float64      `json:"width"`
	Height  float64      `json:"height"`
	OriginX float64      `json:"origin_x"`
	OriginY float64      `json:"origin_y"`
}
