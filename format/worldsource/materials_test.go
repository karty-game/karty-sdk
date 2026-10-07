package worldsource_test

import (
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/karty-game/karty-sdk/format/worldsource"
)

func pointer[T any](v T) *T { return &v }
func TestAdvancedSourceVersionInheritanceAndBounds(t *testing.T) {
	t.Parallel()
	d := validSource()
	d.Version = worldsource.MaterialsVersion
	room := &d.Rooms[0]
	room.WallBands = &worldsource.BandSettings{
		Top:    &worldsource.HorizontalBandSettings{Texture: "brick-top", Height: pointer(.25), RepeatWidth: pointer(2.0)},
		Bottom: &worldsource.HorizontalBandSettings{Texture: "brick-bottom", Height: pointer(.5)},
	}
	room.Boundary[0].Bands = &worldsource.BandSettings{
		Top:    &worldsource.HorizontalBandSettings{Enabled: pointer(false)},
		Bottom: &worldsource.HorizontalBandSettings{Texture: "stone-bottom"},
	}
	room.WallSecondary = &worldsource.SecondarySettings{
		Texture:  "grain",
		Strength: pointer(.3),
		UV:       &worldsource.UVSettings{Scale: &worldsource.Vec2{X: 7, Y: 7}},
	}
	if err := worldsource.Validate(&d); err != nil {
		t.Fatal(err)
	}
	merged := worldsource.MergeBandSettings(room.WallBands, room.Boundary[0].Bands)
	if merged.Top.Texture != "brick-top" || *merged.Top.Height != .25 || *merged.Top.Enabled || merged.Bottom.Texture != "stone-bottom" ||
		*merged.Bottom.Height != .5 {
		t.Fatalf("inheritance: %+v", merged)
	}
	if !reflect.DeepEqual(room.WallBands.Top.Enabled, (*bool)(nil)) {
		t.Fatal("merge modified input")
	}
	clone := worldsource.MergeBandSettings(room.WallBands, nil)
	clone.Top.Texture = "replacement"
	if room.WallBands.Top.Texture != "brick-top" {
		t.Fatal("base-only merge shared band records")
	}
	encoded, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	var round worldsource.Document
	if err = json.Unmarshal(encoded, &round); err != nil || !reflect.DeepEqual(d, round) {
		t.Fatalf("source fields roundtrip: %v", err)
	}
	for _, version := range []uint16{1, 2, 3, 4, 5, 6} {
		d.Version = version
		if err := worldsource.Validate(&d); !errors.Is(err, worldsource.ErrVersion) {
			t.Fatalf("source v%d accepted bands: %v", version, err)
		}
	}
	for _, value := range []float64{-1, 0, .0009, math.NaN(), math.Inf(1), 1_000_001} {
		h := &worldsource.BandSettings{Top: &worldsource.HorizontalBandSettings{Height: pointer(value)}}
		if err := worldsource.ValidateBandSettings(h); !errors.Is(err, worldsource.ErrBounds) {
			t.Fatalf("invalid height %g: %v", value, err)
		}
	}
}
func TestAdvancedSourceChecksLatePrefabRecords(t *testing.T) {
	t.Parallel()
	d := validSource()
	d.Version = worldsource.MaterialsVersion
	last := &d.Prefabs[len(d.Prefabs)-1].Rooms[0].Boundary[0]
	last.Bands = &worldsource.BandSettings{Bottom: &worldsource.HorizontalBandSettings{Texture: string([]byte{0xff})}}
	if err := worldsource.Validate(&d); !errors.Is(err, worldsource.ErrIdentity) {
		t.Fatalf("invalid late setting: %v", err)
	}
	last.Bands = nil
	last.Secondary = &worldsource.SecondarySettings{Texture: "variation", Strength: pointer(1.1)}
	if err := worldsource.Validate(&d); !errors.Is(err, worldsource.ErrBounds) {
		t.Fatalf("invalid late secondary: %v", err)
	}
}

func TestSecondaryHorizontalMappingScope(t *testing.T) {
	t.Parallel()
	for _, uv := range []*worldsource.UVSettings{{Mode: worldsource.UVWrap}, {Anchor: worldsource.UVTop}, {Anchor: worldsource.UVBottom}} {
		d := validSource()
		d.Version = worldsource.MaterialsVersion
		d.Rooms[0].FloorSecondary = &worldsource.SecondarySettings{Texture: "detail", UV: uv}
		if err := worldsource.Validate(&d); !errors.Is(err, worldsource.ErrUVMapping) {
			t.Fatalf("invalid floor detail mapping: %v", err)
		}
	}
}
