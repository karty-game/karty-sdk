package world

import (
	"fmt"
	"math"

	"github.com/karty-game/karty-sdk/format/cartridge"
)

const (
	MaterialLayersVersion = uint32(1)
	FeatureMaterialLayers = cartridge.FeatureWorldMaterialLayersV1
	FrameCoverageMain     = "main"
	FrameCoverageOpaque   = "opaque"
	FrameCoverageMasked   = "masked"
	// Each wall has <=7 profile intervals, <=2 solid spans, <=2 band-overlap
	// branches, <=3 vertical columns and <=3 horizontal rows. Every cell is a quad.
	MaxFrameRegionsPerWall = 7 * 2 * 2 * 3 * 3
	MaxFrameRegionVertices = 4
	MaxFrameRegions        = 32768
)

type MaterialLayers struct {
	Version uint32 `json:"version"`
}

type SurfaceSecondary struct {
	Material uint32     `json:"material"`
	Strength float64    `json:"strength"`
	UV       *SurfaceUV `json:"uv"`
}

// WallFrameRegion is one complete, nonoverlapping wall partition, already
// selected by the builder. Vertices use X=edge fraction and Y=world elevation,
// ordered counterclockwise in this two-dimensional coordinate system.
// Main regions retain the wall's mapping and have no region UV. Opaque regions
// sample their sole material; masked regions blend over the wall's main material.
type WallFrameRegion struct {
	Vertices []Vec2     `json:"vertices"`
	Material uint32     `json:"material"`
	UV       *SurfaceUV `json:"uv,omitempty"`
	Coverage string     `json:"coverage"`
	RepeatU  bool       `json:"repeat_u,omitempty"`
	RepeatV  bool       `json:"repeat_v,omitempty"`
}

