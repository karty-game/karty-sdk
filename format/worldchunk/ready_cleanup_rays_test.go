package worldchunk

import (
	"math"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
)

type cleanupRayTriangle struct {
	points   [3][3]float64
	normal   [3]float64
	uv, lm   [3][2]float64
	material uint32
}

type cleanupRayHit struct {
	distance float64
	normal   [3]float64
	uv, lm   [2]float64
	material uint32
}

// This oracle casts rays through the original authored triangles and final
// streams. It does not call the subtraction, interpolation or coverage helpers.
func TestReadyCleanupPreservesFirstVisibleSurfaceAndCoordinates(t *testing.T) {
	t.Parallel()
	for _, baked := range []bool{false, true} {
		t.Run(map[bool]string{false: "unbaked", true: "baked"}[baked], func(t *testing.T) {
			document := cleanupRayDocument()
			plan, err := Plan(document, Options{NoLightmap: !baked,
				Lightmap: worldlightmap.Options{PageSize: 512, TexelsPerUnit: 2}})
			if err != nil || len(plan.Chunks) != 1 {
				t.Fatal("reference room did not form one complete chunk", err)
			}
			mesh := plan.Chunks[0].Mesh
			encoded, err := EncodeMesh(mesh, &document)
			if err != nil {
				t.Fatal(err)
			}
			var layout *worldlightmap.Layout
			if baked {
				layout = &plan.Chunks[0].Layout
			}
			options := ReadyOptions{CleanupStatic: true}
			ready, err := CompileReady(&mesh, encoded, &document, layout, options)
			if err != nil {
				t.Fatal(err)
			}
			packed, err := EncodeReady(ready)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := DecodeReady(packed, &mesh, encoded, &document, layout, options); err != nil {
				t.Fatal("cleaned artifact failed complete offline source proof", err)
			}
			before := cleanupRaySource(mesh)
			after := cleanupRayStreams(ready, world.MaterialIDs(&document))
			area := 0.0
			for _, triangle := range after {
				if triangle.normal[2] != 1 || triangle.points[0][2] != 0 ||
					triangle.points[1][2] != 0 || triangle.points[2][2] != 0 {
					continue
				}
				a, b, c := triangle.points[0], triangle.points[1], triangle.points[2]
				area += math.Abs((b[0]-a[0])*(c[1]-a[1])-(b[1]-a[1])*(c[0]-a[0])) / 2
			}
			// Floor 8x8 minus grounded footprints 2x2 and 1x2. The raised,
			// intersecting second box must leave the floor beneath it intact.
			if math.Abs(area-58) > 0.00001 {
				t.Fatalf("grounded/raised occluders left wrong visible floor area: %.9f", area)
			}
			checked := 0
			for _, z := range []float64{.35, 1.25, 2.65, 3.6} {
				for x := .35; x < 8; x++ {
					for y := .35; y < 8; y++ {
						eye := [3]float64{x + y*.00173, y + z*.00239, z}
						if cleanupRayInsideSolid(document, eye) {
							continue
						}
						for dx := -2; dx <= 2; dx++ {
							for dy := -2; dy <= 2; dy++ {
								for dz := -2; dz <= 2; dz++ {
									if dx == 0 && dy == 0 && dz == 0 {
										continue
									}
									direction := [3]float64{float64(dx), float64(dy), float64(dz)}
									want, got := cleanupRayNearest(before, eye, direction), cleanupRayNearest(after, eye, direction)
									if math.IsInf(want.distance, 1) || math.IsInf(got.distance, 1) ||
										math.Abs(
											got.distance-want.distance,
										) > 0.00001 || got.material != want.material || got.normal != want.normal {
										t.Fatalf(
											"visible surface changed from eye %v direction %v: got %+v want %+v",
											eye,
											direction,
											got,
											want,
										)
									}
									for axis := range 2 {
										if math.Abs(got.uv[axis]-want.uv[axis]) > 0.00001 ||
											baked && math.Abs(got.lm[axis]-want.lm[axis]) > 0.00001 {
											t.Fatalf(
												"visible coordinates changed from eye %v direction %v: got %+v want %+v",
												eye,
												direction,
												got,
												want,
											)
										}
									}
									checked++
								}
							}
						}
					}
				}
			}
			t.Logf("%d independent first-hit rays, triangles %d -> %d, visible floor %.3f", checked, len(before), len(after), area)
		})
	}
}

