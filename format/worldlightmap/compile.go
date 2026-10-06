package worldlightmap

import (
	"fmt"
	"math"
	"slices"

	"github.com/karty-game/karty-sdk/format/world"
)

func resolvedOptions(options Options) (Options, error) {
	if options.Lights != nil && (options.Light != "" || len(options.Lights) == 0 || len(options.Lights) > MaxBakeLights) {
		return Options{}, fmt.Errorf("mutually exclusive or empty light selection: %w", ErrLayout)
	}
	if options.TexelsPerUnit == 0 {
		options.TexelsPerUnit = 16
	}
	if options.PageSize == 0 {
		options.PageSize = 1024
	}
	if options.Padding == 0 {
		options.Padding = 4
	}
	if options.MaxPages == 0 {
		options.MaxPages = 1
	}
	if options.MaxTexels == 0 {
		options.MaxTexels = MaxTexels
	}
	if options.ShadowSize == 0 {
		options.ShadowSize = 512
	}
	if !finite(options.TexelsPerUnit) || options.TexelsPerUnit <= 0 || options.TexelsPerUnit > 64 ||
		(options.PageSize != 512 && options.PageSize != 1024) || options.Padding < 1 || options.Padding > 16 ||
		options.MaxPages != 1 || options.MaxTexels < options.PageSize*options.PageSize || options.MaxTexels > MaxTexels ||
		options.ShadowSize < 32 || options.ShadowSize > 512 {
		return Options{}, fmt.Errorf("density, page or byte budget: %w", ErrLayout)
	}
	return options, nil
}

// Compile discovers and packs unique receiver coordinates without triangulating
// sectors or importing runtime code. Algorithm 1 merges only coplanar floors
// and ceilings across ordinary reciprocal portals, never across transformed or
// disconnected spaces. Walls and solid sides retain semantic chart identities.
func Compile(document world.Document, options Options) (Layout, error) {
	options, err := resolvedOptions(options)
	if err != nil {
		return Layout{}, err
	}
	if err := world.Validate(&document); err != nil {
		return Layout{}, err
	}
	surfaces, err := surfacesValidated(&document)
	if err != nil {
		return Layout{}, err
	}
	digest, err := geometryDigestValidated(&document)
	if err != nil {
		return Layout{}, err
	}
	layout := Layout{
		Schema:         Schema,
		Algorithm:      Algorithm,
		GeometrySHA256: digest,
		TexelsPerUnit:  options.TexelsPerUnit,
		Padding:        options.Padding,
		Pages:          []Page{{options.PageSize, options.PageSize}},
		Bindings:       make([]Binding, len(surfaces)),
	}
	if options.Light != "" {
		layout.RuntimeBake = &RuntimeBake{Encoding: PointVisibilityEncoding, LightID: options.Light, ShadowSize: options.ShadowSize}
	}
	if options.Lights != nil {
		layout.RuntimeBake = &RuntimeBake{
			Encoding:   DirectRNMEncoding,
			LightIDs:   slices.Clone(options.Lights),
			ShadowSize: options.ShadowSize,
		}
	}
	if err := validateBake(layout.RuntimeBake, &document); err != nil {
		return Layout{}, err
	}
	parents := make([]int, len(surfaces))
	for i := range parents {
		parents[i] = i
	}
	sectorCaps := make([][2]int, len(document.Sectors))
	for i, s := range surfaces {
		if s.Binding.Kind == "sector-floor" {
			sectorCaps[s.Binding.Index][0] = i
		}
		if s.Binding.Kind == "sector-ceiling" {
			sectorCaps[s.Binding.Index][1] = i
		}
	}
	for i, sector := range document.Sectors {
		for edge, wall := range sector.Walls {
			if wall.Portal < 0 {
				continue
			}
			if !ordinaryPortal(&document, i, edge, wall) {
				continue
			}
			for cap := range 2 {
				a, b := sectorCaps[i][cap], sectorCaps[wall.Portal][cap]
				if samePlane(surfaces[a], surfaces[b]) {
					ra, rb := root(parents, a), root(parents, b)
					parents[max(ra, rb)] = min(ra, rb)
				}
			}
		}
	}
	chartForRoot := make(map[int]int)
	points := make([][]world.Vec3, 0)
	for i, surface := range surfaces {
		layout.Bindings[i] = surface.Binding
		if len(surface.Polygons) == 0 {
			continue
		}
		parent := root(parents, i)
		chart, found := chartForRoot[parent]
		if !found {
			chart = len(layout.Charts)
			if chart >= MaxCharts {
				return Layout{}, fmt.Errorf("chart count: %w", ErrLayout)
			}
			chartForRoot[parent] = chart
			normal := surface.Normal
			tangent, bitangent := basis(normal)
			layout.Charts = append(layout.Charts, Chart{ID: chart, Normal: normal, Tangent: tangent, Bitangent: bitangent})
			points = append(points, nil)
		}
		layout.Bindings[i].Chart = chart
		for _, polygon := range surface.Polygons {
			points[chart] = append(points[chart], polygon...)
		}
	}
	if err := pack(&layout, points, options); err != nil {
		return Layout{}, err
	}
	if err := validatePrepared(&layout, &document, digest, surfaces); err != nil {
		return Layout{}, err
	}
	return layout, nil
}

