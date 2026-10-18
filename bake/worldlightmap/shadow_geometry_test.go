package worldlightmapbake

import (
	"bytes"
	"math"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/internal/binwire"
)

func shadowRoof() [][3]world.Vec3 {
	return [][3]world.Vec3{
		{{X: -20, Y: -20, Z: 1}, {X: 20, Y: -20, Z: 1}, {X: 20, Y: 20, Z: 1}},
		{{X: -20, Y: -20, Z: 1}, {X: 20, Y: 20, Z: 1}, {X: -20, Y: 20, Z: 1}},
	}
}

func TestShadowGeometryGroundsSunAndAbsorbsSky(t *testing.T) {
	t.Parallel()
	document, layout, materials := environmentRoom(t, true, 0)
	settings := world.EnvironmentSettings{TimeOfDay: 12}
	options := Options{Samples: 32, Bounces: 2, Workers: 1, Seed: 7}
	before, err := BakeEnvironment(t.Context(), document, layout, materials, settings, options)
	if err != nil {
		t.Fatal(err)
	}
	options.ShadowTriangles = shadowRoof()
	after, err := BakeEnvironment(t.Context(), document, layout, materials, settings, options)
	if err != nil {
		t.Fatal(err)
	}
	x, y := floorAt(layout, vec{X: 2, Y: 2})
	lit, shaded := maximum(decoded(before, x, y)), maximum(decoded(after, x, y))
	if lit < .15 || shaded > .01 || after.Image.NRGBAAt(x, y).A == 0 {
		t.Fatalf("opaque absorbing roof failed sun/sky shadow: lit=%g shaded=%g", lit, shaded)
	}
	if before.ReflectanceSHA256 != after.ReflectanceSHA256 || after.Stats.Triangles != before.Stats.Triangles+2 ||
		after.Stats.ReceiverTexels != before.Stats.ReceiverTexels || before.RGBMRange != after.RGBMRange {
		t.Fatal("shadow proxies changed reflectance provenance, receiver layout or energy bound")
	}
}

func TestShadowGeometryBlocksPointLightWithoutReceivers(t *testing.T) {
	t.Parallel()
	document, layout, materials := room(t, 0)
	options := Options{Samples: 4, Workers: 1}
	before, err := Bake(t.Context(), document, layout, materials, options)
	if err != nil {
		t.Fatal(err)
	}
	options.ShadowTriangles = shadowRoof()
	for i := range options.ShadowTriangles {
		for j := range options.ShadowTriangles[i] {
			options.ShadowTriangles[i][j].Z = .5
		}
	}
	after, err := Bake(t.Context(), document, layout, materials, options)
	if err != nil {
		t.Fatal(err)
	}
	x, y := floorAt(layout, vec{X: 1, Y: 2})
	if maximum(decoded(before, x, y)) < .1 || maximum(decoded(after, x, y)) > 1e-8 || after.Image.NRGBAAt(x, y).A == 0 {
		t.Fatal("shadow proxy failed point-light grounding")
	}
	if before.ReflectanceSHA256 != after.ReflectanceSHA256 || before.Stats.ReceiverTexels != after.Stats.ReceiverTexels {
		t.Fatal("point shadow changed source provenance/receiver count")
	}
}

func TestShadowGeometryEmptyPreservesPixelsAndDigests(t *testing.T) {
	t.Parallel()
	document, layout, materials := environmentRoom(t, true, 0)
	settings := world.EnvironmentSettings{TimeOfDay: 12}
	options := Options{Samples: 4, Bounces: 1, Workers: 1, Seed: 7}
	before, err := BakeEnvironment(t.Context(), document, layout, materials, settings, options)
	if err != nil {
		t.Fatal(err)
	}
	key, err := EnvironmentInputDigest(document, layout, materials, settings, options)
	if err != nil {
		t.Fatal(err)
	}
	options.ShadowTriangles = [][3]world.Vec3{}
	after, err := BakeEnvironment(t.Context(), document, layout, materials, settings, options)
	if err != nil {
		t.Fatal(err)
	}
	other, err := EnvironmentInputDigest(document, layout, materials, settings, options)
	if err != nil || key != other || !bytes.Equal(before.Image.Pix, after.Image.Pix) || before.Stats.Rays != after.Stats.Rays {
		t.Fatalf("empty geometry changed legacy identity/pixels: %v", err)
	}
	w := binwire.NewWriter("TEST", 16)
	writeShadowTriangles(w, nil)
	writeShadowTriangles(w, options.ShadowTriangles)
	encoded, err := w.Bytes()
	baseline, baselineErr := binwire.NewWriter("TEST", 16).Bytes()
	if err != nil || baselineErr != nil || !bytes.Equal(encoded, baseline) {
		t.Fatalf("empty geometry appended digest bytes: %x %v", encoded, err)
	}
}

