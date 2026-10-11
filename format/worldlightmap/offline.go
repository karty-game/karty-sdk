package worldlightmap

import (
	"encoding/hex"
	"fmt"
	"math"
	"slices"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/internal/binwire"
)

const (
	OfflinePrebakeAlgorithm       = 2
	OfflinePrebakeProducer        = "cpu-rnm3-pathtrace@2"
	BoundedOfflinePrebakeProducer = "cpu-rnm3-pathtrace@3"
	LegacyOfflinePrebakeProducer  = "cpu-rnm3-pathtrace@1"
	MaxOfflineSamples             = 256
	MaxOfflineBounces             = 4
	MaxDiffuseReflectance         = .95
	MaxBakeRayDistance            = world.MaxCoordinate
)

// OfflineBakeInputs identifies the public deterministic CPU producer. Worker
// count is deliberately absent: changing parallelism must not change pixels.
type OfflineBakeInputs struct {
	Samples, Bounces  int
	Seed              uint64
	ReflectanceSHA256 string
	RGBMRange         float64
	Denoise           string  // empty/off, low or medium; encoded in the producer identity
	MaxRayDistance    float64 // zero is unbounded; positive selects producer revision 3
}

// OfflineProducer selects a separately versioned bounded producer only for a
// finite positive ray limit. Zero preserves the unbounded revision-2 producer.
func OfflineProducer(mode string, maxRayDistance float64) (string, error) {
	producer, err := OfflineDenoiseProducer(mode)
	if err != nil || !validMaxRayDistance(maxRayDistance) {
		return "", ErrLayout
	}
	if maxRayDistance == 0 {
		return producer, nil
	}
	if mode == "" || mode == "off" {
		return BoundedOfflinePrebakeProducer, nil
	}
	return "cpu-rnm3-pathtrace-atrous-" + mode + "@3", nil
}

func boundedOfflineProducer(producer string) bool {
	return producer == BoundedOfflinePrebakeProducer || producer == "cpu-rnm3-pathtrace-atrous-low@3" ||
		producer == "cpu-rnm3-pathtrace-atrous-medium@3"
}

func validMaxRayDistance(distance float64) bool {
	return finite(distance) && distance >= 0 && distance <= MaxBakeRayDistance
}

// OfflineDenoiseProducer versions the complete filter, including its preset.
// Empty and off select the same revision-2 producer and manifest identity.
func OfflineDenoiseProducer(mode string) (string, error) {
	switch mode {
	case "", "off":
		return OfflinePrebakeProducer, nil
	case "low", "medium":
		return "cpu-rnm3-pathtrace-atrous-" + mode + "@2", nil
	default:
		return "", fmt.Errorf("denoise must be off, low or medium: %w", ErrLayout)
	}
}

// OfflineDenoiseMode rejects unknown producer versions before image allocation.
func OfflineDenoiseMode(producer string) (string, error) {
	switch producer {
	case OfflinePrebakeProducer, LegacyOfflinePrebakeProducer, BoundedOfflinePrebakeProducer:
		return "off", nil
	case "cpu-rnm3-pathtrace-atrous-low@1", "cpu-rnm3-pathtrace-atrous-low@2", "cpu-rnm3-pathtrace-atrous-low@3":
		return "low", nil
	case "cpu-rnm3-pathtrace-atrous-medium@1", "cpu-rnm3-pathtrace-atrous-medium@2", "cpu-rnm3-pathtrace-atrous-medium@3":
		return "medium", nil
	default:
		return "", fmt.Errorf("unknown offline producer: %w", ErrLayout)
	}
}

// OfflineSurfaceDigest binds material identities and authored projection data
// in addition to geometry. Actor state and artistic ambient are not transport
// inputs. The original albedo pixels are checked separately by the public CLI.
func OfflineSurfaceDigest(document *world.Document) (string, error) {
	if document == nil {
		return "", ErrLayout
	}
	snapshot := *document
	snapshot.Contents, snapshot.Lighting, snapshot.Animations, snapshot.Emission = nil, nil, nil, nil
	encoded, err := world.Encode(snapshot)
	if err != nil {
		return "", err
	}
	return digest(encoded), nil
}

