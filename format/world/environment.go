package world

import (
	"fmt"
	"math"
)

type EnvironmentPreset string

const (
	EnvironmentDaylight EnvironmentPreset = "daylight"
	EnvironmentSpace    EnvironmentPreset = "space"
	EnvironmentCartoon  EnvironmentPreset = "cartoon"
	EnvironmentFantasy  EnvironmentPreset = "fantasy"
)

type EnvironmentPalette string

const (
	EnvironmentPaletteDefault  EnvironmentPalette = "default"
	EnvironmentPaletteViolet   EnvironmentPalette = "violet"
	EnvironmentPaletteTwilight EnvironmentPalette = "twilight"
)

// EnvironmentBackdrop selects a decorative horizon layer. Empty selects the
// preset default; explicit none suppresses the layer. It adds no geometry/light.
type EnvironmentBackdrop string

const (
	EnvironmentBackdropNone      EnvironmentBackdrop = "none"
	EnvironmentBackdropMountains EnvironmentBackdrop = "mountains"
	EnvironmentBackdropCity      EnvironmentBackdrop = "city"
)

// Environment is the optional world/environment@1 payload, available in
// compiled-world v3. Preset defaults are fixed by this payload version. Palette
// affects sky appearance only, independently of surface illumination.
type Environment struct {
	Version     uint32
	Preset      EnvironmentPreset
	Palette     EnvironmentPalette
	Sun         *EnvironmentSun
	TimeOfDay   *float64
	CloudAmount *float64
	Backdrop    EnvironmentBackdrop
}

const (
	DefaultEnvironmentTimeOfDay   = 15.0
	DefaultEnvironmentCloudAmount = .35
)

// EnvironmentSettings is a complete live override of the clock and cloud
// coverage. TimeOfDay is hours in [0,24); CloudAmount is in [0,1]. A live clock
// overrides the authored fixed sun. Preset and palette remain authored.
type EnvironmentSettings struct {
	TimeOfDay   float64
	CloudAmount float64
}

// EnvironmentSun supplies a fixed global sun direction in degrees. Azimuth is
// [0,360), measured from +X toward +Y. Elevation is [0,90] above the XY plane.
// If present both angles are explicit; omission selects the preset defaults.
type EnvironmentSun struct {
	Azimuth   float64 `yaml:"azimuth"`
	Elevation float64 `yaml:"elevation"`
}

// ResolvedEnvironment supplies bounded linear RGB defaults to consumers without
// runtime allocation. SunDirection points toward the light; incoming rays travel
// in the opposite direction. AmbientCube is outdoor fill, not global indoor
// illumination. Consumers must restrict fill and sun visibility using sky
// openings and geometry. Decorative sky elements add no additional lights.
type ResolvedEnvironment struct {
	Preset       EnvironmentPreset
	Backdrop     EnvironmentBackdrop
	SkyZenith    Vec3
	SkyHorizon   Vec3
	SkyNadir     Vec3
	CloudColor   Vec3
	SunDiskColor Vec3
	SunDirection Vec3
	SunColor     Vec3
	SunIntensity float64
	Ambient      Vec3
	AmbientCube  AmbientCube
	TimeOfDay    float64
	CloudAmount  float64
}

