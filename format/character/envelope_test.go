package character

import (
	"encoding/binary"
	"testing"
)

func TestHierarchyEnvelopeRejectsAccumulatedScale(t *testing.T) {
	for _, scale := range []float32{1e6, 1e-6} {
		document := fixture()
		document.Models[0].Nodes[0].Scale = [3]float32{scale, scale, scale}
		document.Models[0].Nodes = append(
			document.Models[0].Nodes,
			Node{Name: "child", Parent: 0, Rotation: [4]float32{0, 0, 0, 1}, Scale: [3]float32{scale, scale, scale}},
		)
		if err := document.Validate(); err == nil {
			t.Fatalf("accepted accumulated scale %g", scale)
		}
	}
	document := fixture()
	document.Models[0].Nodes = append(
		document.Models[0].Nodes,
		Node{Name: "child", Parent: 0, Rotation: [4]float32{0, 0, 0, 1}, Scale: [3]float32{1, 1, 1}},
	)
	document.Banks[0].Clips[0].Tracks = append(
		document.Banks[0].Clips[0].Tracks,
		Track{Node: "root", Path: Scale, Times: []float32{0}, Values: []float32{1e6, 1e6, 1e6}},
		Track{Node: "child", Path: Scale, Times: []float32{0}, Values: []float32{1e6, 1e6, 1e6}},
	)
	if err := document.Validate(); err == nil {
		t.Fatal("accepted overflowing animated hierarchy")
	}
}

func TestLargestRGBAImageFitsPerImageBound(t *testing.T) {
	document := fixture()
	model := &document.Models[0]
	model.Width, model.Height = MaxTextureSize, MaxTextureSize
	model.AlbedoQOI = make([]byte, MaxImageBytes)
	copy(model.AlbedoQOI, "qoif")
	binary.BigEndian.PutUint32(model.AlbedoQOI[4:], MaxTextureSize)
	binary.BigEndian.PutUint32(model.AlbedoQOI[8:], MaxTextureSize)
	model.AlbedoQOI[12] = 4
	for at := 14; at < len(model.AlbedoQOI)-8; at += 5 {
		model.AlbedoQOI[at] = 255
		model.AlbedoQOI[at+1] = byte(at)
		model.AlbedoQOI[at+2] = byte(at >> 8)
		model.AlbedoQOI[at+3] = byte(at >> 16)
		model.AlbedoQOI[at+4] = 255
	}
	model.AlbedoQOI[len(model.AlbedoQOI)-1] = 1
	encoded, err := Encode(document)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Decode(encoded); err != nil {
		t.Fatal(err)
	}
}