func root(parents []int, i int) int {
	for parents[i] != i {
		parents[i] = parents[parents[i]]
		i = parents[i]
	}
	return i
}
func basis(normal world.Vec3) (world.Vec3, world.Vec3) {
	axis := world.Vec3{X: 1}
	if math.Abs(normal.X) > .9 {
		axis = world.Vec3{Y: 1}
	}
	tangent := unit(sub(axis, scale(normal, dot(axis, normal))))
	return tangent, cross(normal, tangent)
}
func samePlane(a, b Surface) bool {
	if dot(sub(a.Normal, b.Normal), sub(a.Normal, b.Normal)) > 1e-12 {
		return false
	}
	origin := a.Polygons[0][0]
	for _, polygon := range b.Polygons {
		for _, v := range polygon {
			if math.Abs(dot(a.Normal, sub(v, origin))) > 1e-9 {
				return false
			}
		}
	}
	return true
}

type chartBounds struct {
	minimum       [2]float64
	density       [2]float64
	width, height int
}
type shelf struct{ x, y, height int }

func pack(layout *Layout, points [][]world.Vec3, options Options) error {
	bounds := make([]chartBounds, len(points))
	order := make([]int, len(points))
	for i, vertices := range points {
		chart := layout.Charts[i]
		minimum := [2]float64{math.Inf(1), math.Inf(1)}
		maximum := [2]float64{math.Inf(-1), math.Inf(-1)}
		for _, v := range vertices {
			u, w := dot(v, chart.Tangent), dot(v, chart.Bitangent)
			minimum[0] = math.Min(minimum[0], u)
			minimum[1] = math.Min(minimum[1], w)
			maximum[0] = math.Max(maximum[0], u)
			maximum[1] = math.Max(maximum[1], w)
		}
		span := [2]float64{maximum[0] - minimum[0], maximum[1] - minimum[1]}
		density := [2]float64{math.Max(options.TexelsPerUnit, 2/span[0]), math.Max(options.TexelsPerUnit, 2/span[1])}
		width, height := math.Ceil(span[0]*density[0])+1, math.Ceil(span[1]*density[1])+1
		if !finite(width) || !finite(height) || width+float64(options.Padding*2) > float64(options.PageSize) ||
			height+float64(options.Padding*2) > float64(options.PageSize) {
			return fmt.Errorf("chart %d is oversized; change density or split surface: %w", i, ErrLayout)
		}
		bounds[i] = chartBounds{minimum, density, int(width), int(height)}
		order[i] = i
	}
	slices.SortFunc(order, func(a, b int) int {
		if bounds[a].height != bounds[b].height {
			return bounds[b].height - bounds[a].height
		}
		if bounds[a].width != bounds[b].width {
			return bounds[b].width - bounds[a].width
		}
		return a - b
	})
	rows := make([]shelf, 0)
	for _, i := range order {
		bound := bounds[i]
		width, height := bound.width+2*options.Padding, bound.height+2*options.Padding
		x, y, placed := 0, 0, false
		for j, row := range rows {
			if height <= row.height && row.x+width <= options.PageSize {
				x, y, placed = row.x, row.y, true
				rows[j].x += width
				break
			}
		}
		if !placed {
			if len(rows) > 0 {
				last := rows[len(rows)-1]
				y = last.y + last.height
			}
			if y+height > options.PageSize {
				return fmt.Errorf("one-page lightmap budget exhausted; change density or split asset: %w", ErrLayout)
			}
			rows = append(rows, shelf{width, y, height})
		}
		chart := &layout.Charts[i]
		chart.Rect = [4]int{x, y, x + width, y + height}
		chart.ReceiverRect = [4]int{x + options.Padding, y + options.Padding, x + width - options.Padding, y + height - options.Padding}
		page := float64(options.PageSize)
		chart.UPlane = [4]float64{
			chart.Tangent.X * bound.density[0] / page,
			chart.Tangent.Y * bound.density[0] / page,
			chart.Tangent.Z * bound.density[0] / page,
			(float64(chart.ReceiverRect[0]) + .5 - bound.minimum[0]*bound.density[0]) / page,
		}
		chart.VPlane = [4]float64{
			chart.Bitangent.X * bound.density[1] / page,
			chart.Bitangent.Y * bound.density[1] / page,
			chart.Bitangent.Z * bound.density[1] / page,
			(float64(chart.ReceiverRect[1]) + .5 - bound.minimum[1]*bound.density[1]) / page,
		}
	}
	return nil
}
