package worldlightmap

import (
	"github.com/karty-game/karty-sdk/format/world"
	"testing"
)

func TestCapConnectivityRetainsOriginPlaneTolerance(t *testing.T) {
	d := chainDocument(3)
	d.Sectors[1].Floor.C = .75e-9
	d.Sectors[2].Floor.C = 1.5e-9
	if err := world.Validate(&d); err != nil {
		t.Fatal(err)
	}
	fromFirst := connectedCaps(&d, 0, "sector-floor")
	if !fromFirst[0] || !fromFirst[1] || fromFirst[2] {
		t.Fatalf("origin plane tolerance relaxed: %v", fromFirst)
	}
	fromMiddle := connectedCaps(&d, 1, "sector-floor")
	if !fromMiddle[0] || !fromMiddle[1] || !fromMiddle[2] {
		t.Fatalf("origin-specific connectivity: %v", fromMiddle)
	}
	// Pairwise near-equal planes do not justify a transitive merged chart.
	if _, err := Compile(d, Options{TexelsPerUnit: 1}); err == nil {
		t.Fatal("compiler accepted a merged chart outside its origin tolerance")
	}
	d.Sectors[2].Floor.C = .5e-9
	if _, err := Compile(d, Options{TexelsPerUnit: 1}); err != nil {
		t.Fatalf("valid connected caps: %v", err)
	}
}
