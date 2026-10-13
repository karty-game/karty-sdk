package worldchunk

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
)

func TestBalancedSolidOwnersPreservesDensityAndCompleteCoverage(t *testing.T) {
	t.Parallel()
	document := fixture(t)
	document.StaticSolids = &world.StaticSolids{Version: 1}
	for i := range 10 {
		document.StaticSolids.Items = append(document.StaticSolids.Items, world.Solid{ID: fmt.Sprintf("overlapping-%d", i),
			Footprint: []world.Vec2{{X: 3, Y: 0}, {X: 5, Y: 0}, {X: 5, Y: 4}, {X: 3, Y: 4}},
			Bottom:    world.Plane{C: 1 + float64(i)*.1}, Top: world.Plane{C: 1.02 + float64(i)*.1},
			SideMaterial: 1, TopMaterial: 1, BottomMaterial: 1})
	}
	options := Options{MaxSectors: 1, Lightmap: worldlightmap.Options{PageSize: 1024, TexelsPerUnit: 64, Padding: 4}}
	if _, err := Plan(document, options); !errors.Is(err, worldlightmap.ErrPageCapacity) {
		t.Fatalf("default ownership changed: %v", err)
	}
	owners, err := BalancedSolidOwners(document, options.Lightmap)
	if err != nil {
		t.Fatal(err)
	}
	options.SolidOwners = owners
	plan, err := Plan(document, options)
	if err != nil {
		t.Fatal(err)
	}
	seen := make([]bool, len(owners))
	for _, chunk := range plan.Chunks {
		if chunk.Layout.TexelsPerUnit != 64 {
			t.Fatal("density silently changed")
		}
		for _, solid := range chunk.Solids {
			if seen[solid] {
				t.Fatal("duplicate solid ownership")
			}
			seen[solid] = true
		}
	}
	if slices.Contains(seen, false) {
		t.Fatal("solid lost during balancing")
	}
	second, err := BalancedSolidOwners(document, options.Lightmap)
	if err != nil || !slices.Equal(second, owners) {
		t.Fatalf("owner policy is nondeterministic: %v", err)
	}
	for _, invalid := range [][]int{owners[:len(owners)-1], append([]int{-1}, owners[1:]...), append([]int{len(document.Sectors)}, owners[1:]...)} {
		options.SolidOwners = invalid
		if _, err := Plan(document, options); err == nil {
			t.Fatal("incomplete or invalid owner override accepted")
		}
	}
	document.StaticSolids.Items[0].Footprint = []world.Vec2{{X: .5, Y: .5}, {X: 1.5, Y: .5}, {X: 1.5, Y: 1.5}, {X: .5, Y: 1.5}}
	options.SolidOwners = slices.Clone(owners)
	options.SolidOwners[0] = 1
	if _, err := Plan(document, options); err == nil {
		t.Fatal("nonintersecting owner accepted")
	}
}
