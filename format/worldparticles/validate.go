package worldparticles

import (
	"fmt"
	"math"
	"slices"
	"unicode/utf8"

	"github.com/karty-game/karty-sdk/format/world"
)

func identifier(s string) bool {
	return len(s) > 0 && len(s) <= world.MaxIdentifierBytes && utf8.ValidString(s)
}
func finite(x float64) bool         { return !math.IsNaN(x) && !math.IsInf(x, 0) }
func number(x, lo, hi float64) bool { return finite(x) && x >= lo && x <= hi }

// Some graphics backends flush float32 subnormals to zero. Interpolation
// denominators and animated frame rates must remain positive on those backends.
const minNormalFloat32 = 0x1p-126

func vector(v world.Vec3, bound float64) bool {
	return number(v.X, -bound, bound) && number(v.Y, -bound, bound) && number(v.Z, -bound, bound)
}
func validRange(r Range, lo, hi float64) bool {
	return number(r.Min, lo, hi) && number(r.Max, r.Min, hi)
}

// Validate checks all resources, references, fixed-slot scheduling and derived
// bounds before allocation by a host. Geometry confinement is checked separately.
func Validate(d *Document) error {
	if d == nil {
		return ErrVersion
	}
	if _, err := FeatureForVersion(d.Version); err != nil {
		return err
	}
	if len(d.Presets) == 0 || len(d.Presets) > MaxPresets || len(d.Emitters) == 0 || len(d.Emitters) > MaxEmitters {
		return ErrBounds
	}
	presets := make(map[string]*Preset, len(d.Presets))
	for i := range d.Presets {
		p := &d.Presets[i]
		if err := ValidatePresetForVersion(p, d.Version); err != nil {
			return fmt.Errorf("preset %q: %w", p.Name, err)
		}
		if presets[p.Name] != nil {
			return ErrIdentity
		}
		presets[p.Name] = p
	}
	ids := make(map[string]bool, len(d.Emitters))
	total := uint64(0)
	for _, e := range d.Emitters {
		p := presets[e.Preset]
		if !identifier(e.ID) || ids[e.ID] || p == nil {
			return ErrIdentity
		}
		ids[e.ID] = true
		if !vector(e.Position, world.MaxCoordinate) || !number(e.YawDegrees, -180, 180) || !number(e.Scale, 0.000001, 1000) {
			return ErrParticle
		}
		required, err := RequiredCapacity(*p, e.Playback)
		if err != nil {
			return fmt.Errorf("emitter %q: %w", e.ID, err)
		}
		if required > p.Capacity {
			return fmt.Errorf("emitter %q needs %d slots: %w", e.ID, required, ErrBounds)
		}
		total += uint64(p.Capacity)
		if total > MaxTotalCapacity {
			return ErrBounds
		}
		expected := ConservativeEmitterBounds(*p, e)
		if e.Bounds != expected || !vector(e.Bounds.Min, world.MaxCoordinate) || !vector(e.Bounds.Max, world.MaxCoordinate) {
			return ErrBounds
		}
	}
	return nil
}

func ValidatePreset(p *Preset) error {
	return ValidatePresetForVersion(p, Version)
}

