package worldlightmap

import (
	"bytes"
	"image"
	"image/color"
	"reflect"
	"testing"

	"github.com/karty-game/karty-sdk/codec/qoi"
	"github.com/karty-game/karty-sdk/format/world"
)

func prebakeFixture(t *testing.T) (world.Document, Layout, PrebakePair) {
	t.Helper()
	document := directFixture(t)
	layout, err := Compile(document, Options{PageSize: 512, Lights: []string{"blue", "red"}, ShadowSize: 128})
	if err != nil {
		t.Fatal(err)
	}
	pixels := image.NewNRGBA(image.Rect(0, 0, 1536, 512))
	for index := 0; index < len(pixels.Pix); index += 4 {
		pixels.Pix[index], pixels.Pix[index+1], pixels.Pix[index+2], pixels.Pix[index+3] = 240, 100, 8, 1
	}
	encoded, _, err := qoi.Encode(pixels, qoi.Options{Channels: qoi.ChannelsRGBA, Colorspace: qoi.ColorspaceLinear})
	if err != nil {
		t.Fatal(err)
	}
	pair, err := NewPrebake(layout, &document, encoded)
	if err != nil {
		t.Fatal(err)
	}
	return document, layout, pair
}

func TestPrebakeRoundTripAndIdentity(t *testing.T) {
	document, layout, pair := prebakeFixture(t)
	encoded, err := EncodePrebake(pair, layout, &document)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodePrebake(encoded, pair.Image, layout, &document)
	if err != nil || !reflect.DeepEqual(decoded, pair) || pair.Manifest.RGBMRange != 6 {
		t.Fatalf("round trip or range: %v %+v", err, pair.Manifest)
	}
	_, pixels, err := qoi.Decode(decoded.Image)
	if err != nil || pixels.NRGBAAt(0, 0) != (color.NRGBA{R: 240, G: 100, B: 8, A: 1}) {
		t.Fatalf("raw RGBM channels were premultiplied: %v", err)
	}
	// Unselected and non-baked lighting state does not invalidate the image.
	document.Lighting.Ambient.X = .2
	document.Lighting.Lights = append(document.Lighting.Lights, world.PointLight{ID: "other", Position: world.Vec3{X: 100}, Color: world.Vec3{Y: 1}, Radius: 10})
	if err := pair.Validate(layout, &document); err != nil {
		t.Fatalf("nonbaked state invalidated atlas: %v", err)
	}
	for name, change := range map[string]func(*world.PointLight){
		"colour":   func(l *world.PointLight) { l.Color.X = .5 },
		"position": func(l *world.PointLight) { l.Position.X += .1 },
		"radius":   func(l *world.PointLight) { l.Radius += 1 },
	} {
		t.Run(name, func(t *testing.T) {
			copy := document
			copy.Lighting = &world.Lighting{Version: 1, Lights: append([]world.PointLight(nil), document.Lighting.Lights...)}
			change(&copy.Lighting.Lights[0])
			if err := pair.Validate(layout, &copy); err == nil {
				t.Fatal("stale selected light accepted")
			}
		})
	}
	changed, err := Compile(document, Options{PageSize: 512, Lights: []string{"red", "blue"}, ShadowSize: 128})
	if err != nil {
		t.Fatal(err)
	}
	if err := pair.Validate(changed, &document); err == nil {
		t.Fatal("changed ordered layout accepted")
	}
}

