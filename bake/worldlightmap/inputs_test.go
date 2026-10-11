package worldlightmapbake

import (
	"bytes"
	"context"
	"fmt"
	"image/color"
	"math"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
)

func boundedRooms(t testing.TB) (world.Document, worldlightmap.Layout, []Material) {
	t.Helper()
	d, _, materials := room(t, 0)
	base := d.Sectors[0]
	d.Sectors = nil
	for i := range 8 {
		s := base
		s.ID = fmt.Sprintf("room-%d", i)
		s.Walls = append([]world.Wall(nil), base.Walls...)
		for j := range s.Walls {
			s.Walls[j].Start.X += float64(4 * i)
			s.Walls[j].End.X += float64(4 * i)
		}
		if i > 0 {
			s.Walls[3].Portal, s.Walls[3].PortalWall = int32(i-1), 2
		}
		if i < 7 {
			s.Walls[1].Portal, s.Walls[1].PortalWall = int32(i+1), 4
		}
		d.Sectors = append(d.Sectors, s)
	}
	d.Sectors[7].CeilingMaterial = 3
	d.Lighting.Lights = append(d.Lighting.Lights,
		world.PointLight{ID: "far", Position: vec{X: 30, Y: 2, Z: 1}, Color: vec{X: 1, Y: 1, Z: 1}, Radius: 20})
	materials = append(materials, Material{ID: 3, Albedo: solid(color.NRGBA{G: 255, A: 255})})
	return d, boundedLayout(t, d, 0), materials
}

