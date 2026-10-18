package worldterrain

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
)

func fixture() Document {
	opaque := func(w, h int, normal bool) Image {
		p := make([]byte, w*h*4)
		for i := 0; i < len(p); i += 4 {
			p[i], p[i+1], p[i+2], p[i+3] = 50, 80, 40, 255
			if normal {
				p[i], p[i+1], p[i+2] = 128, 128, 255
			}
		}
		return Image{uint16(w), uint16(h), p}
	}
	weights := opaque(2, 2, false)
	for i := 0; i < len(weights.Pixels); i += 4 {
		copy(weights.Pixels[i:i+4], []byte{255, 0, 0, 0})
	}
	return Document{
		Version:         Version,
		Origin:          [2]float32{0, 0},
		CellSize:        1,
		Columns:         3,
		Rows:            3,
		Heights:         []float32{0, 1, 0, 1, 4, 1, 0, 1, 0},
		Normals:         [][3]float32{{0, 0, 1}, {0, 0, 1}, {0, 0, 1}, {0, 0, 1}, {0, 0, 1}, {0, 0, 1}, {0, 0, 1}, {0, 0, 1}, {0, 0, 1}},
		ExteriorSectors: []uint16{0},
		Holes:           []byte{0, 0, 0, 0},
		Materials: []Material{
			{
				Albedo:         opaque(MaterialSize, MaterialSize, false),
				Normal:         opaque(MaterialSize, MaterialSize, true),
				Scale:          2,
				NormalStrength: 1,
			},
		},
		Patches:   []Patch{{X: 0, Y: 0, Columns: 2, Rows: 2, Palette: [4]uint16{}, Weights: weights}},
		FarColour: opaque(2, 2, false),
	}
}
func TestCodecOwnershipAndCanonical(t *testing.T) {
	d := fixture()
	raw, err := Encode(d)
	if err != nil {
		t.Fatal(err)
	}
	copyraw := bytes.Clone(raw)
	got, err := Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	again, err := Encode(got)
	if err != nil || !bytes.Equal(raw, again) {
		t.Fatal("noncanonical roundtrip", err)
	}
	for i := range raw {
		raw[i] = 0
	}
	if got.Materials[0].Albedo.Pixels[3] != 255 || got.Heights[4] != 4 {
		t.Fatal("decode aliases input")
	}
	clone := Clone(&got)
	clone.Heights[4] = 99
	clone.Materials[0].Albedo.Pixels[3] = 0
	if got.Heights[4] != 4 || got.Materials[0].Albedo.Pixels[3] != 255 {
		t.Fatal("clone aliases input")
	}
	if len(copyraw) > MaxEncodedSize || DecodedBytes(&d) > MaxDecodedBytes {
		t.Fatal("fixture budget")
	}
}
func TestCodecRejectMalformed(t *testing.T) {
	raw, _ := Encode(fixture())
	cases := map[string][]byte{"empty": {}, "truncated": raw[:len(raw)-1], "trailing": append(bytes.Clone(raw), 0)}
	change := func(at int, v uint32) []byte {
		b := bytes.Clone(raw)
		binary.LittleEndian.PutUint32(b[at:], v)
		return b
	}
	cases["header size"] = change(8, 1)
	cases["nonfinite origin"] = change(18, math.Float32bits(float32(math.Inf(1))))
	cases["negative zero origin"] = change(18, 0x80000000)
	b := bytes.Clone(raw)
	binary.LittleEndian.PutUint16(b[30:], 65535)
	cases["dimensions"] = b
	b = bytes.Clone(raw)
	binary.LittleEndian.PutUint16(b[36:], 65535)
	cases["materials"] = b
	for name, b := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Decode(b); err == nil {
				t.Fatal("accepted malformed payload")
			}
		})
	}
}
func TestValidation(t *testing.T) {
	cases := map[string]func(*Document){
		"normal":             func(d *Document) { d.Normals[0] = [3]float32{1, 1, 1} },
		"holes":              func(d *Document) { d.Holes[0] = 2 },
		"all holes":          func(d *Document) { d.Holes = []byte{1, 1, 1, 1} },
		"weights":            func(d *Document) { d.Patches[0].Weights.Pixels[0] = 254 },
		"palette":            func(d *Document) { d.Patches[0].Palette[0] = 1 },
		"patch gap":          func(d *Document) { d.Patches[0].Columns = 1 },
		"opaque":             func(d *Document) { d.Materials[0].Albedo.Pixels[3] = 0 },
		"far opaque":         func(d *Document) { d.FarColour.Pixels[3] = 0 },
		"nan":                func(d *Document) { d.Heights[0] = float32(math.NaN()) },
		"duplicate exterior": func(d *Document) { d.ExteriorSectors = []uint16{0, 0} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			d := fixture()
			mutate(&d)
			if Validate(&d) == nil {
				t.Fatal("accepted invalid terrain")
			}
		})
	}
}
func TestMeshAndSamplingShareDiagonalAndHoles(t *testing.T) {
	d := fixture()
	mesh, err := CompilePatch(&d, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(mesh.Vertices) != 9 || len(mesh.Indices) != 24 {
		t.Fatal("unexpected geometry")
	}
	for i := 0; i < len(mesh.Indices); i += 3 {
		a, b, c := mesh.Vertices[mesh.Indices[i]].Position, mesh.Vertices[mesh.Indices[i+1]].Position, mesh.Vertices[mesh.Indices[i+2]].Position
		if (b[0]-a[0])*(c[1]-a[1])-(b[1]-a[1])*(c[0]-a[0]) <= 0 {
			t.Fatal("downward winding")
		}
	}
	for _, p := range [][3]float64{{.25, .25, .5}, {.75, .75, 2.5}, {1, 1, 4}, {2, 2, 0}} {
		h, ok := HeightAt(&d, p[0], p[1])
		if !ok || h != p[2] {
			t.Fatalf("sample %v got %v %v", p, h, ok)
		}
	}
	d.Holes[0] = 1
	mesh, err = CompilePatch(&d, 0)
	if err != nil || len(mesh.Indices) != 18 {
		t.Fatal("hole emitted triangles", err)
	}
	if _, ok := HeightAt(&d, .5, .5); ok {
		t.Fatal("hole supplies floor")
	}
	for _, p := range [][2]float64{{-1, 0}, {0, 3}, {math.NaN(), 0}, {math.Inf(1), 0}} {
		if _, ok := HeightAt(&d, p[0], p[1]); ok {
			t.Fatal("outside sample accepted")
		}
	}
}
func TestWorldOwnership(t *testing.T) {
	d := fixture()
	w := world.Document{
		Sectors: []world.Sector{
			{
				Sky: true,
				Walls: []world.Wall{
					{Start: world.Vec2{X: 0, Y: 0}, End: world.Vec2{X: 2, Y: 0}},
					{Start: world.Vec2{X: 2, Y: 0}, End: world.Vec2{X: 2, Y: 2}},
					{Start: world.Vec2{X: 2, Y: 2}, End: world.Vec2{X: 0, Y: 2}},
					{Start: world.Vec2{X: 0, Y: 2}, End: world.Vec2{X: 0, Y: 0}},
				},
			},
		},
	}
	w.Sectors[0].Ceiling.C = 10
	for i := range w.Sectors[0].Walls {
		w.Sectors[0].Walls[i].Portal = -1
	}
	if err := ValidateForWorld(&d, &w); err != nil {
		t.Fatal(err)
	}
	w.Sectors[0].Sky = false
	if ValidateForWorld(&d, &w) == nil {
		t.Fatal("indoor floor replaced")
	}
	w.Sectors[0].Sky = true
	d.Origin[0] = -2
	if ValidateForWorld(&d, &w) == nil {
		t.Fatal("unowned cells accepted")
	}
}
func FuzzDecode(f *testing.F) {
	raw, _ := Encode(fixture())
	f.Add(raw)
	f.Add([]byte("KWT1"))
	f.Fuzz(func(t *testing.T, raw []byte) {
		d, err := Decode(raw)
		if err != nil {
			return
		}
		encoded, err := Encode(d)
		if err != nil || !bytes.Equal(encoded, raw) {
			t.Fatal("accepted noncanonical payload")
		}
	})
}
