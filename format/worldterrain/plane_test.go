package worldterrain

import (
	"math"
	"testing"
)

func TestPlaneAtMatchesRenderedTriangleAndHoles(t *testing.T) {
	document := fixture()
	for _, xy := range [][2]float64{{.1, .1}, {.8, .8}, {1.2, .2}, {1.8, .8}, {1, 1}, {2, 2}} {
		plane, ok := PlaneAt(&document, xy[0], xy[1])
		height, expected := HeightAt(&document, xy[0], xy[1])
		if ok != expected || !ok || math.Abs(plane.A*xy[0]+plane.B*xy[1]+plane.C-height) > 1e-10 {
			t.Fatal("plane disagrees with rendered triangle", xy, plane, height)
		}
	}
	document.Holes[0] = 1
	if _, ok := PlaneAt(&document, .1, .1); ok {
		t.Fatal("plane covers terrain hole")
	}
}
