package worldlightmap

import "testing"

// Pin canonical layout and both prebake algorithms to the candidate typed binary SDK.
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
	if digest(encoded) != "2844bea4a81b4c1ccf391cf36d824facda7945eccfd0bdce8de176d6972220f0" ||
		l.GeometrySHA256 != "fc8153827dfcf7ca747f431dec300b5be4a1d6a1972dc04a2bc6df502cd9b336" {
		t.Fatalf("layout %s geometry %s", digest(encoded), l.GeometrySHA256)
	}
	d, l, pair := prebakeFixture(t)
	encoded, err = EncodePrebake(pair, l, &d)
	if err != nil {
		t.Fatal(err)
	}
	if digest(encoded) != "4409cd70e378f77bfff50d114717279b964a4f84483facc9a7a113c87952dc3f" {
		t.Fatalf("algorithm1 %s", digest(encoded))
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
	if digest(encoded) != "8f8e84138a82821141a6b6b1f27c03db05b58034af8a595f122303a34f4c2fac" {
		t.Fatalf("algorithm2 %s", digest(encoded))
	}
	// Producer revision 1 remains independently verifiable with its exact old
	// global-sum range and identity, rather than reinterpreting old pixels.
	legacyRange := offlineRGBMRangeForProducer(l, &d, 2, true)
	legacy, err := newOfflinePrebakeForProducer(l, &d, pair.Image,
		OfflineBakeInputs{Samples: 16, Bounces: 2, Seed: 22, RGBMRange: legacyRange, ReflectanceSHA256: digest([]byte("reflectance"))},
		LegacyOfflinePrebakeProducer)
	if err != nil {
		t.Fatal(err)
	}
	legacyEncoded, err := EncodePrebake(legacy, l, &d)
	if err != nil || digest(legacyEncoded) != "0270e799fc94644f936736938b43616acc44ccf1904813d173869da4590b1078" {
		t.Fatalf("legacy producer identity changed: %s %v", digest(legacyEncoded), err)
	}
	if _, err := DecodePrebake(legacyEncoded, legacy.Image, l, &d); err != nil {
		t.Fatal("legacy producer no longer validates", err)
	}
}
