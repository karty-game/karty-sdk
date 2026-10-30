package worldlightmapbake

import (
	"context"
	"image"
	"image/color"
	"math"

	"github.com/karty-game/karty-sdk/format/worldlightmap"
	"github.com/karty-game/karty-sdk/format/worldvegetation"
)

type vegetationReflectance struct {
	image        *image.NRGBA
	origin, u, v vec
	uv           [2]float64
	leaf         bool
}

func (r vegetationReflectance) pixel(p vec) color.NRGBA {
	relative := sub(p, r.origin)
	u, v := r.uv[0]+dot(relative, r.u), r.uv[1]+dot(relative, r.v)
	bounds := r.image.Rect
	x := bounds.Min.X + min(bounds.Dx()-1, max(0, int(math.Floor(u*float64(bounds.Dx())))))
	y := bounds.Min.Y + min(bounds.Dy()-1, max(0, int(math.Floor(v*float64(bounds.Dy())))))
	return r.image.NRGBAAt(x, y)
}
func (r vegetationReflectance) covered(p vec) bool { return !r.leaf || r.pixel(p).A >= 128 }

type vegetationPlant struct {
	layer            vegetationBakeLayer
	root             [2]float32
	height, width, z float32
	identity         uint32
}

func (p *preparedTransport) appendVegetationLayer(
	ctx context.Context,
	layer vegetationBakeLayer,
	species []float32,
	albedo *image.NRGBA,
) error {
	for ordinal := range layer.candidates {
		if ordinal%64 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		// #nosec G115 -- Candidate count is bounded before expansion; uint32 hashing intentionally wraps.
		identity := hashVegetation(layer.seed ^ hashVegetation(uint32(ordinal)+977))
		if float32(ordinal)+randomVegetation(identity, 3) >= layer.mean {
			continue
		}
		root := [2]float32{
			vegetationMix(layer.bounds[0], layer.bounds[2], randomVegetation(identity, 1)),
			vegetationMix(layer.bounds[1], layer.bounds[3], randomVegetation(identity, 2)),
		}
		eligible := true
		for _, plane := range layer.planes {
			if plane[0]*root[0]+plane[1]*root[1]+plane[2] < plane[3] {
				eligible = false
				break
			}
		}
		if !eligible {
			continue
		}
		height := layer.plane[0]*root[0] + layer.plane[1]*root[1] + layer.plane[2]
		if layer.terrain != nil {
			var valid bool
			height, valid = vegetationTerrainHeight(layer.terrain, root)
			if !valid {
				continue
			}
		}
		plant := vegetationPlant{layer: layer, root: root, z: height, identity: identity,
			width:  vegetationMix(float32(layer.preset.Width.Min), float32(layer.preset.Width.Max), randomVegetation(identity, 4)),
			height: vegetationMix(float32(layer.preset.Height.Min), float32(layer.preset.Height.Max), randomVegetation(identity, 5))}
		if len(species) == 0 {
			p.appendGrassPlant(plant, albedo)
			continue
		}
		for offset := 0; offset < len(species); offset += 3 * worldvegetation.SpeciesVertexFloats {
			var points [3]vec
			var coordinates [3][2]float32
			for corner := range 3 {
				vertex := species[offset+corner*worldvegetation.SpeciesVertexFloats:]
				points[corner] = plant.position([3]float32{vertex[0], vertex[1], vertex[2]}, 0)
				coordinates[corner] = [2]float32{vertex[6], vertex[7]}
			}
			p.appendVegetationTriangle(points, coordinates, albedo, coordinates[0][1] < .5)
		}
	}
	return nil
}

