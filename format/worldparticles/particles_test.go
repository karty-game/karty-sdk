package worldparticles_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldparticles"
)

func fixture() worldparticles.Document {
	p := worldparticles.Preset{
		Name:            "spark",
		Capacity:        8,
		Emission:        worldparticles.Emission{Rate: 4, DurationSeconds: 1},
		LifetimeSeconds: worldparticles.Range{Min: .5, Max: 1.5},
		Spawn:           worldparticles.Spawn{Shape: "box", HalfExtent: world.Vec3{X: .1, Y: .2, Z: .05}},
		VelocityMin:     world.Vec3{X: -.1, Z: .3},
		VelocityMax:     world.Vec3{X: .1, Z: .5},
		Acceleration:    world.Vec3{Z: -.1},
		Drag:            .2,
		Sprite: worldparticles.Sprite{
			Frames:   []uint32{17, 18},
			FPS:      16,
			Playback: "cycle",
			Facing:   "camera-facing",
			Blend:    "additive",
		},
		SizeOverLife:  []worldparticles.ScalarKey{{Age: 0, Value: .1}, {Age: 1, Value: 0}},
		ColorOverLife: []worldparticles.ColorKey{{Age: 0, RGBA: [4]float64{.3, 1, .2, 1}}, {Age: 1, RGBA: [4]float64{0, 1, 0, 0}}},
	}
	e := worldparticles.Emitter{
		ID:       "pool",
		Preset:   p.Name,
		Position: world.Vec3{X: 4, Y: 4, Z: 1},
		Scale:    1,
		Seed:     17,
		Playback: worldparticles.Playback{Mode: "cycle", PeriodSeconds: 1},
	}
	e.Bounds = worldparticles.ConservativeEmitterBounds(p, e)
	return worldparticles.Document{Version: 1, Presets: []worldparticles.Preset{p}, Emitters: []worldparticles.Emitter{e}}
}
func TestBinaryParticlesCanonicalAndBounded(t *testing.T) {
	d := fixture()
	data, err := worldparticles.Encode(d)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := worldparticles.Decode(data)
	if err != nil || !reflect.DeepEqual(d, decoded) {
		t.Fatalf("roundtrip: %v %+v", err, decoded)
	}
	again, err := worldparticles.Encode(decoded)
	if err != nil || !bytes.Equal(data, again) {
		t.Fatal("noncanonical roundtrip")
	}
	for end := range len(data) {
		if _, err := worldparticles.Decode(data[:end]); err == nil {
			t.Fatalf("accepted truncation %d", end)
		}
	}
	bad := bytes.Clone(data)
	binary.LittleEndian.PutUint32(bad[18:22], math.MaxUint32)
	if _, err := worldparticles.Decode(bad); !errors.Is(err, worldparticles.ErrBounds) {
		t.Fatalf("unbounded preset count: %v", err)
	}
	if n := testing.AllocsPerRun(20, func() { _, _ = worldparticles.Decode(bad) }); n > 1 {
		t.Fatalf("malformed preflight allocated %.1f", n)
	}
	for _, bad := range [][]byte{append(bytes.Clone(data), 0), []byte(`{"version":1}`)} {
		if _, err := worldparticles.Decode(bad); err == nil {
			t.Fatal("accepted trailing/JSON bytes")
		}
	}
	bad = bytes.Clone(data)
	bad[4] = 2
	if _, err := worldparticles.Decode(bad); !errors.Is(err, worldparticles.ErrVersion) {
		t.Fatalf("wire version: %v", err)
	}
}
func TestSchedulingBoundsAndOverlap(t *testing.T) {
	t.Parallel()
	d := fixture()
	p := d.Presets[0]
	births, err := worldparticles.BirthSchedule(p)
	if err != nil || !reflect.DeepEqual(births, []float32{0, .25, .5, .75}) {
		t.Fatalf("schedule=%v err=%v", births, err)
	}
	count, err := worldparticles.RequiredCapacity(p, d.Emitters[0].Playback)
	if err != nil || count != 8 {
		t.Fatalf("capacity=%d err=%v", count, err)
	}
	p.Emission.Bursts = []worldparticles.Burst{{AtSeconds: .25, Count: 2}}
	count, err = worldparticles.RequiredCapacity(p, d.Emitters[0].Playback)
	if err != nil || count != 12 {
		t.Fatalf("burst overlap capacity=%d err=%v", count, err)
	}
	d.Presets[0].Capacity = 7
	if err := worldparticles.Validate(&d); !errors.Is(err, worldparticles.ErrBounds) {
		t.Fatalf("unsafe reuse: %v", err)
	}
	p = fixture().Presets[0]
	p.LifetimeSeconds.Min = .1
	p.LifetimeSeconds.Max = .3
	groups, err := worldparticles.CycleGroups(p, worldparticles.Playback{Mode: "cycle", PeriodSeconds: .1})
	if err == nil || groups != 0 {
		t.Fatal("accepted rate emission longer than period")
	}
	p.Emission = worldparticles.Emission{Bursts: []worldparticles.Burst{{Count: 1}}}
	groups, err = worldparticles.CycleGroups(p, worldparticles.Playback{Mode: "cycle", PeriodSeconds: .1})
	if err != nil || groups != 4 {
		t.Fatalf("rounded float32 cycle overlap=%d %v", groups, err)
	}
	p.Emission.Rate = math.Inf(1)
	if _, err := worldparticles.BirthSchedule(p); err == nil {
		t.Fatal("nonfinite rate")
	}
}

