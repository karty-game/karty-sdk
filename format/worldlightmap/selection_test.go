package worldlightmap

import (
	"bytes"
	"math"
	"testing"
)

func TestSelectedReceiversClipBeforePackingAndRetainDensity(t *testing.T) {
	document := directFixture(t)
	binding := Binding{Kind: "sector-floor", Index: 0, Edge: -1, Chart: -1}
	tile := [4]float64{0, 0, 2, 4}
	layout, err := CompileSelectedReceivers(
		document,
		Options{PageSize: 32, TexelsPerUnit: 4, AllStatic: true},
		[]int{0},
		nil,
		SurfaceSelection{Bindings: []Binding{binding}, Clip: &tile},
	)
	if err != nil {
		t.Fatal(err)
	}
	if layout.Schema != SelectedSchema || layout.Algorithm != SelectedAlgorithm || len(layout.Charts) != 1 {
		t.Fatalf("wrong selected layout: %+v", layout)
	}
	chart := layout.Charts[0]
	for _, plane := range [2][4]float64{chart.UPlane, chart.VPlane} {
		if math.Abs(math.Sqrt(plane[0]*plane[0]+plane[1]*plane[1]+plane[2]*plane[2])*32-4) > 1e-8 {
			t.Fatal("density changed")
		}
	}
	data, err := Encode(layout, &document)
	if err != nil {
		t.Fatal(err)
	}
	if string(data[:4]) != "KLM3" {
		t.Fatal("missing versioned envelope")
	}
	decoded, err := Decode(data, &document)
	if err != nil {
		t.Fatal(err)
	}
	again, err := Encode(decoded, &document)
	if err != nil || !bytes.Equal(data, again) {
		t.Fatal("noncanonical selection", err)
	}
	if _, err := DecodeLayoutArtifact(data, LayoutAdmission{SectorCount: len(document.Sectors)}); err != nil {
		t.Fatal(err)
	}
	for _, broken := range [][]byte{data[:len(data)-1], append(bytes.Clone(data), 0)} {
		if _, err := Decode(broken, &document); err == nil {
			t.Fatal("malformed wrapper accepted")
		}
	}
	decoded.Schema = Schema
	if _, err := Encode(decoded, &document); err == nil {
		t.Fatal("old schema accepted selected faces")
	}
	if _, err := CompileReceivers(document, Options{PageSize: 32}, []int{0}, nil); err == nil {
		t.Fatal("old page-size contract weakened")
	}
}

func TestSelectedReceiversRejectMalformedFacesAndClip(t *testing.T) {
	document := directFixture(t)
	binding := Binding{Kind: "sector-floor", Index: 0, Edge: -1, Chart: -1}
	for _, selection := range []SurfaceSelection{
		{Bindings: []Binding{binding, binding}},
		{Bindings: []Binding{{Kind: "unknown", Index: 0, Edge: -1, Chart: -1}}},
		{Bindings: []Binding{binding}, Clip: &[4]float64{2, 0, 1, 4}},
		{Bindings: []Binding{binding}, Clip: &[4]float64{0, 0, math.NaN(), 4}},
		{Bindings: []Binding{binding}, Clip: &[4]float64{0, 0, 2, 4}, LiquidEdgeStep: math.NaN()},
		{Bindings: []Binding{binding}, Clip: &[4]float64{0, 0, 2, 4}, LiquidEdgeStep: 3},
		{Bindings: []Binding{binding}, LiquidEdgeStep: 2},
	} {
		if _, err := CompileSelectedReceivers(document, Options{PageSize: 32, TexelsPerUnit: 1}, []int{0}, nil, selection); err == nil {
			t.Fatal("malformed selected receiver accepted")
		}
	}
}