func TestShadowGeometryChangesBothPixelDigestsAndSharedPreparation(t *testing.T) {
	t.Parallel()
	environment, envLayout, materials := environmentRoom(t, true, 0)
	point, pointLayout, pointMaterials := room(t, 0)
	settings := world.EnvironmentSettings{TimeOfDay: 12}
	options := Options{Samples: 4, Bounces: 1, Workers: 1, Seed: 7}
	shared := NewBaker(environment, materials)
	var previous *preparedTransport
	var envKey, pointKey string
	for change := range 4 {
		switch change {
		case 1:
			options.ShadowTriangles = shadowRoof()
		case 2:
			for i := range options.ShadowTriangles {
				for j := range options.ShadowTriangles[i] {
					options.ShadowTriangles[i][j].X += 50
				}
			}
		case 3:
			options.ShadowTriangles = nil
		}
		newEnvKey, err := EnvironmentInputDigest(environment, envLayout, materials, settings, options)
		if err != nil {
			t.Fatal(err)
		}
		newPointKey, err := BakeInputDigest(point, pointLayout, pointMaterials, options)
		if err != nil {
			t.Fatal(err)
		}
		if change > 0 && (newEnvKey == envKey || newPointKey == pointKey) {
			t.Fatal("changed shadow coordinates failed to invalidate a pixel digest")
		}
		envKey, pointKey = newEnvKey, newPointKey
		warm, err := shared.BakeEnvironment(t.Context(), envLayout, settings, options)
		if err != nil {
			t.Fatal(err)
		}
		cold, err := BakeEnvironment(t.Context(), environment, envLayout, materials, settings, options)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(warm.Image.Pix, cold.Image.Pix) || warm.Stats.Rays != cold.Stats.Rays || previous == shared.prepared {
			t.Fatal("changed shadow geometry reused stale preparation or changed cold/warm output")
		}
		previous = shared.prepared
		if _, err := shared.BakeEnvironment(t.Context(), envLayout, settings, options); err != nil || previous != shared.prepared {
			t.Fatalf("identical shadow geometry failed preparation reuse: %v", err)
		}
	}
}

func TestShadowGeometryBoundsBeforeBakeAllocation(t *testing.T) {
	t.Parallel()
	environment, envLayout, materials := environmentRoom(t, true, 0)
	point, pointLayout, pointMaterials := room(t, 0)
	cases := map[string][][3]world.Vec3{
		"too many":   make([][3]world.Vec3, MaxShadowTriangles+1),
		"degenerate": {{{}, {X: 1}, {X: 2}}},
	}
	for name, value := range map[string]float64{"nan": math.NaN(), "infinity": math.Inf(1), "coordinate": world.MaxCoordinate + 1} {
		points := shadowRoof()[:1]
		points[0][0].X = value
		cases[name] = points
	}
	for name, points := range cases {
		t.Run(name, func(t *testing.T) {
			options := Options{ShadowTriangles: points}
			if result, err := Bake(t.Context(), point, pointLayout, pointMaterials, options); err == nil || result.Image != nil {
				t.Fatal("point producer accepted invalid shadow geometry")
			}
			if result, err := BakeEnvironment(
				t.Context(),
				environment,
				envLayout,
				materials,
				world.EnvironmentSettings{},
				options,
			); err == nil ||
				result.Image != nil {
				t.Fatal("environment producer accepted invalid shadow geometry")
			}
			if _, err := BakeInputDigest(point, pointLayout, pointMaterials, options); err == nil {
				t.Fatal("point digest accepted invalid shadow geometry")
			}
			if _, err := EnvironmentInputDigest(environment, envLayout, materials, world.EnvironmentSettings{}, options); err == nil {
				t.Fatal("environment digest accepted invalid shadow geometry")
			}
		})
	}
	valid := make([][3]world.Vec3, MaxShadowTriangles)
	for i := range valid {
		valid[i] = [3]world.Vec3{{X: -world.MaxCoordinate}, {X: world.MaxCoordinate}, {Y: world.MaxCoordinate}}
	}
	if err := validateShadowTriangles(valid); err != nil {
		t.Fatalf("inclusive geometry/coordinate bounds rejected: %v", err)
	}
	options := Options{Samples: 1, Workers: 1, ShadowTriangles: valid}
	if _, err := EnvironmentInputDigest(environment, envLayout, materials, world.EnvironmentSettings{}, options); err != nil {
		t.Fatalf("maximum shadow geometry overflowed environment digest: %v", err)
	}
	if _, err := BakeInputDigest(point, pointLayout, pointMaterials, options); err != nil {
		t.Fatalf("maximum shadow geometry overflowed point digest: %v", err)
	}
}

func TestShadowGeometryNegativeSurfaceOccludesNearestAndAnyHit(t *testing.T) {
	t.Parallel()
	for _, absorbingFirst := range []bool{false, true} {
		triangles := []triangle{
			{a: vec{X: -1, Y: -1, Z: 1}, b: vec{X: 1, Y: -1, Z: 1}, c: vec{Y: 1, Z: 1}, surface: 0},
			{a: vec{X: -1, Y: -1, Z: 1}, b: vec{X: 1, Y: -1, Z: 1}, c: vec{Y: 1, Z: 1}, surface: -1},
		}
		if absorbingFirst {
			triangles[0], triangles[1] = triangles[1], triangles[0]
		}
		accel := accelerator{triangles: triangles}
		accel.prepare()
		hit, distance := accel.hit(vec{}, vec{Z: 1}, 2)
		if hit != -1 || distance != 1 || !accel.blocked(vec{}, vec{Z: 1}, 2) || accel.blocked(vec{}, vec{Z: 1}, 1) {
			t.Fatal("absorbing negative-surface tie, strict endpoint or any-hit visibility failed")
		}
	}
}
