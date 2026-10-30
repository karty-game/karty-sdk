package worldchunk

import (
	"errors"
	"math"
	"testing"

	"github.com/karty-game/karty-sdk/format/worldlightmap"
)

func TestEnclosureSpanLimitsGrowthAndWholeChunkCompaction(t *testing.T) {
	t.Parallel()
	for _, noLightmap := range []bool{true, false} {
		for _, maximum := range []float64{4, 8} {
			options := Options{EnclosureAware: true, MaxSpan: maximum, NoLightmap: noLightmap,
				CompactLightmaps: !noLightmap, Lightmap: worldlightmap.Options{PageSize: 512, TexelsPerUnit: 2}}
			plan, err := Plan(enclosureFixture(), options)
			if err != nil {
				t.Fatal(err)
			}
			want := 4
			if maximum == 8 {
				want = 1
			}
			if len(plan.Chunks) != want {
				t.Fatalf("mesh-only=%v span=%g: got %d chunks, want %d", noLightmap, maximum, len(plan.Chunks), want)
			}
			for _, chunk := range plan.Chunks {
				if chunk.Bounds != chunk.Mesh.Bounds || chunk.Bounds[3]-chunk.Bounds[0] > maximum ||
					chunk.Bounds[4]-chunk.Bounds[1] > maximum {
					t.Fatal("render span escaped planning/compaction limit", chunk.Bounds, chunk.Mesh.Bounds)
				}
			}
		}
	}
}

func TestSpanRejectsInvalidLimitAndOversizedSingleSector(t *testing.T) {
	t.Parallel()
	for _, maximum := range []float64{-1, math.NaN(), math.Inf(1), math.Inf(-1), 3} {
		for _, noLightmap := range []bool{true, false} {
			_, err := Plan(enclosureFixture(), Options{MaxSpan: maximum, NoLightmap: noLightmap,
				Lightmap: worldlightmap.Options{PageSize: 512, TexelsPerUnit: 2}})
			if !errors.Is(err, ErrChunk) {
				t.Fatalf("mesh-only=%v span=%g: accepted invalid limit/oversized sector: %v", noLightmap, maximum, err)
			}
		}
	}
}
