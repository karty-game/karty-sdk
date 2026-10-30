//nolint:mnd // Built-in normalized shrub topology and fixed vertex layout.
package worldvegetation

import (
	"math"
)

// SpeciesVertexFloats is the position, normal, UV, tangent, bitangent and canopy-occlusion layout.
const SpeciesVertexFloats = 15

const shrubTriangles = 300
const treeTriangles = 588
const shrubVertexFloats = SpeciesVertexFloats

type shrubMeshBuilder struct{ vertices []float32 }

// Branch placement and canopy occlusion are prepared once for each built-in
// species. A golden-angle sequence breaks up regular rings without per-plant
// CPU work, extra triangles, or resources that authors must model themselves.
func builtInShrubVertices() []float32 {
	mesh := shrubMeshBuilder{vertices: make([]float32, 0, shrubTriangles*3*shrubVertexFloats)}
	mesh.branch([3]float32{0, 0, .001}, [3]float32{.025, -.013, .81}, .022, .006, 0)

	for cluster := range 16 {
		fraction := (float32(cluster) + .5) / 16
		height := .14 + fraction*.55 + .04*(speciesVariation(cluster, 1)-.5)
		angle := float64(cluster)*2.39996323 + float64(speciesVariation(cluster, 2))*.6
		radius := (.09 + .12*float32(math.Sin(float64(fraction)*math.Pi))) * (.8 + .2*speciesVariation(cluster, 3))
		center := [3]float32{radius*float32(math.Cos(angle)) + height*.025, radius*float32(math.Sin(angle)) - .013, height}
		mesh.branch([3]float32{height * .025, -.013, height - .13}, center, .013-.005*fraction, .003, cluster+1)

		for spray := range 3 {
			yaw := angle + float64(spray)*1.9 + float64(speciesVariation(cluster, spray+4))*.7
			tilt := -.25 + float64(speciesVariation(cluster, spray+8))*.87
			width := .23 + speciesVariation(cluster, spray+12)*.05
			rise := min(.19+speciesVariation(cluster, spray+16)*.055, .99-height)
			mesh.leafSpray(center, yaw, tilt, width, rise)
		}
	}

	return mesh.vertices
}

type treeLimb struct {
	start, end [3]float32
	base, tip  float32
}

// The bent leader supports six ascending scaffold limbs, each of which bends
// once and forks twice. A fork starts on its parent, rather than floating in a
// radial crown. These are immutable species coordinates, not live plant data.
func builtInTreeSkeleton() [28]treeLimb {
	var limbs [28]treeLimb

	leader := [5][3]float32{{0, 0, .002}, {.009, -.012, .25}, {-.012, -.016, .43}, {.016, .009, .62}, {.005, -.004, .83}}

	radii := [5]float32{.026, .022, .017, .011, .003}
	for segment := range 4 {
		limbs[segment] = treeLimb{leader[segment], leader[segment+1], radii[segment], radii[segment+1]}
	}

	for scaffold := range 6 {
		fraction := (float32(scaffold) + .35) / 6
		yaw := float64(scaffold)*2.39996323 + .45*float64(speciesVariation(scaffold, 2))
		height := .34 + fraction*.24

		parent := 1
		if height > leader[2][2] {
			parent = 2
		}

		along := (height - leader[parent][2]) / (leader[parent+1][2] - leader[parent][2])

		start := [3]float32{}
		for axis := range 3 {
			start[axis] = leader[parent][axis] + (leader[parent+1][axis]-leader[parent][axis])*along
		}
		// Lower limbs spread further; uneven yaw and rise leave readable gaps
		// between lobes instead of a regular umbrella or concentric comb.
		reach := .24 - fraction*.035 + .04*speciesVariation(scaffold, 3)
		knee := [3]float32{start[0] + float32(math.Cos(yaw))*reach*.53, start[1] + float32(math.Sin(yaw))*reach*.53, height + .13}
		tipYaw := yaw + .22*float64(speciesVariation(scaffold, 4)-.5)
		tip := [3]float32{
			start[0] + float32(math.Cos(tipYaw))*reach,
			start[1] + float32(math.Sin(tipYaw))*reach,
			height + .23 + .045*speciesVariation(scaffold, 5),
		}
		base := .015 - fraction*.004
		limbs[4+scaffold*2] = treeLimb{start, knee, base, base * .63}

		limbs[5+scaffold*2] = treeLimb{knee, tip, base * .63, .0035}
		for fork := range 2 {
			origin := knee
			if fork == 1 {
				origin = tip
			}

			forkYaw := yaw + float64(fork*2-1)*(.55+.2*float64(speciesVariation(scaffold, fork+7)))
			reach := .085 + .02*speciesVariation(scaffold, fork+9)
			end := [3]float32{
				origin[0] + float32(math.Cos(forkYaw))*reach,
				origin[1] + float32(math.Sin(forkYaw))*reach,
				origin[2] + .065 + .035*speciesVariation(scaffold, fork+11),
			}
			limbs[16+scaffold*2+fork] = treeLimb{origin, end, .0048 - float32(fork)*.0013, .0018}
		}
	}

	return limbs
}