func ValidateEnvironment(environment *Environment) error {
	if environment == nil {
		return ErrEnvironment
	}
	if environment.Version != EnvironmentVersion {
		return fmt.Errorf("environment: %w", ErrVersion)
	}
	switch environment.Preset {
	case EnvironmentDaylight, EnvironmentSpace, EnvironmentCartoon, EnvironmentFantasy:
	default:
		return fmt.Errorf("environment preset: %w", ErrEnvironment)
	}
	switch environment.Palette {
	case "", EnvironmentPaletteDefault, EnvironmentPaletteViolet, EnvironmentPaletteTwilight:
	default:
		return fmt.Errorf("environment palette: %w", ErrEnvironment)
	}
	switch environment.Backdrop {
	case "", EnvironmentBackdropNone, EnvironmentBackdropMountains, EnvironmentBackdropCity:
	default:
		return fmt.Errorf("environment backdrop: %w", ErrEnvironment)
	}
	if sun := environment.Sun; sun != nil {
		if !finiteBounded(sun.Azimuth) || !finiteBounded(sun.Elevation) ||
			sun.Azimuth < 0 || sun.Azimuth >= 360 || sun.Elevation < 0 || sun.Elevation > 90 {
			return fmt.Errorf("environment sun: %w", ErrEnvironment)
		}
	}
	if environment.TimeOfDay != nil {
		if environment.Sun != nil || !validEnvironmentTime(*environment.TimeOfDay) {
			return fmt.Errorf("environment time of day: %w", ErrEnvironment)
		}
	}
	if environment.CloudAmount != nil && !validEnvironmentClouds(*environment.CloudAmount) {
		return fmt.Errorf("environment cloud amount: %w", ErrEnvironment)
	}
	return nil
}

func ValidateEnvironmentSettings(settings EnvironmentSettings) error {
	if !validEnvironmentTime(settings.TimeOfDay) || !validEnvironmentClouds(settings.CloudAmount) {
		return ErrEnvironment
	}
	return nil
}

func validEnvironmentTime(value float64) bool {
	return finiteBounded(value) && value >= 0 && value < 24
}

func validEnvironmentClouds(value float64) bool {
	return finiteBounded(value) && value >= 0 && value <= 1
}