func boundedLayout(t testing.TB, d world.Document, receiver int) worldlightmap.Layout {
	t.Helper()
	l, err := worldlightmap.CompileReceivers(d,
		worldlightmap.Options{TexelsPerUnit: 1, PageSize: 512, Padding: 2, AllStatic: true}, []int{receiver}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestBoundedInputDigestMatchesPixelLocalityAndFreshProvenance(t *testing.T) {
	d, l, materials := boundedRooms(t)
	options := Options{Samples: 4, Bounces: 1, Seed: 9, Workers: 1, MaxRayDistance: 2, Denoise: "low"}
	beforeKey, err := BakeInputDigest(d, l, materials, options)
	if err != nil {
		t.Fatal(err)
	}
	before, err := Bake(context.Background(), d, l, materials, options)
	if err != nil {
		t.Fatal(err)
	}
	if expected, err := ReflectanceDigestForLayout(d, l, materials, options); err != nil || expected != before.ReflectanceSHA256 {
		t.Fatalf("provenance does not match producer: %s %v", expected, err)
	}
	// Present content decides reuse even when many remote geometry, source and
	// asset edits have occurred since the cached bake. No Git snapshot is used.
	d.Sectors[7].Ceiling.C = 4
	d.Lighting.Lights[1].Color = vec{Y: .5}
	materials[2].Albedo = solid(color.NRGBA{B: 255, A: 255})
	remote := boundedLayout(t, d, 0)
	remoteKey, err := BakeInputDigest(d, remote, materials, options)
	if err != nil || remoteKey != beforeKey || remote.GeometrySHA256 == l.GeometrySHA256 {
		t.Fatalf("remote edit invalidated pixel cache or failed global binding: %s %v", remoteKey, err)
	}
	after, err := Bake(context.Background(), d, remote, materials, options)
	if err != nil || !bytes.Equal(before.Image.Pix, after.Image.Pix) || before.RGBMRange != after.RGBMRange {
		t.Fatalf("local digest did not identify equivalent actual bake: %v", err)
	}
	// Inserting an unrelated sector shifts every global binding without changing
	// local chart packing or pixels. Fresh artifact provenance must still rebind.
	extra := d.Sectors[7]
	extra.ID = "unrelated-first"
	extra.Walls = append([]world.Wall(nil), extra.Walls...)
	for i := range extra.Walls {
		extra.Walls[i].Start.X += 100
		extra.Walls[i].End.X += 100
		extra.Walls[i].Portal, extra.Walls[i].PortalWall = -1, 0
	}
	for i := range d.Sectors {
		for j := range d.Sectors[i].Walls {
			if d.Sectors[i].Walls[j].Portal >= 0 {
				d.Sectors[i].Walls[j].Portal++
			}
		}
	}
	d.Sectors = append([]world.Sector{extra}, d.Sectors...)
	shifted := boundedLayout(t, d, 1)
	shiftedKey, err := BakeInputDigest(d, shifted, materials, options)
	if err != nil || shiftedKey != beforeKey {
		t.Fatalf("remote index shift invalidated unchanged pixels: %s %v", shiftedKey, err)
	}
	shiftedBake, err := Bake(context.Background(), d, shifted, materials, options)
	if err != nil || !bytes.Equal(before.Image.Pix, shiftedBake.Image.Pix) || before.ReflectanceSHA256 == shiftedBake.ReflectanceSHA256 {
		t.Fatalf("shifted reuse or fresh provenance failed: %v", err)
	}
	// A nearby source edit changes both the recomputed key and actual atlas.
	d.Lighting.Lights[0].Color.X = .25
	localKey, err := BakeInputDigest(d, shifted, materials, options)
	if err != nil || localKey == beforeKey {
		t.Fatalf("local source edit kept stale key: %s %v", localKey, err)
	}
	localBake, err := Bake(context.Background(), d, shifted, materials, options)
	if err != nil || bytes.Equal(before.Image.Pix, localBake.Image.Pix) {
		t.Fatalf("local source edit did not change pixels: %v", err)
	}
}

func TestBakeInputDigestBindsTransportReceiverMaterialAndNeighborProfile(t *testing.T) {
	d, l, materials := boundedRooms(t)
	options := Options{Samples: 4, Bounces: 1, Seed: 9, Workers: 1, MaxRayDistance: 2}
	key, err := BakeInputDigest(d, l, materials, options)
	if err != nil {
		t.Fatal(err)
	}
	for name, changed := range map[string]Options{
		"samples":  {Samples: 5, Bounces: 1, Seed: 9, MaxRayDistance: 2},
		"bounces":  {Samples: 4, Bounces: 2, Seed: 9, MaxRayDistance: 2},
		"seed":     {Samples: 4, Bounces: 1, Seed: 10, MaxRayDistance: 2},
		"distance": {Samples: 4, Bounces: 1, Seed: 9, MaxRayDistance: 3},
		"filter":   {Samples: 4, Bounces: 1, Seed: 9, MaxRayDistance: 2, Denoise: "low"},
	} {
		t.Run(name, func(t *testing.T) {
			other, err := BakeInputDigest(d, l, materials, changed)
			if err != nil || other == key {
				t.Fatalf("transport input not bound: %s %v", other, err)
			}
		})
	}
	options.Workers = 2
	options.Denoise = "off"
	if other, err := BakeInputDigest(d, l, materials, options); err != nil || other != key {
		t.Fatalf("worker/default filter altered key: %s %v", other, err)
	}
	materials[1].Albedo = solid(color.NRGBA{B: 255, A: 255})
	if other, err := BakeInputDigest(d, l, materials, options); err != nil || other == key {
		t.Fatalf("local decoded reflectance not bound: %s %v", other, err)
	}
	_, _, materials = boundedRooms(t)
	d.Sectors[1].Floor.C = .5
	neighbor := boundedLayout(t, d, 0)
	if other, err := BakeInputDigest(d, neighbor, materials, options); err != nil || other == key {
		t.Fatalf("derived portal wall profile not bound: %s %v", other, err)
	}
}

func TestBoundedBakeClipsDirectAndBounceSegments(t *testing.T) {
	d, l, materials := room(t, 1.5)
	options := Options{Samples: 64, Bounces: 1, Workers: 1, Seed: 17}
	unbounded, err := Bake(context.Background(), d, l, materials, options)
	if err != nil {
		t.Fatal(err)
	}
	options.MaxRayDistance = .9
	bounded, err := Bake(context.Background(), d, l, materials, options)
	if err != nil {
		t.Fatal(err)
	}
	x, y := floorAt(l, vec{X: 3, Y: 2})
	if decoded(unbounded, x, y).X < .01 || maximum(decoded(bounded, x, y)) != 0 {
		t.Fatal("bounce transport exceeded its segment distance")
	}
	x, y = floorAt(l, vec{X: 1, Y: 2})
	if maximum(decoded(unbounded, x, y)) < .1 || maximum(decoded(bounded, x, y)) != 0 || bounded.Image.NRGBAAt(x, y).A == 0 {
		t.Fatal("direct source exceeded distance or covered black became uncovered")
	}
	options.MaxRayDistance = 100
	large, err := Bake(context.Background(), d, l, materials, options)
	if err != nil || !bytes.Equal(unbounded.Image.Pix, large.Image.Pix) || unbounded.RGBMRange != large.RGBMRange {
		t.Fatalf("nonbinding limit changed transport: %v", err)
	}
}

func TestBoundedBakeRejectsInvalidDistancesBeforeWork(t *testing.T) {
	d, l, materials := room(t, 0)
	for _, distance := range []float64{-1, math.NaN(), math.Inf(1), math.Inf(-1), worldlightmap.MaxBakeRayDistance + 1} {
		options := Options{Workers: 1, MaxRayDistance: distance}
		if _, err := Bake(context.Background(), d, l, materials, options); err == nil {
			t.Fatalf("invalid distance accepted: %v", distance)
		}
		if _, err := BakeInputDigest(d, l, materials, options); err == nil {
			t.Fatalf("invalid distance cached: %v", distance)
		}
	}
}
