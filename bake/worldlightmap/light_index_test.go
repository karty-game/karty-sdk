package worldlightmapbake

import (
	"fmt"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
)

func sparseSourceScene(count int) scene {
	scene := scene{lights: make([]preparedLight, count)}
	for i := range scene.lights {
		position := vec{X: float64(i%37) * 8, Y: float64(i/37) * 8, Z: 2}
		// Reverse discovery order to exercise ordered accumulation independently
		// of spatial BVH order.
		scene.lights[count-i-1] = preparedLight{
			PointLight:    world.PointLight{Position: position, Radius: 5, Color: vec{X: float64(i%11) / 10, Y: .3, Z: .8}},
			radiusSquared: 25,
		}
	}
	scene.lightIndex.prepare(scene.lights)
	return scene
}

func TestInfluenceBVHPreservesOrderedDirectAndBounceLighting(t *testing.T) {
	indexed := sparseSourceScene(worldlightmap.MaxBakeLights)
	linear := indexed
	linear.lightIndex = lightIndex{}
	chart := worldlightmap.Chart{Normal: vec{Z: 1}, Tangent: vec{X: 1}, Bitangent: vec{Y: 1}}
	for i := range 200 {
		position := vec{X: float64(i%37) * 8, Y: float64(i%113) * 8, Z: float64(i%7) * .25}
		var indexedCounts, linearCounts counters
		got := indexed.direct(position, chart, &indexedCounts)
		want := linear.direct(position, chart, &linearCounts)
		if got != want || indexedCounts != linearCounts {
			t.Fatalf("direct accumulation or rays changed at %v", position)
		}
		if got, want := indexed.lambertDirect(
			position,
			chart.Normal,
			&indexedCounts,
		), linear.lambertDirect(
			position,
			chart.Normal,
			&linearCounts,
		); got != want ||
			indexedCounts != linearCounts {
			t.Fatalf("bounce-hit illumination changed at %v", position)
		}
	}
	iterator := indexed.lightIndex.query(vec{}, len(indexed.lights))
	count, previous := 0, -1
	for source := iterator.next(); source >= 0; source = iterator.next() {
		if source <= previous {
			t.Fatal("candidate order differs from recipe order")
		}
		previous = source
		count++
	}
	if count > 4 {
		t.Fatalf("sparse query retained %d of 4096 source candidates", count)
	}
	if allocations := testing.AllocsPerRun(100, func() {
		iterator := indexed.lightIndex.query(vec{}, len(indexed.lights))
		for iterator.next() >= 0 {
		}
	}); allocations != 0 {
		t.Fatalf("influence query allocated %g times", allocations)
	}
}

func BenchmarkSparseSourceLighting(b *testing.B) {
	chart := worldlightmap.Chart{Normal: vec{Z: 1}, Tangent: vec{X: 1}, Bitangent: vec{Y: 1}}
	for _, count := range []int{1000, worldlightmap.MaxBakeLights} {
		indexed := sparseSourceScene(count)
		linear := indexed
		linear.lightIndex = lightIndex{}
		for _, variant := range []struct {
			name  string
			scene scene
		}{{"linear", linear}, {"influence-bvh", indexed}} {
			b.Run(fmt.Sprintf("Sources%d/%s", count, variant.name), func(b *testing.B) {
				b.ReportAllocs()
				var counts counters
				for b.Loop() {
					variant.scene.direct(vec{}, chart, &counts)
					variant.scene.lambertDirect(vec{}, chart.Normal, &counts)
				}
			})
		}
	}
}