func TestRateBirthScheduleUsesHalfOpenWindow(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		rate, duration float64
		count          int
	}{
		{rate: 100, duration: .07, count: 7},
		{rate: 100, duration: .07000000000000002, count: 8},
		{rate: 1e-320, duration: 1e-10, count: 1},
	} {
		preset := fixture().Presets[0]
		preset.Emission = worldparticles.Emission{Rate: test.rate, DurationSeconds: test.duration}
		births, err := worldparticles.BirthSchedule(preset)
		if err != nil || len(births) != test.count {
			t.Fatalf("rate %.17g duration %.17g: got %v, %v", test.rate, test.duration, births, err)
		}
		for i := range test.count {
			if float64(i)/test.rate >= test.duration {
				t.Fatal("birth at or beyond excluded duration")
			}
		}
		capacity, err := worldparticles.RequiredCapacity(preset, worldparticles.Playback{Mode: "once"})
		if err != nil || capacity != uint32(test.count) {
			t.Fatalf("once capacity=%d, %v", capacity, err)
		}
	}
}
func TestBoundsRecomputationAndSectorConfinement(t *testing.T) {
	t.Parallel()
	d := fixture()
	s := world.Sector{
		Floor:   world.Plane{},
		Ceiling: world.Plane{C: 4},
		Walls: []world.Wall{
			{Start: world.Vec2{X: 0, Y: 0}, End: world.Vec2{X: 8, Y: 0}},
			{Start: world.Vec2{X: 8, Y: 0}, End: world.Vec2{X: 8, Y: 8}},
			{Start: world.Vec2{X: 8, Y: 8}, End: world.Vec2{X: 0, Y: 8}},
			{Start: world.Vec2{X: 0, Y: 8}, End: world.Vec2{X: 0, Y: 0}},
		},
	}
	w := world.Document{Sectors: []world.Sector{s}}
	if err := worldparticles.ValidateForWorld(&d, &w); err != nil {
		t.Fatal(err)
	}
	d.Emitters[0].Sector = math.MaxUint32
	if err := worldparticles.ValidateForWorld(&d, &w); !errors.Is(err, worldparticles.ErrConfinement) {
		t.Fatalf("high sector index was not safely rejected: %v", err)
	}
	d.Emitters[0].Sector = 0
	d.Emitters[0].Bounds.Max.X -= .01
	if err := worldparticles.Validate(&d); !errors.Is(err, worldparticles.ErrBounds) {
		t.Fatalf("forged culling envelope accepted: %v", err)
	}
	d = fixture()
	d.Emitters[0].Position.Z = 3.8
	d.Emitters[0].Bounds = worldparticles.ConservativeEmitterBounds(d.Presets[0], d.Emitters[0])
	if err := worldparticles.ValidateForWorld(&d, &w); !errors.Is(err, worldparticles.ErrConfinement) {
		t.Fatalf("ceiling escape: %v", err)
	}
	s.Ceiling = world.Plane{A: -.4, C: 3}
	d = fixture()
	if worldparticles.BoundsInSector(d.Emitters[0].Bounds, &s) {
		t.Fatal("sloped ceiling escape")
	}
}
func TestBoundsCoverAnalyticalTrajectoryAndYaw(t *testing.T) {
	t.Parallel()
	d := fixture()
	p := d.Presets[0]
	e := d.Emitters[0]
	e.YawDegrees = 37
	e.Scale = 2
	p.VelocityMin = world.Vec3{X: -.9, Y: .2, Z: 2}
	p.VelocityMax = world.Vec3{X: .3, Y: .8, Z: 3}
	p.Acceleration = world.Vec3{X: .2, Y: -.5, Z: -4}
	p.LifetimeSeconds.Max = 2
	for _, drag := range []float64{0, .0095, .01, .011, .05, .1, .2, 2} {
		p.Drag = drag
		b := worldparticles.ConservativeEmitterBounds(p, e)
		for i := range 201 {
			age := float64(i) / 100
			for _, vx := range []float64{p.VelocityMin.X, p.VelocityMax.X} {
				for _, vy := range []float64{p.VelocityMin.Y, p.VelocityMax.Y} {
					for _, vz := range []float64{p.VelocityMin.Z, p.VelocityMax.Z} {
						move := func(v, a float64) float64 {
							if drag == 0 {
								return v*age + .5*a*age*age
							}
							f := -math.Expm1(-drag*age) / drag
							return v*f + a*(age-f)/drag
						}
						x, y, z := move(vx, p.Acceleration.X), move(vy, p.Acceleration.Y), move(vz, p.Acceleration.Z)
						sin, cos := math.Sincos(e.YawDegrees * math.Pi / 180)
						x, y = e.Position.X+e.Scale*(x*cos-y*sin), e.Position.Y+e.Scale*(x*sin+y*cos)
						z = e.Position.Z + e.Scale*z
						if x < b.Min.X || x > b.Max.X || y < b.Min.Y || y > b.Max.Y || z < b.Min.Z || z > b.Max.Z {
							t.Fatalf("trajectory escaped %+v at %.2f", b, age)
						}
					}
				}
			}
		}
	}
}
func FuzzParticleDecode(f *testing.F) {
	data, _ := worldparticles.Encode(fixture())
	f.Add(data)
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		d, err := worldparticles.Decode(data)
		if err != nil {
			return
		}
		encoded, err := worldparticles.Encode(d)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(data, encoded) {
			t.Fatal("accepted noncanonical wire")
		}
	})
}