func (plant vegetationPlant) position(point [3]float32, card int) vec {
	woody := plant.layer.preset.Geometry != worldvegetation.GeometryCrossedCards
	yaw := randomVegetation(plant.identity, 6) * 6.28318530718
	if !woody {
		yaw += float32(card) * 1.57079632679
	}
	right := [2]float32{float32(math.Cos(float64(yaw))), float32(math.Sin(float64(yaw)))}
	x, y := right[0]*(point[0]-.5)*plant.width, right[1]*(point[0]-.5)*plant.width
	tip := 1 - point[1]
	if woody {
		sx := vegetationMix(.82, 1, randomVegetation(plant.identity, 7))
		sy := vegetationMix(.82, 1, randomVegetation(plant.identity, 8))
		x = (right[0]*point[0]*sx - right[1]*point[1]*sy) * plant.width
		y = (right[1]*point[0]*sx + right[0]*point[1]*sy) * plant.width
		tip = point[2]
	}
	xy := [2]float32{plant.root[0] + x, plant.root[1] + y}
	floor := plant.z
	if !woody {
		floor = plant.layer.plane[0]*xy[0] + plant.layer.plane[1]*xy[1] + plant.layer.plane[2]
		if plant.layer.terrain != nil {
			var valid bool
			floor, valid = vegetationTerrainHeight(plant.layer.terrain, xy)
			if !valid {
				floor = plant.z
			}
		}
		tip -= .025
	}
	direction := float32(1)
	if plant.layer.ceiling {
		direction = -1
	}
	return vec{X: float64(xy[0]), Y: float64(xy[1]), Z: float64(floor + direction*plant.height*tip)}
}

func (p *preparedTransport) appendGrassPlant(plant vegetationPlant, albedo *image.NRGBA) {
	points := [4][3]float32{{0, 0, 0}, {1, 0, 0}, {1, 1, 0}, {0, 1, 0}}
	uv := [4][2]float32{{0, 0}, {1, 0}, {1, 1}, {0, 1}}
	for card := range 2 {
		for _, indices := range [][3]int{{0, 2, 1}, {0, 3, 2}} {
			var triangle [3]vec
			var coordinates [3][2]float32
			for corner, index := range indices {
				triangle[corner] = plant.position(points[index], card)
				coordinates[corner] = uv[index]
			}
			p.appendVegetationTriangle(triangle, coordinates, albedo, true)
		}
	}
}

func (p *preparedTransport) appendVegetationTriangle(points [3]vec, uv [3][2]float32, albedo *image.NRGBA, leaf bool) {
	t := triangle{a: points[0], b: points[1], c: points[2], surface: len(p.surfaces)}
	if degenerate(t) {
		return
	}
	e1, e2 := sub(t.b, t.a), sub(t.c, t.a)
	d11, d22, d12 := dot(e1, e1), dot(e2, e2), dot(e1, e2)
	normalVector := cross(e1, e2)
	denominator := dot(normalVector, normalVector)
	dualU := scale(sub(scale(e1, d22), scale(e2, d12)), 1/denominator)
	dualV := scale(sub(scale(e2, d11), scale(e1, d12)), 1/denominator)
	projection := vegetationReflectance{image: albedo, origin: t.a, uv: [2]float64{float64(uv[0][0]), float64(uv[0][1])}, leaf: leaf,
		u: add(scale(dualU, float64(uv[1][0]-uv[0][0])), scale(dualV, float64(uv[2][0]-uv[0][0]))),
		v: add(scale(dualU, float64(uv[1][1]-uv[0][1])), scale(dualV, float64(uv[2][1]-uv[0][1])))}
	p.accel.vegetation = append(p.accel.vegetation, projection)
	cutout := &p.accel.vegetation[len(p.accel.vegetation)-1]
	normal := unit(normalVector)
	p.surfaces = append(p.surfaces, worldlightmap.Surface{Normal: normal})
	p.surfaceFrames = append(p.surfaceFrames, newSampleFrame(normal))
	p.materials = append(p.materials, reflectance{vegetation: cutout})
	t.bounds = emptyBox().point(t.a).point(t.b).point(t.c)
	p.accel.triangles = append(p.accel.triangles, t)
}
