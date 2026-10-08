package world

import (
	"math"

	"github.com/karty-game/karty-sdk/format/cartridge"
)

const (
	FeatureEmission      = cartridge.FeatureWorldEmissionV1
	EmissionVersion      = uint32(1)
	MaxEmissionMaterials = 196
	MaxEmissionIntensity = 8.0
)

// Emission is an optional world/emission@1 payload. Intensity is an eight-bit
// linear-albedo multiplier, decoded as code*8/255. Pulse is independent of the
// material's optional flipbook/liquid animation; these are two bounded effects.
type Emission struct {
	Version   uint32             `json:"version"`
	Materials []MaterialEmission `json:"materials"`
}

type MaterialEmission struct {
	Material  uint32         `json:"material"`
	Intensity uint8          `json:"intensity"`
	Pulse     *EmissionPulse `json:"pulse,omitempty"`
	Lights    []string       `json:"lights,omitempty"`
}

// EmissionPulse has depth is the fraction of intensity removed at the dimmest point.
// A cosine pulse begins at full brightness and reaches its minimum after half a period.
type EmissionPulse struct {
	Depth         float64 `json:"depth"`
	PeriodSeconds float64 `json:"period_seconds"`
	PhaseSeconds  float64 `json:"phase_seconds,omitempty"`
}

func (p EmissionPulse) Validate() error {
	if math.IsNaN(p.Depth) || math.IsInf(p.Depth, 0) || p.Depth < 0 || p.Depth > 1 ||
		!animationSeconds(p.PeriodSeconds, false) || !animationSeconds(p.PhaseSeconds, true) {
		return ErrBounds
	}
	return nil
}

func EncodeEmissionIntensity(value float64) (uint8, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > MaxEmissionIntensity {
		return 0, ErrBounds
	}
	return uint8(math.Round(value * 255 / MaxEmissionIntensity)), nil
}

func ValidateEmission(document *Document) error {
	if document == nil {
		return ErrSyntax
	}
	e := document.Emission
	if e == nil {
		return nil
	}
	if document.Version != Version || e.Version != EmissionVersion {
		return ErrVersion
	}
	if len(e.Materials) == 0 || len(e.Materials) > MaxEmissionMaterials {
		return ErrBounds
	}
	// Animation-only frames and secondary colour modulation are not emission
	// owners. An emitting material must occur on a real primary surface/band.
	used := map[uint32]bool{}
	for _, s := range document.Sectors {
		used[s.FloorMaterial], used[s.CeilingMaterial] = true, true
		for _, w := range s.Walls {
			used[w.Material] = true
			for _, r := range w.FrameRegions {
				used[r.Material] = true
			}
		}
	}
	if document.StaticSolids != nil {
		for _, s := range document.StaticSolids.Items {
			used[s.SideMaterial], used[s.TopMaterial], used[s.BottomMaterial] = true, true, true
		}
	}
	seen := map[uint32]bool{}
	linkedLights := map[string]bool{}
	for _, m := range e.Materials {
		if m.Material == 0 || !used[m.Material] || seen[m.Material] {
			return ErrContent
		}
		seen[m.Material] = true
		if m.Pulse != nil {
			if err := m.Pulse.Validate(); err != nil {
				return err
			}
		}
		if len(m.Lights) > MaxLights {
			return ErrBounds
		}
		for _, id := range m.Lights {
			if !validIdentifier(id) || linkedLights[id] || document.Lighting == nil || len(linkedLights) >= MaxLights {
				return ErrContent
			}
			found := false
			for _, light := range document.Lighting.Lights {
				found = found || light.ID == id
			}
			if !found {
				return ErrContent
			}
			linkedLights[id] = true
		}
	}
	return nil
}
