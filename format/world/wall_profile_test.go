package world

import (
	"math"
	"testing"
)

func TestWallProfileCrossingControllerIntervals(t *testing.T) {
	p, err := NewWallProfile([4][2]float64{{0, 0}, {4, 4}, {-1, 1}, {5, 3}}, 1e-9)
	if err != nil || p.Count != 3 || p.Cuts[1] != .5 {
		t.Fatalf("profile=%+v error=%v", p, err)
	}
	if low, high := p.Controls(.25); low != 0 || high != 1 {
		t.Fatalf("before crossing: %d,%d", low, high)
	}
	if low, high := p.Controls(.75); low != 2 || high != 3 {
		t.Fatalf("after crossing: %d,%d", low, high)
	}
	if p.Height(2, .75) != .5 || p.Height(3, .75) != 3.5 {
		t.Fatal("incorrect endpoint interpolation")
	}
}

func TestWallProfileFullOpeningAndInvalidInput(t *testing.T) {
	p, err := NewWallProfile([4][2]float64{{0, 0}, {4, 4}, {0, 0}, {4, 4}}, 1e-9)
	if err != nil || p.Count != 2 {
		t.Fatalf("profile=%+v error=%v", p, err)
	}
	for _, tolerance := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		if _, err := NewWallProfile(p.Heights, tolerance); err == nil {
			t.Fatal("accepted invalid tolerance")
		}
	}
	p.Heights[3][1] = math.Inf(1)
	if _, err := NewWallProfile(p.Heights, 1e-9); err == nil {
		t.Fatal("accepted nonfinite final height")
	}
}