// The same 588 triangles now describe a connected 28-segment skeleton and 126
// foliage sprays. Leaves follow terminal twigs and branch tips, so the visible
// wood explains the crown silhouette rather than radiating into arbitrary cards.
func builtInTreeVertices() []float32 {
	mesh := shrubMeshBuilder{vertices: make([]float32, 0, treeTriangles*3*shrubVertexFloats)}

	limbs := builtInTreeSkeleton()
	for index, limb := range limbs {
		mesh.branch(limb.start, limb.end, limb.base, limb.tip, index)
	}

	for scaffold := range 6 {
		for tip := range 3 {
			limb := limbs[5+scaffold*2]
			sprays := 9

			if tip != 0 {
				limb = limbs[15+scaffold*2+tip]
				sprays = 3 + tip
			}

			mesh.treeLeafSprays(limb, scaffold*3+tip, sprays)
		}
	}
	// Cover the leader as well as the outer forks: bare central wood and flat
	// peripheral leaf pads read as a bonsai rather than an overlapping crown.
	mesh.treeLeafSprays(limbs[3], 18, 18)

	return mesh.vertices
}

func (mesh *shrubMeshBuilder) treeLeafSprays(limb treeLimb, cluster, sprays int) {
	angle := math.Atan2(float64(limb.end[1]-limb.start[1]), float64(limb.end[0]-limb.start[0]))

	for spray := range sprays {
		// Grow sprays along the supporting twig as well as its end. Broad inner
		// sprays overlap the forks, forming a crown instead of isolated tip tufts.
		fraction := .18 + .82*speciesVariation(cluster, spray+80)
		if spray == 0 {
			fraction = 1
		}

		base := [3]float32{}
		for axis := range 3 {
			base[axis] = limb.start[axis] + fraction*(limb.end[axis]-limb.start[axis])
		}

		yaw := angle + float64(spray)*2.39996323 + .7*float64(speciesVariation(cluster, spray+16))
		available := .495 - float32(math.Hypot(float64(base[0]), float64(base[1])))
		width := min(.27+.055*speciesVariation(cluster, spray+48), available*1.8)
		rise := min(.20+.07*speciesVariation(cluster, spray+64), .995-base[2])
		maxTilt := math.Asin(float64(max(0, min(1, (available-width*.5)/rise))))
		tilt := max(-maxTilt, min(maxTilt, -.18+.66*float64(speciesVariation(cluster, spray+32))))
		mesh.leafSpray(base, yaw, tilt, width, rise)
	}
}

// This local hash is only used during immutable mesh preparation, never once
// per live plant or per frame. It does not affect GPU seed/ordinal root placement.
func speciesVariation(cluster, salt int) float32 {
	// #nosec G115 -- The deterministic procedural seed intentionally wraps modulo 2^32.
	value := uint32(cluster+1)*0x9e3779b9 ^ uint32(salt)*0x85ebca6b
	value ^= value >> 16
	value *= 0x7feb352d
	value ^= value >> 15
	value *= 0x846ca68b
	value ^= value >> 16

	return float32(value>>8) / 16777216
}

// A small analytic canopy model supplies stable low-frequency occlusion. Outer
// leaf tips receive more sky light, while inner branches and the base darken.
// It is an approximate species bake, not a runtime shadow or ray tracing pass.
func canopyOcclusion(position [3]float32, leaf bool) float32 {
	radius := float32(math.Sqrt(float64(position[0]*position[0] + position[1]*position[1])))
	if leaf {
		return min(.97, .47+min(radius/.35, 1)*.36+position[2]*.14)
	}

	return min(.9, .4+position[2]*.25+min(radius/.35, 1)*.25)
}

