package worldlightmap

import (
	"bytes"
	"math"
	"slices"
	"testing"
)

func TestSelectedGeometryCellsRequireCompleteDisjointCoverage(t *testing.T) {
	t.Parallel()
	whole := [4]float64{0, 0, 4, 4}
	valid := SurfaceSelection{Clip: &whole, LiquidEdgeStep: 1,
		LiquidTiles: []LiquidTile{{Clip: [4]float64{0, 0, 2, 4}}, {Clip: [4]float64{2, 0, 4, 4}, LiquidMaxSubdivisions: 1}}}
	admission := LayoutAdmission{}
	if err := ValidateSurfaceSelection(&valid, admission); err != nil {
		t.Fatal(err)
	}
	encoded, err := EncodeSurfaceSelection(valid)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeSurfaceSelection(encoded)
	if err != nil || !EqualSurfaceSelections(&valid, &decoded) {
		t.Fatal("lost independent geometry cell policy", err)
	}
	again, err := EncodeSurfaceSelection(decoded)
	if err != nil || !bytes.Equal(encoded, again) || string(encoded[:4]) != "KLS2" {
		t.Fatal("noncanonical geometry-cell envelope", err)
	}
	for _, change := range []func(*SurfaceSelection){
		func(s *SurfaceSelection) { s.Clip = nil },
		func(s *SurfaceSelection) { s.LiquidTiles = s.LiquidTiles[:1] },
		func(s *SurfaceSelection) { s.LiquidTiles[1].Clip[0] = 1 },
		func(s *SurfaceSelection) { s.LiquidTiles[1].Clip[2] = 5 },
		func(s *SurfaceSelection) { s.LiquidTiles[1].Clip[0] = math.NaN() },
		func(s *SurfaceSelection) { s.LiquidTiles[1].LiquidMaxSubdivisions = 33 },
		func(s *SurfaceSelection) { s.LiquidTiles[1].Clip[0] = 2.5 },
		func(s *SurfaceSelection) { s.LiquidTiles = make([]LiquidTile, MaxLiquidTiles+1) },
	} {
		broken := valid
		broken.LiquidTiles = slices.Clone(valid.LiquidTiles)
		change(&broken)
		if err := ValidateSurfaceSelection(&broken, admission); err == nil {
			t.Fatal("invalid geometry-cell coverage accepted", broken)
		}
	}
}
