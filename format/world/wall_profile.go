package world

import (
	"math"
	"slices"
)

// WallProfile divides a wall where any of its four linear elevation functions
// cross. Heights 0/1 are source floor/ceiling; 2/3 are destination elevations
// already transformed into the source coordinate frame. It contains no mesh or
// renderer state and can be used by public builders and private hosts alike.
type WallProfile struct {
	Heights [4][2]float64
	Cuts    [8]float64
	Count   int
}

// NewWallProfile requires finite endpoint heights and a finite positive
// tolerance. The four pairs have at most six interior crossings.
func NewWallProfile(heights [4][2]float64, tolerance float64) (WallProfile, error) {
	p := WallProfile{Heights: heights, Cuts: [8]float64{0, 1}, Count: 2}
	if math.IsNaN(tolerance) || math.IsInf(tolerance, 0) || tolerance <= 0 {
		return WallProfile{}, ErrGeometry
	}
	for _, endpoints := range heights {
		for _, z := range endpoints {
			if math.IsNaN(z) || math.IsInf(z, 0) {
				return WallProfile{}, ErrGeometry
			}
		}
	}
	for i := range 4 {
		for j := i + 1; j < 4; j++ {
			a, b := heights[i][0]-heights[j][0], heights[i][1]-heights[j][1]
			if (a < 0 && b > 0) || (a > 0 && b < 0) {
				t := a / (a - b)
				duplicate := false
				for _, existing := range p.Cuts[:p.Count] {
					duplicate = duplicate || math.Abs(t-existing) <= tolerance
				}
				if !duplicate {
					p.Cuts[p.Count] = t
					p.Count++
				}
			}
		}
	}
	slices.Sort(p.Cuts[:p.Count])
	return p, nil
}

// Height evaluates one linear elevation at an edge fraction.
func (p WallProfile) Height(kind int, t float64) float64 {
	return p.Heights[kind][0] + (p.Heights[kind][1]-p.Heights[kind][0])*t
}

// Controls returns the maximum floor and minimum ceiling at an edge fraction.
func (p WallProfile) Controls(t float64) (int, int) {
	low, high := 0, 1
	if p.Height(2, t) > p.Height(0, t) {
		low = 2
	}
	if p.Height(3, t) < p.Height(1, t) {
		high = 3
	}
	return low, high
}