// ResolveEnvironment validates and resolves the versioned preset. Palette
// overrides never modify the sun or ambient light defaults. Calling this at
// world preparation keeps preset work outside the render loop.
func ResolveEnvironment(environment *Environment) (ResolvedEnvironment, error) {
	if err := ValidateEnvironment(environment); err != nil {
		return ResolvedEnvironment{}, err
	}
	result := ResolvedEnvironment{
		Preset:       environment.Preset,
		Backdrop:     EnvironmentBackdropMountains,
		SunColor:     Vec3{X: 1, Y: .94, Z: .82},
		SunIntensity: .72,
		TimeOfDay:    DefaultEnvironmentTimeOfDay,
		CloudAmount:  DefaultEnvironmentCloudAmount,
	}
	sun := EnvironmentSun{Azimuth: 135, Elevation: 40}
	switch environment.Preset {
	case EnvironmentDaylight:
		result.SkyZenith, result.SkyHorizon, result.SkyNadir = Vec3{
			X: .055,
			Y: .22,
			Z: .56,
		}, Vec3{
			X: .57,
			Y: .72,
			Z: .83,
		}, Vec3{
			X: .12,
			Y: .16,
			Z: .2,
		}
		result.CloudColor, result.SunDiskColor = Vec3{X: .94, Y: .96, Z: 1}, Vec3{X: 1, Y: .96, Z: .78}
		result.Ambient = Vec3{X: .10, Y: .13, Z: .18}
	case EnvironmentSpace:
		result.Backdrop = EnvironmentBackdropCity
		sun.Elevation = 25
		result.SkyZenith, result.SkyHorizon, result.SkyNadir = Vec3{
			X: .004,
			Y: .008,
			Z: .025,
		}, Vec3{
			X: .025,
			Y: .035,
			Z: .085,
		}, Vec3{
			X: .002,
			Y: .004,
			Z: .012,
		}
		result.CloudColor, result.SunDiskColor = Vec3{X: .14, Y: .07, Z: .28}, Vec3{X: .88, Y: .94, Z: 1}
		result.SunColor, result.SunIntensity = Vec3{X: .86, Y: .93, Z: 1}, .85
		result.Ambient = Vec3{X: .025, Y: .03, Z: .05}
	case EnvironmentCartoon:
		result.Backdrop = EnvironmentBackdropCity
		sun.Elevation = 50
		result.SkyZenith, result.SkyHorizon, result.SkyNadir = Vec3{
			X: .06,
			Y: .42,
			Z: .8,
		}, Vec3{
			X: .45,
			Y: .78,
			Z: 1,
		}, Vec3{
			X: .16,
			Y: .35,
			Z: .48,
		}
		result.CloudColor, result.SunDiskColor = Vec3{X: 1, Y: 1, Z: 1}, Vec3{X: 1, Y: .88, Z: .3}
		result.SunColor, result.SunIntensity = Vec3{X: 1, Y: .95, Z: .82}, .58
		result.Ambient = Vec3{X: .23, Y: .26, Z: .30}
	case EnvironmentFantasy:
		sun.Elevation = 32
		result.SkyZenith, result.SkyHorizon, result.SkyNadir = Vec3{
			X: .11,
			Y: .08,
			Z: .32,
		}, Vec3{
			X: .69,
			Y: .35,
			Z: .47,
		}, Vec3{
			X: .08,
			Y: .07,
			Z: .16,
		}
		result.CloudColor, result.SunDiskColor = Vec3{X: .78, Y: .61, Z: .82}, Vec3{X: 1, Y: .77, Z: .56}
		result.SunColor, result.SunIntensity = Vec3{X: 1, Y: .79, Z: .63}, .68
		result.Ambient = Vec3{X: .13, Y: .11, Z: .20}
	}
	switch environment.Palette {
	case EnvironmentPaletteViolet:
		result.SkyZenith, result.SkyHorizon, result.SkyNadir = Vec3{
			X: .14,
			Y: .035,
			Z: .30,
		}, Vec3{
			X: .62,
			Y: .30,
			Z: .69,
		}, Vec3{
			X: .065,
			Y: .025,
			Z: .12,
		}
		result.CloudColor, result.SunDiskColor = Vec3{X: .84, Y: .64, Z: .93}, Vec3{X: 1, Y: .80, Z: .90}
	case EnvironmentPaletteTwilight:
		result.SkyZenith, result.SkyHorizon, result.SkyNadir = Vec3{
			X: .035,
			Y: .045,
			Z: .16,
		}, Vec3{
			X: .85,
			Y: .31,
			Z: .16,
		}, Vec3{
			X: .045,
			Y: .04,
			Z: .07,
		}
		result.CloudColor, result.SunDiskColor = Vec3{X: .74, Y: .40, Z: .39}, Vec3{X: 1, Y: .62, Z: .33}
	}
	if environment.Backdrop != "" {
		result.Backdrop = environment.Backdrop
	}
	if environment.Sun != nil {
		sun = *environment.Sun
	}
	azimuth, elevation := sun.Azimuth*math.Pi/180, sun.Elevation*math.Pi/180
	result.SunDirection = Vec3{
		X: math.Cos(azimuth) * math.Cos(elevation),
		Y: math.Sin(azimuth) * math.Cos(elevation),
		Z: math.Sin(elevation),
	}
	result.AmbientCube = AmbientCube{
		PositiveX: result.Ambient, NegativeX: result.Ambient, PositiveY: result.Ambient, NegativeY: result.Ambient,
		PositiveZ: environmentScale(result.Ambient, 1.35), NegativeZ: environmentScale(result.Ambient, .35),
	}
	if environment.CloudAmount != nil {
		result.CloudAmount = *environment.CloudAmount
	}
	if environment.TimeOfDay != nil {
		resolveEnvironmentTime(&result, *environment.TimeOfDay)
	}
	return result, nil
}

// ResolveEnvironmentSettings applies a validated live clock to the authored
// style. It allocates no memory and never changes the authored payload.
func ResolveEnvironmentSettings(environment *Environment, settings EnvironmentSettings) (ResolvedEnvironment, error) {
	if err := ValidateEnvironment(environment); err != nil {
		return ResolvedEnvironment{}, err
	}
	if err := ValidateEnvironmentSettings(settings); err != nil {
		return ResolvedEnvironment{}, err
	}
	// Resolve from the preset palette once, rather than applying night scaling
	// twice when the source already supplies a clock.
	authored := *environment
	authored.TimeOfDay = nil
	result, err := ResolveEnvironment(&authored)
	if err != nil {
		return ResolvedEnvironment{}, err
	}
	resolveEnvironmentTime(&result, settings.TimeOfDay)
	result.CloudAmount = settings.CloudAmount
	return result, nil
}

