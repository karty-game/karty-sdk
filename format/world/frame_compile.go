package world

import (
	"math"
	"slices"
)

// FramePiece is an already resolved sheet region and its tile coverage class.
type FramePiece struct {
	Material uint32
	Coverage string
}
type FrameHorizontal struct {
	Height, Repeat float64
	Offset         Vec2
	Piece          FramePiece
}
type FrameVertical struct {
	Width, Repeat, Phase float64
	Offset               Vec2
	Piece                FramePiece
}

// WallFrameConfig is a build-time recipe; it is never serialized to the host.
// Patches are top/start, top/end, bottom/start, bottom/end in that order.
type WallFrameConfig struct {
	Top, Bottom     FrameHorizontal
	Start, End      FrameVertical
	Patches         [4]FramePiece
	PerimeterOffset float64
}

type frameSpan struct {
	a, b        float64
	bottom, top [2]float64
}

func spansForProfile(p WallProfile, portal bool) []frameSpan {
	spans := make([]frameSpan, 0, 14)
	for cut := 0; cut < p.Count-1; cut++ {
		a, b := p.Cuts[cut], p.Cuts[cut+1]
		mid := (a + b) * .5
		pairs := [2][2]int{{0, 1}, {0, 1}}
		count := 1
		low, high := p.Controls(mid)
		if portal && p.Height(high, mid) > p.Height(low, mid)+geometryEpsilon {
			pairs = [2][2]int{{0, low}, {high, 1}}
			count = 2
		}
		for _, pair := range pairs[:count] {
			if p.Height(pair[1], mid) <= p.Height(pair[0], mid)+geometryEpsilon {
				continue
			}
			spans = append(spans, frameSpan{a, b, p.Heights[pair[0]], p.Heights[pair[1]]})
		}
	}
	return spans
}
func affineHeight(h [2]float64, t float64) float64 { return h[0] + (h[1]-h[0])*t }
func shiftedHeight(h [2]float64, offset float64) [2]float64 {
	return [2]float64{h[0] + offset, h[1] + offset}
}
func mixedHeight(a, b [2]float64, t float64) [2]float64 {
	return [2]float64{a[0] + (b[0]-a[0])*t, a[1] + (b[1]-a[1])*t}
}

