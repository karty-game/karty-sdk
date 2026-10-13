// Package worldsource defines the high-level, separately versioned room and
// prefab schema consumed by public world compilers. It intentionally has no
// YAML implementation dependency; tags describe the stable field vocabulary.
package worldsource

import (
	"errors"

	sdkworld "github.com/karty-game/karty-sdk/format/world"
)

const (
	LegacyVersion         = uint16(1)
	ActorVersion          = uint16(2)
	PortalVersion         = uint16(3)
	LightingVersion       = uint16(4)
	MappingVersion        = uint16(5)
	SolidsVersion         = uint16(6)
	MaterialsVersion      = uint16(7)
	AnimationsVersion     = uint16(8)
	EnvironmentVersion    = uint16(9)
	ParticlesVersion      = uint16(10)
	SolidRenderersVersion = uint16(11)
	SkyVersion            = uint16(12)
	Version               = SkyVersion
	MaxRooms              = 1024
	MaxPrefabs            = 256
	MaxInstances          = 4096
	MaxConnections        = 8192
	MaxPorts              = 4096
	MaxEdges              = 8192
	MaxEdgesPerRoom       = 256
	MaxContents           = 4096
	MaxLights             = sdkworld.MaxLights
	MinLightRadius        = sdkworld.MinLightRadius
	MaxMaterialOverrides  = 256
	MaxIdentifierBytes    = 128
	MaxTags               = 8
	MaxTagBytes           = 64
	MaxCoordinate         = 1_000_000.0
	MinEdgeLength         = 1e-6
	MinClearance          = 1e-6
	geometryEpsilon       = 1e-9
)

const EnvironmentBackdropVersion = uint16(11)

var (
	ErrVersion     = errors.New("world source version is unsupported")
	ErrBounds      = errors.New("world source exceeds a declared bound")
	ErrIdentity    = errors.New("world source identity is invalid")
	ErrGeometry    = errors.New("world source geometry is invalid")
	ErrReference   = errors.New("world source reference is invalid")
	ErrConnection  = errors.New("world source connection is invalid")
	ErrContent     = errors.New("world source content is invalid")
	ErrUVMapping   = errors.New("world source UV mapping is invalid")
	ErrLighting    = errors.New("world source lighting is invalid")
	ErrEnvironment = errors.New("world source environment is invalid")
	ErrPrefabCycle = errors.New("world source prefab cycle is invalid")
)

type Vec2 struct {
	X float64 `yaml:"x"`
	Y float64 `yaml:"y"`
}

type Vec3 struct {
	X float64 `yaml:"x"`
	Y float64 `yaml:"y"`
	Z float64 `yaml:"z"`
}

// Plane describes elevation z=A*x+B*y+C in source coordinates.
type Plane struct {
	A float64 `yaml:"a"`
	B float64 `yaml:"b"`
	C float64 `yaml:"c"`
}

type Document struct {
	Version     uint16       `yaml:"version"`
	Rooms       []Room       `yaml:"rooms"`
	Prefabs     []Prefab     `yaml:"prefabs"`
	Instances   []Instance   `yaml:"instances"`
	Connections []Connection `yaml:"connections"`
	// Lighting is available in source v4 and later, in global world coordinates.
	// It is not part of any room, prefab or instance transform.
	Lighting    *Lighting    `yaml:"lighting,omitempty"`
	UV          *UVSettings  `yaml:"uv,omitempty"`
	Solids      []Solid      `yaml:"solids,omitempty"`
	Contents    []Content    `yaml:"contents,omitempty"`
	Animations  *Animations  `yaml:"animations,omitempty"`
	Environment *Environment `yaml:"environment,omitempty"`
	Particles   *Particles   `yaml:"particles,omitempty"`
}

// Room may be concave. The compiler decomposes it into convex sectors after
// prefab expansion and connection resolution.
type Room struct {
	ID               string             `yaml:"id"`
	Boundary         []Edge             `yaml:"boundary"`
	Floor            Plane              `yaml:"floor"`
	Ceiling          Plane              `yaml:"ceiling"`
	FloorMaterial    string             `yaml:"floor_material"`
	CeilingMaterial  string             `yaml:"ceiling_material"`
	Contents         []Content          `yaml:"contents"`
	FloorUV          *UVSettings        `yaml:"floor_uv,omitempty"`
	CeilingUV        *UVSettings        `yaml:"ceiling_uv,omitempty"`
	WallUV           *UVSettings        `yaml:"wall_uv,omitempty"`
	WallBands        *BandSettings      `yaml:"wall_bands,omitempty"`
	FloorSecondary   *SecondarySettings `yaml:"floor_secondary,omitempty"`
	CeilingSecondary *SecondarySettings `yaml:"ceiling_secondary,omitempty"`
	WallSecondary    *SecondarySettings `yaml:"wall_secondary,omitempty"`
	Sky              bool               `yaml:"sky,omitempty"`
	ParticleEmitters []ParticleEmitter  `yaml:"particle_emitters,omitempty"`
}