func TestCurveKeysRejectNonportableGPUAges(t *testing.T) {
	t.Parallel()
	for _, ages := range [][]float64{{0, 1 - 1e-12, 1}, {0, 1e-40, 1}, {0, 0x1.8p-126, 0x1p-125, 1}} {
		for _, color := range []bool{false, true} {
			document := fixture()
			if color {
				document.Presets[0].ColorOverLife = nil
				for _, age := range ages {
					document.Presets[0].ColorOverLife = append(document.Presets[0].ColorOverLife, worldparticles.ColorKey{
						Age: age, RGBA: [4]float64{1, 1, 1, 1},
					})
				}
			} else {
				document.Presets[0].SizeOverLife = nil
				for _, age := range ages {
					document.Presets[0].SizeOverLife = append(
						document.Presets[0].SizeOverLife,
						worldparticles.ScalarKey{Age: age, Value: .1},
					)
				}
			}
			if err := worldparticles.ValidatePreset(&document.Presets[0]); !errors.Is(err, worldparticles.ErrParticle) {
				t.Fatalf("nonportable float32 ages %v accepted color=%v: %v", ages, color, err)
			}
			if _, err := worldparticles.Encode(document); !errors.Is(err, worldparticles.ErrParticle) {
				t.Fatalf("nonportable GPU curve encoded color=%v: %v", color, err)
			}
		}
	}
}

func TestFlipbookFPSRemainsNormalOnGPU(t *testing.T) {
	t.Parallel()
	for _, fps := range []float64{0, 1e-100, math.SmallestNonzeroFloat32} {
		preset := fixture().Presets[0]
		preset.Sprite.FPS = fps
		if err := worldparticles.ValidatePreset(&preset); !errors.Is(err, worldparticles.ErrParticle) {
			t.Fatalf("nonportable animated FPS %.17g accepted: %v", fps, err)
		}
	}
	preset := fixture().Presets[0]
	preset.Sprite.FPS = 0x1p-126
	if err := worldparticles.ValidatePreset(&preset); err != nil {
		t.Fatalf("smallest normal GPU FPS rejected: %v", err)
	}
	preset.Sprite.Frames = preset.Sprite.Frames[:1]
	preset.Sprite.FPS = 0
	if err := worldparticles.ValidatePreset(&preset); err != nil {
		t.Fatalf("static single frame requires FPS: %v", err)
	}
}
