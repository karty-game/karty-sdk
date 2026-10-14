package worldlightmapbake

import (
	"math"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
)

func TestBrightSkyBakeEnergyAndCacheIdentity(t *testing.T) {
	t.Parallel()
	document, layout, materials := environmentRoom(t, true, 0)
	settings := world.EnvironmentSettings{TimeOfDay: 12, CloudAmount: 1}
	options := Options{Samples: 8, Bounces: 1, Workers: 1}
	base, err := BakeEnvironment(t.Context(), document, layout, materials, settings, options)
	if err != nil {
		t.Fatal(err)
	}
	baseKey, err := EnvironmentInputDigest(document, layout, materials, settings, options)
	if err != nil {
		t.Fatal(err)
	}
	document.Environment.Preset = world.EnvironmentDaylightBright
	bright, err := BakeEnvironment(t.Context(), document, layout, materials, settings, options)
	if err != nil {
		t.Fatal(err)
	}
	brightKey, err := EnvironmentInputDigest(document, layout, materials, settings, options)
	if err != nil || brightKey == baseKey {
		t.Fatal("brighter sky reused stale bake identity", err)
	}
	x, y := floorAt(layout, vec{X: 2, Y: 2})
	want, got := scale(decoded(base, x, y), 4), decoded(bright, x, y)
	if math.Abs(got.X-want.X) > .005 || math.Abs(got.Y-want.Y) > .005 || math.Abs(got.Z-want.Z) > .005 {
		t.Fatalf("sky-only energy did not scale by four: got %+v want %+v", got, want)
	}
	if maximum(got) > bright.RGBMRange || bright.Stats.Rays != base.Stats.Rays {
		t.Fatal("brighter sky changed tracing work or exceeded its RGBM range")
	}
}