// Edge is one named, directed CCW boundary edge.
type Edge struct {
	ID        string             `yaml:"id"`
	Start     Vec2               `yaml:"start"`
	End       Vec2               `yaml:"end"`
	Material  string             `yaml:"material"`
	UV        *UVSettings        `yaml:"uv,omitempty"`
	Bands     *BandSettings      `yaml:"bands,omitempty"`
	Secondary *SecondarySettings `yaml:"secondary,omitempty"`
	// Sky opens a finite boundary visually, while retaining player confinement.
	// It requires a sky room and cannot be a connection or port endpoint.
	Sky bool `yaml:"sky,omitempty"`
}

type Content struct {
	ID       string `yaml:"id"`
	Kind     string `yaml:"kind"`
	Position Vec3   `yaml:"position"`
	Actor    *Actor `yaml:"actor,omitempty"`
}

type Actor struct {
	YawDegrees    float64           `yaml:"yaw_degrees"`
	PitchDegrees  float64           `yaml:"pitch_degrees"`
	RollDegrees   float64           `yaml:"roll_degrees"`
	Scale         Vec3              `yaml:"scale"`
	SolidRenderer string            `yaml:"solid_renderer,omitempty"`
	Sprite        *Sprite           `yaml:"sprite,omitempty"`
	Tags          []string          `yaml:"tags,omitempty"`
	Animation     *AnimationBinding `yaml:"animation,omitempty"`
}

type Sprite struct {
	Texture string  `yaml:"texture"`
	Facing  string  `yaml:"facing"`
	Alpha   string  `yaml:"alpha"`
	Width   float64 `yaml:"width"`
	Height  float64 `yaml:"height"`
	OriginX float64 `yaml:"origin_x"`
	OriginY float64 `yaml:"origin_y"`
}

type Prefab struct {
	Renderable       bool              `yaml:"renderable,omitempty"`
	ID               string            `yaml:"id"`
	Rooms            []Room            `yaml:"rooms"`
	Instances        []Instance        `yaml:"instances"`
	Connections      []Connection      `yaml:"connections"`
	Ports            []Port            `yaml:"ports"`
	Solids           []Solid           `yaml:"solids,omitempty"`
	Contents         []Content         `yaml:"contents,omitempty"`
	ParticleEmitters []ParticleEmitter `yaml:"particle_emitters,omitempty"`
}

type Instance struct {
	ID        string             `yaml:"id"`
	Prefab    string             `yaml:"prefab"`
	Transform Transform          `yaml:"transform"`
	Materials []MaterialOverride `yaml:"materials"`
	// Tags replaces the actor tags of every content in this prefab instance.
	// An empty list preserves the prefab-authored tags.
	Tags []string `yaml:"tags,omitempty"`
}

// Transform applies uniform scale, Z-axis yaw in degrees and translation.
// Scale zero means the schema default of one.
type Transform struct {
	Translation Vec3    `yaml:"translation"`
	YawDegrees  float64 `yaml:"yaw_degrees"`
	Scale       float64 `yaml:"scale"`
}

type MaterialOverride struct {
	From string `yaml:"from"`
	To   string `yaml:"to"`
}

// Endpoint selects either a room edge in the current scope or a named port on
// an instance in that scope. Exactly one pair must be populated.
type Endpoint struct {
	Room     string `yaml:"room"`
	Edge     string `yaml:"edge"`
	Instance string `yaml:"instance"`
	Port     string `yaml:"port"`
}

type PortalDirection string

const (
	PortalBoth PortalDirection = "both"
	PortalAToB PortalDirection = "a-to-b"
	PortalBToA PortalDirection = "b-to-a"
)

// Connection links two aperture endpoints. Direction defaults to both for
// compatibility. Each endpoint may own one outgoing link and may be the target
// of any number of links. NonEuclidean permits equal-length edges at different
// positions or orientations.
type Connection struct {
	ID           string          `yaml:"id"`
	A            Endpoint        `yaml:"a"`
	B            Endpoint        `yaml:"b"`
	Direction    PortalDirection `yaml:"direction,omitempty"`
	NonEuclidean bool            `yaml:"non_euclidean,omitempty"`
}

// Port exposes one otherwise unconnected prefab endpoint to its instances.
type Port struct {
	ID       string   `yaml:"id"`
	Endpoint Endpoint `yaml:"endpoint"`
}
