package world_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"math/rand/v2"
	"reflect"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
)

func frameWorld(t *testing.T) world.Document {
	t.Helper()
	d := validWorld()
	d.Version = world.Version
	d.MaterialMapping = &world.MaterialMapping{Version: 1}
	d.MaterialLayers = &world.MaterialLayers{Version: 1}
	d.Sectors[0].Walls[1].PortalWall = 4
	d.Sectors[1].Walls[3].PortalWall = 2
	uv := func() *world.SurfaceUV {
		return &world.SurfaceUV{Projections: []world.UVProjection{{U: world.UVPlane{X: 1}, V: world.UVPlane{Z: 1}}}, Weights: []float64{1}}
	}
	for si := range d.Sectors {
		s := &d.Sectors[si]
		s.FloorUV = uv()
		s.CeilingUV = uv()
		for wi := range s.Walls {
			s.Walls[wi].UV = uv()
		}
	}
	return d
}
func frameConfig() world.WallFrameConfig {
	piece := func(id uint32) world.FramePiece {
		return world.FramePiece{Material: id, Coverage: world.FrameCoverageOpaque}
	}
	return world.WallFrameConfig{
		Top:     world.FrameHorizontal{Height: .25, Repeat: 1, Piece: piece(10)},
		Bottom:  world.FrameHorizontal{Height: .4, Repeat: 1, Piece: piece(11)},
		Start:   world.FrameVertical{Width: .15, Repeat: 1, Piece: piece(12)},
		End:     world.FrameVertical{Width: .15, Repeat: 1, Piece: piece(13)},
		Patches: [4]world.FramePiece{piece(14), piece(15), piece(16), piece(17)},
	}
}
func compileFrame(t *testing.T, d *world.Document, si, wi int, cfg world.WallFrameConfig) {
	t.Helper()
	p, err := world.ProfileForWall(d, si, wi)
	if err != nil {
		t.Fatal(err)
	}
	r, err := world.CompileWallFrame(d.Sectors[si].Walls[wi], p, cfg)
	if err != nil {
		t.Fatal(err)
	}
	d.Sectors[si].Walls[wi].FrameRegions = r
}
func TestFrameCompilationCanonicalAndLegacyEncoding(t *testing.T) {
	t.Parallel()
	d := frameWorld(t)
	compileFrame(t, &d, 0, 0, frameConfig())
	if len(d.Sectors[0].Walls[0].FrameRegions) != 9 {
		t.Fatalf("expected nine regions: %+v", d.Sectors[0].Walls[0].FrameRegions)
	}
	encoded, err := world.Encode(d)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := world.Decode(encoded)
	if err != nil || !reflect.DeepEqual(d, decoded) {
		t.Fatalf("roundtrip: %v", err)
	}
	for _, mutation := range [][]byte{
		bytes.Replace(encoded, []byte(`"material_layers":{"version":1}`), []byte(`"material_layers":null`), 1),
		bytes.Replace(encoded, []byte(`"coverage":"opaque"`), []byte(`"coverage":"opaque","coverage":"opaque"`), 1),
		bytes.Replace(encoded, []byte(`"repeat_u":true`), []byte(`"repeat_u":null`), 1),
	} {
		if _, err := world.Decode(mutation); err == nil {
			t.Fatal("accepted noncanonical frame fields")
		}
	}
	legacy := validWorld()
	encoded, err = world.Encode(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("frame_regions")) || bytes.Contains(encoded, []byte("material_layers")) {
		t.Fatal("legacy encoding changed")
	}
}
func TestFramePartitionsFollowSlopesPortalsAndCrop(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name         string
		sector, wall int
		change       func(*world.Document, *world.WallFrameConfig)
	}{
		{"slope", 0, 0, func(d *world.Document, f *world.WallFrameConfig) {
			d.Sectors[0].Floor.A = .2
			d.Sectors[0].Ceiling.A = .1
		}},
		{"short", 0, 0, func(d *world.Document, f *world.WallFrameConfig) { d.Sectors[0].Ceiling.C = .3 }},
		{"narrow", 0, 0, func(d *world.Document, f *world.WallFrameConfig) { f.Start.Width = 3; f.End.Width = 4 }},
		{"portal", 0, 1, func(d *world.Document, f *world.WallFrameConfig) {
			d.Sectors[0].Walls[1].PortalWall = 4
			d.Sectors[1].Walls[3].PortalWall = 2
			d.Sectors[1].Floor.C = .5
			d.Sectors[1].Ceiling.C = 2.5
		}},
		{"crossing portal slope", 0, 1, func(d *world.Document, f *world.WallFrameConfig) {
			d.Sectors[0].Walls[1].PortalWall = 4
			d.Sectors[1].Walls[3].PortalWall = 2
			d.Sectors[1].Floor.B = .3
			d.Sectors[1].Floor.C = -.5
			d.Sectors[1].Ceiling.B = -.2
			d.Sectors[1].Ceiling.C = 3.5
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			d := frameWorld(t)
			f := frameConfig()
			c.change(&d, &f)
			compileFrame(t, &d, c.sector, c.wall, f)
			if err := world.Validate(&d); err != nil {
				t.Fatal(err)
			}
			if len(d.Sectors[c.sector].Walls[c.wall].FrameRegions) > world.MaxFrameRegionsPerWall {
				t.Fatal("unbounded frame")
			}
			if c.name == "slope" {
				for _, r := range d.Sectors[0].Walls[0].FrameRegions {
					if r.Material == 11 {
						v := r.UV.Projections[0].V
						if math.Abs(v.X-.5) > 1e-9 || math.Abs(v.Z+2.5) > 1e-9 {
							t.Fatalf("band UV does not track slope: %+v", v)
						}
					}
				}
			}
		})
	}
}
func TestFrameValidationRejectsMalformedFinalRegions(t *testing.T) {
	t.Parallel()
	mutations := map[string]func(*world.Document){
		"missing marker":  func(d *world.Document) { d.MaterialLayers = nil },
		"wrong marker":    func(d *world.Document) { d.MaterialLayers.Version = 2 },
		"missing mapping": func(d *world.Document) { d.MaterialMapping = nil },
		"decomposition":   func(d *world.Document) { d.Sectors[0].Walls[0].SourceEdge = "" },
		"gap": func(d *world.Document) {
			w := &d.Sectors[0].Walls[0]
			w.FrameRegions = w.FrameRegions[:len(w.FrameRegions)-1]
		},
		"overlap": func(d *world.Document) {
			w := &d.Sectors[0].Walls[0]
			w.FrameRegions = append(w.FrameRegions, w.FrameRegions[0])
		},
		"escape": func(d *world.Document) {
			w := &d.Sectors[0].Walls[0]
			w.FrameRegions[len(w.FrameRegions)-1].Vertices[0].X = -.1
		},
		"nonfinite": func(d *world.Document) {
			w := &d.Sectors[0].Walls[0]
			w.FrameRegions[len(w.FrameRegions)-1].Vertices[0].Y = math.NaN()
		},
		"unknown coverage": func(d *world.Document) {
			w := &d.Sectors[0].Walls[0]
			w.FrameRegions[len(w.FrameRegions)-1].Coverage = "blend"
		},
		"bad UV": func(d *world.Document) {
			w := &d.Sectors[0].Walls[0]
			w.FrameRegions[len(w.FrameRegions)-1].UV.Weights[0] = .5
		},
		"opaque outside domain": func(d *world.Document) {
			w := &d.Sectors[0].Walls[0]
			w.FrameRegions[len(w.FrameRegions)-1].UV.Projections[0].U.Offset += 2
		},
		"too many vertices": func(d *world.Document) {
			w := &d.Sectors[0].Walls[0]
			r := &w.FrameRegions[len(w.FrameRegions)-1]
			r.Vertices = append(r.Vertices, r.Vertices[0])
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d := frameWorld(t)
			compileFrame(t, &d, 0, 0, frameConfig())
			mutate(&d)
			if err := world.Validate(&d); err == nil {
				t.Fatal("invalid complete partition accepted")
			}
		})
	}
	d := frameWorld(t)
	d.Sectors[0].Walls[1].PortalWall = 4
	d.Sectors[1].Walls[3].PortalWall = 2
	d.Sectors[1].Floor.C = .5
	d.Sectors[1].Ceiling.C = 2.5
	compileFrame(t, &d, 0, 1, frameConfig())
	w := &d.Sectors[0].Walls[1]
	w.FrameRegions = append(
		w.FrameRegions,
		world.WallFrameRegion{
			Material: w.Material,
			Coverage: world.FrameCoverageMain,
			Vertices: []world.Vec2{{X: 0, Y: 1}, {X: 1, Y: 1}, {X: 1, Y: 2}, {X: 0, Y: 2}},
		},
	)
	if err := world.Validate(&d); !errors.Is(err, world.ErrGeometry) {
		t.Fatalf("portal aperture: %v", err)
	}
}
func TestMaterialIDsStableLayersAndCoverageDomains(t *testing.T) {
	t.Parallel()
	d := frameWorld(t)
	uv := d.Sectors[0].Walls[0].UV
	d.Sectors[0].FloorSecondary = &world.SurfaceSecondary{Material: 30, Strength: .5, UV: uv}
	d.Sectors[0].Walls[0].Secondary = &world.SurfaceSecondary{Material: 3, Strength: 1, UV: uv}
	f := frameConfig()
	f.Top.Offset.Y = .1
	compileFrame(t, &d, 0, 0, f)
	ids := world.MaterialIDs(&d)
	if !reflect.DeepEqual(ids[:4], []uint32{1, 2, 3, 0}) || ids[4] != 4 || ids[5] != 30 {
		t.Fatalf("primary-first traversal: %v", ids)
	}
	foundMasked := false
	for _, r := range d.Sectors[0].Walls[0].FrameRegions {
		if r.Material == 10 {
			foundMasked = r.Coverage == world.FrameCoverageMasked
		}
	}
	if !foundMasked {
		t.Fatal("opaque tile with uncovered UV domain was incorrectly optimized")
	}
	if err := world.Validate(&d); err != nil {
		t.Fatal(err)
	}
	d.Sectors[0].FloorSecondary.Strength = 2
	if err := world.Validate(&d); !errors.Is(err, world.ErrMaterialMapping) {
		t.Fatalf("invalid secondary: %v", err)
	}
	// The compiled record is meaningful public data, rather than a source recipe.
	b, err := json.Marshal(d.Sectors[0].Walls[0].FrameRegions[0])
	if err != nil || bytes.Contains(b, []byte("frame")) {
		t.Fatal("unresolved frame recipe escaped compilation")
	}
}

