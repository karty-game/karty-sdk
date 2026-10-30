package character

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

func fixture() Document {
	return Document{
		Banks: []Bank{
			{
				ID: "bank",
				Clips: []Clip{
					{
						Name:     "walk",
						Duration: 1,
						Tracks:   []Track{{Node: "root", Path: Translation, Times: []float32{0, 1}, Values: []float32{0, 0, 0, 1, 2, 3}}},
					},
				},
			},
		},
		Models: []Model{
			{
				ID:          "model",
				Nodes:       []Node{{Name: "root", Parent: -1, Rotation: [4]float32{0, 0, 0, 1}, Scale: [3]float32{1, 1, 1}}},
				Joints:      []uint16{0},
				InverseBind: []float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1},
				Vertices:    []float32{0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0},
				Indices:     []uint32{0, 0, 0},
				Width:       1,
				Height:      1,
				AlbedoQOI:   []byte{'q', 'o', 'i', 'f', 0, 0, 0, 1, 0, 0, 0, 1, 4, 0, 0xff, 255, 255, 255, 255, 0, 0, 0, 0, 0, 0, 0, 1},
			},
		},
	}
}

func TestRoundTripAndOwnership(t *testing.T) {
	original := fixture()
	encoded, err := Encode(original)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := Encode(*decoded)
	if err != nil || !bytes.Equal(encoded, canonical) {
		t.Fatalf("noncanonical roundtrip: %v", err)
	}
	clear(encoded)
	if decoded.Models[0].AlbedoQOI[0] != 'q' || decoded.Banks[0].Clips[0].Tracks[0].Values[3] != 1 {
		t.Fatal("decode borrows input")
	}
	decoded.Models[0].Nodes[0].Name = "changed"
	decoded.Models[0].Vertices[0] = 42
	if original.Models[0].Nodes[0].Name != "root" || original.Models[0].Vertices[0] != 0 {
		t.Fatal("decode aliases producer")
	}
}

func TestRejectMalformed(t *testing.T) {
	tests := map[string]func(*Document){
		"missing bank":    func(d *Document) { d.Banks = nil },
		"duplicate model": func(d *Document) { d.Models = append(d.Models, d.Models[0]) },
		"cycle":           func(d *Document) { d.Models[0].Nodes[0].Parent = 0 },
		"bad future parent": func(d *Document) {
			d.Models[0].Nodes = append(d.Models[0].Nodes, Node{Parent: 999})
			d.Models[0].Nodes[0].Parent = 1
		},
		"singular bind":        func(d *Document) { d.Models[0].InverseBind[0] = 0 },
		"projective bind":      func(d *Document) { d.Models[0].InverseBind[3] = .1 },
		"missing target":       func(d *Document) { d.Banks[0].Clips[0].Tracks[0].Node = "missing" },
		"duplicate channel":    func(d *Document) { c := &d.Banks[0].Clips[0]; c.Tracks = append(c.Tracks, c.Tracks[0]) },
		"unordered keys":       func(d *Document) { d.Banks[0].Clips[0].Tracks[0].Times[1] = 0 },
		"out of clip":          func(d *Document) { d.Banks[0].Clips[0].Tracks[0].Times[1] = 2 },
		"nonfinite":            func(d *Document) { d.Models[0].Vertices[0] = float32(math.Inf(1)) },
		"quaternion":           func(d *Document) { d.Models[0].Nodes[0].Rotation[3] = 0 },
		"fractional joint":     func(d *Document) { d.Models[0].Vertices[8] = .5 },
		"bad weights":          func(d *Document) { d.Models[0].Vertices[12] = .5 },
		"index":                func(d *Document) { d.Models[0].Indices[0] = 1 },
		"texture mismatch":     func(d *Document) { d.Models[0].Width = 2 },
		"linear albedo":        func(d *Document) { d.Models[0].AlbedoQOI[13] = 1 },
		"texture truncated op": func(d *Document) { d.Models[0].AlbedoQOI[14] = 0xfe },
	}
	for name, modify := range tests {
		t.Run(name, func(t *testing.T) {
			d := fixture()
			modify(&d)
			if _, err := Encode(d); err == nil {
				t.Fatal("accepted invalid resource")
			}
		})
	}
}

func TestTruncationAndBounds(t *testing.T) {
	data, err := Encode(fixture())
	if err != nil {
		t.Fatal(err)
	}
	for size := range len(data) {
		if _, err = Decode(data[:size]); err == nil {
			t.Fatalf("accepted prefix %d", size)
		}
	}
	if _, err = Decode(append(bytes.Clone(data), 0)); err == nil {
		t.Fatal("accepted trailing byte")
	}
	for _, offset := range []int{4, 6, 8, 10, 12} {
		bad := bytes.Clone(data)
		binary.LittleEndian.PutUint16(bad[offset:], 65535)
		if _, err = Decode(bad); err == nil {
			t.Fatalf("accepted bad header/count at %d", offset)
		}
	}
}

func FuzzDecode(f *testing.F) {
	data, err := Encode(fixture())
	if err != nil {
		f.Fatal(err)
	}
	f.Add(data)
	f.Add([]byte("KCHR"))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, input []byte) {
		d, err := Decode(input)
		if err != nil {
			return
		}
		canonical, err := Encode(*d)
		if err != nil || !bytes.Equal(canonical, input) {
			t.Fatalf("accepted noncanonical data: %v", err)
		}
	})
}
