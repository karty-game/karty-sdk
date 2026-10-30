// Tree geometry and complete-frame triangle workload fixtures.
package worldvegetation

import (
	"math"
	"testing"
)

func TestVegetationTreeNormalizedMesh(t *testing.T) {
	t.Parallel()

	vertices := builtInTreeVertices()
	if len(vertices) != treeTriangles*3*shrubVertexFloats {
		t.Fatal("unexpected shared tree topology", len(vertices))
	}

	canopy, stem := 0, 0

	for vertex := 0; vertex < len(vertices); vertex += shrubVertexFloats {
		if math.Hypot(float64(vertices[vertex]), float64(vertices[vertex+1])) > .5 {
			t.Fatal("tree left yaw-invariant radial bounds", vertex)
		}

		for axis := range 3 {
			low, high := float32(-.5), float32(.5)
			if axis == 2 {
				low, high = 0, 1
			}

			if vertices[vertex+axis] < low || vertices[vertex+axis] > high {
				t.Fatal("tree left normalized volume", vertex, axis)
			}
		}

		for _, attribute := range []int{3, 8, 11} {
			x, y, z := vertices[vertex+attribute], vertices[vertex+attribute+1], vertices[vertex+attribute+2]
			if math.Abs(float64(x*x+y*y+z*z)-1) > 1e-5 {
				t.Fatal("tree has invalid normal/tangent basis", vertex, attribute)
			}
		}

		if vertices[vertex+7] < .5 {
			canopy++

			if vertices[vertex+2] < .4 {
				t.Fatal("tree foliage obscured the lower trunk")
			}
		} else if vertices[vertex+2] < .4 {
			stem++
		}
	}

	if canopy == 0 || stem == 0 {
		t.Fatal("tree needs a leaf canopy and exposed branch geometry")
	}

}

func TestVegetationTreeLimbsHaveConnectedParentsAndTaper(t *testing.T) {
	t.Parallel()

	limbs := builtInTreeSkeleton()
	for index, limb := range limbs {
		if limb.base <= limb.tip || limb.tip <= 0 {
			t.Fatal("branch does not taper toward its leaves", index, limb)
		}

		if index == 0 {
			continue
		}

		connected := false

		for _, parent := range limbs[:index] {
			direction, offset := [3]float32{}, [3]float32{}
			length, along := float32(0), float32(0)

			for axis := range 3 {
				direction[axis] = parent.end[axis] - parent.start[axis]
				offset[axis] = limb.start[axis] - parent.start[axis]
				length += direction[axis] * direction[axis]
				along += offset[axis] * direction[axis]
			}

			fraction := along / length
			distance := float32(0)

			for axis := range 3 {
				delta := offset[axis] - direction[axis]*fraction
				distance += delta * delta
			}

			if fraction >= -1e-5 && fraction <= 1.00001 && distance < 1e-10 {
				connected = true

				break
			}
		}

		if !connected {
			t.Fatal("floating branch has no supporting trunk or limb", index, limb)
		}
	}
}

func TestVegetationTreeFoliageHasSupportingTipsAndCoversLeader(t *testing.T) {
	t.Parallel()

	limbs := builtInTreeSkeleton()
	// Every foliage card grows from one of those supporting limbs. Recover its base
	// midpoint from the actual mesh rather than asserting a construction order.
	vertices := builtInTreeVertices()
	leaderCovered := false

	for base := 0; base < len(vertices); base += 6 * shrubVertexFloats {
		if vertices[base+7] >= .5 {
			continue
		}

		connected := false

		for limbIndex, limb := range limbs {
			direction, offset := [3]float32{}, [3]float32{}
			length, projection := float32(0), float32(0)

			for axis := range 3 {
				midpoint := (vertices[base+axis] + vertices[base+shrubVertexFloats+axis]) * .5
				direction[axis] = limb.end[axis] - limb.start[axis]
				offset[axis] = midpoint - limb.start[axis]
				length += direction[axis] * direction[axis]
				projection += offset[axis] * direction[axis]
			}

			fraction := projection / length
			distance := float32(0)

			for axis := range 3 {
				delta := offset[axis] - fraction*direction[axis]
				distance += delta * delta
			}

			connected = connected || distance < 1e-10 && fraction >= -1e-5 && fraction <= 1.00001
			if limbIndex == 3 && distance < 1e-10 && vertices[base+2*shrubVertexFloats+2] > limb.end[2]+.08 {
				leaderCovered = true
			}
		}

		if !connected {
			t.Fatal("foliage has no supporting branch", base)
		}
	}

	if !leaderCovered {
		t.Fatal("tree leader protrudes above its foliage")
	}
}

func TestVegetationInclinedBranchRetainsRoundCrossSection(t *testing.T) {
	t.Parallel()

	for _, end := range [][3]float32{{.4, 0, .02}, {0, .4, .02}, {.15, -.21, .3}, {0, 0, .4}} {
		mesh := shrubMeshBuilder{}
		mesh.branch([3]float32{}, end, .03, .01, 0)

		for side := range 6 {
			base := side * 6 * shrubVertexFloats
			radius, projection := float32(0), float32(0)

			for axis := range 3 {
				coordinate := mesh.vertices[base+axis]
				radius += coordinate * coordinate
				projection += coordinate * end[axis]
			}

			if math.Abs(float64(radius-.03*.03)) > 1e-8 || math.Abs(float64(projection)) > 1e-8 {
				t.Fatal("angled branch flattened or its ring left the perpendicular plane", end, side, radius, projection)
			}
		}
	}
}