func cleanupRayDocument() world.Document {
	points := []world.Vec2{{}, {X: 8}, {X: 8, Y: 8}, {Y: 8}}
	sector := world.Sector{ID: "room", SourceRoom: "room", Ceiling: world.Plane{C: 4}, FloorMaterial: 1, CeilingMaterial: 1}
	for index, point := range points {
		sector.Walls = append(sector.Walls, world.Wall{Start: point, End: points[(index+1)%len(points)], Material: 1, Portal: -1})
	}
	document := world.Document{Version: world.Version, Sectors: []world.Sector{sector}, StaticSolids: &world.StaticSolids{Version: 1}}
	for index, box := range [][6]float64{{2, 2, 0, 4, 4, 1.5}, {3, 3, .5, 5, 5, 2}, {0, 5, 0, 1, 7, 2}} {
		id := []string{"grounded", "intersecting-raised", "wall-contact"}[index]
		document.StaticSolids.Items = append(document.StaticSolids.Items, world.Solid{ID: id, Collision: true,
			Footprint: []world.Vec2{{X: box[0], Y: box[1]}, {X: box[3], Y: box[1]}, {X: box[3], Y: box[4]}, {X: box[0], Y: box[4]}},
			Bottom:    world.Plane{C: box[2]}, Top: world.Plane{C: box[5]}, SideMaterial: 1, TopMaterial: 1, BottomMaterial: 1})
	}
	return document
}

func TestReadyCleanupPreservesPackedSlopedSurfaceAtLargeCoordinates(t *testing.T) {
	t.Parallel()
	document := cleanupRayDocument()
	const offsetX, offsetY, slopeX, slopeY = 1000.0137, 2000.0193, .1234, .0765
	movePoint := func(point *world.Vec2) {
		point.X += offsetX
		point.Y += offsetY
	}
	movePlane := func(plane *world.Plane) {
		plane.A, plane.B = slopeX, slopeY
		plane.C -= slopeX*offsetX + slopeY*offsetY
	}
	sector := &document.Sectors[0]
	movePlane(&sector.Floor)
	movePlane(&sector.Ceiling)
	for index := range sector.Walls {
		movePoint(&sector.Walls[index].Start)
		movePoint(&sector.Walls[index].End)
	}
	for index := range document.StaticSolids.Items {
		solid := &document.StaticSolids.Items[index]
		if index < 2 {
			center := float64(index + 3)
			sine, cosine := math.Sincos(.37)
			for corner := range solid.Footprint {
				point := &solid.Footprint[corner]
				x, y := point.X-center, point.Y-center
				point.X, point.Y = center+x*cosine-y*sine, center+x*sine+y*cosine
			}
		}
		movePlane(&solid.Bottom)
		movePlane(&solid.Top)
		for corner := range solid.Footprint {
			movePoint(&solid.Footprint[corner])
		}
	}
	plan, err := Plan(document, Options{NoLightmap: true})
	if err != nil || len(plan.Chunks) != 1 {
		t.Fatal("sloped translated room failed canonical preparation", err)
	}
	mesh := plan.Chunks[0].Mesh
	encoded, err := EncodeMesh(mesh, &document)
	if err != nil {
		t.Fatal(err)
	}
	ready, err := CompileReady(&mesh, encoded, &document, nil, ReadyOptions{CleanupStatic: true})
	if err != nil {
		t.Fatal(err)
	}
	before, after := cleanupRaySource(mesh), cleanupRayStreams(ready, world.MaterialIDs(&document))
	checkRay := func(eye [3]float64) {
		t.Helper()
		direction := [3]float64{0, 0, -1}
		want, got := cleanupRayNearest(before, eye, direction), cleanupRayNearest(after, eye, direction)
		if math.IsInf(want.distance, 1) || math.IsInf(got.distance, 1) ||
			math.Abs(got.distance-want.distance) > .000001 || got.normal != want.normal {
			t.Fatalf("cleanup moved a packed sloped surface at large coordinates: eye %v got %+v want %+v", eye, got, want)
		}
	}
	for x := .137; x < 7.9; x += .07 {
		for y := .293; y < 7.9; y += .07 {
			eye := [3]float64{offsetX + x, offsetY + y, slopeX*x + slopeY*y + 3.5}
			checkRay(eye)
		}
	}
	// Probe both sides of the actual packed footprint edges. A uniform grid can
	// miss the thin sliver between an authored plane and its rendered edge.
	for _, solid := range document.StaticSolids.Items[:2] {
		for index, start := range solid.Footprint {
			end := solid.Footprint[(index+1)%len(solid.Footprint)]
			ax, ay := float64(float32(start.X)), float64(float32(start.Y))
			bx, by := float64(float32(end.X)), float64(float32(end.Y))
			nx, ny := by-ay, ax-bx
			length := math.Hypot(nx, ny)
			for _, distance := range []float64{-.0001, -.000002, .000002, .0001} {
				x, y := ax*.63+bx*.37+nx*distance/length, ay*.63+by*.37+ny*distance/length
				checkRay([3]float64{x, y, slopeX*(x-offsetX) + slopeY*(y-offsetY) + 3.5})
			}
		}
	}
}

