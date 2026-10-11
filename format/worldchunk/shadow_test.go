package worldchunk

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
)

func TestShadowProxyOfflineRoundTripAndSourceCoverage(t *testing.T) {
	document := fixture(t)
	proxy, err := CompileShadowProxy(document)
	if err != nil || len(proxy.Triangles) == 0 {
		t.Fatalf("compile shadow: %v", err)
	}
	encoded, err := EncodeShadowProxy(proxy, &document)
	if err != nil {
		t.Fatal(err)
	}
	second, err := CompileShadowProxy(document)
	if err != nil {
		t.Fatal(err)
	}
	again, err := EncodeShadowProxy(second, &document)
	if err != nil || !bytes.Equal(encoded, again) {
		t.Fatalf("shadow compilation is not deterministic: %v", err)
	}
	decoded, err := DecodeShadowProxy(encoded, &document)
	if err != nil || !slices.Equal(decoded.Triangles, proxy.Triangles) ||
		DecodedShadowBytes(decoded) != uint64(len(proxy.Triangles))*68 {
		t.Fatalf("shadow round trip: %v", err)
	}
	for name, change := range map[string]func(*ShadowProxy){
		"missing face": func(p *ShadowProxy) { p.Triangles = p.Triangles[1:] },
		"extra face":   func(p *ShadowProxy) { p.Triangles = append(p.Triangles, p.Triangles[0]) },
		"position":     func(p *ShadowProxy) { p.Triangles[0].Positions[0][0] += .1 },
		"normal":       func(p *ShadowProxy) { p.Triangles[0].Normal[2] *= -1 },
		"material":     func(p *ShadowProxy) { p.Triangles[0].Material++ },
		"owner":        func(p *ShadowProxy) { p.Triangles[0].Sector++ },
		"semantic":     func(p *ShadowProxy) { p.Triangles[0].Surface = "sector-ceiling" },
		"edge":         func(p *ShadowProxy) { p.Triangles[0].SurfaceEdge++ },
		"digest":       func(p *ShadowProxy) { p.GeometrySHA256 = strings.Repeat("0", 64) },
	} {
		t.Run(name, func(t *testing.T) {
			corrupt := proxy
			corrupt.Triangles = slices.Clone(proxy.Triangles)
			change(&corrupt)
			if err := ValidateShadowProxy(&corrupt, &document); err == nil {
				t.Fatal("accepted arbitrary or incomplete shadow geometry")
			}
		})
	}
	document.Sectors[0].FloorMaterial++
	if err := ValidateShadowProxy(&proxy, &document); err == nil {
		t.Fatal("stale source material accepted")
	}
}

func TestShadowProxyRestGeometryOpacityAndSky(t *testing.T) {
	document := fixture(t)
	for index := range document.Sectors {
		document.Sectors[index].Floor = world.Plane{}
	}
	base, err := CompileShadowProxy(document)
	if err != nil {
		t.Fatal(err)
	}
	opacity := .5
	document.Animations = &world.Animations{
		Version: world.AnimationVersion,
		Presets: []world.AnimationPreset{
			{Name: "water", Kind: "liquid", SurfaceAmplitude: .1, Frequency: 2, Speed: 1, Opacity: &opacity},
		},
		Materials: []world.MaterialAnimation{{Material: 1, Preset: "water"}},
	}
	transparent, err := CompileShadowProxy(document)
	if err != nil {
		t.Fatal(err)
	}
	if len(transparent.Triangles) != len(base.Triangles)-4 {
		t.Fatal("transparent floor must not cast an opaque shadow")
	}
	for _, triangle := range transparent.Triangles {
		if triangle.Material == 1 {
			t.Fatal("transparent liquid retained")
		}
	}
	opacity = 1
	opaque, err := CompileShadowProxy(document)
	if err != nil || !slices.Equal(opaque.Triangles, base.Triangles) {
		t.Fatalf("opaque displaced liquid changed the rest caster mesh: %v", err)
	}
	if err := ValidateShadowProxy(&transparent, &document); err == nil {
		t.Fatal("opacity crossing the opaque boundary must invalidate coverage")
	}
	document.Environment = &world.Environment{Version: 1, Preset: world.EnvironmentDaylight}
	document.Sectors[0].Sky = true
	document.Sectors[0].Walls[0].Sky = true
	sky, err := CompileShadowProxy(document)
	if err != nil {
		t.Fatal(err)
	}
	for _, triangle := range sky.Triangles {
		if triangle.Sector == 0 &&
			(triangle.Surface == "sector-ceiling" || triangle.Surface == "sector-wall" && triangle.SurfaceEdge == 0) {
			t.Fatal("sky retained as an opaque shadow caster")
		}
	}
}

