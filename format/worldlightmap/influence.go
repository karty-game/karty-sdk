package worldlightmap

import (
	"fmt"
	"math"

	"github.com/karty-game/karty-sdk/format/world"
)

// InfluenceScope describes conservative pixel dependencies in stable semantic
// surface and recipe order. SurfaceIndices index Surfaces(document); LightIDs
// never change which authored sources are classified as completed static lights.
type InfluenceScope struct {
	Bounds         [6]float64
	SurfaceIndices []int
	LightIDs       []string
}

// BoundedInfluenceScope keeps geometry within (bounces+1)*distance of receiver
// bounds, and sources whose min(radius,distance) sphere reaches receiver bounds
// expanded by bounces*distance. Small guards cover ray-origin and raster rounding.
// Zero distance retains the complete ordinary physical-component dependency set.
func BoundedInfluenceScope(document *world.Document, layout Layout, bounces int, maxRayDistance float64) (InfluenceScope, error) {
	if bounces < 0 || bounces > MaxOfflineBounces || !validMaxRayDistance(maxRayDistance) || document == nil || document.Lighting == nil {
		return InfluenceScope{}, ErrLayout
	}
	if _, err := Encode(layout, document); err != nil || layout.RuntimeBake == nil || layout.RuntimeBake.Encoding != DirectRNMEncoding {
		return InfluenceScope{}, ErrLayout
	}
	surfaces, err := surfacesValidated(document)
	if err != nil {
		return InfluenceScope{}, err
	}
	return influenceScopeValidated(document, layout, surfaces, bounces, maxRayDistance)
}

func influenceScopeValidated(
	document *world.Document,
	layout Layout,
	surfaces []Surface,
	bounces int,
	distance float64,
) (InfluenceScope, error) {
	var sectors, solids []bool
	if layout.Receivers != nil {
		var err error
		sectors, solids, err = ReceiverScope(document, layout.Receivers)
		if err != nil {
			return InfluenceScope{}, err
		}
	}
	receivers := [6]float64{math.Inf(1), math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1), math.Inf(-1)}
	for i, surface := range surfaces {
		if layout.Bindings[i].Chart < 0 {
			continue
		}
		for _, polygon := range surface.Polygons {
			for _, point := range polygon {
				expandPoint(&receivers, point)
			}
		}
	}
	if math.IsInf(receivers[0], 1) {
		return InfluenceScope{}, fmt.Errorf("no bounded receivers: %w", ErrLayout)
	}
	scope := InfluenceScope{Bounds: receivers, SurfaceIndices: make([]int, 0), LightIDs: make([]string, 0)}
	bounceBounds := receivers
	if distance > 0 {
		span := max(receivers[3]-receivers[0], receivers[4]-receivers[1], receivers[5]-receivers[2])
		// Raster inclusion permits barycentric roundoff at surface boundaries;
		// ray-origin offsets use at most 1e-6 inside authored coordinate bounds.
		guard := max(1e-5, span*1e-8) + float64(bounces+2)*8e-6
		for axis := range 3 {
			scope.Bounds[axis] = math.Nextafter(receivers[axis]-float64(bounces+1)*distance-guard, math.Inf(-1))
			scope.Bounds[axis+3] = math.Nextafter(receivers[axis+3]+float64(bounces+1)*distance+guard, math.Inf(1))
			bounceBounds[axis] = math.Nextafter(receivers[axis]-float64(bounces)*distance-guard, math.Inf(-1))
			bounceBounds[axis+3] = math.Nextafter(receivers[axis+3]+float64(bounces)*distance+guard, math.Inf(1))
		}
	}
	for i, surface := range surfaces {
		if len(surface.Polygons) == 0 {
			continue
		}
		if sectors != nil {
			binding := surface.Binding
			included := false
			if binding.Kind == "sector-floor" || binding.Kind == "sector-ceiling" || binding.Kind == "sector-wall" {
				included = sectors[binding.Index]
			} else {
				included = solids[binding.Index]
			}
			if !included {
				continue
			}
		}
		bounds := [6]float64{math.Inf(1), math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1), math.Inf(-1)}
		for _, polygon := range surface.Polygons {
			for _, point := range polygon {
				expandPoint(&bounds, point)
			}
		}
		if distance == 0 || boxesOverlap(bounds, scope.Bounds) {
			scope.SurfaceIndices = append(scope.SurfaceIndices, i)
		}
	}
	sources := make(map[string]world.PointLight, len(document.Lighting.Lights))
	for _, light := range document.Lighting.Lights {
		sources[light.ID] = light
	}
	for _, id := range layout.RuntimeBake.LightIDs {
		light := sources[id]
		if distance == 0 || sphereOverlapsBox(light.Position, min(light.Radius, distance), bounceBounds) {
			scope.LightIDs = append(scope.LightIDs, id)
		}
	}
	return scope, nil
}

func expandPoint(bounds *[6]float64, point world.Vec3) {
	for axis, value := range [...]float64{point.X, point.Y, point.Z} {
		bounds[axis] = min(bounds[axis], value)
		bounds[axis+3] = max(bounds[axis+3], value)
	}
}

func boxesOverlap(a, b [6]float64) bool {
	for axis := range 3 {
		if a[axis+3] < b[axis] || a[axis] > b[axis+3] {
			return false
		}
	}
	return true
}

func sphereOverlapsBox(position world.Vec3, radius float64, bounds [6]float64) bool {
	distanceSquared := 0.0
	for axis, value := range [...]float64{position.X, position.Y, position.Z} {
		delta := max(bounds[axis]-value, 0, value-bounds[axis+3])
		distanceSquared += delta * delta
	}
	return distanceSquared <= radius*radius
}

// OfflineRGBMRangeBounded uses exactly the producer's conservative source
// scope and clipped influence radii. Zero is identical to OfflineRGBMRange.
func OfflineRGBMRangeBounded(layout Layout, document *world.Document, bounces int, maxRayDistance float64) (float64, error) {
	if maxRayDistance == 0 {
		return OfflineRGBMRange(layout, document, bounces)
	}
	scope, err := BoundedInfluenceScope(document, layout, bounces, maxRayDistance)
	if err != nil {
		return 0, err
	}
	sources := make(map[string]world.PointLight, len(document.Lighting.Lights))
	for _, light := range document.Lighting.Lights {
		sources[light.ID] = light
	}
	lights := make([]world.PointLight, 0, len(scope.LightIDs))
	for _, id := range scope.LightIDs {
		light := sources[id]
		light.Radius = min(light.Radius, maxRayDistance)
		lights = append(lights, light)
	}
	factor, energy := 1.0, 1.0
	for range bounces {
		energy *= MaxDiffuseReflectance
		factor += energy
	}
	return max(1, 3*influenceEnergyBound(lights)*factor), nil
}