// MaterialIDs returns canonical deduplicated atlas order. All legacy primary
// references retain their original order, followed by secondary references,
// then non-main frame regions, in sector/floor/ceiling/wall order.
func MaterialIDs(d *Document) []uint32 {
	if d == nil {
		return nil
	}
	ids := make([]uint32, 0)
	seen := make(map[uint32]bool)
	add := func(id uint32) {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	for _, s := range d.Sectors {
		add(s.FloorMaterial)
		add(s.CeilingMaterial)
		for _, w := range s.Walls {
			add(w.Material)
		}
	}
	if d.StaticSolids != nil {
		for _, s := range d.StaticSolids.Items {
			add(s.SideMaterial)
			add(s.TopMaterial)
			add(s.BottomMaterial)
		}
	}
	for _, s := range d.Sectors {
		for _, layer := range []*SurfaceSecondary{s.FloorSecondary, s.CeilingSecondary} {
			if layer != nil {
				add(layer.Material)
			}
		}
		for _, w := range s.Walls {
			if w.Secondary != nil {
				add(w.Secondary.Material)
			}
		}
	}
	for _, s := range d.Sectors {
		for _, w := range s.Walls {
			for _, r := range w.FrameRegions {
				if r.Coverage != FrameCoverageMain {
					add(r.Material)
				}
			}
		}
	}
	return ids
}

func ValidateSurfaceSecondary(s *SurfaceSecondary) error {
	if s == nil || math.IsNaN(s.Strength) || math.IsInf(s.Strength, 0) || s.Strength < 0 || s.Strength > 1 {
		return ErrMaterialMapping
	}
	return ValidateSurfaceUV(s.UV)
}

func validateDocumentLayers(d *Document) error {
	declared := d.MaterialLayers != nil
	if declared && (d.Version != Version || d.MaterialLayers.Version != MaterialLayersVersion) {
		return ErrVersion
	}
	if declared && d.MaterialMapping == nil {
		return ErrMaterialMapping
	}
	total := 0
	for si := range d.Sectors {
		s := &d.Sectors[si]
		for _, layer := range []*SurfaceSecondary{s.FloorSecondary, s.CeilingSecondary} {
			if layer != nil {
				if !declared {
					return ErrMaterialMapping
				}
				if err := ValidateSurfaceSecondary(layer); err != nil {
					return err
				}
			}
		}
		for wi := range s.Walls {
			w := &s.Walls[wi]
			if w.Secondary != nil {
				if !declared {
					return ErrMaterialMapping
				}
				if err := ValidateSurfaceSecondary(w.Secondary); err != nil {
					return err
				}
			}
			if len(w.FrameRegions) == 0 && !w.FrameCompiled {
				continue
			}
			total += len(w.FrameRegions)
			if !declared || w.SourceEdge == "" {
				return ErrMaterialMapping
			}
			if len(w.FrameRegions) > MaxFrameRegionsPerWall || total > MaxFrameRegions {
				return ErrBounds
			}
			if err := validateFramePartition(d, si, wi); err != nil {
				return fmt.Errorf("sector %q wall %d frame: %w", s.ID, wi, err)
			}
		}
	}
	return nil
}

// ProfileForWall resolves destination endpoints and elevation translation using
// the compiled portal pair. Callers validate the world before invoking it.
func ProfileForWall(d *Document, sector, index int) (WallProfile, error) {
	if d == nil || sector < 0 || sector >= len(d.Sectors) || index < 0 || index >= len(d.Sectors[sector].Walls) {
		return WallProfile{}, ErrBounds
	}
	s := &d.Sectors[sector]
	w := s.Walls[index]
	heights := [4][2]float64{
		{planeHeight(s.Floor, w.Start), planeHeight(s.Floor, w.End)},
		{planeHeight(s.Ceiling, w.Start), planeHeight(s.Ceiling, w.End)},
	}
	heights[2], heights[3] = heights[0], heights[1]
	if w.Portal >= 0 {
		if int(w.Portal) >= len(d.Sectors) {
			return WallProfile{}, ErrPortal
		}
		dest := &d.Sectors[w.Portal]
		target := Wall{}
		found := false
		if w.PortalWall > 0 && int(w.PortalWall) <= len(dest.Walls) {
			target = dest.Walls[int(w.PortalWall)-1]
			found = true
		} else if w.PortalWall == 0 {
			for _, candidate := range dest.Walls {
				if candidate.Start == w.End && candidate.End == w.Start {
					target = candidate
					found = true
					break
				}
			}
		}
		if !found {
			return WallProfile{}, ErrPortal
		}
		dx, dy := target.Start.X-target.End.X, target.Start.Y-target.End.Y
		sx, sy := w.End.X-w.Start.X, w.End.Y-w.Start.Y
		denominator := dx*dx + dy*dy
		if denominator <= 0 {
			return WallProfile{}, ErrPortal
		}
		cos, sin := (dx*sx+dy*sy)/denominator, (dx*sy-dy*sx)/denominator
		tx, ty := w.Start.X-(cos*target.End.X-sin*target.End.Y), w.Start.Y-(sin*target.End.X+cos*target.End.Y)
		tz := 0.0
		if math.Abs(cos-1) > geometryEpsilon || math.Abs(sin) > geometryEpsilon || math.Abs(tx) > geometryEpsilon ||
			math.Abs(ty) > geometryEpsilon {
			sourceMid := Vec2{(w.Start.X + w.End.X) * .5, (w.Start.Y + w.End.Y) * .5}
			targetMid := Vec2{(target.Start.X + target.End.X) * .5, (target.Start.Y + target.End.Y) * .5}
			tz = planeHeight(s.Floor, sourceMid) - planeHeight(dest.Floor, targetMid)
		}
		heights[2] = [2]float64{planeHeight(dest.Floor, target.End) + tz, planeHeight(dest.Floor, target.Start) + tz}
		heights[3] = [2]float64{planeHeight(dest.Ceiling, target.End) + tz, planeHeight(dest.Ceiling, target.Start) + tz}
	}
	return NewWallProfile(heights, geometryEpsilon)
}
