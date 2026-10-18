package worldsource

import "github.com/karty-game/karty-sdk/format/world"

type Fluids struct {
	Version uint32  `yaml:"version"`
	Items   []Fluid `yaml:"items"`
}
type Fluid struct {
	ID           string   `yaml:"id"`
	Solids       []string `yaml:"solids"`
	Density      float64  `yaml:"density"`
	LinearDrag   float64  `yaml:"linear_drag"`
	AngularDrag  float64  `yaml:"angular_drag"`
	FoamWidth    float64  `yaml:"foam_width"`
	FoamStrength float64  `yaml:"foam_strength"`
	FoamColor    Vec3     `yaml:"foam_color"`
	FogDensity   float64  `yaml:"fog_density"`
	FogColor     Vec3     `yaml:"fog_color"`
	Distortion   float64  `yaml:"distortion"`
}

func CompileFluids(f *Fluids) *world.Fluids {
	if f == nil {
		return nil
	}
	result := &world.Fluids{Version: f.Version}
	for _, p := range f.Items {
		result.Items = append(
			result.Items,
			world.Fluid{
				ID:           p.ID,
				Solids:       append([]string(nil), p.Solids...),
				Density:      p.Density,
				LinearDrag:   p.LinearDrag,
				AngularDrag:  p.AngularDrag,
				FoamWidth:    p.FoamWidth,
				FoamStrength: p.FoamStrength,
				FoamColor:    world.Vec3(p.FoamColor),
				FogDensity:   p.FogDensity,
				FogColor:     world.Vec3(p.FogColor),
				Distortion:   p.Distortion,
			},
		)
	}
	return result
}
func validateFluids(d *Document) error {
	if d.Fluids == nil {
		return nil
	}
	if d.Version < FluidsVersion {
		return ErrVersion
	}
	if d.Fluids.Version != world.FluidsVersion {
		return ErrVersion
	}
	if len(d.Fluids.Items) == 0 || len(d.Fluids.Items) > world.MaxFluids {
		return ErrBounds
	}
	ids := map[string]bool{}
	for i := range d.Fluids.Items {
		p := CompileFluids(&Fluids{Items: []Fluid{d.Fluids.Items[i]}}).Items[0]
		if err := world.ValidateFluid(&p); err != nil {
			return ErrContent
		}
		if ids[p.ID] {
			return ErrIdentity
		}
		ids[p.ID] = true
	}
	return nil
}
