package world_test

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
)

func uncheckedWorld(t *testing.T, document world.Document) []byte {
	t.Helper()
	b, err := world.EncodeUncheckedForTest(document)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func word32(v uint32) []byte { b := make([]byte, 4); binary.LittleEndian.PutUint32(b, v); return b }
func replaceByte(encoded []byte, offset int, value byte) []byte {
	out := bytes.Clone(encoded)
	out[offset] = value
	return out
}
func assertBinaryMalformed(t *testing.T, encoded []byte) {
	t.Helper()
	for name, data := range map[string][]byte{
		"JSON": []byte(`{"version":3}`), "unknown version": replaceByte(encoded, 4, 2),
		"flags": replaceByte(encoded, 6, 1), "reserved": replaceByte(encoded, 12, 1),
		"wrong size": replaceByte(encoded, 8, 0), "trailing": append(bytes.Clone(encoded), 0),
	} {
		t.Run(name, func(t *testing.T) {
			d, err := world.Decode(data)
			if err == nil || len(d.Sectors) != 0 || d.Lighting != nil {
				t.Fatalf("accepted or published malformed payload: %v", err)
			}
		})
	}
}
func lightCountOverflow(t *testing.T, document world.Document) []byte {
	t.Helper()
	document.Lighting.Lights = document.Lighting.Lights[:world.MaxLights]
	b := uncheckedWorld(t, document)
	marker := append(
		word32(world.MaxLights+1),
		append(word32(uint32(len(document.Lighting.Lights[0].ID))), []byte(document.Lighting.Lights[0].ID)...)...)
	at := bytes.Index(b, marker)
	if at < 0 {
		t.Fatal("missing light count")
	}
	copy(b[at:], word32(world.MaxLights+2))
	return b
}
func TestBinaryWorldRejectsCountAndNoncanonicalFloatsBeforePublishing(t *testing.T) {
	b := encodeValid(t)
	copy(b[18:], word32(world.MaxSectors+2))
	if d, err := world.Decode(b); err == nil || len(d.Sectors) != 0 {
		t.Fatalf("count accepted: %v", err)
	}
	b = encodeValid(t)
	target := make([]byte, 8)
	binary.LittleEndian.PutUint64(target, math.Float64bits(.01))
	at := bytes.Index(b[16:], target) + 16
	if at < 16 {
		t.Fatal("missing float")
	}
	for _, bits := range []uint64{math.Float64bits(math.NaN()), math.Float64bits(math.Inf(1)), 1 << 63} {
		out := bytes.Clone(b)
		binary.LittleEndian.PutUint64(out[at:], bits)
		if _, err := world.Decode(out); err == nil {
			t.Fatalf("accepted float bits%x", bits)
		}
	}
}
