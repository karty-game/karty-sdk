// Package worldlightmap defines the independently versioned, build-generated
// static receiver layout. It has no renderer or private engine dependencies.
package worldlightmap

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math"

	"github.com/karty-game/karty-sdk/format/cartridge"
	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/internal/binwire"
)

const (
	Schema                  = "karty.world-lightmaps@1"
	Algorithm               = uint32(1)
	ReceiverAlgorithm       = uint32(2)
	EntryName               = "@world/lightmaps/layout"
	MetadataKey             = "kartyWorldLightmaps"
	Feature                 = cartridge.FeatureWorldLightmapsV1
	MaxEncodedSize          = 8 * 1024 * 1024
	MaxCharts               = 8192
	MaxTexels               = 1024 * 1024
	MaxBakeLights           = world.MaxLights
	PointVisibilityEncoding = "point-visibility@1"
	DirectRNMEncoding       = "direct-rnm3@1"
	BinaryMagic             = "KLM1"
)

var (
	ErrLayout = errors.New("world lightmap layout is invalid")
	// ErrPageCapacity identifies finite receiver charts that cannot fit the
	// requested page. Capacity errors also wrap ErrLayout. Invalid geometry,
	// options and lighting recipes do not wrap ErrPageCapacity.
	ErrPageCapacity = errors.New("world lightmap page capacity is exhausted")
)

type Layout struct {
	Schema         string
	Algorithm      uint32
	GeometrySHA256 string
	TexelsPerUnit  float64
	Padding        int
	Pages          []Page
	Charts         []Chart
	Bindings       []Binding
	RuntimeBake    *RuntimeBake
	Receivers      *ReceiverSelection
}

// ReceiverSelection scopes an atlas to global sector/solid identities. The
// complete ordinary physical component remains the transport context.
type ReceiverSelection struct {
	Sectors []int
	Solids  []int
}

type Page struct {
	Width  int
	Height int
}

// Chart describes a receiver's atlas placement and basis.
// Rectangles are [minimumX,minimumY,maximumX,maximumY] in page pixels,
// maximums exclusive. Plane values are normalized page coordinates.
type Chart struct {
	ID           int
	Page         int
	Rect         [4]int
	ReceiverRect [4]int
	Normal       world.Vec3
	Tangent      world.Vec3
	Bitangent    world.Vec3
	UPlane       [4]float64
	VPlane       [4]float64
}

// Binding associates a world surface with its receiver chart.
// Index addresses the world sector or solid array. Edge is -1 for caps, and
// addresses the directed wall or footprint edge otherwise. Chart -1 explicitly
// marks a wall with no opaque receiver. Array order is semantic discovery order.
type Binding struct {
	Kind  string
	Index int
	Edge  int
	Chart int
}

type RuntimeBake struct {
	Encoding   string
	LightID    string
	LightIDs   []string
	ShadowSize int
}

type Options struct {
	TexelsPerUnit                          float64
	PageSize, Padding, MaxPages, MaxTexels int
	Light                                  string
	Lights                                 []string
	// AllStatic selects every fixed source without a nonzero emission pulse.
	// It is build policy, not an additional runtime wire field.
	AllStatic  bool
	ShadowSize int
	// CapsWithinSourceRoom prevents cap charts from spanning authored room or
	// prefab-instance boundaries, while retaining efficient cap merging inside
	// each room. It is optional build policy; canonical layouts remain unchanged.
	CapsWithinSourceRoom bool
	// CompactPacking uses bounded free-rectangle packing and optional 90-degree
	// chart turns instead of default shelves. Density and limits are retained.
	CompactPacking bool
}

func Encode(layout Layout, document *world.Document) ([]byte, error) {
	if err := Validate(&layout, document); err != nil {
		return nil, err
	}
	return encodeLayoutBinary(layout)
}

func encodeLayoutBinary(layout Layout) ([]byte, error) {
	w := binwire.NewWriter(BinaryMagic, MaxEncodedSize)
	writeWorldlightmapLayout(w, &layout)
	encoded, err := w.Bytes()
	if err != nil {
		return nil, ErrLayout
	}
	return encoded, nil
}