func (mesh *shrubMeshBuilder) leafSpray(base [3]float32, yaw, tilt float64, width, rise float32) {
	right := [3]float32{-float32(math.Sin(yaw)), float32(math.Cos(yaw)), 0}
	upward := [3]float32{float32(math.Cos(yaw) * math.Sin(tilt)), float32(math.Sin(yaw) * math.Sin(tilt)), float32(math.Cos(tilt))}

	positions := [4][3]float32{}
	for corner := range positions {
		across := float32(-.5)
		if corner == 1 || corner == 2 {
			across = .5
		}

		top := float32(0)
		if corner >= 2 {
			top = 1
		}

		for axis := range 3 {
			positions[corner][axis] = base[axis] + right[axis]*width*across + upward[axis]*rise*top
		}
	}

	coordinates := [4][2]float32{{.01, .495}, {.99, .495}, {.99, .005}, {.01, .005}}

	for _, corner := range [6]int{0, 1, 2, 0, 2, 3} {
		position := positions[corner]
		// Rounded crown normals soften leaf lighting while the branches retain
		// physical face normals. Silhouette and depth still use each actual plane.
		normal := [3]float32{position[0] * 2, position[1] * 2, .65 + (position[2]-.5)*.5}

		length := float32(math.Sqrt(float64(normal[0]*normal[0] + normal[1]*normal[1] + normal[2]*normal[2])))
		for axis := range normal {
			normal[axis] /= length
		}

		mesh.vertices = append(
			mesh.vertices,
			position[0], position[1], position[2],
			normal[0], normal[1], normal[2],
			coordinates[corner][0], coordinates[corner][1],
			right[0], right[1], right[2],
			-upward[0], -upward[1], -upward[2],
			canopyOcclusion(position, true),
		)
	}
}
func (mesh *shrubMeshBuilder) branch(start, end [3]float32, base, tip float32, index int) {
	// Rings must be perpendicular to each limb. Horizontal rings flatten an
	// inclined branch into a ribbon that disappears when viewed from its side.
	direction := [3]float32{end[0] - start[0], end[1] - start[1], end[2] - start[2]}

	length := float32(math.Sqrt(float64(direction[0]*direction[0] + direction[1]*direction[1] + direction[2]*direction[2])))
	for axis := range direction {
		direction[axis] /= length
	}

	right := [3]float32{-direction[1], direction[0], 0}
	if math.Abs(float64(direction[2])) > .9 {
		right = [3]float32{direction[2], 0, -direction[0]}
	}

	length = float32(math.Sqrt(float64(right[0]*right[0] + right[1]*right[1] + right[2]*right[2])))
	for axis := range right {
		right[axis] /= length
	}

	radialUp := [3]float32{
		direction[1]*right[2] - direction[2]*right[1],
		direction[2]*right[0] - direction[0]*right[2],
		direction[0]*right[1] - direction[1]*right[0],
	}

	for side := range 6 {
		angle, next := float64(side)*math.Pi/3, float64(side+1)*math.Pi/3

		first, second, third, fourth := start, start, end, end
		for axis := range 3 {
			first[axis] += base * (right[axis]*float32(math.Cos(angle)) + radialUp[axis]*float32(math.Sin(angle)))
			second[axis] += base * (right[axis]*float32(math.Cos(next)) + radialUp[axis]*float32(math.Sin(next)))
			third[axis] += tip * (right[axis]*float32(math.Cos(angle)) + radialUp[axis]*float32(math.Sin(angle)))
			fourth[axis] += tip * (right[axis]*float32(math.Cos(next)) + radialUp[axis]*float32(math.Sin(next)))
		}

		mesh.triangle(first, second, fourth, index, true)
		mesh.triangle(first, fourth, third, index, true)
	}
}
func (mesh *shrubMeshBuilder) triangle(first, second, third [3]float32, index int, bark bool) {
	edge := [3]float32{second[0] - first[0], second[1] - first[1], second[2] - first[2]}
	diagonal := [3]float32{third[0] - first[0], third[1] - first[1], third[2] - first[2]}
	normal := [3]float32{
		edge[1]*diagonal[2] - edge[2]*diagonal[1],
		edge[2]*diagonal[0] - edge[0]*diagonal[2],
		edge[0]*diagonal[1] - edge[1]*diagonal[0],
	}

	length := float32(math.Sqrt(float64(normal[0]*normal[0] + normal[1]*normal[1] + normal[2]*normal[2])))
	for component := range normal {
		normal[component] /= length
	}

	tangent := edge

	tangentLength := float32(math.Sqrt(float64(edge[0]*edge[0] + edge[1]*edge[1] + edge[2]*edge[2])))
	for axis := range tangent {
		tangent[axis] /= tangentLength
	}

	bitangent := [3]float32{
		normal[1]*tangent[2] - normal[2]*tangent[1],
		normal[2]*tangent[0] - normal[0]*tangent[2],
		normal[0]*tangent[1] - normal[1]*tangent[0],
	}

	materialU, materialV := .125+float32(index%4)*.2, float32(.125+float32(index%3)*.08)
	if bark {
		materialV += .5
	}

	for _, position := range [3][3]float32{first, second, third} {
		mesh.vertices = append(mesh.vertices,
			position[0], position[1], position[2],
			normal[0], normal[1], normal[2], materialU, materialV,
			tangent[0], tangent[1], tangent[2], bitangent[0], bitangent[1], bitangent[2],
			canopyOcclusion(position, false),
		)
	}
}

// SpeciesVertices returns a fresh immutable rest-pose shrub, tree or palm mesh.
// Each triangle has three unindexed vertices of SpeciesVertexFloats floats.
// Crossed cards and unknown geometry values return nil.
func SpeciesVertices(geometry uint8) []float32 {
	switch geometry {
	case GeometryShrub:
		return builtInShrubVertices()
	case GeometryTree:
		return builtInTreeVertices()
	case GeometryPalm:
		return builtInPalmVertices()
	default:
		return nil
	}
}