// CompileWallFrame partitions visible solid spans into at most 252 convex
// regions. Bands follow affine floor/ceiling profiles; opposing envelopes crop
// proportionally when a face is shorter than their combined requested sizes.
// Original authored corners and texture IDs have already been resolved by the
// builder. The output contains every main and frame region with no overlap.
func CompileWallFrame(w Wall, p WallProfile, f WallFrameConfig) ([]WallFrameRegion, error) {
	if err := validateFrameConfig(w, p, f); err != nil {
		return nil, err
	}
	if f.Top.Height == 0 && f.Bottom.Height == 0 && f.Start.Width == 0 && f.End.Width == 0 {
		return nil, nil
	}
	length := math.Hypot(w.End.X-w.Start.X, w.End.Y-w.Start.Y)
	startEnd, endStart := math.Min(f.Start.Width, length), math.Max(0, length-f.End.Width)
	if f.Start.Width+f.End.Width > length && f.Start.Width > 0 && f.End.Width > 0 {
		startEnd = length * f.Start.Width / (f.Start.Width + f.End.Width)
		endStart = startEnd
	}
	uCuts := []float64{0, 1}
	if f.Start.Width > 0 {
		uCuts = append(uCuts, startEnd/length)
	}
	if f.End.Width > 0 {
		uCuts = append(uCuts, endStart/length)
	}
	slices.Sort(uCuts)
	uCuts = slices.Compact(uCuts)
	regions := make([]WallFrameRegion, 0, 9)
	for _, span := range spansForProfile(p, w.Portal >= 0) {
		cuts := []float64{span.a, span.b}
		bandSum := f.Bottom.Height + f.Top.Height
		da := affineHeight(span.top, span.a) - affineHeight(span.bottom, span.a) - bandSum
		db := affineHeight(span.top, span.b) - affineHeight(span.bottom, span.b) - bandSum
		if da*db < 0 {
			cuts = append(cuts, span.a+(span.b-span.a)*da/(da-db))
		}
		for _, t := range uCuts {
			if t > span.a && t < span.b {
				cuts = append(cuts, t)
			}
		}
		slices.Sort(cuts)
		cuts = slices.Compact(cuts)
		for ci := 0; ci < len(cuts)-1; ci++ {
			a, b := cuts[ci], cuts[ci+1]
			if b-a <= geometryEpsilon {
				continue
			}
			mid := (a + b) * .5
			col := 0
			if f.Start.Width > 0 && mid < startEnd/length {
				col = -1
			} else if f.End.Width > 0 && mid > endStart/length {
				col = 1
			}
			bottomEnd := shiftedHeight(span.bottom, f.Bottom.Height)
			topStart := shiftedHeight(span.top, -f.Top.Height)
			if affineHeight(span.top, mid)-affineHeight(span.bottom, mid) < bandSum && bandSum > 0 {
				boundary := mixedHeight(span.bottom, span.top, f.Bottom.Height/bandSum)
				bottomEnd, topStart = boundary, boundary
			} else {
				if f.Bottom.Height == 0 {
					bottomEnd = span.bottom
				}
				if f.Top.Height == 0 {
					topStart = span.top
				}
			}
			rows := [3][2][2]float64{{span.bottom, bottomEnd}, {bottomEnd, topStart}, {topStart, span.top}}
			for row, bounds := range rows {
				if affineHeight(bounds[1], mid)-affineHeight(bounds[0], mid) <= geometryEpsilon {
					continue
				}
				vertices := []Vec2{
					{a, affineHeight(bounds[0], a)},
					{b, affineHeight(bounds[0], b)},
					{b, affineHeight(bounds[1], b)},
					{a, affineHeight(bounds[1], a)},
				}
				vertices = compactFrameVertices(vertices)
				if len(vertices) < 3 {
					continue
				}
				r := WallFrameRegion{Vertices: vertices, Material: w.Material, Coverage: FrameCoverageMain}
				if row != 1 || col != 0 {
					piece, projection, repeatU, repeatV := frameMapping(w, length, span, f, row, col)
					r.Material, r.Coverage = piece.Material, piece.Coverage
					r.RepeatU, r.RepeatV = repeatU, repeatV
					r.UV = &SurfaceUV{Projections: []UVProjection{projection}, Weights: []float64{1}}
					if r.Coverage == FrameCoverageOpaque && !frameDomainCovered(w, r) {
						r.Coverage = FrameCoverageMasked
					}
				}
				regions = append(regions, r)
				if len(regions) > MaxFrameRegionsPerWall {
					return nil, ErrBounds
				}
			}
		}
	}
	return regions, nil
}
func compactFrameVertices(v []Vec2) []Vec2 {
	out := v[:0]
	for _, p := range v {
		if len(out) == 0 || !sameFramePoint(p, out[len(out)-1]) {
			out = append(out, p)
		}
	}
	if len(out) > 1 && sameFramePoint(out[0], out[len(out)-1]) {
		out = out[:len(out)-1]
	}
	return out
}
func edgePlane(w Wall, length float64, scale, offset float64) UVPlane {
	x, y := (w.End.X-w.Start.X)/length, (w.End.Y-w.Start.Y)/length
	return UVPlane{X: x / scale, Y: y / scale, Offset: offset - (x*w.Start.X+y*w.Start.Y)/scale}
}
func heightPlane(w Wall, length float64, h [2]float64, scale, offset float64) UVPlane {
	p := edgePlane(w, length, 1, 0)
	slope := (h[1] - h[0]) / length
	return UVPlane{X: -slope * p.X / scale, Y: -slope * p.Y / scale, Z: 1 / scale, Offset: offset - (h[0]+slope*p.Offset)/scale}
}
func frameMapping(w Wall, length float64, span frameSpan, f WallFrameConfig, row, col int) (FramePiece, UVProjection, bool, bool) {
	var piece FramePiece
	var uv UVProjection
	repeatU, repeatV := false, false
	if row == 1 {
		v := f.Start
		if col > 0 {
			v = f.End
		}
		origin := 0.0
		if col > 0 {
			origin = length - v.Width
		}
		uv.U = edgePlane(w, length, v.Width, v.Offset.X-origin/v.Width)
		uv.V = UVPlane{Z: 1 / v.Repeat, Offset: v.Offset.Y - v.Phase/v.Repeat}
		return v.Piece, uv, false, true
	}
	h := f.Bottom
	if row == 2 {
		h = f.Top
	}
	anchor := shiftedHeight(span.bottom, h.Height)
	if row == 2 {
		anchor = span.top
	}
	// PNG rows increase downwards: top outer edge is row 0 and bottom
	// outer edge is row 1. Corner patches use the same sheet orientation.
	uv.V = heightPlane(w, length, anchor, -h.Height, h.Offset.Y)
	uv.U = edgePlane(w, length, h.Repeat, f.PerimeterOffset/h.Repeat+h.Offset.X)
	piece = h.Piece
	repeatU = true
	if col != 0 {
		v := f.Start
		if col > 0 {
			v = f.End
		}
		origin := 0.0
		if col > 0 {
			origin = length - v.Width
		}
		uv.U = edgePlane(w, length, v.Width, v.Offset.X-origin/v.Width)
		idx := 0
		if row == 0 {
			idx = 2
		}
		if col > 0 {
			idx++
		}
		piece = f.Patches[idx]
		repeatU = false
	}
	return piece, uv, repeatU, repeatV
}
func frameDomainCovered(w Wall, r WallFrameRegion) bool {
	uv := r.UV.Projections[0]
	for _, v := range r.Vertices {
		pos := Vec3{w.Start.X + (w.End.X-w.Start.X)*v.X, w.Start.Y + (w.End.Y-w.Start.Y)*v.X, v.Y}
		u := uv.U.X*pos.X + uv.U.Y*pos.Y + uv.U.Z*pos.Z + uv.U.Offset
		vv := uv.V.X*pos.X + uv.V.Y*pos.Y + uv.V.Z*pos.Z + uv.V.Offset
		if !r.RepeatU && (u < -geometryEpsilon || u > 1+geometryEpsilon) ||
			!r.RepeatV && (vv < -geometryEpsilon || vv > 1+geometryEpsilon) {
			return false
		}
	}
	return true
}
func validateFrameConfig(w Wall, p WallProfile, f WallFrameConfig) error {
	if !validVec2(w.Start) || !validVec2(w.End) || distanceSquared(w.Start, w.End) < MinEdgeLength*MinEdgeLength || p.Count < 2 ||
		p.Count > 8 {
		return ErrGeometry
	}
	canonical, err := NewWallProfile(p.Heights, geometryEpsilon)
	if err != nil {
		return err
	}
	if canonical.Count != p.Count || canonical.Cuts != p.Cuts {
		return ErrGeometry
	}
	if math.IsNaN(f.PerimeterOffset) || math.IsInf(f.PerimeterOffset, 0) || math.Abs(f.PerimeterOffset) > MaxUVProjectionValue {
		return ErrBounds
	}
	validPiece := func(piece FramePiece) bool {
		return piece.Coverage == FrameCoverageOpaque || piece.Coverage == FrameCoverageMasked
	}
	for _, h := range []FrameHorizontal{f.Top, f.Bottom} {
		if h.Height == 0 {
			continue
		}
		if !finiteBounded(h.Height) || h.Height < .001 || !finiteBounded(h.Repeat) || h.Repeat < .001 || !validVec2(h.Offset) ||
			!validPiece(h.Piece) {
			return ErrBounds
		}
	}
	for _, v := range []FrameVertical{f.Start, f.End} {
		if v.Width == 0 {
			continue
		}
		if !finiteBounded(v.Width) || v.Width < .001 || !finiteBounded(v.Repeat) || v.Repeat < .001 || !finiteBounded(v.Phase) ||
			!validVec2(v.Offset) ||
			!validPiece(v.Piece) {
			return ErrBounds
		}
	}
	for _, pair := range [][3]int{{0, 0, 0}, {0, 1, 1}, {1, 0, 2}, {1, 1, 3}} {
		horizontal := f.Top
		if pair[0] == 1 {
			horizontal = f.Bottom
		}
		vertical := f.Start
		if pair[1] == 1 {
			vertical = f.End
		}
		if horizontal.Height > 0 && vertical.Width > 0 && !validPiece(f.Patches[pair[2]]) {
			return ErrBounds
		}
	}
	return nil
}

func sameFramePoint(a, b Vec2) bool {
	return math.Abs(a.X-b.X) <= geometryEpsilon && math.Abs(a.Y-b.Y) <= geometryEpsilon
}
