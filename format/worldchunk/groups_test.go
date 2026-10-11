package worldchunk

import (
	"bytes"
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
)

func TestAuthoredGroupsPreserveDefaultAndDeterministicIdentity(t *testing.T) {
	document := fixture(t)
	baseline, err := Plan(document, Options{NoLightmap: true})
	if err != nil {
		t.Fatal(err)
	}
	same, err := Plan(document, Options{NoLightmap: true, SectorGroups: []int{255, 255}})
	if err != nil {
		t.Fatal(err)
	}
	before, err := json.Marshal(baseline)
	if err != nil {
		t.Fatal(err)
	}
	after, err := json.Marshal(same)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("one authored group changed budget-only output")
	}
	groups := []int{5, 8}
	options := Options{NoLightmap: true, SectorGroups: groups}
	first, err := Plan(document, options)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Plan(document, options)
	if err != nil {
		t.Fatal(err)
	}
	before, err = json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	after, err = json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) || !slices.Equal(groups, []int{5, 8}) || len(first.Chunks) != 2 ||
		!slices.Equal(first.SectorChunks, []int{0, 1}) {
		t.Fatal("authored group identity or determinism lost")
	}
	for i, chunk := range first.Chunks {
		if !slices.Equal(chunk.Sectors, []int{i}) {
			t.Fatal("global sector identities changed")
		}
		if !slices.Equal(first.Candidates[i], []int{0, 1}) {
			t.Fatal("cross-area visibility omitted")
		}
	}
}

func TestAuthoredGroupsCannotOverrideHardBudgetsOrTransformedEdges(t *testing.T) {
	document := fixture(t)
	individual, err := Plan(document, Options{NoLightmap: true, MaxSectors: 1})
	if err != nil {
		t.Fatal(err)
	}
	budget := max(individual.Chunks[0].MeshBytes, individual.Chunks[1].MeshBytes)
	for name, options := range map[string]Options{
		"mesh budget":  {NoLightmap: true, MaxMeshBytes: budget, SectorGroups: []int{0, 0}},
		"sector guard": {NoLightmap: true, MaxSectors: 1, SectorGroups: []int{0, 0}},
	} {
		t.Run(name, func(t *testing.T) {
			plan, err := Plan(document, options)
			if err != nil {
				t.Fatal(err)
			}
			if len(plan.Chunks) != 2 {
				t.Fatal("authored group bypassed a hard budget")
			}
		})
	}
	for i := range document.Sectors[1].Walls {
		document.Sectors[1].Walls[i].Start.X += 100
		document.Sectors[1].Walls[i].End.X += 100
	}
	document.Sectors[1].Floor.C -= document.Sectors[1].Floor.A * 100
	document.Sectors[1].Ceiling.C -= document.Sectors[1].Ceiling.A * 100
	document.Contents[0].Position.X += 100
	plan, err := Plan(document, Options{NoLightmap: true, SectorGroups: []int{0, 0}})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Chunks) != 2 {
		t.Fatal("authored group grew across transformed portal")
	}
	for _, set := range plan.Candidates {
		if !slices.Equal(set, []int{0, 1}) {
			t.Fatal("authored group removed transformed visibility")
		}
	}
}

func TestAuthoredGroupInputsValidateEveryGlobalSector(t *testing.T) {
	document := fixture(t)
	for name, groups := range map[string][]int{
		"empty": {}, "short": {0}, "long": {0, 1, 2}, "negative final": {0, -1}, "oversized final": {0, world.MaxSectors},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Plan(document, Options{NoLightmap: true, SectorGroups: groups}); !errors.Is(err, ErrChunk) {
				t.Fatalf("invalid group map accepted: %v", err)
			}
		})
	}
}

func TestAuthoredAreaBoundaryPreservesNeighborLightingContext(t *testing.T) {
	document := fixture(t)
	document.Lighting = &world.Lighting{
		Version: 1,
		Lights: []world.PointLight{
			{ID: "neighbor", Position: world.Vec3{X: 6, Y: 2, Z: 2}, Color: world.Vec3{X: 1, Y: 1, Z: 1}, Radius: 10},
		},
	}
	plan, err := Plan(
		document,
		Options{SectorGroups: []int{0, 1}, Lightmap: worldlightmap.Options{PageSize: 512, TexelsPerUnit: 2, Lights: []string{"neighbor"}}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Chunks) != 2 {
		t.Fatal("authored boundary did not preserve two areas")
	}
	for _, chunk := range plan.Chunks {
		component, _, err := worldlightmap.ReceiverScope(&document, chunk.Layout.Receivers)
		if err != nil {
			t.Fatal(err)
		}
		if !component[0] || !component[1] || !slices.Equal(chunk.Layout.RuntimeBake.LightIDs, []string{"neighbor"}) {
			t.Fatal("authored boundary became a lighting boundary")
		}
	}
}