// OfflineRGBMRange reserves a conservative finite range for direct lighting
// plus bounded diffuse transport. Each bounce attenuates maximum radiance by
// MaxDiffuseReflectance; three coefficient lobes need the same factor as the
// direct producer. This is a storage bound, not a promised illumination level.
func OfflineRGBMRange(layout Layout, document *world.Document, bounces int) (float64, error) {
	if document == nil || document.Lighting == nil || bounces < 0 || bounces > MaxOfflineBounces {
		return 0, ErrLayout
	}
	if _, err := Encode(layout, document); err != nil || layout.RuntimeBake == nil || layout.RuntimeBake.Encoding != DirectRNMEncoding {
		return 0, ErrLayout
	}
	return offlineRGBMRangeValidated(layout, document, bounces), nil
}

func offlineRGBMRangeValidated(layout Layout, document *world.Document, bounces int) float64 {
	return offlineRGBMRangeForProducer(layout, document, bounces, false)
}

func offlineRGBMRangeForProducer(layout Layout, document *world.Document, bounces int, legacy bool) float64 {
	sum := 0.0
	sources := make(map[string]world.PointLight, len(document.Lighting.Lights))
	for _, light := range document.Lighting.Lights {
		sources[light.ID] = light
	}
	lights := make([]world.PointLight, 0, len(layout.RuntimeBake.LightIDs))
	for _, id := range layout.RuntimeBake.LightIDs {
		light := sources[id]
		sum += max(light.Color.X, light.Color.Y, light.Color.Z)
		lights = append(lights, light)
	}
	if !legacy {
		sum = influenceEnergyBound(lights)
	}
	factor, energy := 1.0, 1.0
	for bounce := 0; bounce < bounces; bounce++ {
		energy *= MaxDiffuseReflectance
		factor += energy
	}
	return max(1, 3*sum*factor)
}

type influenceEndpoint struct{ position, weight float64 }

// Every influencing sphere contains a point in all three projected intervals.
// For each axis, maximum overlapping source energy therefore bounds radiance
// at every point; the minimum of those independent bounds remains conservative.
// Unlike the old global sum, distant lights do not exhaust RGBM precision.
func influenceEnergyBound(lights []world.PointLight) float64 {
	if len(lights) == 0 {
		return 0
	}
	if len(lights) == 1 {
		return max(lights[0].Color.X, lights[0].Color.Y, lights[0].Color.Z)
	}
	bound := math.Inf(1)
	events := make([]influenceEndpoint, 0, len(lights)*2)
	for axis := range 3 {
		events = events[:0]
		for _, light := range lights {
			position := [...]float64{light.Position.X, light.Position.Y, light.Position.Z}[axis]
			weight := max(light.Color.X, light.Color.Y, light.Color.Z)
			events = append(events,
				influenceEndpoint{math.Nextafter(position-light.Radius, math.Inf(-1)), weight},
				influenceEndpoint{math.Nextafter(position+light.Radius, math.Inf(1)), -weight})
		}
		slices.SortFunc(events, func(a, b influenceEndpoint) int {
			if a.position < b.position {
				return -1
			}
			if a.position > b.position {
				return 1
			}
			// Add starts before removing ends at a shared endpoint.
			if a.weight > b.weight {
				return -1
			}
			if a.weight < b.weight {
				return 1
			}
			return 0
		})
		current, maximum := 0.0, 0.0
		for _, event := range events {
			// Directed rounding avoids underestimating overlap after many
			// additions/removals, including differently scaled light colors.
			current = math.Nextafter(current+event.weight, math.Inf(1))
			maximum = max(maximum, current)
		}
		bound = min(bound, maximum)
	}
	return bound
}