// ValidatePresetForVersion preserves strict version-1 blend validation while
// version 2 additionally accepts alpha and premultiplied particle materials.
func ValidatePresetForVersion(p *Preset, version uint16) error {
	if _, err := FeatureForVersion(version); err != nil {
		return err
	}
	if p == nil || !identifier(p.Name) {
		return ErrIdentity
	}
	if p.Capacity == 0 || p.Capacity > MaxCapacity || len(p.Emission.Bursts) > MaxBursts || len(p.Sprite.Frames) == 0 ||
		len(p.Sprite.Frames) > MaxFrames {
		return ErrBounds
	}
	if !validRange(p.LifetimeSeconds, 0.001, MaxSeconds) || !vector(p.VelocityMin, MaxVelocity) || !vector(p.VelocityMax, MaxVelocity) ||
		!vector(p.Acceleration, MaxAcceleration) ||
		!number(p.Drag, 0, 1000) {
		return ErrParticle
	}
	if p.VelocityMin.X > p.VelocityMax.X || p.VelocityMin.Y > p.VelocityMax.Y || p.VelocityMin.Z > p.VelocityMax.Z {
		return ErrParticle
	}
	h := p.Spawn.HalfExtent
	if !vector(h, world.MaxCoordinate) || h.X < 0 || h.Y < 0 || h.Z < 0 || !slices.Contains([]string{"point", "box"}, p.Spawn.Shape) ||
		p.Spawn.Shape == "point" && h != (world.Vec3{}) {
		return ErrParticle
	}
	if !number(p.Sprite.FPS, 0, 1000) || len(p.Sprite.Frames) > 1 && float32(p.Sprite.FPS) < minNormalFloat32 ||
		!slices.Contains([]string{"once", "cycle"}, p.Sprite.Playback) ||
		!slices.Contains([]string{"camera-facing", "upright"}, p.Sprite.Facing) ||
		!slices.Contains([]string{"additive", "cutout"}, p.Sprite.Blend) &&
			(version != AlphaVersion || !slices.Contains([]string{"alpha", "premultiplied"}, p.Sprite.Blend)) {
		return ErrParticle
	}
	for _, id := range p.Sprite.Frames {
		if id == 0 {
			return ErrIdentity
		}
	}
	if !validRange(p.RotationDegrees, -360000, 360000) || !validRange(p.RotationSpeedDegrees, -360000, 360000) {
		return ErrParticle
	}
	if len(p.SizeOverLife) < 2 || len(p.SizeOverLife) > MaxCurveKeys || len(p.ColorOverLife) < 2 || len(p.ColorOverLife) > MaxCurveKeys {
		return ErrBounds
	}
	for i, k := range p.SizeOverLife {
		if !number(k.Age, 0, 1) || !number(k.Value, 0, MaxSize) ||
			i > 0 && float32(k.Age)-float32(p.SizeOverLife[i-1].Age) < minNormalFloat32 {
			return ErrParticle
		}
	}
	for i, k := range p.ColorOverLife {
		if !number(k.Age, 0, 1) || i > 0 && float32(k.Age)-float32(p.ColorOverLife[i-1].Age) < minNormalFloat32 {
			return ErrParticle
		}
		for _, c := range k.RGBA {
			if !number(c, 0, 1) {
				return ErrParticle
			}
		}
	}
	if p.SizeOverLife[0].Age != 0 || p.SizeOverLife[len(p.SizeOverLife)-1].Age != 1 || p.ColorOverLife[0].Age != 0 ||
		p.ColorOverLife[len(p.ColorOverLife)-1].Age != 1 {
		return ErrParticle
	}
	_, err := BirthSchedule(*p)
	return err
}

// BirthSchedule returns a sorted finite schedule shared by all emitter cycles.
// Rate births occur at n/rate in [0,duration). Bursts are additional births.
func BirthSchedule(p Preset) ([]float32, error) {
	e := p.Emission
	if !number(e.Rate, 0, MaxRate) || !number(e.DurationSeconds, 0, MaxSeconds) || e.Rate > 0 && e.DurationSeconds <= 0 ||
		len(e.Bursts) > MaxBursts {
		return nil, ErrParticle
	}
	count := 0.0
	if e.Rate > 0 {
		count = max(1, math.Ceil(e.Rate*e.DurationSeconds))
		// Multiplication and division can round differently at the excluded
		// endpoint. Count the actual n/rate offsets, including a birth at zero
		// when a tiny positive rate*duration underflows.
		for count > 1 && (count-1)/e.Rate >= e.DurationSeconds {
			count--
		}
		for count/e.Rate < e.DurationSeconds {
			count++
		}
	}
	if !number(count, 0, MaxCapacity) {
		return nil, ErrBounds
	}
	total := uint64(count)
	for _, b := range e.Bursts {
		if !number(b.AtSeconds, 0, MaxSeconds) || b.Count == 0 {
			return nil, ErrParticle
		}
		total += uint64(b.Count)
		if total > MaxCapacity {
			return nil, ErrBounds
		}
	}
	if total == 0 {
		return nil, ErrParticle
	}
	births := make([]float32, 0, int(total))
	for i := range int(count) {
		births = append(births, float32(float64(i)/e.Rate))
	}
	for _, b := range e.Bursts {
		for range b.Count {
			births = append(births, float32(b.AtSeconds))
		}
	}
	slices.Sort(births)
	return births, nil
}

