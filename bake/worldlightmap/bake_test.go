package worldlightmapbake

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"math"
	"testing"
	"time"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
)

func room(t *testing.T, barrierHeight float64) (world.Document, worldlightmap.Layout, []Material) {
	t.Helper()
	points := []world.Vec2{{X: 0, Y: 0}, {X: 4, Y: 0}, {X: 4, Y: 4}, {X: 0, Y: 4}}
	walls := make([]world.Wall, 4)
	for i, p := range points {
		walls[i] = world.Wall{Start: p, End: points[(i+1)%4], Portal: -1, Material: 1, SourceEdge: "wall"}
	}
	d := world.Document{Version: world.Version, Sectors: []world.Sector{{ID: "room", SourceRoom: "room", Walls: walls, Floor: world.Plane{}, Ceiling: world.Plane{C: 3}, FloorMaterial: 1, CeilingMaterial: 2}}, Lighting: &world.Lighting{Version: world.LightingVersion, Lights: []world.PointLight{{ID: "white", Position: vec{X: 1, Y: 2, Z: 1}, Color: vec{X: 1, Y: 1, Z: 1}, Radius: 20}}}}
	if barrierHeight > 0 {
		d.StaticSolids = &world.StaticSolids{Version: world.StaticSolidsVersion, Items: []world.Solid{{ID: "barrier", Footprint: []world.Vec2{{X: 1.9, Y: 0}, {X: 2.1, Y: 0}, {X: 2.1, Y: 4}, {X: 1.9, Y: 4}}, Bottom: world.Plane{}, Top: world.Plane{C: barrierHeight}, SideMaterial: 1, TopMaterial: 1, BottomMaterial: 1}}}
	}
	l, err := worldlightmap.Compile(d, worldlightmap.Options{TexelsPerUnit: 4, PageSize: 512, Padding: 2, Lights: []string{"white"}, ShadowSize: 32})
	if err != nil {
		t.Fatal(err)
	}
	return d, l, []Material{{ID: 1, Albedo: solid(color.NRGBA{A: 255})}, {ID: 2, Albedo: solid(color.NRGBA{R: 255, A: 255})}}
}
func solid(c color.NRGBA) *image.NRGBA {
	im := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	im.SetNRGBA(0, 0, c)
	return im
}
func decoded(result Result, x, y int) vec {
	var v vec
	w := result.Image.Bounds().Dx() / 3
	for i := range 3 {
		p := result.Image.NRGBAAt(x+i*w, y)
		m := float64(p.A) / 255 * result.RGBMRange / 255
		v = add(v, vec{X: float64(p.R) * m / 3, Y: float64(p.G) * m / 3, Z: float64(p.B) * m / 3})
	}
	return v
}
func floorAt(l worldlightmap.Layout, p vec) (int, int) {
	chart := l.Charts[l.Bindings[0].Chart]
	w, h := float64(l.Pages[0].Width), float64(l.Pages[0].Height)
	return int(math.Round(w*(chart.UPlane[0]*p.X+chart.UPlane[1]*p.Y+chart.UPlane[2]*p.Z+chart.UPlane[3]) - .5)), int(math.Round(h*(chart.VPlane[0]*p.X+chart.VPlane[1]*p.Y+chart.VPlane[2]*p.Z+chart.VPlane[3]) - .5))
}
func TestRedCeilingBouncesIntoDirectShadow(t *testing.T) {
	d, l, materials := room(t, 1.5)
	direct, err := Bake(context.Background(), d, l, materials, Options{Samples: 128, Workers: 2})
	if err != nil {
		t.Fatal(err)
	}
	bounce, err := Bake(context.Background(), d, l, materials, Options{Samples: 128, Bounces: 1, Workers: 2, Seed: 17})
	if err != nil {
		t.Fatal(err)
	}
	x, y := floorAt(l, vec{X: 3, Y: 2})
	before, after := decoded(direct, x, y), decoded(bounce, x, y)
	if maximum(before) > 1e-8 {
		t.Fatalf("barrier failed to cast direct shadow: %+v", before)
	}
	if after.X < .01 || after.Y > .02*after.X || after.Z > .02*after.X {
		t.Fatalf("red ceiling did not produce red indirect illumination: %+v", after)
	}
	if bounce.Stats.BounceRays == 0 || bounce.Stats.ShadowRays == 0 || bounce.Stats.ReceiverTexels == 0 {
		t.Fatalf("no actual transport work: %+v", bounce.Stats)
	}
	// Replacing the source's reflectance with black eliminates that bounce.
	materials[1].Albedo = solid(color.NRGBA{A: 255})
	black, err := Bake(context.Background(), d, l, materials, Options{Samples: 128, Bounces: 1, Workers: 2, Seed: 17})
	if err != nil {
		t.Fatal(err)
	}
	if got := decoded(black, x, y); maximum(got) > 1e-8 {
		t.Fatalf("black surfaces reflected energy: %+v", got)
	}
}
func TestClosedBarrierDoesNotLeakAndCoveredBlackSurvives(t *testing.T) {
	d, l, materials := room(t, 3)
	result, err := Bake(context.Background(), d, l, materials, Options{Samples: 128, Bounces: 3, Workers: 2, Seed: 7})
	if err != nil {
		t.Fatal(err)
	}
	x, y := floorAt(l, vec{X: 3, Y: 2})
	if v := decoded(result, x, y); maximum(v) > 1e-8 {
		t.Fatalf("light leaked through closed barrier: %+v", v)
	}
	if result.Image.NRGBAAt(x, y).A == 0 {
		t.Fatal("covered black encoded as uncovered")
	}
	// Atlas space outside every reserved rectangle must remain uncovered.
	if result.Image.NRGBAAt(511, 511).A != 0 {
		t.Fatal("dilation escaped its chart")
	}
}
func TestDirectMatchesNeutralHalfLambertAndRangeBound(t *testing.T) {
	d, l, materials := room(t, 0)
	result, err := Bake(context.Background(), d, l, materials, Options{Workers: 1})
	if err != nil {
		t.Fatal(err)
	}
	chart := l.Charts[l.Bindings[0].Chart]
	x, y := floorAt(l, vec{X: 1, Y: 2})
	// Reconstruct the actual texel centre from affine planes, which for this
	// horizontal chart are independent X/Y mappings.
	px := (float64(x)+.5)/float64(l.Pages[0].Width) - chart.UPlane[3]
	py := (float64(y)+.5)/float64(l.Pages[0].Height) - chart.VPlane[3]
	p := vec{X: px / chart.UPlane[0], Y: py / chart.VPlane[1]}
	light := d.Lighting.Lights[0]
	delta := sub(light.Position, p)
	distance2 := dot(delta, delta)
	cosine := delta.Z / math.Sqrt(distance2)
	expected := math.Pow(1-distance2/(light.Radius*light.Radius), 2) * math.Pow((cosine+1)*.5, 2)
	if got := decoded(result, x, y).X; math.Abs(got-expected) > .006 {
		t.Fatalf("direct differs from neutral Half Lambert: got%g expected%g point%+v", got, expected, p)
	}
	for i := range materials {
		materials[i].Albedo = solid(color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	}
	white, err := Bake(context.Background(), d, l, materials, Options{Samples: 32, Bounces: 4, Workers: 2})
	if err != nil {
		t.Fatal(err)
	}
	bound, _ := worldlightmap.OfflineRGBMRange(l, &d, 4)
	if white.RGBMRange != bound {
		t.Fatal("storage bound differs")
	}
	if decoded(white, x, y).X <= decoded(result, x, y).X {
		t.Fatal("white bounce did not add indirect energy")
	}
	for offset := 0; offset < len(white.Image.Pix); offset += 4 {
		p := white.Image.Pix[offset : offset+4]
		for channel := range 3 {
			value := float64(p[channel]) * float64(p[3]) * bound / (255 * 255)
			if math.IsNaN(value) || value > bound {
				t.Fatal("unbounded transport")
			}
		}
	}
	// Neutral irradiance has the stricter transport bound range/3; RGBM's
	// three-coefficient storage limit alone would hide accidental extra energy.
	for y := 0; y < l.Pages[0].Height; y++ {
		for x := 0; x < l.Pages[0].Width; x++ {
			if white.Image.NRGBAAt(x, y).A == 0 {
				continue
			}
			if maximum(decoded(white, x, y)) > bound/3+.01 {
				t.Fatal("diffuse transport exceeded geometric-series energy bound")
			}
		}
	}
}
func TestDeterminismCancellationAndReflectanceIdentity(t *testing.T) {
	d, l, materials := room(t, 1.5)
	options := Options{Samples: 16, Bounces: 2, Workers: 1, Seed: 22}
	a, err := Bake(context.Background(), d, l, materials, options)
	if err != nil {
		t.Fatal(err)
	}
	options.Workers = 4
	b, err := Bake(context.Background(), d, l, materials, options)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a.Image.Pix, b.Image.Pix) || a.ReflectanceSHA256 != b.ReflectanceSHA256 || a.Stats.Rays != b.Stats.Rays {
		t.Fatal("worker scheduling changed bake")
	}
	before := a.ReflectanceSHA256
	materials[1].Albedo = solid(color.NRGBA{B: 255, A: 255})
	after, err := ReflectanceDigest(d, materials)
	if err != nil || before == after {
		t.Fatal("source colour did not invalidate reflectance")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Bake(ctx, d, l, materials, options); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
	ctx, cancel = context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	options.Samples = 256
	options.Bounces = 4
	if result, err := Bake(ctx, d, l, materials, options); !errors.Is(err, context.DeadlineExceeded) || result.Image != nil {
		t.Fatalf("partial/canceled atlas published: %v", err)
	}
}

func TestPartialPortalBandsOccludeWhileDoorwayTransmits(t *testing.T) {
	d, _, materials := room(t, 0)
	right := d.Sectors[0]
	right.ID = "right"
	right.SourceRoom = "right"
	right.Walls = append([]world.Wall(nil), right.Walls...)
	for i := range right.Walls {
		right.Walls[i].Start.X += 4
		right.Walls[i].End.X += 4
	}
	right.Floor.C = 1
	right.Ceiling.C = 2
	d.Sectors[0].Walls[1].Portal = 1
	d.Sectors[0].Walls[1].PortalWall = 4
	right.Walls[3].Portal = 0
	right.Walls[3].PortalWall = 2
	d.Sectors = append(d.Sectors, right)
	d.Lighting.Lights[0].Position.Z = .2
	l, err := worldlightmap.Compile(d, worldlightmap.Options{TexelsPerUnit: 8, PageSize: 512, Padding: 2, Lights: []string{"white"}, ShadowSize: 32})
	if err != nil {
		t.Fatal(err)
	}
	shadow, err := Bake(context.Background(), d, l, materials, Options{Workers: 1})
	if err != nil {
		t.Fatal(err)
	}
	chart := l.Charts[l.Bindings[9].Chart]
	p := vec{X: 8, Y: 2, Z: 1.5}
	x := int(math.Round(float64(l.Pages[0].Width)*(chart.UPlane[0]*p.X+chart.UPlane[1]*p.Y+chart.UPlane[2]*p.Z+chart.UPlane[3]) - .5))
	y := int(math.Round(float64(l.Pages[0].Height)*(chart.VPlane[0]*p.X+chart.VPlane[1]*p.Y+chart.VPlane[2]*p.Z+chart.VPlane[3]) - .5))
	if got := decoded(shadow, x, y); maximum(got) > 1e-8 {
		t.Fatalf("lower opaque portal band leaked: %+v", got)
	}
	d.Lighting.Lights[0].Position.Z = 1.5
	open, err := Bake(context.Background(), d, l, materials, Options{Workers: 1})
	if err != nil {
		t.Fatal(err)
	}
	if got := decoded(open, x, y); got.X < .1 {
		t.Fatalf("open portal incorrectly blocked direct light: %+v", got)
	}
}

func TestBounceUsesCompiledUVTexturePixels(t *testing.T) {
	d, l, materials := room(t, 0)
	// Every authored mapping is required when material mapping is enabled.
	projection := func(u, v world.UVPlane) *world.SurfaceUV {
		return &world.SurfaceUV{Projections: []world.UVProjection{{U: u, V: v}}, Weights: []float64{1}}
	}
	d.MaterialMapping = &world.MaterialMapping{Version: world.MaterialMappingVersion}
	d.Sectors[0].FloorUV = projection(world.UVPlane{X: .25}, world.UVPlane{Y: .25})
	d.Sectors[0].CeilingUV = projection(world.UVPlane{X: .25}, world.UVPlane{Y: .25})
	for i := range d.Sectors[0].Walls {
		d.Sectors[0].Walls[i].UV = projection(world.UVPlane{X: .25, Y: .25}, world.UVPlane{Z: .25})
	}
	texture := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	texture.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	texture.SetNRGBA(1, 0, color.NRGBA{B: 255, A: 255})
	materials[1].Albedo = texture
	pattern, err := Bake(context.Background(), d, l, materials, Options{Samples: 128, Bounces: 1, Workers: 2, Seed: 19})
	if err != nil {
		t.Fatal(err)
	}
	x, y := floorAt(l, vec{X: 2, Y: 2})
	withPattern := decoded(pattern, x, y)
	// A repeat offset of .5 changes only which half of the source ceiling is
	// red/blue; geometry and source image bytes remain identical.
	d.Sectors[0].CeilingUV.Projections[0].U.Offset = .5
	shifted, err := Bake(context.Background(), d, l, materials, Options{Samples: 128, Bounces: 1, Workers: 2, Seed: 19})
	if err != nil {
		t.Fatal(err)
	}
	withOffset := decoded(shifted, x, y)
	if withPattern.X < .005 || withPattern.Z < .005 {
		t.Fatalf("source texture halves did not both contribute: %+v", withPattern)
	}
	if math.Abs(withPattern.X-withOffset.Z) > .002 || math.Abs(withPattern.Z-withOffset.X) > .002 {
		t.Fatalf("repeat offset did not swap reflected colours: %+v / %+v", withPattern, withOffset)
	}
	if pattern.ReflectanceSHA256 == shifted.ReflectanceSHA256 {
		t.Fatal("UV edit did not invalidate reflected-light identity")
	}
}
