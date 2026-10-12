package worldparticles_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"reflect"
	"testing"

	"github.com/karty-game/karty-sdk/format/worldparticles"
)

func TestPremultipliedParticleMaterialRequiresVersion2(t *testing.T) {
	t.Parallel()
	document := fixture()
	document.Version = worldparticles.AlphaVersion
	preset := &document.Presets[0]
	preset.Sprite.Blend = "premultiplied"
	preset.Sprite.Frames = []uint32{17}
	preset.Sprite.FPS = 0
	// Emissive RGB survives zero curve opacity in this material. The format
	// retains the independently authored channels without premultiplying them.
	preset.ColorOverLife[0].RGBA = [4]float64{.96862745, .32156864, 0, 0}
	if err := worldparticles.ValidatePreset(preset); !errors.Is(err, worldparticles.ErrParticle) {
		t.Fatal("legacy helper accepted premultiplied material", err)
	}
	encoded, err := worldparticles.Encode(document)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := worldparticles.Decode(encoded)
	if err != nil || !reflect.DeepEqual(document, decoded) {
		t.Fatalf("premultiplied channels changed during roundtrip: %v", err)
	}
	again, err := worldparticles.Encode(decoded)
	if err != nil || !bytes.Equal(encoded, again) {
		t.Fatal("noncanonical premultiplied bytes", err)
	}
	for _, version := range []uint16{worldparticles.Version, 3} {
		forged := bytes.Clone(encoded)
		binary.LittleEndian.PutUint16(forged[16:18], version)
		if _, err := worldparticles.Decode(forged); err == nil {
			t.Fatalf("premultiplied accepted for document version %d", version)
		}
	}
	preset.Sprite.Blend = "premultiplied-unknown"
	if _, err := worldparticles.Encode(document); !errors.Is(err, worldparticles.ErrParticle) {
		t.Fatal("unknown material accepted", err)
	}
}