func resolveEnvironmentTime(result *ResolvedEnvironment, hours float64) {
	peakElevation := 40.0
	switch result.Preset {
	case EnvironmentSpace:
		peakElevation = 25
	case EnvironmentCartoon:
		peakElevation = 50
	case EnvironmentFantasy:
		peakElevation = 32
	}
	phase := (hours - 6) * math.Pi / 12
	height := math.Sin(phase)
	if math.Abs(height) < 1e-12 {
		height = 0
	}
	azimuth := (hours - 6) * math.Pi / 12
	elevation := peakElevation * math.Sqrt2 * height * math.Pi / 180
	result.SunDirection = Vec3{
		X: math.Cos(azimuth) * math.Cos(elevation),
		Y: math.Sin(azimuth) * math.Cos(elevation),
		Z: math.Sin(elevation),
	}
	daylight := environmentSmoothstep(0, .35, height)
	sky := .03 + .97*environmentSmoothstep(-.15, .30, height)
	warmth := .65 * (1 - environmentSmoothstep(.05, .65, height)) * environmentSmoothstep(-.15, .05, height)
	warmHorizon := Vec3{X: math.Max(result.SkyHorizon.X, .75), Y: result.SkyHorizon.Y * .50, Z: result.SkyHorizon.Z * .35}
	result.SkyHorizon = environmentMix(result.SkyHorizon, warmHorizon, warmth)
	warmClouds := Vec3{X: result.CloudColor.X, Y: result.CloudColor.Y * .72, Z: result.CloudColor.Z * .52}
	result.CloudColor = environmentMix(result.CloudColor, warmClouds, warmth)
	result.SunIntensity *= daylight
	result.SunColor.Y *= .58 + .42*daylight
	result.SunColor.Z *= .32 + .68*daylight
	result.SunDiskColor = environmentScale(result.SunDiskColor, daylight)
	result.SkyZenith = environmentScale(result.SkyZenith, sky)
	result.SkyHorizon = environmentScale(result.SkyHorizon, sky)
	result.SkyNadir = environmentScale(result.SkyNadir, sky)
	result.CloudColor = environmentScale(result.CloudColor, .08+.92*sky)
	fill := .15 + .85*sky
	result.Ambient = environmentScale(result.Ambient, fill)
	result.AmbientCube = AmbientCube{
		PositiveX: result.Ambient, NegativeX: result.Ambient, PositiveY: result.Ambient, NegativeY: result.Ambient,
		PositiveZ: environmentScale(result.Ambient, 1.35), NegativeZ: environmentScale(result.Ambient, .35),
	}
	result.TimeOfDay = hours
}

func environmentSmoothstep(low, high, value float64) float64 {
	fraction := math.Max(0, math.Min(1, (value-low)/(high-low)))
	return fraction * fraction * (3 - 2*fraction)
}

func environmentMix(a, b Vec3, amount float64) Vec3 {
	return Vec3{X: a.X + (b.X-a.X)*amount, Y: a.Y + (b.Y-a.Y)*amount, Z: a.Z + (b.Z-a.Z)*amount}
}

func environmentScale(color Vec3, scale float64) Vec3 {
	return Vec3{X: color.X * scale, Y: color.Y * scale, Z: color.Z * scale}
}

func validateDocumentEnvironment(document *Document) error {
	usesSky := false
	for _, sector := range document.Sectors {
		usesSky = usesSky || sector.Sky
		for _, wall := range sector.Walls {
			if wall.Sky && (!sector.Sky || wall.Portal != -1) {
				return fmt.Errorf("sky boundary requires sky sector and opaque wall: %w", ErrEnvironment)
			}
		}
	}
	if document.Environment == nil {
		if usesSky {
			return fmt.Errorf("sky ceiling requires environment: %w", ErrEnvironment)
		}
		return nil
	}
	if document.Version != Version {
		return fmt.Errorf("environment requires compiled world version %d: %w", Version, ErrVersion)
	}
	return ValidateEnvironment(document.Environment)
}
