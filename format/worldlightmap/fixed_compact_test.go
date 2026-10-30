package worldlightmap

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

func compactFormatFixture(t *testing.T) (Layout, FixedLighting) {
	t.Helper()
	_, _, lighting, _ := fixedFormatFixture(t)
	document := directFixture(t)
	layout, err := CompileSelectedReceivers(document, Options{PageSize: 32, TexelsPerUnit: 2, AllStatic: true}, []int{0}, nil,
		SurfaceSelection{Bindings: []Binding{{Kind: "sector-floor", Index: 0, Edge: -1, Chart: -1}}, Clip: &[4]float64{0, 0, 2, 4}})
	if err != nil {
		t.Fatal(err)
	}
	lighting.GeometrySHA256 = layout.GeometrySHA256
	lighting.Schema, lighting.Producer = FixedCompactSchema, FixedSpatialProducer
	lighting.Probes = nil
	lighting.ImageBytes, lighting.Width, lighting.Height, lighting.ImageSHA256 = 0, 0, 0, ""
	lighting.ConstantRNM = make([]ChartRNM, len(layout.Charts))
	for index := range lighting.ConstantRNM {
		lighting.ConstantRNM[index].Chart = index
	}
	lighting.LayoutSHA256, err = LayoutDigest(&layout)
	if err != nil {
		t.Fatal(err)
	}
	return layout, lighting
}

func TestCompactFixedWireRoundTripAndCoveredBlack(t *testing.T) {
	layout, lighting := compactFormatFixture(t)
	encoded, err := EncodeFixedLighting(lighting)
	if err != nil || string(encoded[:4]) != "KFL2" {
		t.Fatal("compact wire", err)
	}
	decoded, err := DecodeFixedLightingArtifact(encoded, nil, layout)
	if err != nil || len(decoded.ConstantRNM) == 0 || decoded.ConstantRNM[0].Coefficients != [9]float32{} {
		t.Fatal("covered black missing", decoded.ConstantRNM, err)
	}
	again, err := EncodeFixedLighting(decoded)
	if err != nil || !bytes.Equal(encoded, again) {
		t.Fatal("noncanonical compact wire", err)
	}
	if err := ValidateFixedLightingArtifact(decoded, []byte{1}, layout); err == nil {
		t.Fatal("compact manifest admitted an image")
	}
	clear(encoded)
	if decoded.ConstantRNM[0].Chart != 0 || decoded.Schema != FixedCompactSchema {
		t.Fatal("compact payload borrows input")
	}
}

func TestCompactFixedWireRejectsMalformedCoefficientsAndSchemas(t *testing.T) {
	_, lighting := compactFormatFixture(t)
	encoded, err := EncodeFixedLighting(lighting)
	if err != nil {
		t.Fatal(err)
	}
	for _, broken := range [][]byte{encoded[:len(encoded)-1], append(bytes.Clone(encoded), 0)} {
		if _, err := DecodeFixedLighting(broken); err == nil {
			t.Fatal("malformed compact wire accepted")
		}
	}
	broken := bytes.Clone(encoded)
	copy(broken[:4], "KFL1")
	if _, err := DecodeFixedLighting(broken); err == nil {
		t.Fatal("compact payload accepted under legacy magic")
	}
	for _, value := range []float32{-1, float32(math.NaN()), float32(math.Inf(1)), float32(math.Copysign(0, -1))} {
		lighting.ConstantRNM[0].Coefficients[0] = value
		if _, err := EncodeFixedLighting(lighting); err == nil {
			t.Fatal("invalid coefficient accepted", value)
		}
	}
	lighting.ConstantRNM[0].Coefficients[0] = 0
	lighting.ConstantRNM[0].Chart = 1
	if _, err := EncodeFixedLighting(lighting); err == nil {
		t.Fatal("noncanonical chart order accepted")
	}
	broken = bytes.Clone(encoded)
	binary.LittleEndian.PutUint32(broken[len(broken)-len(lighting.ConstantRNM)*40-4:], MaxCharts+1)
	if _, err := DecodeFixedLighting(broken); err == nil {
		t.Fatal("unbounded chart count accepted")
	}
}

func TestCompactFixedAcceptsFloat32RoundedRange(t *testing.T) {
	_, lighting := compactFormatFixture(t)
	lighting.RGBMRange = 1.1
	lighting.ConstantRNM[0].Coefficients[0] = float32(lighting.RGBMRange)
	if _, err := EncodeFixedLighting(lighting); err != nil {
		t.Fatal("valid float32 coefficient rounded above float64 range", err)
	}
}