// RequiredCapacity reserves complete schedule groups. A cycling slot is reused
// only after groups*period >= maximum lifetime, so surviving particles never die
// because of capacity pressure. Inactive spare slots are allowed.
func RequiredCapacity(p Preset, playback Playback) (uint32, error) {
	if !validRange(p.LifetimeSeconds, 0.001, MaxSeconds) {
		return 0, ErrParticle
	}
	if !number(playback.DelaySeconds, 0, MaxSeconds) {
		return 0, ErrParticle
	}
	births, err := BirthSchedule(p)
	if err != nil {
		return 0, err
	}
	groups := 1.0
	switch playback.Mode {
	case "once":
		if playback.PeriodSeconds != 0 {
			return 0, ErrParticle
		}
	case "cycle":
		if !number(playback.PeriodSeconds, 0.001, MaxSeconds) || p.Emission.DurationSeconds > playback.PeriodSeconds ||
			births[len(births)-1] >= float32(playback.PeriodSeconds) {
			return 0, ErrParticle
		}
		groups = max(1, math.Ceil(float64(float32(p.LifetimeSeconds.Max))/float64(float32(playback.PeriodSeconds))))
	default:
		return 0, ErrParticle
	}
	required := float64(len(births)) * groups
	if required > MaxCapacity {
		return 0, ErrBounds
	}
	return uint32(required), nil
}

// ConservativeBounds returns the full identity-transform motion envelope.
func ConservativeBounds(p Preset, position world.Vec3) Bounds {
	return ConservativeEmitterBounds(p, Emitter{Position: position, Scale: 1})
}

// ConservativeEmitterBounds includes spawn support, exact analytical axis
// extrema with linear drag, emitter yaw/scale, and every billboard rotation.
func ConservativeEmitterBounds(p Preset, e Emitter) Bounds {
	extent := p.Spawn.HalfExtent
	mins := [3]float64{p.VelocityMin.X, p.VelocityMin.Y, p.VelocityMin.Z}
	maxs := [3]float64{p.VelocityMax.X, p.VelocityMax.Y, p.VelocityMax.Z}
	acc := [3]float64{p.Acceleration.X, p.Acceleration.Y, p.Acceleration.Z}
	half := [3]float64{extent.X, extent.Y, extent.Z}
	lo, hi := [3]float64{}, [3]float64{}
	size := 0.0
	for _, k := range p.SizeOverLife {
		size = max(size, k.Value)
	}
	radius := size / math.Sqrt2
	for axis := range 3 {
		lo[axis], _ = axisBounds(mins[axis], acc[axis], p.Drag, p.LifetimeSeconds.Max)
		_, hi[axis] = axisBounds(maxs[axis], acc[axis], p.Drag, p.LifetimeSeconds.Max)
		lo[axis] -= half[axis]
		hi[axis] += half[axis]
	}
	sine, cosine := math.Sincos(e.YawDegrees * math.Pi / 180)
	xlo, xhi := linearBounds(cosine, lo[0], hi[0], -sine, lo[1], hi[1])
	ylo, yhi := linearBounds(sine, lo[0], hi[0], cosine, lo[1], hi[1])
	scale := e.Scale
	bound := Bounds{
		Min: world.Vec3{X: e.Position.X + scale*(xlo-radius), Y: e.Position.Y + scale*(ylo-radius), Z: e.Position.Z + scale*(lo[2]-radius)},
		Max: world.Vec3{X: e.Position.X + scale*(xhi+radius), Y: e.Position.Y + scale*(yhi+radius), Z: e.Position.Z + scale*(hi[2]+radius)},
	}
	magnitude := max(
		1,
		math.Abs(bound.Min.X),
		math.Abs(bound.Min.Y),
		math.Abs(bound.Min.Z),
		math.Abs(bound.Max.X),
		math.Abs(bound.Max.Y),
		math.Abs(bound.Max.Z),
		scale*motionMagnitude(p),
	)
	padding := 8*(1.0/(1<<23))*magnitude + 1e-5*scale*motionMagnitude(p)
	bound.Min.X -= padding
	bound.Min.Y -= padding
	bound.Min.Z -= padding
	bound.Max.X += padding
	bound.Max.Y += padding
	bound.Max.Z += padding
	return bound
}
func motionMagnitude(p Preset) float64 {
	t := p.LifetimeSeconds.Max
	return max(
		math.Abs(p.VelocityMin.X),
		math.Abs(p.VelocityMin.Y),
		math.Abs(p.VelocityMin.Z),
		math.Abs(p.VelocityMax.X),
		math.Abs(p.VelocityMax.Y),
		math.Abs(p.VelocityMax.Z),
	)*t + max(
		math.Abs(p.Acceleration.X),
		math.Abs(p.Acceleration.Y),
		math.Abs(p.Acceleration.Z),
	)*t*t*.5
}

