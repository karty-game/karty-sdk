package worldlightmap

import (
	"fmt"
	"math"

	"github.com/karty-game/karty-sdk/format/world"
)

// Validate checks every record, rectangle, semantic binding and receiver before
// a caller can allocate GPU resources. The geometry digest prevents stale UVs.
func Validate(layout *Layout, document *world.Document) error {
	if layout == nil || layout.Schema != Schema || layout.Algorithm != Algorithm || !finite(layout.TexelsPerUnit) ||
		layout.TexelsPerUnit <= 0 ||
		layout.TexelsPerUnit > 64 ||
		layout.Padding < 1 ||
		layout.Padding > 16 ||
		len(layout.Pages) != 1 ||
		len(layout.Charts) == 0 ||
		len(layout.Charts) > MaxCharts {
		return ErrLayout
	}
	page := layout.Pages[0]
	if page.Width != page.Height || (page.Width != 512 && page.Width != 1024) {
		return ErrLayout
	}
	if err := world.Validate(document); err != nil {
		return fmt.Errorf("geometry: %w", err)
	}
	digest, err := geometryDigestValidated(document)
	if err != nil {
		return fmt.Errorf("geometry: %w", err)
	}
	surfaces, err := surfacesValidated(document)
	if err != nil {
		return err
	}
	return validatePrepared(layout, document, digest, surfaces)
}

// Compile has already bounded the document and built these immutable inputs.
// Reusing them keeps complete layout validation without rediscovering geometry.
func validatePrepared(layout *Layout, document *world.Document, digest string, surfaces []Surface) error {
	page := layout.Pages[0]
	if digest != layout.GeometrySHA256 {
		return fmt.Errorf("geometry digest mismatch: %w", ErrLayout)
	}
	if err := validateBake(layout.RuntimeBake, document); err != nil {
		return err
	}
	if len(layout.Bindings) != len(surfaces) {
		return fmt.Errorf("incomplete semantic bindings: %w", ErrLayout)
	}
	occupied := make([]uint64, (page.Width*page.Height+63)/64)
	for i, chart := range layout.Charts {
		if chart.ID != i || chart.Page != 0 || !validRect(chart.Rect, page) || !validRect(chart.ReceiverRect, page) {
			return fmt.Errorf("chart %d bounds: %w", i, ErrLayout)
		}
		expected := [4]int{
			chart.Rect[0] + layout.Padding,
			chart.Rect[1] + layout.Padding,
			chart.Rect[2] - layout.Padding,
			chart.Rect[3] - layout.Padding,
		}
		if chart.ReceiverRect != expected || expected[2]-expected[0] < 3 || expected[3]-expected[1] < 3 || !validBasis(chart) {
			return fmt.Errorf("chart %d basis or gutter: %w", i, ErrLayout)
		}
		for y := chart.Rect[1]; y < chart.Rect[3]; y++ {
			for x := chart.Rect[0]; x < chart.Rect[2]; x++ {
				at := y*page.Width + x
				mask := uint64(1) << (at % 64)
				if occupied[at/64]&mask != 0 {
					return fmt.Errorf("overlapping chart rectangles: %w", ErrLayout)
				}
				occupied[at/64] |= mask
			}
		}
		for _, plane := range [2][4]float64{chart.UPlane, chart.VPlane} {
			for _, v := range plane {
				if !finite(v) || math.Abs(v) > 1e15 {
					return ErrLayout
				}
			}
		}
		for axis, plane := range [2][4]float64{chart.UPlane, chart.VPlane} {
			vector := world.Vec3{X: plane[0], Y: plane[1], Z: plane[2]}
			direction := chart.Tangent
			if axis == 1 {
				direction = chart.Bitangent
			}
			factor := dot(vector, direction)
			if factor <= 0 || factor*float64(page.Width) < layout.TexelsPerUnit-1e-7 ||
				dot(sub(vector, scale(direction, factor)), sub(vector, scale(direction, factor))) > 1e-16 {
				return fmt.Errorf("chart %d affine density: %w", i, ErrLayout)
			}
		}
	}
	used := make([]bool, len(layout.Charts))
	chartOrigin := make([]world.Vec3, len(layout.Charts))
	chartSurface := make([]Binding, len(layout.Charts))
	chartConnections := make([][]bool, len(layout.Charts))
	for i, surface := range surfaces {
		binding := layout.Bindings[i]
		if binding.Kind != surface.Binding.Kind || binding.Index != surface.Binding.Index || binding.Edge != surface.Binding.Edge ||
			binding.Chart < -1 ||
			binding.Chart >= len(layout.Charts) {
			return fmt.Errorf("binding %d identity: %w", i, ErrLayout)
		}
		if len(surface.Polygons) == 0 {
			if binding.Chart != -1 {
				return ErrLayout
			}
			continue
		}
		if binding.Chart < 0 {
			return fmt.Errorf("missing receiver binding: %w", ErrLayout)
		}
		chart := layout.Charts[binding.Chart]
		if dot(sub(chart.Normal, surface.Normal), sub(chart.Normal, surface.Normal)) > 1e-12 {
			return fmt.Errorf("receiver normal: %w", ErrLayout)
		}
		if !used[binding.Chart] {
			chartOrigin[binding.Chart] = surface.Polygons[0][0]
			chartSurface[binding.Chart] = binding
		} else {
			// Algorithm 1 joins sector caps only. Walls and solid edges cannot
			// share chart identity even when their coordinates happen to overlap.
			previous := chartSurface[binding.Chart]
			if previous.Kind != binding.Kind || (binding.Kind != "sector-floor" && binding.Kind != "sector-ceiling") {
				return fmt.Errorf("unrelated receivers share chart: %w", ErrLayout)
			}
			if chartConnections[binding.Chart] == nil {
				chartConnections[binding.Chart] = connectedCaps(document, previous.Index, binding.Kind)
			}
			if !chartConnections[binding.Chart][binding.Index] {
				return fmt.Errorf("disconnected receivers share chart: %w", ErrLayout)
			}
		}
		used[binding.Chart] = true
		for _, polygon := range surface.Polygons {
			for _, v := range polygon {
				if math.Abs(dot(chart.Normal, sub(v, chartOrigin[binding.Chart]))) > 1e-8 {
					return fmt.Errorf("nonplanar chart: %w", ErrLayout)
				}
				u, w := planeValue(chart.UPlane, v)*float64(page.Width), planeValue(chart.VPlane, v)*float64(page.Height)
				if u < float64(chart.ReceiverRect[0])+.5-1e-7 || u > float64(chart.ReceiverRect[2])-.5+1e-7 ||
					w < float64(chart.ReceiverRect[1])+.5-1e-7 ||
					w > float64(chart.ReceiverRect[3])-.5+1e-7 {
					return fmt.Errorf("receiver falls outside chart: %w", ErrLayout)
				}
			}
		}
	}
	for _, present := range used {
		if !present {
			return fmt.Errorf("unused chart: %w", ErrLayout)
		}
	}
	return nil
}

