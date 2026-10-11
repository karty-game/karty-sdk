package world_test

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"reflect"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
)

func validSolid() world.Solid {
	return world.Solid{
		ID:             "pillar",
		Footprint:      []world.Vec2{{X: 1, Y: 1}, {X: 2, Y: 1}, {X: 2, Y: 2}, {X: 1, Y: 2}},
		Top:            world.Plane{C: 3},
		SideMaterial:   2,
		TopMaterial:    3,
		BottomMaterial: 4,
	}
}

func solidBatch(count int) []world.Solid {
	items := make([]world.Solid, count)
	for index := range items {
		items[index] = validSolid()
		items[index].ID = fmt.Sprintf("pillar-%04d", index)
	}
	return items
}

func TestStaticSolidsThousandPillarsAndCountBoundary(t *testing.T) {
	for _, count := range []int{1000, 1024} {
		t.Run(fmt.Sprintf("count-%d", count), func(t *testing.T) {
			document := lightingWorld()
			document.StaticSolids = &world.StaticSolids{Version: 1, Items: solidBatch(count)}
			encoded, err := world.Encode(document)
			if err != nil {
				t.Fatalf("encode %d pillars: %v", count, err)
			}
			decoded, err := world.Decode(encoded)
			if err != nil || !reflect.DeepEqual(decoded, document) {
				t.Fatalf("canonical %d-pillar roundtrip: %v", count, err)
			}
			document.StaticSolids.Items[count-1].Top = document.StaticSolids.Items[count-1].Bottom
			if err := world.Validate(&document); !errors.Is(err, world.ErrGeometry) {
				t.Fatalf("invalid final pillar accepted: %v", err)
			}
		})
	}
	oversized := lightingWorld()
	oversized.StaticSolids = &world.StaticSolids{Version: 1, Items: solidBatch(1025)}
	if err := world.Validate(&oversized); !errors.Is(err, world.ErrBounds) {
		t.Fatalf("1,025 pillars passed typed validation: %v", err)
	}
	if _, err := world.Encode(oversized); !errors.Is(err, world.ErrBounds) {
		t.Fatalf("1,025 pillars passed canonical encoding: %v", err)
	}
	oversized.StaticSolids.Items = oversized.StaticSolids.Items[:world.MaxStaticSolids]
	encoded := uncheckedWorld(t, oversized)
	marker := append(word32(uint32(world.MaxStaticSolids+1)), append(word32(11), []byte("pillar-0000")...)...)
	at := bytes.Index(encoded, marker)
	if at < 0 {
		t.Fatal("missing solid count")
	}
	copy(encoded[at:], word32(world.MaxStaticSolids+2))
	decoded, err := world.Decode(encoded)
	if !errors.Is(err, world.ErrBounds) || len(decoded.Sectors) != 0 || decoded.StaticSolids != nil {
		t.Fatalf("oversized partial world: %v", err)
	}

}

func TestStaticSolidsCanonicalOptionalContract(t *testing.T) {
	document := lightingWorld()
	document.StaticSolids = &world.StaticSolids{Version: 1, Items: []world.Solid{validSolid()}}
	encoded, err := world.Encode(document)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := world.Decode(encoded)
	if err != nil || !reflect.DeepEqual(decoded, document) {
		t.Fatalf("roundtrip: %v", err)
	}
	assertBinaryMalformed(t, encoded)
	for _, version := range []uint16{1, 2} {
		document.Version = version
		if _, err := world.Decode(uncheckedWorld(t, document)); !errors.Is(err, world.ErrVersion) {
			t.Fatalf("v%d accepted solids: %v", version, err)
		}
	}
}

func TestStaticSolidsValidateCompleteBounds(t *testing.T) {
	tests := map[string]func(*world.StaticSolids){
		"version":         func(p *world.StaticSolids) { p.Version = 2 },
		"too many":        func(p *world.StaticSolids) { p.Items = make([]world.Solid, world.MaxStaticSolids+1) },
		"duplicate":       func(p *world.StaticSolids) { p.Items[1].ID = p.Items[0].ID },
		"zero material":   func(p *world.StaticSolids) { p.Items[1].SideMaterial = 0 },
		"NaN":             func(p *world.StaticSolids) { p.Items[1].Footprint[0].X = math.NaN() },
		"flat":            func(p *world.StaticSolids) { p.Items[1].Top = p.Items[1].Bottom },
		"sloped crossing": func(p *world.StaticSolids) { p.Items[1].Top = world.Plane{A: -2, C: 3} },
		"collinear":       func(p *world.StaticSolids) { p.Items[1].Footprint[1] = world.Vec2{X: 1.5, Y: 1.5} },
		"clockwise": func(p *world.StaticSolids) {
			p.Items[1].Footprint[1], p.Items[1].Footprint[3] = p.Items[1].Footprint[3], p.Items[1].Footprint[1]
		},
		"many vertices": func(p *world.StaticSolids) { p.Items[1].Footprint = make([]world.Vec2, world.MaxSolidVertices+1) },
		"bad UV":        func(p *world.StaticSolids) { p.Items[1].TopUV = &world.SurfaceUV{} },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			a, b := validSolid(), validSolid()
			b.ID = "last"
			payload := &world.StaticSolids{Version: 1, Items: []world.Solid{a, b}}
			mutate(payload)
			if err := world.ValidateStaticSolids(payload); err == nil {
				t.Fatal("invalid last solid accepted")
			}
		})
	}
	document := lightingWorld()
	solid := validSolid()
	solid.TopUV = &world.SurfaceUV{
		Projections: []world.UVProjection{{U: world.UVPlane{X: 1}, V: world.UVPlane{Y: 1}}},
		Weights:     []float64{1},
	}
	document.StaticSolids = &world.StaticSolids{Version: 1, Items: []world.Solid{solid}}
	if err := world.Validate(&document); !errors.Is(err, world.ErrMaterialMapping) {
		t.Fatalf("undeclared mapping accepted: %v", err)
	}
}

func TestStaticSolidsPresenceVersionAndNullRejection(t *testing.T) {
	for version := uint16(1); version <= world.Version; version++ {
		d := validWorld()
		if version == world.Version {
			d = lightingWorld()
		}
		d.Version = version
		d.StaticSolids = &world.StaticSolids{Version: 1, Items: []world.Solid{}}
		decoded, err := world.Decode(uncheckedWorld(t, d))
		if version < world.Version {
			if !errors.Is(err, world.ErrVersion) || len(decoded.Sectors) != 0 {
				t.Fatalf("v%d accepted solids: %v", version, err)
			}
		} else if err != nil || decoded.StaticSolids == nil || decoded.StaticSolids.Items == nil {
			t.Fatalf("v3 empty presence lost: %v", err)
		}
	}
}
