package worldreflection_test

import (
	"bytes"
	"image"
	"math"
	"testing"

	"github.com/karty-game/karty-sdk/codec/qoi"
	"github.com/karty-game/karty-sdk/format/level"
	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldreflection"
)

func fixture(t *testing.T) (worldreflection.Bundle, level.Envelope) {
	t.Helper()
	metadata, err := level.EncodeMetadata(level.Metadata{})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := level.Encode(metadata, []level.SourceEntry{{Name: "input", Kind: level.EntryData, Data: []byte("static world")}})
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := level.Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	pixels := image.NewNRGBA(image.Rect(0, 0, worldreflection.FaceSize*worldreflection.Faces, worldreflection.FaceSize))
	for index := range pixels.Pix {
		pixels.Pix[index] = 255
	}
	encoded, _, err := qoi.Encode(pixels, qoi.Options{Channels: qoi.ChannelsRGBA, Colorspace: qoi.ColorspaceSRGB})
	if err != nil {
		t.Fatal(err)
	}
	return worldreflection.Bundle{
		Producer:    worldreflection.Producer,
		InputSHA256: worldreflection.InputDigest(envelope),
		Sectors:     []int{7, -1},
		Solids:      []int{7},
		Probes: []worldreflection.Probe{
			{ID: 7, Origin: [3]float32{1, 2, 3}, Low: [3]float32{-2, -3, -4}, High: [3]float32{4, 5, 6}, Image: encoded},
		},
	}, envelope
}

func TestSavedCubemapRoundTripAndInputIdentity(t *testing.T) {
	t.Parallel()
	bundle, envelope := fixture(t)
	data, err := worldreflection.Encode(bundle)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := worldreflection.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	again, err := worldreflection.Encode(decoded)
	if err != nil || !bytes.Equal(again, data) {
		t.Fatalf("noncanonical round trip: %v", err)
	}
	document := &world.Document{Sectors: make([]world.Sector, 2), StaticSolids: &world.StaticSolids{Items: make([]world.Solid, 1)}}
	if err := decoded.ValidateSource(envelope, document); err != nil {
		t.Fatal(err)
	}
	raw, err := level.Encode(
		envelope.Metadata,
		[]level.SourceEntry{
			{Name: "input", Kind: level.EntryData, Data: []byte("static world")},
			{Name: worldreflection.EntryName, Kind: level.EntryData, Data: data},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	extended, err := level.Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := decoded.ValidateSource(extended, document); err != nil {
		t.Fatal("self entry changed input identity", err)
	}
	extended.Metadata = []byte("changed lighting")
	if decoded.ValidateSource(extended, document) == nil {
		t.Fatal("changed inputs accepted")
	}
	if decoded.ValidateSource(envelope, &world.Document{}) == nil {
		t.Fatal("incomplete binding table accepted")
	}
}

func TestSavedCubemapRejectsMalformedCompleteBatch(t *testing.T) {
	t.Parallel()
	mutations := map[string]func(*worldreflection.Bundle){
		"missing image":      func(b *worldreflection.Bundle) { b.Probes[0].Image = nil },
		"last image invalid": func(b *worldreflection.Bundle) { b.Probes = append(b.Probes, worldreflection.Probe{ID: 8}) },
		"duplicate":          func(b *worldreflection.Bundle) { b.Probes = append(b.Probes, b.Probes[0]) },
		"missing binding":    func(b *worldreflection.Bundle) { b.Solids[0] = 8 },
		"invalid binding":    func(b *worldreflection.Bundle) { b.Sectors[0] = -2 },
		"infinite":           func(b *worldreflection.Bundle) { b.Probes[0].High[0] = float32(math.Inf(1)) },
		"origin on wall":     func(b *worldreflection.Bundle) { b.Probes[0].Origin[0] = b.Probes[0].Low[0] },
		"producer":           func(b *worldreflection.Bundle) { b.Producer = "other" },
		"digest":             func(b *worldreflection.Bundle) { b.InputSHA256 = "invalid" },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			bundle, _ := fixture(t)
			mutate(&bundle)
			if _, err := worldreflection.Encode(bundle); err == nil {
				t.Fatal("accepted invalid bundle")
			}
		})
	}
	bundle, _ := fixture(t)
	data, err := worldreflection.Encode(bundle)
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range []int{0, 3, 8, 64, len(data) - 1} {
		if _, err := worldreflection.Decode(data[:size]); err == nil {
			t.Fatalf("accepted truncation %d", size)
		}
	}
	if _, err := worldreflection.Decode(append(data, 0)); err == nil {
		t.Fatal("accepted trailing bytes")
	}
}