// CycleGroups returns the immutable group count using the actual float32
// lifetime/period sent to shaders. Extra groups prevent rounded slot collisions.
func CycleGroups(p Preset, playback Playback) (uint32, error) {
	capacity, err := RequiredCapacity(p, playback)
	if err != nil {
		return 0, err
	}
	births, err := BirthSchedule(p)
	if err != nil {
		return 0, err
	}
	return capacity / uint32(len(births)), nil
}
func linearBounds(a, loA, hiA, b, loB, hiB float64) (float64, float64) {
	return min(a*loA, a*hiA) + min(b*loB, b*hiB), max(a*loA, a*hiA) + max(b*loB, b*hiB)
}
func axisBounds(v, a, drag, t float64) (float64, float64) {
	displacement := func(age float64) float64 {
		q := drag * age
		if q < .5 {
			f := age * (1 + q*(-.5+q*(1.0/6+q*(-1.0/24+q*(1.0/120-q/720)))))
			g := age * age * (.5 + q*(-1.0/6+q*(1.0/24+q*(-1.0/120+q*(1.0/720-q/5040)))))
			return v*f + a*g
		}
		f := -math.Expm1(-drag*age) / drag
		return v*f + a*(age-f)/drag
	}
	end := displacement(t)
	lo, hi := min(0, end), max(0, end)
	apex := -1.0
	if drag == 0 {
		if a != 0 {
			apex = -v / a
		}
	} else if a-drag*v != 0 {
		ratio := a / (a - drag*v)
		if ratio > 0 && ratio < 1 {
			apex = -math.Log(ratio) / drag
		}
	}
	if apex > 0 && apex < t {
		value := displacement(apex)
		lo = min(lo, value)
		hi = max(hi, value)
	}
	return lo, hi
}

// ValidateForWorld rejects an emitter whose complete motion and billboard
// envelope crosses a compiled convex sector, including sloped floor/ceiling.
func ValidateForWorld(d *Document, w *world.Document) error {
	if err := Validate(d); err != nil {
		return err
	}
	if w == nil {
		return ErrConfinement
	}
	for _, e := range d.Emitters {
		if uint64(e.Sector) >= uint64(len(w.Sectors)) || !BoundsInSector(e.Bounds, &w.Sectors[e.Sector]) {
			return fmt.Errorf("emitter %q: %w", e.ID, ErrConfinement)
		}
	}
	return nil
}

// BoundsInSector checks all eight corners against the convex sector planes.
func BoundsInSector(b Bounds, s *world.Sector) bool {
	if s == nil || len(s.Walls) < 3 || !vector(b.Min, world.MaxCoordinate) || !vector(b.Max, world.MaxCoordinate) || b.Min.X > b.Max.X ||
		b.Min.Y > b.Max.Y ||
		b.Min.Z > b.Max.Z {
		return false
	}
	const epsilon = 1e-7
	for _, x := range []float64{b.Min.X, b.Max.X} {
		for _, y := range []float64{b.Min.Y, b.Max.Y} {
			for _, z := range []float64{b.Min.Z, b.Max.Z} {
				if z < s.Floor.A*x+s.Floor.B*y+s.Floor.C-epsilon || z > s.Ceiling.A*x+s.Ceiling.B*y+s.Ceiling.C+epsilon {
					return false
				}
				for _, wall := range s.Walls {
					cross := (wall.End.X-wall.Start.X)*(y-wall.Start.Y) - (wall.End.Y-wall.Start.Y)*(x-wall.Start.X)
					length := math.Hypot(wall.End.X-wall.Start.X, wall.End.Y-wall.Start.Y)
					if cross < -epsilon*length {
						return false
					}
				}
			}
		}
	}
	return true
}
