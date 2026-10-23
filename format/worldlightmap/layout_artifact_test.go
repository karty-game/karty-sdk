package worldlightmap

import (
	"math"
	"reflect"
	"slices"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
)

func TestLayoutArtifactRoundTripAndBuildProofBoundary(t *testing.T) {
	document := directFixture(t)
	layout, err := CompileReceivers(document, Options{PageSize: 512, Lights: []string{"red", "blue"}}, []int{0}, nil)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := Encode(layout, &document)
	if err != nil {
		t.Fatal(err)
	}
	admission := LayoutAdmission{SectorCount: len(document.Sectors)}
	decoded, err := DecodeLayoutArtifact(encoded, admission)
	if err != nil || !reflect.DeepEqual(decoded, layout) {
		t.Fatal("compiled layout changed", err)
	}
	for _, bad := range [][]byte{nil, encoded[:len(encoded)-1], append(slices.Clone(encoded), 0)} {
		if _, err := DecodeLayoutArtifact(bad, admission); err == nil {
			t.Fatal("invalid layout wire accepted")
		}
	}
	// Source geometry, chart basis/density and atlas-overlap proof stay offline.
	decoded.Charts[0].Normal.X = .123
	decoded.Charts[1].Rect = decoded.Charts[0].Rect
	decoded.Charts[1].ReceiverRect = decoded.Charts[0].ReceiverRect
	if err := ValidateLayoutArtifact(&decoded, admission); err != nil {
		t.Fatal("runtime repeated geometric chart proof", err)
	}
	if err := Validate(&decoded, &document); err == nil {
		t.Fatal("offline chart proof weakened")
	}
	decoded.Bindings[0].Chart = -1
	if layout.Bindings[0].Chart == -1 {
		t.Fatal("returned binding data aliased source")
	}
}

func TestLayoutArtifactRejectsReferenceRectAndFieldBounds(t *testing.T) {
	document := directFixture(t)
	original, err := CompileReceivers(document, Options{PageSize: 512, Lights: []string{"red"}}, []int{0}, nil)
	if err != nil {
		t.Fatal(err)
	}
	admission := LayoutAdmission{SectorCount: len(document.Sectors)}
	for name, change := range map[string]func(*Layout){
		"schema":               func(l *Layout) { l.Schema = "other" },
		"geometry hash format": func(l *Layout) { l.GeometrySHA256 = "bad" },
		"algorithm":            func(l *Layout) { l.Algorithm = 99 },
		"receiver algorithm":   func(l *Layout) { l.Receivers = nil },
		"receiver sectors":     func(l *Layout) { l.Receivers = &ReceiverSelection{Sectors: []int{2}} },
		"receiver solid":       func(l *Layout) { l.Receivers = &ReceiverSelection{Sectors: []int{0}, Solids: []int{0}} },
		"page":                 func(l *Layout) { l.Pages[0].Width = 2048 },
		"chart id":             func(l *Layout) { l.Charts[0].ID = 1 },
		"chart page":           func(l *Layout) { l.Charts[0].Page = 1 },
		"chart rect":           func(l *Layout) { l.Charts[0].Rect[0] = -1 },
		"receiver rect":        func(l *Layout) { l.Charts[0].ReceiverRect[2] = l.Pages[0].Width + 1 },
		"receiver containment": func(l *Layout) { l.Charts[0].ReceiverRect[0] = l.Charts[0].Rect[0] - 1 },
		"normal nonfinite":     func(l *Layout) { l.Charts[0].Normal.X = math.NaN() },
		"uv nonfinite":         func(l *Layout) { l.Charts[0].UPlane[0] = math.Inf(1) },
		"uv numeric bound":     func(l *Layout) { l.Charts[0].UPlane[0] = 1e16 },
		"binding index":        func(l *Layout) { l.Bindings[0].Index = 2 },
		"binding chart":        func(l *Layout) { l.Bindings[0].Chart = len(l.Charts) },
		"binding kind":         func(l *Layout) { l.Bindings[0].Kind = "unknown" },
		"binding cap edge":     func(l *Layout) { l.Bindings[0].Edge = 0 },
		"binding wall edge":    func(l *Layout) { l.Bindings[0].Kind = "sector-wall"; l.Bindings[0].Edge = world.MaxWallsPerSector },
		"padding":              func(l *Layout) { l.Padding = 0 },
		"density":              func(l *Layout) { l.TexelsPerUnit = math.NaN() },
		"recipe nil ids":       func(l *Layout) { l.RuntimeBake.LightIDs = nil },
		"recipe duplicate ids": func(l *Layout) { l.RuntimeBake.LightIDs = []string{"red", "red"} },
		"recipe encoding":      func(l *Layout) { l.RuntimeBake.Encoding = "unknown" },
		"recipe shadow size":   func(l *Layout) { l.RuntimeBake.ShadowSize = 513 },
	} {
		t.Run(name, func(t *testing.T) {
			layout := original
			layout.Pages = slices.Clone(original.Pages)
			layout.Charts = slices.Clone(original.Charts)
			layout.Bindings = slices.Clone(original.Bindings)
			recipe := *original.RuntimeBake
			layout.RuntimeBake = &recipe
			change(&layout)
			if err := ValidateLayoutArtifact(&layout, admission); err == nil {
				t.Fatal("invalid layout references accepted")
			}
		})
	}
	if ValidateLayoutArtifact(nil, admission) == nil {
		t.Fatal("nil layout accepted")
	}
	for _, bad := range []LayoutAdmission{{}, {SectorCount: world.MaxSectors + 1}, {SectorCount: 1, SolidCount: -1}, {SectorCount: 1, SolidCount: world.MaxStaticSolids + 1}} {
		if ValidateLayoutArtifact(&original, bad) == nil {
			t.Fatal("invalid root bounds accepted")
		}
	}
}
