package world_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/karty-game/karty-sdk/format/world"
	"math"
	"reflect"
	"testing"
)

func validSolid() world.Solid {
	return world.Solid{ID: "pillar", Footprint: []world.Vec2{{X: 1, Y: 1}, {X: 2, Y: 1}, {X: 2, Y: 2}, {X: 1, Y: 2}}, Top: world.Plane{C: 3}, SideMaterial: 2, TopMaterial: 3, BottomMaterial: 4}
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
	encoded, err := json.Marshal(oversized)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := world.Decode(encoded)
	if !errors.Is(err, world.ErrBounds) || len(decoded.Sectors) != 0 || decoded.StaticSolids != nil {
		t.Fatalf("oversized canonical input published partial world: %v", err)
	}
}

func TestStaticSolidsCanonicalOptionalContract(t *testing.T) {
	document := lightingWorld()
	old, err := world.Encode(document)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(old, []byte("static_solids")) {
		t.Fatal("absent field changes encoding")
	}
	document.StaticSolids = &world.StaticSolids{Version: 1, Items: []world.Solid{validSolid()}}
	encoded, err := world.Encode(document)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := world.Decode(encoded)
	if err != nil || !reflect.DeepEqual(decoded, document) {
		t.Fatalf("roundtrip: %v", err)
	}
	for _, version := range []uint16{1, 2} {
		document.Version = version
		if err := world.Validate(&document); !errors.Is(err, world.ErrVersion) {
			t.Fatalf("v%d accepted solids: %v", version, err)
		}
	}
	document.Version = 3
	for name, malformed := range map[string][]byte{
		"null":           bytes.Replace(old, []byte(`"contents":`), []byte(`"static_solids":null,"contents":`), 1),
		"unknown":        bytes.Replace(encoded, []byte(`"collision"`), []byte(`"oops"`), 1),
		"missing top":    bytes.Replace(encoded, []byte(`"top":{"a":0,"b":0,"c":3},`), nil, 1),
		"explicit false": bytes.Replace(encoded, []byte(`"bottom_material":4`), []byte(`"bottom_material":4,"collision":false`), 1),
		"duplicate":      bytes.Replace(encoded, []byte(`"static_solids":`), []byte(`"static_solids":null,"static_solids":`), 1),
	} {
		if bytes.Equal(malformed, encoded) && name == "unknown" {
			malformed = bytes.Replace(encoded, []byte(`"bottom_material":4`), []byte(`"bottom_material":4,"oops":1`), 1)
		}
		if _, err := world.Decode(malformed); err == nil {
			t.Fatalf("accepted %s", name)
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
	solid.TopUV = &world.SurfaceUV{Projections: []world.UVProjection{{U: world.UVPlane{X: 1}, V: world.UVPlane{Y: 1}}}, Weights: []float64{1}}
	document.StaticSolids = &world.StaticSolids{Version: 1, Items: []world.Solid{solid}}
	if err := world.Validate(&document); !errors.Is(err, world.ErrMaterialMapping) {
		t.Fatalf("undeclared mapping accepted: %v", err)
	}
}

func TestStaticSolidsPresenceVersionAndNullRejection(t *testing.T) {
	for _, version := range []uint16{1, 2, 3} {
		document := validWorld()
		if version == 3 {
			document = lightingWorld()
		}
		document.Version = version
		absent, err := world.Encode(document)
		if err != nil {
			t.Fatal(err)
		}
		explicitNull := append(append([]byte(nil), absent[:len(absent)-1]...), []byte(`,"static_solids":null}`)...)
		decoded, err := world.Decode(explicitNull)
		if err == nil || decoded.StaticSolids != nil || len(decoded.Sectors) != 0 {
			t.Fatalf("v%d accepted null/published partial world: %v", version, err)
		}
		// A real payload is valid only in v3, even when it declares no items.
		emptyPayload := append(append([]byte(nil), absent[:len(absent)-1]...), []byte(`,"static_solids":{"version":1,"items":[]}}`)...)
		_, err = world.Decode(emptyPayload)
		if version < 3 && !errors.Is(err, world.ErrVersion) {
			t.Fatalf("v%d accepted payload: %v", version, err)
		}
		if version == 3 && err != nil {
			t.Fatalf("v3 rejected empty payload: %v", err)
		}
	}
}
