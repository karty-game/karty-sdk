package worldlightmapbake

import (
	"context"
	"fmt"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
	"testing"
)

func BenchmarkBake(b *testing.B) {
	d, _, materials := room(b, 0)
	for _, size := range []int{512, 1024} {
		layout, err := worldlightmap.Compile(d, worldlightmap.Options{TexelsPerUnit: 4, PageSize: size, Padding: 2, Lights: []string{"white"}, ShadowSize: 32})
		if err != nil {
			b.Fatal(err)
		}
		for _, bounces := range []int{0, 2} {
			b.Run(fmt.Sprintf("Page%dBounces%d", size, bounces), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					result, err := Bake(context.Background(), d, layout, materials, Options{Workers: 1, Samples: 16, Bounces: bounces, Seed: 22})
					if err != nil {
						b.Fatal(err)
					}
					b.ReportMetric(float64(result.Stats.ReceiverTexels), "receivers/op")
				}
			})
		}
	}
}
