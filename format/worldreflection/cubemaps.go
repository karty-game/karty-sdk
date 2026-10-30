// Package worldreflection defines saved static parallax-corrected environments.
package worldreflection

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"math"

	"github.com/karty-game/karty-sdk/codec/qoi"
	"github.com/karty-game/karty-sdk/format/level"
	"github.com/karty-game/karty-sdk/format/world"
)

const (
	Feature         = "world/reflection-cubemaps@1"
	EntryName       = "@world/reflection-cubemaps"
	Producer        = "karty-static-cubemap@1"
	FaceSize        = 128
	Faces           = 6
	MaxProbes       = world.MaxSectors
	MaxEncodedSize  = level.MaxEntrySize
	ImageBytes      = FaceSize * FaceSize * Faces * 4
	MaxDecodedBytes = MaxProbes * ImageBytes
)

var ErrCubemaps = errors.New("saved reflection cubemaps are invalid")

type Probe struct {
	ID                int
	Origin, Low, High [3]float32
	Image             []byte // Straight RGBA QOI, six faces in +X,-X,+Y,-Y,+Z,-Z order.
}
type Bundle struct {
	InputSHA256     string
	Producer        string
	Sectors, Solids []int // -1 means no environment; otherwise an explicit probe ID.
	Probes          []Probe
}

// InputDigest binds every original level payload and metadata, transitively
// including immutable child hashes and their baked lighting through the catalog.
// Only this package's own payload is omitted, avoiding a self-referential hash.
func InputDigest(envelope level.Envelope) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte("karty-reflection-input@1"))
	var size [8]byte
	write := func(data []byte) {
		binary.LittleEndian.PutUint64(size[:], uint64(len(data)))
		_, _ = hash.Write(size[:])
		_, _ = hash.Write(data)
	}
	write(envelope.Metadata)
	for _, entry := range envelope.Entries {
		if entry.Name == EntryName {
			continue
		}
		write([]byte(entry.Name))
		write([]byte{byte(entry.Kind)})
		data, _, _ := envelope.Read(entry.Name, 0, level.MaxEntrySize)
		write(data)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func (bundle *Bundle) Validate() error {
	if bundle.Producer != Producer || len(bundle.InputSHA256) != 64 || len(bundle.Sectors) > world.MaxSectors ||
		len(bundle.Solids) > world.MaxStaticSolids || len(bundle.Probes) > MaxProbes {
		return ErrCubemaps
	}
	digest, err := hex.DecodeString(bundle.InputSHA256)
	if err != nil || len(digest) != sha256.Size {
		return ErrCubemaps
	}
	var present [MaxProbes]bool
	previous := -1
	for _, probe := range bundle.Probes {
		if probe.ID <= previous || probe.ID >= MaxProbes {
			return ErrCubemaps
		}
		previous = probe.ID
		present[probe.ID] = true
		for axis := range 3 {
			if !finite(probe.Low[axis]) || !finite(probe.High[axis]) || !finite(probe.Origin[axis]) ||
				probe.Low[axis] >= probe.Origin[axis] || probe.Origin[axis] >= probe.High[axis] || probe.High[axis]-probe.Low[axis] > 65536 {
				return ErrCubemaps
			}
		}
		meta, err := qoi.Inspect(probe.Image)
		if err != nil || meta.Width != FaceSize*Faces || meta.Height != FaceSize || meta.Channels != qoi.ChannelsRGBA ||
			meta.Colorspace != qoi.ColorspaceSRGB ||
			meta.DecodedBytes != ImageBytes {
			return ErrCubemaps
		}
		if _, err := qoi.Validate(probe.Image); err != nil {
			return ErrCubemaps
		}
	}
	for _, bindings := range [][]int{bundle.Sectors, bundle.Solids} {
		for _, id := range bindings {
			if id < -1 || id >= MaxProbes || id >= 0 && !present[id] {
				return ErrCubemaps
			}
		}
	}
	return nil
}

func (bundle *Bundle) ValidateSource(envelope level.Envelope, document *world.Document) error {
	if document == nil {
		return ErrCubemaps
	}
	solids := 0
	if document.StaticSolids != nil {
		solids = len(document.StaticSolids.Items)
	}
	if len(bundle.Sectors) != len(document.Sectors) || len(bundle.Solids) != solids || bundle.InputSHA256 != InputDigest(envelope) {
		return ErrCubemaps
	}
	return bundle.Validate()
}
func finite(value float32) bool { return !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0) }
