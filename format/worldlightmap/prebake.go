package worldlightmap

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/karty-game/karty-sdk/codec/qoi"
	"github.com/karty-game/karty-sdk/format/cartridge"
	"github.com/karty-game/karty-sdk/format/level"
	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/internal/binwire"
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
	PrebakeBinaryMagic     = "KPB1"
)

// PrebakeManifest binds one completed direct RNM image to exact layout and
// selected lighting inputs. Its canonical binary is the public manifest entry.
type PrebakeManifest struct {
	Schema            string
	Algorithm         int
	Encoding          string
	LayoutSHA256      string
	BakeSHA256        string
	ImageSHA256       string
	ImageBytes        int
	Width             int
	Height            int
	RGBMRange         float64
	Producer          string
	Samples           int
	Bounces           int
	Seed              uint64
	ReflectanceSHA256 string
	SurfaceSHA256     string
	// MaxRayDistance is present on the wire only for bounded producer revision
	// 3. Zero retains the exact unbounded revision-1/2 manifest bytes.
	MaxRayDistance float64
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
	sources := make(map[string]world.PointLight, len(document.Lighting.Lights))
	for _, light := range document.Lighting.Lights {
		sources[light.ID] = light
	}
	rangeSum := 0.0
	for _, id := range layout.RuntimeBake.LightIDs {
		light := sources[id]
		lights = append(lights, light)
		rangeSum += max(light.Color.X, light.Color.Y, light.Color.Z)
	}
	layoutDigest := digest(encoded)
	// Selected source records are hashed, not carried in the small published
	// manifest. Their bounded scratch must accommodate all authored sources.
	w := binwire.NewWriter("KDI1", world.MaxEncodedSize)
	w.String(PrebakeSchema, 128)
	w.I32(PrebakeAlgorithm)
	w.String(layoutDigest, 64)
	w.Count(len(lights), false, MaxBakeLights)
	for _, light := range lights {
		writeWorldPointLight(w, &light)
	}
	inputs, err := w.Bytes()
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
		expected, err = newOfflinePrebakeForProducer(layout, document, pair.Image, OfflineBakeInputs{
			Denoise: mode, Samples: pair.Manifest.Samples, Bounces: pair.Manifest.Bounces, Seed: pair.Manifest.Seed,
			ReflectanceSHA256: pair.Manifest.ReflectanceSHA256, RGBMRange: pair.Manifest.RGBMRange,
			MaxRayDistance: pair.Manifest.MaxRayDistance,
		}, pair.Manifest.Producer)
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
	return encodePrebakeManifest(pair.Manifest)
}

func encodePrebakeManifest(manifest PrebakeManifest) ([]byte, error) {
	w := binwire.NewWriter(PrebakeBinaryMagic, MaxPrebakeManifestSize)
	writeWorldlightmapPrebakeManifest(w, &manifest)
	encoded, err := w.Bytes()
	if err != nil {
		return nil, ErrLayout
	}
	return encoded, nil
}

// DecodePrebakeManifest inspects a bounded canonical binary manifest without
// decoding pixels. Full layout, source and image identity requires DecodePrebake.
func DecodePrebakeManifest(encoded []byte) (PrebakeManifest, error) {
	scan := binwire.NewReader(encoded, PrebakeBinaryMagic, MaxPrebakeManifestSize)
	scanWorldlightmapPrebakeManifest(scan)
	if scan.Done() != nil {
		return PrebakeManifest{}, ErrLayout
	}
	var manifest PrebakeManifest
	reader := binwire.NewReader(encoded, PrebakeBinaryMagic, MaxPrebakeManifestSize)
	readWorldlightmapPrebakeManifest(reader, &manifest)
	if reader.Done() != nil || manifest.Schema != PrebakeSchema || manifest.Encoding != DirectRNMEncoding ||
		!lowerSHA256(manifest.LayoutSHA256) || !lowerSHA256(manifest.BakeSHA256) || !lowerSHA256(manifest.ImageSHA256) ||
		manifest.ImageBytes < 1 || manifest.ImageBytes > MaxPrebakeImageSize ||
		(manifest.Height != 512 && manifest.Height != 1024) || manifest.Width != 3*manifest.Height || manifest.RGBMRange < 1 {
		return PrebakeManifest{}, ErrLayout
	}
	switch manifest.Algorithm {
	case PrebakeAlgorithm:
		if manifest.Producer != "" || manifest.Samples != 0 || manifest.Bounces != 0 || manifest.Seed != 0 ||
			manifest.ReflectanceSHA256 != "" ||
			manifest.SurfaceSHA256 != "" || manifest.MaxRayDistance != 0 {
			return PrebakeManifest{}, ErrLayout
		}
	case OfflinePrebakeAlgorithm:
		if _, err := OfflineDenoiseMode(manifest.Producer); err != nil {
			return PrebakeManifest{}, err
		}
		if manifest.Samples < 1 || manifest.Samples > MaxOfflineSamples || manifest.Bounces < 0 || manifest.Bounces > MaxOfflineBounces ||
			!lowerSHA256(manifest.ReflectanceSHA256) ||
			!lowerSHA256(manifest.SurfaceSHA256) || !validMaxRayDistance(manifest.MaxRayDistance) ||
			boundedOfflineProducer(manifest.Producer) != (manifest.MaxRayDistance > 0) {
			return PrebakeManifest{}, ErrLayout
		}
	default:
		return PrebakeManifest{}, ErrLayout
	}
	return manifest, nil
}

// DecodePrebake rejects malformed binary fields before returning a completely
// validated pair. Image remains borrowed; JSON and trailing data are rejected.
func DecodePrebake(encoded, image []byte, layout Layout, document *world.Document) (PrebakePair, error) {
	if len(encoded) == 0 || len(encoded) > MaxPrebakeManifestSize {
		return PrebakePair{}, ErrLayout
	}
	manifest, err := DecodePrebakeManifest(encoded)
	if err != nil {
		return PrebakePair{}, err
	}
	pair := PrebakePair{Manifest: manifest, Image: image}
	if err := pair.Validate(layout, document); err != nil {
		return PrebakePair{}, ErrLayout
	}
	return pair, nil
}

func digest(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}
