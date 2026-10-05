// Package worldlightmap defines the independently versioned, build-generated
// static receiver layout. It has no renderer or private engine dependencies.
package worldlightmap

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"github.com/karty-game/karty-sdk/format/cartridge"
	"github.com/karty-game/karty-sdk/format/world"
)

const (
	Schema                  = "karty.world-lightmaps@1"
	Algorithm               = uint32(1)
	EntryName               = "@world/lightmaps/layout"
	MetadataKey             = "kartyWorldLightmaps"
	Feature                 = cartridge.FeatureWorldLightmapsV1
	MaxEncodedSize          = 8 * 1024 * 1024
	MaxCharts               = 8192
	MaxTexels               = 1024 * 1024
	MaxBakeLights           = 8
	PointVisibilityEncoding = "point-visibility@1"
	DirectRNMEncoding       = "direct-rnm3@1"
)

var ErrLayout = errors.New("world lightmap layout is invalid")

type Layout struct {
	Schema         string       `json:"schema"`
	Algorithm      uint32       `json:"algorithm"`
	GeometrySHA256 string       `json:"geometry_sha256"`
	TexelsPerUnit  float64      `json:"texels_per_unit"`
	Padding        int          `json:"padding"`
	Pages          []Page       `json:"pages"`
	Charts         []Chart      `json:"charts"`
	Bindings       []Binding    `json:"bindings"`
	RuntimeBake    *RuntimeBake `json:"runtime_bake,omitempty"`
}

type Page struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// Rectangles are [minimumX,minimumY,maximumX,maximumY] in page pixels,
// maximums exclusive. Plane values are normalized page coordinates.
type Chart struct {
	ID           int        `json:"id"`
	Page         int        `json:"page"`
	Rect         [4]int     `json:"rect"`
	ReceiverRect [4]int     `json:"receiver_rect"`
	Normal       world.Vec3 `json:"normal"`
	Tangent      world.Vec3 `json:"tangent"`
	Bitangent    world.Vec3 `json:"bitangent"`
	UPlane       [4]float64 `json:"u_plane"`
	VPlane       [4]float64 `json:"v_plane"`
}

// Index addresses the world sector or solid array. Edge is -1 for caps, and
// addresses the directed wall or footprint edge otherwise. Chart -1 explicitly
// marks a wall with no opaque receiver. Array order is semantic discovery order.
type Binding struct {
	Kind  string `json:"kind"`
	Index int    `json:"index"`
	Edge  int    `json:"edge"`
	Chart int    `json:"chart"`
}

type RuntimeBake struct {
	Encoding   string   `json:"encoding"`
	LightID    string   `json:"light_id,omitempty"`
	LightIDs   []string `json:"light_ids,omitempty"`
	ShadowSize int      `json:"shadow_size"`
}

type Options struct {
	TexelsPerUnit                          float64
	PageSize, Padding, MaxPages, MaxTexels int
	Light                                  string
	Lights                                 []string
	ShadowSize                             int
}

func Encode(layout Layout, document *world.Document) ([]byte, error) {
	if err := Validate(&layout, document); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(layout)
	if err != nil || len(encoded) > MaxEncodedSize {
		return nil, ErrLayout
	}
	return encoded, nil
}

func Decode(encoded []byte, document *world.Document) (Layout, error) {
	if len(encoded) == 0 || len(encoded) > MaxEncodedSize {
		return Layout{}, ErrLayout
	}
	var layout Layout
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&layout); err != nil {
		return Layout{}, fmt.Errorf("decode lightmap: %w", ErrLayout)
	}
	if err := Validate(&layout, document); err != nil {
		return Layout{}, err
	}
	canonical, err := json.Marshal(layout)
	if err != nil || !bytes.Equal(encoded, canonical) {
		return Layout{}, fmt.Errorf("canonical lightmap: %w", ErrLayout)
	}
	return layout, nil
}

// GeometryDigest includes stable semantic identity, boundary topology and
// elevation geometry. Actor state, lighting, materials and UVs do not affect it.
func GeometryDigest(document *world.Document) (string, error) {
	if err := world.Validate(document); err != nil {
		return "", err
	}
	type wallGeometry struct {
		Start, End world.Vec2
		Portal     int32
		PortalWall uint16
	}
	type sectorGeometry struct {
		ID             string
		Walls          []wallGeometry
		Floor, Ceiling world.Plane
	}
	type solidGeometry struct {
		ID          string
		Footprint   []world.Vec2
		Bottom, Top world.Plane
	}
	geometry := struct {
		Sectors []sectorGeometry
		Solids  []solidGeometry
	}{}
	for _, s := range document.Sectors {
		g := sectorGeometry{ID: s.ID, Floor: s.Floor, Ceiling: s.Ceiling}
		for _, w := range s.Walls {
			g.Walls = append(g.Walls, wallGeometry{w.Start, w.End, w.Portal, w.PortalWall})
		}
		geometry.Sectors = append(geometry.Sectors, g)
	}
	if document.StaticSolids != nil {
		for _, s := range document.StaticSolids.Items {
			geometry.Solids = append(geometry.Solids, solidGeometry{s.ID, s.Footprint, s.Bottom, s.Top})
		}
	}
	encoded, err := json.Marshal(geometry)
	if err != nil {
		return "", ErrLayout
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
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
