package world

import "github.com/karty-game/karty-sdk/format/cartridge"

const (
	FeatureFluids  = cartridge.FeatureWorldFluidsV1
	FluidsVersion  = uint32(1)
	MaxFluids      = 8
	MaxFluidSolids = 64
)

// Fluids explicitly opt non-colliding horizontal solid volumes into physics
// and camera-medium effects. One item owns all adjoining slabs of one pool.
type Fluids struct {
	Version uint32
	Items   []Fluid
}
type Fluid struct {
	ID                               string
	Solids                           []string
	Density, LinearDrag, AngularDrag float64
	FoamWidth, FoamStrength          float64
	FoamColor                        Vec3
	FogDensity                       float64
	FogColor                         Vec3
	Distortion                       float64
}

func ValidateFluid(f *Fluid) error {
	if f == nil || !validIdentifier(f.ID) {
		return ErrIdentity
	}
	if len(f.Solids) == 0 || len(f.Solids) > MaxFluidSolids {
		return ErrBounds
	}
	for _, v := range []struct{ value, low, high float64 }{
		{f.Density, .001, 20000}, {f.LinearDrag, 0, 30}, {f.AngularDrag, 0, 30},
		{f.FoamWidth, 0, 1}, {f.FoamStrength, 0, 1}, {f.FogDensity, 0, 10}, {f.Distortion, 0, .02},
		{f.FoamColor.X, 0, 1}, {f.FoamColor.Y, 0, 1}, {f.FoamColor.Z, 0, 1},
		{f.FogColor.X, 0, 1}, {f.FogColor.Y, 0, 1}, {f.FogColor.Z, 0, 1},
	} {
		if !finiteBounded(v.value) || v.value < v.low || v.value > v.high {
			return ErrBounds
		}
	}
	seen := map[string]bool{}
	for _, id := range f.Solids {
		if !validIdentifier(id) || seen[id] {
			return ErrIdentity
		}
		seen[id] = true
	}
	return nil
}

func ValidateFluids(d *Document) error {
	if d == nil {
		return ErrSyntax
	}
	if d.Fluids == nil {
		return nil
	}
	if d.Version != Version || d.Fluids.Version != FluidsVersion {
		return ErrVersion
	}
	if len(d.Fluids.Items) == 0 || len(d.Fluids.Items) > MaxFluids {
		return ErrBounds
	}
	if d.StaticSolids == nil || d.Animations == nil {
		return ErrContent
	}
	solids := map[string]*Solid{}
	for i := range d.StaticSolids.Items {
		s := &d.StaticSolids.Items[i]
		solids[s.ID] = s
	}
	used := map[string]bool{}
	owners := map[string]string{}
	ids := map[string]bool{}
	materials := map[uint32]string{}
	for i := range d.Fluids.Items {
		f := &d.Fluids.Items[i]
		if err := ValidateFluid(f); err != nil {
			return err
		}
		if ids[f.ID] {
			return ErrIdentity
		}
		ids[f.ID] = true
		height := 0.0
		for j, id := range f.Solids {
			s := solids[id]
			if s == nil || used[id] || s.Collision || s.Top.A != 0 || s.Top.B != 0 || s.Bottom.A != 0 || s.Bottom.B != 0 {
				return ErrContent
			}
			for other, owner := range owners {
				if owner != f.ID && fluidVolumesOverlap(s, solids[other]) {
					return ErrGeometry
				}
			}
			owners[id] = f.ID
			used[id] = true
			if j == 0 {
				height = s.Top.C
			}
			if s.Top.C != height {
				return ErrGeometry
			}
			if owner := materials[s.TopMaterial]; owner != "" && owner != f.ID {
				return ErrContent
			}
			materials[s.TopMaterial] = f.ID
			liquid := false
			for _, b := range d.Animations.Materials {
				if b.Material == s.TopMaterial {
					for _, p := range d.Animations.Presets {
						liquid = liquid || (b.Preset == p.Name && p.Kind == "liquid")
					}
				}
			}
			if !liquid {
				return ErrContent
			}
		}
	}
	return nil
}

// Different media cannot occupy the same volume. Adjoining slabs within one
// medium remain a union; touching edges and vertically separated pools are valid.
func fluidVolumesOverlap(a, b *Solid) bool {
	if a.Top.C <= b.Bottom.C || b.Top.C <= a.Bottom.C {
		return false
	}
	separated := func(poly, other []Vec2) bool {
		for i, v := range poly {
			n := poly[(i+1)%len(poly)]
			outside := true
			for _, p := range other {
				if (n.X-v.X)*(p.Y-v.Y)-(n.Y-v.Y)*(p.X-v.X) > geometryEpsilon {
					outside = false
					break
				}
			}
			if outside {
				return true
			}
		}
		return false
	}
	return !separated(a.Footprint, b.Footprint) && !separated(b.Footprint, a.Footprint)
}
