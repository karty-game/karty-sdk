package worldlightmap

import (
	"slices"
	"testing"
)

func TestReceiverSelectionKeepsGlobalIdentityAndPhysicalContext(t *testing.T) {
	document := directFixture(t)
	layout, err := CompileReceivers(document, Options{TexelsPerUnit: 2, PageSize: 512, Lights: []string{"red", "blue"}}, []int{0}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if layout.Algorithm != ReceiverAlgorithm || !slices.Equal(layout.RuntimeBake.LightIDs, []string{"red", "blue"}) {
		t.Fatal("neighbor light dropped within physical component")
	}
	sectors, _, err := ReceiverScope(&document, layout.Receivers)
	if err != nil || !sectors[0] || !sectors[1] {
		t.Fatal("adjacent chunk omitted from transport")
	}
	for _, binding := range layout.Bindings {
		if binding.Index == 1 && binding.Chart != -1 {
			t.Fatal("unselected sector allocated chart")
		}
	}
	encoded, err := Encode(layout, &document)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(encoded, &document); err != nil {
		t.Fatal(err)
	}
	for i := range document.Sectors[1].Walls {
		document.Sectors[1].Walls[i].Start.X += 100
		document.Sectors[1].Walls[i].End.X += 100
	}
	document.Sectors[1].Floor.C -= document.Sectors[1].Floor.A * 100
	document.Sectors[1].Ceiling.C -= document.Sectors[1].Ceiling.A * 100
	document.Contents[0].Position.X += 100
	document.Lighting.Lights[1].Position.X += 100
	layout, err = CompileReceivers(document, Options{TexelsPerUnit: 2, PageSize: 512, Lights: []string{"red", "blue"}}, []int{0}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(layout.RuntimeBake.LightIDs, []string{"red"}) {
		t.Fatal("transformed-space light transported")
	}
	sectors, _, err = ReceiverScope(&document, layout.Receivers)
	if err != nil || sectors[1] {
		t.Fatal("transformed-space geometry transported")
	}
	dark, err := CompileReceivers(document, Options{TexelsPerUnit: 2, PageSize: 512, Lights: []string{"red"}}, []int{1}, nil)
	if err != nil || len(dark.RuntimeBake.LightIDs) != 0 {
		t.Fatalf("dark component: %v", err)
	}
	if _, err := CompileReceivers(document, Options{}, []int{0, 1}, nil); err == nil {
		t.Fatal("cross-space receiver selection accepted")
	}
	if _, err := CompileReceivers(document, Options{}, []int{0, 0}, nil); err == nil {
		t.Fatal("duplicate receiver selection accepted")
	}
}
