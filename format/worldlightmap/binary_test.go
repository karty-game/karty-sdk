package worldlightmap

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

func changeByte(encoded []byte, at int, value byte) []byte {
	out := bytes.Clone(encoded)
	out[at] = value
	return out
}
func TestBinaryLayoutAndManifestBoundedCanonicalRecords(t *testing.T) {
	d, l, p := prebakeFixture(t)
	encoded, err := Encode(l, &d)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := LayoutDigest(&l)
	if err != nil || hash != digest(encoded) {
		t.Fatal("layout digest differs", err)
	}
	for _, bad := range [][]byte{[]byte(`{"schema":"karty.world-lightmaps@1"}`), changeByte(encoded, 4, 2), changeByte(encoded, 6, 1), changeByte(encoded, 12, 1), append(bytes.Clone(encoded), 0)} {
		if _, err := Decode(bad, &d); err == nil {
			t.Fatal("malformed layout accepted")
		}
	}
	manifest, err := EncodePrebake(p, l, &d)
	if err != nil {
		t.Fatal(err)
	}
	header, err := DecodePrebakeManifest(manifest)
	if err != nil || header.ImageSHA256 != p.Manifest.ImageSHA256 {
		t.Fatal("manifest inspect", err)
	}
	for _, mutate := range []func(*PrebakeManifest){func(m *PrebakeManifest) { m.ImageBytes = MaxPrebakeImageSize + 1 }, func(m *PrebakeManifest) { m.Width = 1 }, func(m *PrebakeManifest) { m.Height = 1 }, func(m *PrebakeManifest) { m.ImageSHA256 = "" }, func(m *PrebakeManifest) { m.Algorithm = 42 }, func(m *PrebakeManifest) { m.Producer = "extra" }} {
		m := p.Manifest
		mutate(&m)
		raw, err := encodePrebakeManifest(m)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := DecodePrebakeManifest(raw); err == nil {
			t.Fatal("unsafe manifest inspect accepted")
		}
	}
	raw := bytes.Clone(manifest)
	binary.LittleEndian.PutUint64(raw[len(raw)-8:], math.Float64bits(math.NaN()))
	if _, err := DecodePrebakeManifest(raw); err == nil {
		t.Fatal("nonfinite manifest accepted")
	}
}
