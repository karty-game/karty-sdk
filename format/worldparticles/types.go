// Package worldparticles defines bounded, analytically animated GPU sprite effects.
package worldparticles

import (
	"errors"

	"github.com/karty-game/karty-sdk/format/cartridge"
	"github.com/karty-game/karty-sdk/format/world"
)

const (
	Version          = uint16(1)
	EntryName        = "@world/particles"
	Feature          = cartridge.FeatureWorldParticlesV1
	BinaryMagic      = "KWP1"
	MaxEncodedSize   = 1024 * 1024
	MaxPresets       = 64
	MaxEmitters      = 256
	MaxCapacity      = 4096
	MaxTotalCapacity = 65536
	MaxFrames        = 16
	MaxCurveKeys     = 4
	MaxBursts        = 16
	MaxSeconds       = 3600.0
	MaxRate          = 4096.0
	MaxVelocity      = 1000.0
	MaxAcceleration  = 1000.0
	MaxSize          = 1000.0
)

var (
	ErrVersion     = errors.New("particle version is unsupported")
	ErrBounds      = errors.New("particle resource exceeds a bound")
	ErrIdentity    = errors.New("particle identity is invalid")
	ErrParticle    = errors.New("particle configuration is invalid")
	ErrConfinement = errors.New("particle motion envelope leaves its sector")
	ErrSyntax      = errors.New("particle binary syntax is invalid")
)

type Document struct {
	Version  uint16
	Presets  []Preset
	Emitters []Emitter
}
type Range struct {
	Min float64 `yaml:"min"`
	Max float64 `yaml:"max"`
}
type Burst struct {
	AtSeconds float64 `yaml:"at_seconds"`
	Count     uint32  `yaml:"count"`
}
type Emission struct {
	Rate            float64 `yaml:"rate,omitempty"`
	DurationSeconds float64 `yaml:"duration_seconds,omitempty"`
	Bursts          []Burst `yaml:"bursts,omitempty"`
}
type Spawn struct {
	Shape      string
	HalfExtent world.Vec3
}
type Sprite struct {
	Frames   []uint32
	FPS      float64
	Playback string
	Facing   string
	Blend    string
}
type ScalarKey struct {
	Age   float64 `yaml:"age"`
	Value float64 `yaml:"value"`
}
type ColorKey struct {
	Age  float64    `yaml:"age"`
	RGBA [4]float64 `yaml:"rgba"`
}
type Playback struct {
	Mode          string  `yaml:"mode"`
	PeriodSeconds float64 `yaml:"period_seconds,omitempty"`
	DelaySeconds  float64 `yaml:"delay_seconds,omitempty"`
}
type Preset struct {
	Name                 string
	Capacity             uint32
	Emission             Emission
	LifetimeSeconds      Range
	Spawn                Spawn
	VelocityMin          world.Vec3
	VelocityMax          world.Vec3
	Acceleration         world.Vec3
	Drag                 float64
	Sprite               Sprite
	SizeOverLife         []ScalarKey
	ColorOverLife        []ColorKey
	RotationDegrees      Range
	RotationSpeedDegrees Range
}
type Bounds struct {
	Min world.Vec3
	Max world.Vec3
}
type Emitter struct {
	ID         string
	Preset     string
	Sector     uint32
	Position   world.Vec3
	YawDegrees float64
	Scale      float64
	Seed       uint32
	Playback   Playback
	Bounds     Bounds
}
