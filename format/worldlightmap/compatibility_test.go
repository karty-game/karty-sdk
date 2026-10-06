package worldlightmap

import "testing"

// Pin canonical layout and both prebake algorithms to the pre-refactor SDK.
func TestFormatCompatibility(t *testing.T) {
	d := fixture(t)
	l, err := Compile(d, Options{})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := Encode(l, &d)
	if err != nil {
		t.Fatal(err)
	}
	if digest(encoded) != "17480092567bb5990e2357f1675ccaba84f63635eb9c7d3a41ae16e38d59e893" ||
		l.GeometrySHA256 != "4be4c013aee4bed113df4ca756d63ef417ee28370331b2c7b090ea4418c70069" {
		t.Fatal("canonical layout or geometry identity changed")
	}
	d, l, pair := prebakeFixture(t)
	encoded, err = EncodePrebake(pair, l, &d)
	if err != nil {
		t.Fatal(err)
	}
	if digest(encoded) != "d42fbb5909c5b5570763d9a65dd18431594650a86b405c7b85843c80bc66fecf" {
		t.Fatal("algorithm-1 manifest changed")
	}
	rangeValue, err := OfflineRGBMRange(l, &d, 2)
	if err != nil {
		t.Fatal(err)
	}
	pair, err = NewOfflinePrebake(
		l,
		&d,
		pair.Image,
		OfflineBakeInputs{Samples: 16, Bounces: 2, Seed: 22, RGBMRange: rangeValue, ReflectanceSHA256: digest([]byte("reflectance"))},
	)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err = EncodePrebake(pair, l, &d)
	if err != nil {
		t.Fatal(err)
	}
	if digest(encoded) != "4702a5ef723ead4c2bcfcdbf3a27d0d61284866ee074e99962edb1b83fbcf0bf" {
		t.Fatal("algorithm-2 manifest changed")
	}
}
