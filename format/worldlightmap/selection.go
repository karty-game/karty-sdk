package worldlightmap

import (
	"fmt"
	"math"
	"slices"

	"github.com/karty-game/karty-sdk/format/world"
)

const SelectedSchema = "karty.world-lightmaps@3"
const SelectedAlgorithm = uint32(4)

const MaxLiquidTiles = 64

// LiquidTile preserves an original geometry cell inside a larger loading unit.
// Clips cover the whole selection rectangle exactly, with no positive overlap.
type LiquidTile struct {
	Clip                  [4]float64
	LiquidMaxSubdivisions int
}

// SurfaceSelection explicitly selects complete source faces, or their common
// XY rectangle. Transport remains the entire ordinary physical component.
// Bindings are in source discovery order and have Chart=-1. Missing bindings
// mean delegated/replaced geometry only under a versioned root ownership table.
type SurfaceSelection struct {
	// LiquidEdgeStep anchors all spatial water boundaries to one world XY grid.
	LiquidEdgeStep float64
	// LiquidMaxSubdivisions is an explicit geometric detail cap, not lighting density.
	// Zero preserves normal error-based tessellation; nonzero values are 1..32.
	LiquidMaxSubdivisions int
	LiquidTiles           []LiquidTile
	Bindings              []Binding
	Clip                  *[4]float64
}

func validPageSize(size int, selected bool) bool {
	return size == 512 || size == 1024 || selected && (size == 32 || size == 64 || size == 128 || size == 256)
}
func validLayoutVersion(layout *Layout) bool {
	return layout.Schema == Schema && layout.Selection == nil && (layout.Algorithm == Algorithm || layout.Algorithm == ReceiverAlgorithm) ||
		layout.Schema == SelectedSchema && layout.Selection != nil && layout.Algorithm == SelectedAlgorithm
}

// CompileSelectedReceivers clips before chart allocation; it never lowers the
// requested density to accommodate a large unrelated source face.
func CompileSelectedReceivers(document world.Document, options Options, sectors, solids []int, selection SurfaceSelection) (Layout, error) {
	solidCount := 0
	if document.StaticSolids != nil {
		solidCount = len(document.StaticSolids.Items)
	}
	if err := ValidateSurfaceSelection(
		&selection,
		LayoutAdmission{SectorCount: len(document.Sectors), SolidCount: solidCount},
	); err != nil {
		return Layout{}, err
	}
	selection.LiquidTiles = slices.Clone(selection.LiquidTiles)
	selection.Bindings = slices.DeleteFunc(slices.Clone(selection.Bindings), func(b Binding) bool {
		return !receiverSelected(b, &ReceiverSelection{Sectors: sectors, Solids: solids})
	})
	if selection.Clip != nil {
		clip := *selection.Clip
		selection.Clip = &clip
	}
	return compileReceivers(document, options, &ReceiverSelection{Sectors: slices.Clone(sectors), Solids: slices.Clone(solids)}, &selection)
}

func ValidateSurfaceSelection(selection *SurfaceSelection, admission LayoutAdmission) error {
	if selection == nil || selection.LiquidMaxSubdivisions < 0 || selection.LiquidMaxSubdivisions > 32 ||
		len(selection.Bindings) > world.MaxWalls+world.MaxSectors*2+world.MaxStaticSolids*(world.MaxSolidVertices+2) {
		return ErrLayout
	}
	seen := make(map[Binding]bool, len(selection.Bindings))
	for _, b := range selection.Bindings {
		if b.Chart != -1 || !validArtifactBinding(b, admission) || seen[b] {
			return ErrLayout
		}
		seen[b] = true
	}
	if !finite(selection.LiquidEdgeStep) || selection.LiquidEdgeStep < 0 || selection.LiquidEdgeStep > 64 {
		return ErrLayout
	}
	if selection.LiquidEdgeStep > 0 && selection.Clip == nil {
		return ErrLayout
	}
	if selection.Clip != nil {
		c := *selection.Clip
		for _, v := range c {
			if !finite(v) || v < -world.MaxCoordinate || v > world.MaxCoordinate {
				return ErrLayout
			}
		}
		if selection.LiquidEdgeStep > 0 {
			for _, v := range c {
				q := v / selection.LiquidEdgeStep
				if math.Abs(q-math.Round(q)) > 1e-7 {
					return fmt.Errorf("liquid tile is not on common boundary grid: %w", ErrLayout)
				}
			}
		}
		if c[0] >= c[2] || c[1] >= c[3] {
			return ErrLayout
		}
	}
	return validateLiquidTiles(selection)
}

