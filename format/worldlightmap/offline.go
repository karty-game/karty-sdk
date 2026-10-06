package worldlightmap

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"

	"github.com/karty-game/karty-sdk/format/world"
)

const (
	OfflinePrebakeAlgorithm = 2
	OfflinePrebakeProducer  = "cpu-rnm3-pathtrace@1"
	MaxOfflineSamples       = 256
	MaxOfflineBounces       = 4
	MaxDiffuseReflectance   = .95
)

// OfflineBakeInputs identifies the public deterministic CPU producer. Worker
// count is deliberately absent: changing parallelism must not change pixels.
type OfflineBakeInputs struct {
	Samples, Bounces  int
	Seed              uint64
	ReflectanceSHA256 string
	RGBMRange         float64
	Denoise           string // empty/off, low or medium; encoded in the producer identity
}

// OfflineDenoiseProducer versions the complete filter, including its preset.
// Empty and off preserve the original producer and its exact manifest bytes.
func OfflineDenoiseProducer(mode string) (string, error) {
	switch mode {
	case "", "off":
		return OfflinePrebakeProducer, nil
	case "low", "medium":
		return "cpu-rnm3-pathtrace-atrous-" + mode + "@1", nil
	default:
		return "", fmt.Errorf("denoise must be off, low or medium: %w", ErrLayout)
	}
}

// OfflineDenoiseMode rejects unknown producer versions before image allocation.
func OfflineDenoiseMode(producer string) (string, error) {
	switch producer {
	case OfflinePrebakeProducer:
		return "off", nil
	case "cpu-rnm3-pathtrace-atrous-low@1":
		return "low", nil
	case "cpu-rnm3-pathtrace-atrous-medium@1":
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
	snapshot.Contents, snapshot.Lighting = nil, nil
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
	sum := 0.0
	for _, id := range layout.RuntimeBake.LightIDs {
		for _, light := range document.Lighting.Lights {
			if light.ID == id {
				sum += max(light.Color.X, light.Color.Y, light.Color.Z)
				break
			}
		}
	}
	factor, energy := 1.0, 1.0
	for bounce := 0; bounce < bounces; bounce++ {
		energy *= MaxDiffuseReflectance
		factor += energy
	}
	return max(1, 3*sum*factor)
}

// NewOfflinePrebake binds a combined direct/indirect image to transport inputs.
// Format validation verifies identity and QOI integrity, not solver quality.
// ReflectanceSHA256 commits original decoded albedo; the CLI verifies it against
// source before packaging. Hosts can verify surface identity, but do not possess
// those original source images and cannot independently rehash their pixels.
func NewOfflinePrebake(layout Layout, document *world.Document, image []byte, inputs OfflineBakeInputs) (PrebakePair, error) {
	producer, err := OfflineDenoiseProducer(inputs.Denoise)
	if err != nil {
		return PrebakePair{}, err
	}
	if inputs.Samples < 1 || inputs.Samples > MaxOfflineSamples || inputs.Bounces < 0 || inputs.Bounces > MaxOfflineBounces ||
		!lowerSHA256(inputs.ReflectanceSHA256) || math.IsNaN(inputs.RGBMRange) || math.IsInf(inputs.RGBMRange, 0) {
		return PrebakePair{}, fmt.Errorf("offline bake inputs: %w", ErrLayout)
	}
	pair, err := NewPrebake(layout, document, image)
	if err != nil {
		return PrebakePair{}, err
	}
	// NewPrebake already checked the complete layout and its encoded size.
	rangeBound := offlineRGBMRangeValidated(layout, document, inputs.Bounces)
	if inputs.RGBMRange != rangeBound {
		return PrebakePair{}, fmt.Errorf("offline bake range: %w", ErrLayout)
	}
	surfaceDigest, err := OfflineSurfaceDigest(document)
	if err != nil {
		return PrebakePair{}, err
	}
	identity, err := json.Marshal(struct {
		Producer          string `json:"producer"`
		DirectSHA256      string `json:"direct_sha256"`
		SurfaceSHA256     string `json:"surface_sha256"`
		ReflectanceSHA256 string `json:"reflectance_sha256"`
		Samples           int    `json:"samples"`
		Bounces           int    `json:"bounces"`
		Seed              uint64 `json:"seed"`
	}{producer, pair.Manifest.BakeSHA256, surfaceDigest, inputs.ReflectanceSHA256, inputs.Samples, inputs.Bounces, inputs.Seed})
	if err != nil {
		return PrebakePair{}, err
	}
	m := &pair.Manifest
	m.Algorithm, m.Producer = OfflinePrebakeAlgorithm, producer
	m.Samples, m.Bounces, m.Seed = inputs.Samples, inputs.Bounces, inputs.Seed
	m.ReflectanceSHA256, m.SurfaceSHA256 = inputs.ReflectanceSHA256, surfaceDigest
	m.BakeSHA256, m.RGBMRange = digest(identity), rangeBound
	return pair, nil
}

func lowerSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && hex.EncodeToString(decoded) == value
}