func validateBake(recipe *RuntimeBake, document *world.Document) error {
	if recipe == nil {
		return nil
	}
	if recipe.ShadowSize < 32 || recipe.ShadowSize > 512 || document.Lighting == nil {
		return fmt.Errorf("unsupported bake recipe: %w", ErrLayout)
	}
	var ids []string
	switch recipe.Encoding {
	case PointVisibilityEncoding:
		if recipe.LightID == "" || recipe.LightIDs != nil {
			return fmt.Errorf("point visibility requires light_id only: %w", ErrLayout)
		}
		ids = []string{recipe.LightID}
	case DirectRNMEncoding:
		if recipe.LightID != "" || len(recipe.LightIDs) == 0 || len(recipe.LightIDs) > MaxBakeLights {
			return fmt.Errorf("direct RNM requires 1..8 light_ids only: %w", ErrLayout)
		}
		ids = recipe.LightIDs
	default:
		return fmt.Errorf("unsupported bake encoding: %w", ErrLayout)
	}
	if err := validateRuntimeScope(document); err != nil {
		return err
	}
	for index, id := range ids {
		for previous := 0; previous < index; previous++ {
			if ids[previous] == id {
				return fmt.Errorf("duplicate bake light %q: %w", id, ErrLayout)
			}
		}
		found := false
		for _, light := range document.Lighting.Lights {
			if light.ID != id {
				continue
			}
			if light.Motion != nil {
				return fmt.Errorf("bake light %q has runtime motion: %w", id, ErrLayout)
			}
			if emission := document.Emission; emission != nil {
				for _, material := range emission.Materials {
					if material.Pulse == nil || material.Pulse.Depth == 0 {
						continue
					}
					for _, linked := range material.Lights {
						if linked == id {
							return fmt.Errorf("bake light %q has an emission pulse: %w", id, ErrLayout)
						}
					}
				}
			}
			if !runtimeEmitterInside(document, light.Position) {
				return fmt.Errorf("bake emitter %q is outside the physical component: %w", id, ErrLayout)
			}
			found = true
			break
		}
		if !found {
			return fmt.Errorf("bake light %q is not an authored point light: %w", id, ErrLayout)
		}
	}
	return nil
}

func validRect(rect [4]int, page Page) bool {
	return rect[0] >= 0 && rect[1] >= 0 && rect[2] > rect[0] && rect[3] > rect[1] && rect[2] <= page.Width && rect[3] <= page.Height
}
func validBasis(chart Chart) bool {
	for _, v := range []world.Vec3{chart.Normal, chart.Tangent, chart.Bitangent} {
		if !finite(v.X) || !finite(v.Y) || !finite(v.Z) || math.Abs(dot(v, v)-1) > 1e-6 {
			return false
		}
	}
	return math.Abs(dot(chart.Normal, chart.Tangent)) < 1e-6 && math.Abs(dot(chart.Normal, chart.Bitangent)) < 1e-6 &&
		dot(cross(chart.Tangent, chart.Bitangent), chart.Normal) > 1-1e-6
}

// Each shared chart has one fixed origin sector and plane. Explore that origin's
// reachable caps once, preserving its absolute coplanarity tolerance (not a
// transitive union of pairwise near-equal planes).
func connectedCaps(document *world.Document, start int, kind string) []bool {
	visited := make([]bool, len(document.Sectors))
	visited[start] = true
	queue := []int{start}
	first := document.Sectors[start].Floor
	if kind == "sector-ceiling" {
		first = document.Sectors[start].Ceiling
	}
	for next := 0; next < len(queue); next++ {
		i := queue[next]
		for edge, wall := range document.Sectors[i].Walls {
			if wall.Portal < 0 || visited[wall.Portal] {
				continue
			}
			if !ordinaryPortal(document, i, edge, wall) {
				continue
			}
			plane := document.Sectors[wall.Portal].Floor
			if kind == "sector-ceiling" {
				plane = document.Sectors[wall.Portal].Ceiling
			}
			if math.Abs(first.A-plane.A) > 1e-9 || math.Abs(first.B-plane.B) > 1e-9 || math.Abs(first.C-plane.C) > 1e-9 {
				continue
			}
			visited[wall.Portal] = true
			queue = append(queue, int(wall.Portal))
		}
	}
	return visited
}