func TestFrameCompilerDeterministicProfileSweep(t *testing.T) {
	t.Parallel()
	random := rand.New(rand.NewPCG(42, 17))
	for fixture := 0; fixture < 250; fixture++ {
		d := frameWorld(t)
		d.Contents = nil
		for si := range d.Sectors {
			floor0, floor1 := random.Float64()*4-2, random.Float64()*4-2
			ceiling0, ceiling1 := floor0+.01+random.Float64()*4, floor1+.01+random.Float64()*4
			d.Sectors[si].Floor = world.Plane{B: (floor1 - floor0) / 4, C: floor0}
			d.Sectors[si].Ceiling = world.Plane{B: (ceiling1 - ceiling0) / 4, C: ceiling0}
		}
		f := frameConfig()
		f.Top.Height = .001 + random.Float64()*4
		f.Bottom.Height = .001 + random.Float64()*4
		f.Start.Width = .001 + random.Float64()*6
		f.End.Width = .001 + random.Float64()*6
		compileFrame(t, &d, 0, 1, f)
		if err := world.Validate(&d); err != nil {
			t.Fatalf("profile fixture %d: %v", fixture, err)
		}
	}
}

func TestCompiledFrameMarkerPreservesEmptyPortalParticipation(t *testing.T) {
	t.Parallel()
	d := frameWorld(t)
	d.Sectors[1].Floor, d.Sectors[1].Ceiling = d.Sectors[0].Floor, d.Sectors[0].Ceiling
	w := &d.Sectors[0].Walls[1]
	w.FrameCompiled = true
	compileFrame(t, &d, 0, 1, frameConfig())
	if len(w.FrameRegions) != 0 {
		t.Fatal("fully open portal unexpectedly has solid regions")
	}
	b, err := world.Encode(d)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(b, []byte(`"frame_compiled":true`)) {
		t.Fatal("empty portal lost compiled frame marker")
	}
	round, err := world.Decode(b)
	if err != nil || !round.Sectors[0].Walls[1].FrameCompiled {
		t.Fatalf("marker roundtrip: %v", err)
	}
	if _, err := world.Decode(bytes.Replace(b, []byte(`"frame_compiled":true`), []byte(`"frame_compiled":null`), 1)); err == nil {
		t.Fatal("accepted null compiled frame marker")
	}
	d.MaterialLayers = nil
	if err := world.Validate(&d); !errors.Is(err, world.ErrMaterialMapping) {
		t.Fatalf("missing declaration: %v", err)
	}
	d = frameWorld(t)
	d.Sectors[0].Walls[0].FrameCompiled = true
	if err := world.Validate(&d); !errors.Is(err, world.ErrGeometry) {
		t.Fatalf("marked opaque wall missing complete partition: %v", err)
	}
	d = frameWorld(t)
	d.Sectors[0].Walls[1].FrameCompiled = true
	d.Sectors[0].Walls[1].SourceEdge = ""
	if err := world.Validate(&d); !errors.Is(err, world.ErrMaterialMapping) {
		t.Fatalf("marked decomposition edge: %v", err)
	}
}

func TestFrameUVMatchesNineSliceImageRows(t *testing.T) {
	t.Parallel()
	d := frameWorld(t)
	compileFrame(t, &d, 0, 0, frameConfig())
	w := d.Sectors[0].Walls[0]
	for _, r := range w.FrameRegions {
		if r.Material != 10 && r.Material != 11 {
			continue
		}
		uv := r.UV.Projections[0].V
		for _, p := range r.Vertices {
			x, y := w.Start.X+(w.End.X-w.Start.X)*p.X, w.Start.Y+(w.End.Y-w.Start.Y)*p.X
			floor := d.Sectors[0].Floor.A*x + d.Sectors[0].Floor.B*y + d.Sectors[0].Floor.C
			ceiling := d.Sectors[0].Ceiling.A*x + d.Sectors[0].Ceiling.B*y + d.Sectors[0].Ceiling.C
			got := uv.X*x + uv.Y*y + uv.Z*p.Y + uv.Offset
			want := (ceiling - p.Y) / .25
			if r.Material == 11 {
				want = (floor + .4 - p.Y) / .4
			}
			if math.Abs(got-want) > 1e-9 {
				t.Fatalf("material%d image-row V=%g want%g", r.Material, got, want)
			}
		}
	}
}