func TestPrebakeRejectsMalformedManifestAndImage(t *testing.T) {
	document, layout, pair := prebakeFixture(t)
	encoded, err := EncodePrebake(pair, layout, &document)
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*PrebakeManifest){
		"schema":      func(m *PrebakeManifest) { m.Schema = "other" },
		"algorithm":   func(m *PrebakeManifest) { m.Algorithm = 2 },
		"encoding":    func(m *PrebakeManifest) { m.Encoding = PointVisibilityEncoding },
		"layout":      func(m *PrebakeManifest) { m.LayoutSHA256 = "bad" },
		"lighting":    func(m *PrebakeManifest) { m.BakeSHA256 = "bad" },
		"image":       func(m *PrebakeManifest) { m.ImageSHA256 = "bad" },
		"image bytes": func(m *PrebakeManifest) { m.ImageBytes++ },
		"width":       func(m *PrebakeManifest) { m.Width++ },
		"height":      func(m *PrebakeManifest) { m.Height++ },
		"range":       func(m *PrebakeManifest) { m.RGBMRange++ },
	} {
		t.Run(name, func(t *testing.T) {
			copy := pair
			mutate(&copy.Manifest)
			if err := copy.Validate(layout, &document); err == nil {
				t.Fatal("malformed manifest accepted")
			}
		})
	}
	for name, mutation := range map[string][2]string{
		"null":      {`"rgbm_range":6`, `"rgbm_range":null`},
		"missing":   {`,"rgbm_range":6`, ``},
		"duplicate": {`"rgbm_range":6`, `"rgbm_range":6,"rgbm_range":6`},
		"unknown":   {`"rgbm_range":6`, `"rgbm_range":6,"extra":1`},
	} {
		t.Run(name, func(t *testing.T) {
			raw := bytes.Replace(encoded, []byte(mutation[0]), []byte(mutation[1]), 1)
			if _, err := DecodePrebake(raw, pair.Image, layout, &document); err == nil {
				t.Fatal("noncanonical manifest accepted")
			}
		})
	}
	if _, err := DecodePrebake(append(encoded, ' '), pair.Image, layout, &document); err == nil {
		t.Fatal("noncanonical whitespace accepted")
	}
	imageBytes := bytes.Clone(pair.Image)
	imageBytes[12] = qoi.ChannelsRGB
	if _, err := NewPrebake(layout, &document, imageBytes); err == nil {
		t.Fatal("RGB image accepted")
	}
	imageBytes = bytes.Clone(pair.Image)
	imageBytes[13] = qoi.ColorspaceSRGB
	if _, err := NewPrebake(layout, &document, imageBytes); err == nil {
		t.Fatal("sRGB storage accepted")
	}
	imageBytes = append([]byte(nil), pair.Image...)
	imageBytes = append(imageBytes[:len(imageBytes)-8], 0, 0, 0, 0, 0, 0, 0, 0, 1)
	if _, err := NewPrebake(layout, &document, imageBytes); err == nil {
		t.Fatal("overlong QOI stream accepted")
	}
	if _, err := NewPrebake(layout, &document, make([]byte, MaxPrebakeImageSize+1)); err == nil {
		t.Fatal("oversized image accepted")
	}
	layout.RuntimeBake.Encoding = PointVisibilityEncoding
	layout.RuntimeBake.LightID = "red"
	layout.RuntimeBake.LightIDs = nil
	if _, err := NewPrebake(layout, &document, pair.Image); err == nil {
		t.Fatal("visibility recipe reinterpreted as coefficients")
	}
}

func TestPrebakeRejectsMissingDocumentWithoutPanic(t *testing.T) {
	document, layout, pair := prebakeFixture(t)
	encoded, err := EncodePrebake(pair, layout, &document)
	if err != nil {
		t.Fatal(err)
	}
	noLighting := document
	noLighting.Lighting = nil
	for name, candidate := range map[string]*world.Document{
		"nil": nil, "missing lighting": &noLighting, "empty document": {},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewPrebake(layout, candidate, pair.Image); err == nil {
				t.Fatal("missing document accepted by constructor")
			}
			if err := pair.Validate(layout, candidate); err == nil {
				t.Fatal("missing document accepted by validation")
			}
			if _, err := EncodePrebake(pair, layout, candidate); err == nil {
				t.Fatal("missing document accepted by encoder")
			}
			if _, err := DecodePrebake(encoded, pair.Image, layout, candidate); err == nil {
				t.Fatal("missing document accepted by decoder")
			}
		})
	}
}