// ApplySurfaceSelection preserves global surface indexing while selecting and
// clipping convex source polygons. Input polygon backing arrays are not changed.
func ApplySurfaceSelection(surfaces []Surface, selection *SurfaceSelection) error {
	if selection == nil {
		return nil
	}
	selected := make(map[Binding]int, len(selection.Bindings))
	for i, b := range selection.Bindings {
		if b.Chart != -1 {
			return ErrLayout
		}
		if _, ok := selected[b]; ok {
			return ErrLayout
		}
		selected[b] = i
	}
	previous := -1
	for i := range surfaces {
		b := surfaces[i].Binding
		b.Chart = -1
		order, ok := selected[b]
		if !ok {
			surfaces[i].Polygons = nil
			continue
		}
		if order <= previous {
			return fmt.Errorf("surface selection order: %w", ErrLayout)
		}
		previous = order
		delete(selected, b)
		if !finite(selection.LiquidEdgeStep) || selection.LiquidEdgeStep < 0 || selection.LiquidEdgeStep > 64 {
			return ErrLayout
		}
		if selection.LiquidEdgeStep > 0 && selection.Clip == nil {
			return ErrLayout
		}
		if selection.Clip != nil {
			clipped := make([][]world.Vec3, 0, len(surfaces[i].Polygons))
			for _, polygon := range surfaces[i].Polygons {
				if p := ClipSurfacePolygon(polygon, *selection.Clip); len(p) >= 3 {
					clipped = append(clipped, p)
				}
			}
			surfaces[i].Polygons = clipped
		}
	}
	if len(selected) != 0 {
		return ErrLayout
	}
	return nil
}

// ClipSurfacePolygon retains stable authored world coordinates on shared edges.
func ClipSurfacePolygon(polygon []world.Vec3, rectangle [4]float64) []world.Vec3 {
	out := slices.Clone(polygon)
	for axis := range 4 {
		input := out
		out = nil
		if len(input) == 0 {
			break
		}
		coord := func(v world.Vec3) float64 {
			if axis%2 == 0 {
				return v.X
			}
			return v.Y
		}
		boundary := rectangle[axis]
		inside := func(v world.Vec3) bool {
			if axis < 2 {
				return coord(v) >= boundary
			}
			return coord(v) <= boundary
		}
		a := input[len(input)-1]
		for _, b := range input {
			ia, ib := inside(a), inside(b)
			if ia != ib {
				t := (boundary - coord(a)) / (coord(b) - coord(a))
				p := world.Vec3{X: a.X + t*(b.X-a.X), Y: a.Y + t*(b.Y-a.Y), Z: a.Z + t*(b.Z-a.Z)}
				if axis%2 == 0 {
					p.X = boundary
				} else {
					p.Y = boundary
				}
				out = append(out, p)
			}
			if ib {
				out = append(out, b)
			}
			a = b
		}
	}
	if len(out) < 3 {
		return nil
	}
	// A polygon touching a tile along an edge has no receiver area.
	sum := world.Vec3{}
	for i, p := range out {
		q := out[(i+1)%len(out)]
		sum.X += p.Y*q.Z - p.Z*q.Y
		sum.Y += p.Z*q.X - p.X*q.Z
		sum.Z += p.X*q.Y - p.Y*q.X
	}
	if dot(sum, sum) < 1e-20 {
		return nil
	}
	return out
}

// SurfaceSelected tests exact source identity, ignoring a caller's chart slot.
func SurfaceSelected(binding Binding, selection *SurfaceSelection) bool {
	if selection == nil {
		return true
	}
	binding.Chart = -1
	return slices.Contains(selection.Bindings, binding)
}
func EqualSurfaceSelections(a, b *SurfaceSelection) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.LiquidEdgeStep != b.LiquidEdgeStep || a.LiquidMaxSubdivisions != b.LiquidMaxSubdivisions ||
		!slices.Equal(a.Bindings, b.Bindings) || !slices.Equal(a.LiquidTiles, b.LiquidTiles) ||
		(a.Clip == nil) != (b.Clip == nil) {
		return false
	}
	return a.Clip == nil || *a.Clip == *b.Clip
}
