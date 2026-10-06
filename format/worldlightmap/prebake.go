package worldlightmap

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/karty-game/karty-sdk/codec/qoi"
	"github.com/karty-game/karty-sdk/format/cartridge"
	"github.com/karty-game/karty-sdk/format/level"
	"github.com/karty-game/karty-sdk/format/world"
)

const (
	PrebakeSchema          = "karty.world-lightmap-prebake@1"
	PrebakeAlgorithm       = 1
	PrebakeEntryName       = "@world/lightmaps/prebake"
	PrebakeImageEntryName  = "@world/lightmaps/prebake-rnm3"
	PrebakeMetadataKey     = "kartyWorldLightmapPrebake"
	PrebakeFeature         = cartridge.FeatureWorldLightmapsPrebakedV1
	MaxPrebakeManifestSize = 16 * 1024
	MaxPrebakeImageSize    = level.MaxEntrySize
)

// PrebakeManifest binds one completed direct RNM image to exact layout and
// selected lighting inputs. Its canonical JSON is the public manifest entry.
type PrebakeManifest struct {
	Schema            string  `json:"schema"`
	Algorithm         int     `json:"algorithm"`
	Encoding          string  `json:"encoding"`
	LayoutSHA256      string  `json:"layout_sha256"`
	BakeSHA256        string  `json:"bake_sha256"`
	ImageSHA256       string  `json:"image_sha256"`
	ImageBytes        int     `json:"image_bytes"`
	Width             int     `json:"width"`
	Height            int     `json:"height"`
	RGBMRange         float64 `json:"rgbm_range"`
	Producer          string  `json:"producer,omitempty"`
	Samples           int     `json:"samples,omitempty"`
	Bounces           int     `json:"bounces,omitempty"`
	Seed              uint64  `json:"seed,omitempty"`
	ReflectanceSHA256 string  `json:"reflectance_sha256,omitempty"`
	SurfaceSHA256     string  `json:"surface_sha256,omitempty"`
}

// PrebakePair retains caller ownership of Image; consumers clone it before
// publication. It is not the wire format: levels carry two fixed data entries.
type PrebakePair struct {
	Manifest PrebakeManifest
	Image    []byte
}

// NewPrebake validates a completed raw linear RGBM QOI atlas and derives its
// identity. It neither decodes pixels nor imports runtime or filesystem code.
func NewPrebake(layout Layout, document *world.Document, image []byte) (PrebakePair, error) {
	if document == nil || document.Lighting == nil {
		return PrebakePair{}, fmt.Errorf("prebake requires authored lighting document: %w", ErrLayout)
	}
	if len(image) == 0 || len(image) > MaxPrebakeImageSize {
		return PrebakePair{}, fmt.Errorf("prebake image size: %w", ErrLayout)
	}
	encoded, err := Encode(layout, document)
	if err != nil || layout.RuntimeBake == nil || layout.RuntimeBake.Encoding != DirectRNMEncoding {
		return PrebakePair{}, fmt.Errorf("prebake requires direct RNM layout: %w", ErrLayout)
	}
	metadata, err := qoi.Validate(image)
	page := layout.Pages[0]
	if err != nil || int(metadata.Width) != 3*page.Width || int(metadata.Height) != page.Height ||
		metadata.Channels != qoi.ChannelsRGBA || metadata.Colorspace != qoi.ColorspaceLinear {
		return PrebakePair{}, fmt.Errorf("prebake image dimensions, channels or stream: %w", ErrLayout)
	}
	lights := make([]world.PointLight, 0, len(layout.RuntimeBake.LightIDs))
	rangeSum := 0.0
	for _, id := range layout.RuntimeBake.LightIDs {
		for _, light := range document.Lighting.Lights {
			if light.ID == id {
				lights = append(lights, light)
				rangeSum += max(light.Color.X, light.Color.Y, light.Color.Z)
				break
			}
		}
	}
	layoutDigest := digest(encoded)
	inputs, err := json.Marshal(struct {
		Schema       string             `json:"schema"`
		Algorithm    int                `json:"algorithm"`
		LayoutSHA256 string             `json:"layout_sha256"`
		Lights       []world.PointLight `json:"lights"`
	}{PrebakeSchema, PrebakeAlgorithm, layoutDigest, lights})
	if err != nil {
		return PrebakePair{}, fmt.Errorf("prebake inputs: %w", ErrLayout)
	}
	manifest := PrebakeManifest{Schema: PrebakeSchema, Algorithm: PrebakeAlgorithm, Encoding: DirectRNMEncoding,
		LayoutSHA256: layoutDigest, BakeSHA256: digest(inputs), ImageSHA256: digest(image), ImageBytes: len(image),
		Width: int(metadata.Width), Height: int(metadata.Height), RGBMRange: max(1, 3*rangeSum)}
	return PrebakePair{Manifest: manifest, Image: image}, nil
}

// Validate checks every identity and the complete bounded QOI stream without
// allocating decoded pixels. It cannot prove the lighting quality of a producer.
func (pair PrebakePair) Validate(layout Layout, document *world.Document) error {
	var expected PrebakePair
	var err error
	if pair.Manifest.Algorithm == OfflinePrebakeAlgorithm {
		mode, modeErr := OfflineDenoiseMode(pair.Manifest.Producer)
		if modeErr != nil {
			return modeErr
		}
		expected, err = NewOfflinePrebake(layout, document, pair.Image, OfflineBakeInputs{
			Denoise: mode, Samples: pair.Manifest.Samples, Bounces: pair.Manifest.Bounces, Seed: pair.Manifest.Seed,
			ReflectanceSHA256: pair.Manifest.ReflectanceSHA256, RGBMRange: pair.Manifest.RGBMRange,
		})
	} else {
		expected, err = NewPrebake(layout, document, pair.Image)
	}
	if err != nil || pair.Manifest != expected.Manifest {
		return fmt.Errorf("prebake identity or image mismatch: %w", ErrLayout)
	}
	return nil
}

// EncodePrebake returns a canonical manifest only after complete pair validation.
func EncodePrebake(pair PrebakePair, layout Layout, document *world.Document) ([]byte, error) {
	if err := pair.Validate(layout, document); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(pair.Manifest)
	if err != nil || len(encoded) > MaxPrebakeManifestSize {
		return nil, ErrLayout
	}
	return encoded, nil
}

// DecodePrebake rejects unknown, duplicate, missing, null, reordered and
// noncanonical fields before returning a validated pair. Image remains borrowed.
func DecodePrebake(encoded, image []byte, layout Layout, document *world.Document) (PrebakePair, error) {
	if len(encoded) == 0 || len(encoded) > MaxPrebakeManifestSize {
		return PrebakePair{}, ErrLayout
	}
	var manifest PrebakeManifest
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return PrebakePair{}, ErrLayout
	}
	pair := PrebakePair{Manifest: manifest, Image: image}
	canonical, err := EncodePrebake(pair, layout, document)
	if err != nil || !bytes.Equal(encoded, canonical) {
		return PrebakePair{}, ErrLayout
	}
	return pair, nil
}

func digest(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}