func TestShadowProxyStaticSolidsAndWallOpening(t *testing.T) {
	document := fixture(t)
	for index := range document.Sectors {
		document.Sectors[index].Floor = world.Plane{}
		document.Sectors[index].Ceiling = world.Plane{C: 4}
	}
	document.StaticSolids = &world.StaticSolids{Version: 1, Items: []world.Solid{{
		ID: "column", Footprint: []world.Vec2{{X: 1, Y: 1}, {X: 2, Y: 1}, {X: 2, Y: 2}, {X: 1, Y: 2}},
		Bottom: world.Plane{}, Top: world.Plane{C: 2}, TopMaterial: 4, BottomMaterial: 4, SideMaterial: 4,
	}}}
	proxy, err := CompileShadowProxy(document)
	if err != nil {
		t.Fatal(err)
	}
	solids := 0
	for _, triangle := range proxy.Triangles {
		if triangle.Solid == 0 {
			solids++
		}
		if triangle.Sector == 0 && triangle.Surface == "sector-wall" && triangle.SurfaceEdge == 1 ||
			triangle.Sector == 1 && triangle.Surface == "sector-wall" && triangle.SurfaceEdge == 3 {
			t.Fatal("open doorway gained shadow geometry")
		}
	}
	if solids != 12 {
		t.Fatalf("static solid coverage: %d triangles", solids)
	}
	profile, err := world.ProfileForWall(&document, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	regions, err := world.CompileWallFrame(document.Sectors[0].Walls[0], profile, world.WallFrameConfig{
		Top: world.FrameHorizontal{Height: 1, Repeat: 1, Piece: world.FramePiece{Material: 8, Coverage: world.FrameCoverageOpaque}},
	})
	if err != nil {
		t.Fatal(err)
	}
	document.Sectors[0].Walls[0].FrameRegions = regions
	document.Sectors[0].Walls[0].FrameCompiled = true
	document.MaterialLayers = &world.MaterialLayers{Version: 1}
	document.MaterialMapping = &world.MaterialMapping{Version: 1}
	for index := range document.Sectors {
		sector := &document.Sectors[index]
		sector.FloorUV = legacyMapping(&document, Triangle{Sector: index, Surface: "sector-floor"})
		sector.CeilingUV = sector.FloorUV
		for edge := range sector.Walls {
			sector.Walls[edge].UV = legacyMapping(&document, Triangle{Sector: index, Surface: "sector-wall", SurfaceEdge: edge})
		}
	}
	framed, err := CompileShadowProxy(document)
	if err != nil || !slices.Equal(framed.Triangles, proxy.Triangles) {
		t.Fatalf("wall material bands changed legacy caster geometry: %v", err)
	}
}

func TestShadowProxyDecoderBoundsAndCanonicalBinary(t *testing.T) {
	w, err := newWire(shadowHeaderBytes, "KCS1", shadowHeaderBytes)
	if err != nil {
		t.Fatal(err)
	}
	encoded := w.raw
	binary.LittleEndian.PutUint32(encoded[8:12], MaxShadowTriangles+1)
	if _, err := DecodeShadowProxy(encoded, nil); err == nil || !strings.Contains(err.Error(), "array record bound") {
		t.Fatal("oversized tiny records reached owned decoder")
	}
	document := fixture(t)
	proxy, err := CompileShadowProxy(document)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := EncodeShadowProxy(proxy, &document)
	if err != nil {
		t.Fatal(err)
	}
	for _, data := range [][]byte{
		append([]byte(" "), canonical...),
		changedWord(canonical, 12, 1),
		changedWord(canonical, shadowHeaderBytes+60, 7),
		canonical[:len(canonical)-1],
		append(slices.Clone(canonical), []byte(`{}`)...),
	} {
		if _, err := DecodeShadowProxy(data, &document); err == nil {
			t.Fatal("noncanonical shadow accepted")
		}
	}
	proxy.Triangles[0].Positions[0][0] = float32(math.NaN())
	if _, err := EncodeShadowProxy(proxy, &document); err == nil {
		t.Fatal("nonfinite shadow accepted")
	}
	corrupt, err := encodeShadowBinary(proxy)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeShadowProxy(corrupt, &document); err == nil {
		t.Fatal("nonfinite binary shadow accepted")
	}
}

func TestShadowProxyCompilerEnforcesGlobalCasterBudget(t *testing.T) {
	document := world.Document{Version: world.Version, Sectors: make([]world.Sector, 128)}
	for index := range document.Sectors {
		sector := world.Sector{
			ID:         fmt.Sprintf("room-%d", index),
			SourceRoom: "room",
			Ceiling:    world.Plane{C: 4},
			Walls:      make([]world.Wall, 32),
		}
		points := make([]world.Vec2, len(sector.Walls))
		for edge := range points {
			angle := float64(edge) * 2 * math.Pi / float64(len(points))
			points[edge] = world.Vec2{X: float64(index)*4 + math.Cos(angle), Y: math.Sin(angle)}
		}
		for edge, point := range points {
			sector.Walls[edge] = world.Wall{Start: point, End: points[(edge+1)%len(points)], Portal: -1}
		}
		document.Sectors[index] = sector
	}
	if _, err := CompileShadowProxy(document); !errors.Is(err, ErrShadowBudget) || !errors.Is(err, ErrChunk) {
		t.Fatalf("oversized global caster proxy did not fail explicitly: %v", err)
	}
}
