// Package worldsource defines the high-level, separately versioned room and
// prefab schema consumed by public world compilers. It intentionally has no
// YAML implementation dependency; tags describe the stable field vocabulary.
package worldsource

import "errors"

const (
	Version              = uint16(1)
	MaxRooms             = 1024
	MaxPrefabs           = 256
	MaxInstances         = 4096
	MaxConnections       = 8192
	MaxPorts             = 4096
	MaxEdges             = 8192
	MaxEdgesPerRoom      = 256
	MaxContents          = 4096
	MaxMaterialOverrides = 256
	MaxIdentifierBytes   = 128
	MaxCoordinate        = 1_000_000.0
	MinEdgeLength        = 1e-6
	MinClearance         = 1e-6
	geometryEpsilon      = 1e-9
)

var (
	ErrVersion     = errors.New("world source version is unsupported")
	ErrBounds      = errors.New("world source exceeds a declared bound")
	ErrIdentity    = errors.New("world source identity is invalid")
	ErrGeometry    = errors.New("world source geometry is invalid")
	ErrReference   = errors.New("world source reference is invalid")
	ErrConnection  = errors.New("world source connection is invalid")
	ErrContent     = errors.New("world source content is invalid")
	ErrPrefabCycle = errors.New("world source prefab cycle is invalid")
)

type Vec2 struct {
	X float64 `json:"x" yaml:"x"`
	Y float64 `json:"y" yaml:"y"`
}

type Vec3 struct {
	X float64 `json:"x" yaml:"x"`
	Y float64 `json:"y" yaml:"y"`
	Z float64 `json:"z" yaml:"z"`
}

// Plane describes elevation z=A*x+B*y+C in source coordinates.
type Plane struct {
	A float64 `json:"a" yaml:"a"`
	B float64 `json:"b" yaml:"b"`
	C float64 `json:"c" yaml:"c"`
}

type Document struct {
	Version     uint16       `json:"version" yaml:"version"`
	Rooms       []Room       `json:"rooms" yaml:"rooms"`
	Prefabs     []Prefab     `json:"prefabs" yaml:"prefabs"`
	Instances   []Instance   `json:"instances" yaml:"instances"`
	Connections []Connection `json:"connections" yaml:"connections"`
}

// Room may be concave. The compiler decomposes it into convex sectors after
// prefab expansion and connection resolution.
type Room struct {
	ID              string    `json:"id" yaml:"id"`
	Boundary        []Edge    `json:"boundary" yaml:"boundary"`
	Floor           Plane     `json:"floor" yaml:"floor"`
	Ceiling         Plane     `json:"ceiling" yaml:"ceiling"`
	FloorMaterial   string    `json:"floor_material" yaml:"floor_material"`
	CeilingMaterial string    `json:"ceiling_material" yaml:"ceiling_material"`
	Contents        []Content `json:"contents" yaml:"contents"`
}

// Edge is one named, directed CCW boundary edge.
type Edge struct {
	ID       string `json:"id" yaml:"id"`
	Start    Vec2   `json:"start" yaml:"start"`
	End      Vec2   `json:"end" yaml:"end"`
	Material string `json:"material" yaml:"material"`
}

type Content struct {
	ID       string `json:"id" yaml:"id"`
	Kind     string `json:"kind" yaml:"kind"`
	Position Vec3   `json:"position" yaml:"position"`
}

type Prefab struct {
	ID          string       `json:"id" yaml:"id"`
	Rooms       []Room       `json:"rooms" yaml:"rooms"`
	Instances   []Instance   `json:"instances" yaml:"instances"`
	Connections []Connection `json:"connections" yaml:"connections"`
	Ports       []Port       `json:"ports" yaml:"ports"`
}

type Instance struct {
	ID        string             `json:"id" yaml:"id"`
	Prefab    string             `json:"prefab" yaml:"prefab"`
	Transform Transform          `json:"transform" yaml:"transform"`
	Materials []MaterialOverride `json:"materials" yaml:"materials"`
}

// Transform applies uniform scale, Z-axis yaw in degrees and translation.
// Scale zero means the schema default of one.
type Transform struct {
	Translation Vec3    `json:"translation" yaml:"translation"`
	YawDegrees  float64 `json:"yaw_degrees" yaml:"yaw_degrees"`
	Scale       float64 `json:"scale" yaml:"scale"`
}

type MaterialOverride struct {
	From string `json:"from" yaml:"from"`
	To   string `json:"to" yaml:"to"`
}

// Endpoint selects either a room edge in the current scope or a named port on
// an instance in that scope. Exactly one pair must be populated.
type Endpoint struct {
	Room     string `json:"room" yaml:"room"`
	Edge     string `json:"edge" yaml:"edge"`
	Instance string `json:"instance" yaml:"instance"`
	Port     string `json:"port" yaml:"port"`
}

// Connection joins two aperture endpoints once; reciprocal duplicate records
// are neither needed nor accepted.
type Connection struct {
	ID string   `json:"id" yaml:"id"`
	A  Endpoint `json:"a" yaml:"a"`
	B  Endpoint `json:"b" yaml:"b"`
}

// Port exposes one otherwise unconnected prefab endpoint to its instances.
type Port struct {
	ID       string   `json:"id" yaml:"id"`
	Endpoint Endpoint `json:"endpoint" yaml:"endpoint"`
}
