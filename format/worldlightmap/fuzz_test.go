package worldlightmap

import (
	"bytes"
	"testing"
)

func FuzzDecodeLayout(f *testing.F) {
	d := fixture(f)
	l, err := Compile(d, Options{})
	if err != nil {
		f.Fatal(err)
	}
	encoded, err := Encode(l, &d)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(encoded)
	f.Add(append(bytes.Clone(encoded), ' '))
	f.Add([]byte(`{"schema":"karty.world-lightmap-layout@1"}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		layout, err := Decode(data, &d)
		if err != nil {
			return
		}
		canonical, err := Encode(layout, &d)
		if err != nil || !bytes.Equal(data, canonical) {
			t.Fatalf("round trip: %v", err)
		}
	})
}

func FuzzDecodePrebake(f *testing.F) {
	d, l, pair := prebakeFixture(f)
	encoded, err := EncodePrebake(pair, l, &d)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(encoded, pair.Image)
	f.Add(append(bytes.Clone(encoded), ' '), pair.Image)
	f.Add(encoded, []byte("qoif"))
	rangeValue, err := OfflineRGBMRange(l, &d, 2)
	if err != nil {
		f.Fatal(err)
	}
	offline, err := NewOfflinePrebake(l, &d, pair.Image, OfflineBakeInputs{Samples: 16, Bounces: 2, RGBMRange: rangeValue, ReflectanceSHA256: digest([]byte("reflectance"))})
	if err != nil {
		f.Fatal(err)
	}
	encoded, err = EncodePrebake(offline, l, &d)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(encoded, pair.Image)
	f.Fuzz(func(t *testing.T, data, im []byte) {
		pair, err := DecodePrebake(data, im, l, &d)
		if err != nil {
			return
		}
		canonical, err := EncodePrebake(pair, l, &d)
		if err != nil || !bytes.Equal(data, canonical) {
			t.Fatalf("round trip: %v", err)
		}
	})
}