func cleanupRayInsideSolid(document world.Document, eye [3]float64) bool {
	for _, solid := range document.StaticSolids.Items {
		if eye[0] >= solid.Footprint[0].X && eye[0] <= solid.Footprint[2].X &&
			eye[1] >= solid.Footprint[0].Y && eye[1] <= solid.Footprint[2].Y && eye[2] >= solid.Bottom.C && eye[2] <= solid.Top.C {
			return true
		}
	}
	return false
}

func cleanupRaySource(mesh Mesh) []cleanupRayTriangle {
	result := make([]cleanupRayTriangle, len(mesh.Triangles))
	for index, triangle := range mesh.Triangles {
		item := &result[index]
		item.material = triangle.Material
		for axis, value := range triangle.Normal {
			item.normal[axis] = float64(value)
		}
		for corner, vertex := range mesh.Vertices[index*3 : index*3+3] {
			for axis, value := range vertex.Position {
				item.points[corner][axis] = float64(value)
			}
			for axis := range 2 {
				item.uv[corner][axis] = float64(vertex.UV[0][axis])
				item.lm[corner][axis] = float64(vertex.LightmapUV[axis])
			}
		}
	}
	return result
}

func cleanupRayStreams(ready Ready, materials []uint32) []cleanupRayTriangle {
	var result []cleanupRayTriangle
	for _, batch := range ready.Batches {
		for index := range batch.Sources {
			item := cleanupRayTriangle{material: materials[ready.Surfaces[batch.SurfaceIDs[index*3]].Primary]}
			for corner := range 3 {
				vertex := batch.Vertices[(index*3+corner)*ReadyStride:]
				for axis := range 3 {
					item.points[corner][axis], item.normal[axis] = float64(vertex[axis]), float64(vertex[axis+3])
				}
				for axis := range 2 {
					item.uv[corner][axis] = float64(vertex[6+axis])
					if len(batch.LightmapUV) != 0 {
						item.lm[corner][axis] = float64(batch.LightmapUV[(index*3+corner)*2+axis])
					}
				}
			}
			result = append(result, item)
		}
	}
	return result
}

func cleanupRayNearest(triangles []cleanupRayTriangle, eye, direction [3]float64) cleanupRayHit {
	hit := cleanupRayHit{distance: math.Inf(1)}
	for _, triangle := range triangles {
		a, b, c := triangle.points[0], triangle.points[1], triangle.points[2]
		e1 := [3]float64{b[0] - a[0], b[1] - a[1], b[2] - a[2]}
		e2 := [3]float64{c[0] - a[0], c[1] - a[1], c[2] - a[2]}
		p := [3]float64{
			direction[1]*e2[2] - direction[2]*e2[1],
			direction[2]*e2[0] - direction[0]*e2[2],
			direction[0]*e2[1] - direction[1]*e2[0],
		}
		dotNormal := triangle.normal[0]*direction[0] + triangle.normal[1]*direction[1] + triangle.normal[2]*direction[2]
		determinant := e1[0]*p[0] + e1[1]*p[1] + e1[2]*p[2]
		if dotNormal >= -1e-12 || math.Abs(determinant) < 1e-12 {
			continue
		}
		s := [3]float64{eye[0] - a[0], eye[1] - a[1], eye[2] - a[2]}
		u := (s[0]*p[0] + s[1]*p[1] + s[2]*p[2]) / determinant
		q := [3]float64{s[1]*e1[2] - s[2]*e1[1], s[2]*e1[0] - s[0]*e1[2], s[0]*e1[1] - s[1]*e1[0]}
		v := (direction[0]*q[0] + direction[1]*q[1] + direction[2]*q[2]) / determinant
		distance := (e2[0]*q[0] + e2[1]*q[1] + e2[2]*q[2]) / determinant
		if u < -1e-10 || v < -1e-10 || u+v > 1+1e-10 || distance <= 1e-7 || distance >= hit.distance {
			continue
		}
		hit = cleanupRayHit{distance: distance, normal: triangle.normal, material: triangle.material}
		for axis := range 2 {
			hit.uv[axis] = triangle.uv[0][axis]*(1-u-v) + triangle.uv[1][axis]*u + triangle.uv[2][axis]*v
			hit.lm[axis] = triangle.lm[0][axis]*(1-u-v) + triangle.lm[1][axis]*u + triangle.lm[2][axis]*v
		}
	}
	return hit
}