// LayoutDigest hashes the exact canonical binary layout. It checks structural
// bounds and finite fields without a world; callers validate semantic bindings
// against the complete world before mounting a layout.
func LayoutDigest(layout *Layout) (string, error) {
	if layout == nil {
		return "", ErrLayout
	}
	encoded, err := encodeLayoutBinary(*layout)
	if err != nil {
		return "", err
	}
	r := binwire.NewReader(encoded, BinaryMagic, MaxEncodedSize)
	scanWorldlightmapLayout(r)
	if r.Done() != nil {
		return "", ErrLayout
	}
	return digest(encoded), nil
}

func Decode(encoded []byte, document *world.Document) (Layout, error) {
	layout, err := decodeLayoutBinary(encoded)
	if err != nil {
		return Layout{}, err
	}
	if err := Validate(&layout, document); err != nil {
		return Layout{}, err
	}
	return layout, nil
}

// GeometryDigest includes stable semantic identity, boundary topology and
// elevation geometry. Actor state, lighting, materials and UVs do not affect it.
func GeometryDigest(document *world.Document) (string, error) {
	if err := world.Validate(document); err != nil {
		return "", err
	}
	return geometryDigestValidated(document)
}

func geometryDigestValidated(document *world.Document) (string, error) {
	w := binwire.NewWriter("KWG1", world.MaxEncodedSize)
	w.Count(len(document.Sectors), false, world.MaxSectors)
	for _, s := range document.Sectors {
		w.String(s.ID, world.MaxIdentifierBytes)
		w.Count(len(s.Walls), false, world.MaxWallsPerSector)
		for _, wall := range s.Walls {
			writeWorldVec2(w, &wall.Start)
			writeWorldVec2(w, &wall.End)
			w.I32(int(wall.Portal))
			w.U16(wall.PortalWall)
			w.Bool(wall.Sky)
		}
		writeWorldPlane(w, &s.Floor)
		writeWorldPlane(w, &s.Ceiling)
		w.Bool(s.Sky)
	}
	count := 0
	if document.StaticSolids != nil {
		count = len(document.StaticSolids.Items)
	}
	w.Count(count, false, world.MaxStaticSolids)
	if document.StaticSolids != nil {
		for _, s := range document.StaticSolids.Items {
			w.String(s.ID, world.MaxIdentifierBytes)
			w.Count(len(s.Footprint), false, world.MaxSolidVertices)
			for _, point := range s.Footprint {
				writeWorldVec2(w, &point)
			}
			writeWorldPlane(w, &s.Bottom)
			writeWorldPlane(w, &s.Top)
		}
	}
	encoded, err := w.Bytes()
	if err != nil {
		return "", ErrLayout
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func finite(v float64) bool                    { return !math.IsNaN(v) && !math.IsInf(v, 0) }
func sub(a, b world.Vec3) world.Vec3           { return world.Vec3{X: a.X - b.X, Y: a.Y - b.Y, Z: a.Z - b.Z} }
func dot(a, b world.Vec3) float64              { return a.X*b.X + a.Y*b.Y + a.Z*b.Z }
func scale(a world.Vec3, s float64) world.Vec3 { return world.Vec3{X: a.X * s, Y: a.Y * s, Z: a.Z * s} }
func cross(a, b world.Vec3) world.Vec3 {
	return world.Vec3{X: a.Y*b.Z - a.Z*b.Y, Y: a.Z*b.X - a.X*b.Z, Z: a.X*b.Y - a.Y*b.X}
}
func unit(a world.Vec3) world.Vec3                  { return scale(a, 1/math.Sqrt(dot(a, a))) }
func elevation(p world.Plane, v world.Vec2) float64 { return p.A*v.X + p.B*v.Y + p.C }
func point(v world.Vec2, z float64) world.Vec3      { return world.Vec3{X: v.X, Y: v.Y, Z: z} }
func planeValue(p [4]float64, v world.Vec3) float64 { return p[0]*v.X + p[1]*v.Y + p[2]*v.Z + p[3] }
