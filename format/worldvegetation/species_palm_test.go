// Compact immutable palm topology and admission fixtures.
package worldvegetation

import (
	"math"
	"testing"
)

func TestVegetationPalmConnectedFrondsBoundsAndBasis(t *testing.T) {
	t.Parallel()

	vertices := builtInPalmVertices()
	if len(vertices) != palmTriangles*3*shrubVertexFloats {
		t.Fatal("palm template triangle admission drifted", len(vertices))
	}

	leaf, wood := 0, 0

	for at := 0; at < len(vertices); at += shrubVertexFloats {
		vertex := vertices[at : at+shrubVertexFloats]
		if math.Hypot(float64(vertex[0]), float64(vertex[1])) > .5 || vertex[2] < 0 || vertex[2] > 1 {
			t.Fatal("palm exceeds yaw-invariant crown bounds", vertex[:3])
		}

		for _, start := range []int{3, 8, 11} {
			n := vertex[start : start+3]
			if math.Abs(float64(n[0]*n[0]+n[1]*n[1]+n[2]*n[2])-1) > 1e-5 {
				t.Fatal("palm normal/tangent is not unit length", start, n)
			}
		}

		if vertex[7] < .5 {
			leaf++

			if vertex[2] < .5 {
				t.Fatal("frond hangs implausibly far down the trunk")
			}
		} else {
			wood++
		}
	}

	if leaf != 14*6*4*3 || wood != 8*6*2*3 {
		t.Fatal("palm must retain exposed trunk and folded fronds", leaf, wood)
	}
	// Every frond's inner vertices share the leader's top, not floating nearby.
	base := palmLeader(1)

	const (
		woodFloats  = 8 * 6 * 2 * 3 * shrubVertexFloats
		frondFloats = 6 * 4 * 3 * shrubVertexFloats
	)
	for frond := range 14 {
		at := woodFloats + frond*frondFloats + shrubVertexFloats
		for axis := range 3 {
			if math.Abs(float64(vertices[at+axis]-base[axis])) > 1e-6 {
				t.Fatal("frond stem detached from leader", frond, axis)
			}
		}
	}

}
