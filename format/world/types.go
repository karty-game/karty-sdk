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
	EnvironmentVersion = uint32(1)
	FeatureEnvironment = cartridge.FeatureWorldEnvironmentV1
	MaxEncodedSize     = 8 * 1024 * 1024
	MaxSectors         = 256
	MaxWalls           = 8192
	MaxWallsPerSector  = 256
	MaxContents        = 4096
	// MaxLights bounds authored sources, including lights compiled into an
	// offline lightmap. It does not enlarge the runtime direct-light budget.
	MaxLights          = 4096
	MaxRuntimeLights   = 50
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
	ErrEnvironment     = errors.New("world environment is invalid")
	ErrStaticSolids    = errors.New("world static solids are invalid")
	ErrCanonical       = errors.New("world payload is not canonical")
)

type Vec2 struct {
	X float64
	Y float64
}

type Vec3 struct {
	X float64
	Y float64
	Z float64
}

// Plane describes elevation z=A*x+B*y+C.
type Plane struct {
	A float64
	B float64
	C float64
}

// Document is one completely compiled world. Sector order is stable because
// portal references use sector indexes.
type Document struct {
	Version  uint16
	Sectors  []Sector
	Contents []Content
	// Lighting is opt-in in compiled v3 and requires FeatureLighting in addition
	// to Feature. Nil preserves the legacy wire encoding and runtime behavior.
	Lighting        *Lighting
	MaterialMapping *MaterialMapping
	StaticSolids    *StaticSolids
	MaterialLayers  *MaterialLayers
	Animations      *Animations
	Emission        *Emission
	Environment     *Environment
}

// Sector is a strictly convex CCW cell. SourceRoom and Instance retain the
// authoring identity after prefab expansion and decomposition.
type Sector struct {
	ID               string
	SourceRoom       string
	Instance         string
	Walls            []Wall
	Floor            Plane
	Ceiling          Plane
	FloorMaterial    uint32
	CeilingMaterial  uint32
	FloorUV          *SurfaceUV
	CeilingUV        *SurfaceUV
	FloorSecondary   *SurfaceSecondary
	CeilingSecondary *SurfaceSecondary
	// Sky opens the ceiling visually and to environment lighting. The finite
	// ceiling plane still bounds navigation, portals and spatial queries.
	Sky bool
}

// Wall is one directed sector boundary. Portal is the adjacent sector index,
// or -1 for an opaque wall. PortalWall is one plus the reciprocal wall index
// in compiled-world v3; zero is reserved for v1/v2 coincident portals and
// opaque walls. The paired directed edges define a rigid destination-to-source
// portal transform. SourceEdge is empty only for compiler-generated internal
// decomposition edges.
type Wall struct {
	Start        Vec2
	End          Vec2
	Portal       int32
	PortalWall   uint16
	Material     uint32
	SourceEdge   string
	UV           *SurfaceUV
	Secondary    *SurfaceSecondary
	FrameRegions []WallFrameRegion
	// FrameCompiled preserves static-frame participation when a portal has no
	// exposed solid spans and thus no region records.
	FrameCompiled bool
	// Sky is an opaque boundary rendered as the environment. It is valid only
	// in a sky sector and cannot participate in a portal. Confinement remains.
	Sky bool
}

// Content retains both its compiled identity and authored/prefab provenance.
// The component payload is deliberately deferred to its own versioned format.
type Content struct {
	ID       string
	SourceID string
	Instance string
	Kind     string
	Sector   uint32
	Position Vec3
	Actor    *Actor
}

// Actor is the optional typed ECS payload introduced by compiled-world v2.
// Position remains on Content so actor and non-rendered gameplay contents share
// the same stable identity and sector assignment.
type Actor struct {
	Yaw       float64
	Pitch     float64
	Roll      float64
	Scale     Vec3
	Sprite    *Sprite
	Tags      []string
	Animation *AnimationBinding
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
	AssetID uint32
	Facing  SpriteFacing
	Alpha   SpriteAlpha
	Width   float64
	Height  float64
	OriginX float64
	OriginY float64
}
