package world_test

import (
	"bytes"
	"encoding/binary"
	"math"
	"reflect"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
)

func fluidWorld() world.Document {
	d := lightingWorld()
	s := validSolid()
	s.Collision = false
	d.StaticSolids = &world.StaticSolids{Version: 1, Items: []world.Solid{s}}
	d.Animations = &world.Animations{
		Version:   1,
		Presets:   []world.AnimationPreset{{Name: "ripple", Kind: "liquid", Frequency: 1}},
		Materials: []world.MaterialAnimation{{Material: s.TopMaterial, Preset: "ripple"}},
	}
	d.Fluids = &world.Fluids{
		Version: 1,
		Items: []world.Fluid{
			{ID: "pool", Solids: []string{s.ID}, Density: 1000, FoamWidth: .2, FoamStrength: .5, FogDensity: .3, Distortion: .003},
		},
	}
	return d
}
func TestFluidsCodecCompatibilityAndCompleteValidation(t *testing.T) {
	d := fluidWorld()
	encoded, err := world.Encode(d)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded[:4]) != world.FluidBinaryMagic {
		t.Fatal("fluid magic")
	}
	decoded, err := world.Decode(encoded)
	if err != nil || !reflect.DeepEqual(d, decoded) {
		t.Fatal("fluid roundtrip", err)
	}
	for n := range len(encoded) {
		if _, err := world.Decode(encoded[:n]); err == nil {
			t.Fatal("truncation accepted", n)
		}
	}
	if _, err := world.Decode(append(encoded, 0)); err == nil {
		t.Fatal("trailing byte accepted")
	}
	// A corrupted fluid count must fail before publishing the base document.
	marker := append([]byte{1, 0, 0, 0, 2, 0, 0, 0, 4, 0, 0, 0}, []byte("pool")...)
	at := bytes.LastIndex(encoded, marker)
	if at < 0 {
		t.Fatal("missing fluid trailer")
	}
	bad := bytes.Clone(encoded)
	binary.LittleEndian.PutUint32(bad[at+4:], world.MaxFluids+2)
	partial, err := world.Decode(bad)
	if err == nil || len(partial.Sectors) != 0 {
		t.Fatal("unbounded or partial decode")
	}
	for _, mutate := range []func(*world.Document){func(d *world.Document) { d.Fluids.Items[0].Density = math.NaN() }, func(d *world.Document) { d.Fluids.Items[0].Solids = append(d.Fluids.Items[0].Solids, "missing") }, func(d *world.Document) { d.StaticSolids.Items[0].Collision = true }, func(d *world.Document) { d.StaticSolids.Items[0].Top.A = .1 }, func(d *world.Document) { d.Fluids.Version = 2 }} {
		invalid := fluidWorld()
		mutate(&invalid)
		if _, err := world.Encode(invalid); err == nil {
			t.Fatal("invalid fluids accepted")
		}
	}
	d.Fluids = nil
	legacy, err := world.Encode(d)
	if err != nil || string(legacy[:4]) != world.BinaryMagic {
		t.Fatal("legacy encoding changed", err)
	}
	d.Environment = &world.Environment{Version: 1, Preset: world.EnvironmentDaylight, Backdrop: world.EnvironmentBackdropNone}
	legacy, err = world.Encode(d)
	if err != nil {
		t.Fatal(err)
	}
	if string(legacy[:4]) != world.SolidBinaryMagic {
		t.Fatal("KWR2 changed")
	}
}