// NewOfflinePrebake binds a combined direct/indirect image to transport inputs.
// Format validation verifies identity and QOI integrity, not solver quality.
// ReflectanceSHA256 commits original decoded albedo; the CLI verifies it against
// source before packaging. Hosts can verify surface identity, but do not possess
// those original source images and cannot independently rehash their pixels.
func NewOfflinePrebake(layout Layout, document *world.Document, image []byte, inputs OfflineBakeInputs) (PrebakePair, error) {
	producer, err := OfflineProducer(inputs.Denoise, inputs.MaxRayDistance)
	if err != nil {
		return PrebakePair{}, err
	}
	return newOfflinePrebakeForProducer(layout, document, image, inputs, producer)
}

func newOfflinePrebakeForProducer(
	layout Layout,
	document *world.Document,
	image []byte,
	inputs OfflineBakeInputs,
	producer string,
) (PrebakePair, error) {
	if inputs.Samples < 1 || inputs.Samples > MaxOfflineSamples || inputs.Bounces < 0 || inputs.Bounces > MaxOfflineBounces ||
		!lowerSHA256(inputs.ReflectanceSHA256) || math.IsNaN(inputs.RGBMRange) || math.IsInf(inputs.RGBMRange, 0) ||
		!validMaxRayDistance(inputs.MaxRayDistance) || boundedOfflineProducer(producer) != (inputs.MaxRayDistance > 0) {
		return PrebakePair{}, fmt.Errorf("offline bake inputs: %w", ErrLayout)
	}
	pair, err := NewPrebake(layout, document, image)
	if err != nil {
		return PrebakePair{}, err
	}
	// NewPrebake already checked the complete layout and its encoded size.
	legacy := producer == LegacyOfflinePrebakeProducer || producer == "cpu-rnm3-pathtrace-atrous-low@1" ||
		producer == "cpu-rnm3-pathtrace-atrous-medium@1"
	rangeBound := offlineRGBMRangeForProducer(layout, document, inputs.Bounces, legacy)
	if inputs.MaxRayDistance > 0 {
		rangeBound, err = OfflineRGBMRangeBounded(layout, document, inputs.Bounces, inputs.MaxRayDistance)
		if err != nil {
			return PrebakePair{}, err
		}
	}
	if inputs.RGBMRange != rangeBound {
		return PrebakePair{}, fmt.Errorf("offline bake range: %w", ErrLayout)
	}
	surfaceDigest, err := OfflineSurfaceDigest(document)
	if err != nil {
		return PrebakePair{}, err
	}
	w := binwire.NewWriter("KOI1", MaxPrebakeManifestSize)
	w.String(producer, 128)
	w.String(pair.Manifest.BakeSHA256, 64)
	w.String(surfaceDigest, 64)
	w.String(inputs.ReflectanceSHA256, 64)
	w.I32(inputs.Samples)
	w.I32(inputs.Bounces)
	w.U64(inputs.Seed)
	if !legacy {
		w.F64(rangeBound)
	}
	if inputs.MaxRayDistance > 0 {
		w.F64(inputs.MaxRayDistance)
	}
	identity, err := w.Bytes()
	if err != nil {
		return PrebakePair{}, err
	}
	m := &pair.Manifest
	m.Algorithm, m.Producer = OfflinePrebakeAlgorithm, producer
	m.Samples, m.Bounces, m.Seed = inputs.Samples, inputs.Bounces, inputs.Seed
	m.ReflectanceSHA256, m.SurfaceSHA256 = inputs.ReflectanceSHA256, surfaceDigest
	m.BakeSHA256, m.RGBMRange = digest(identity), rangeBound
	m.MaxRayDistance = inputs.MaxRayDistance
	return pair, nil
}

func lowerSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && hex.EncodeToString(decoded) == value
}
