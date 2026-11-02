package reflectionplan_test

import (
	"fmt"
	"image"
	"testing"

	"github.com/karty-game/karty-sdk/bake/reflectionplan"
	"github.com/karty-game/karty-sdk/codec/qoi"
	"github.com/karty-game/karty-sdk/format/level"
	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldreflection"
)

func TestPlanGroupsAuthoredRoomAndEncodesPublicBundle(t *testing.T) {
	t.Parallel()

	document := roomFixture()
	envelope := level.Envelope{}

	bundle, err := reflectionplan.Plan(document, worldreflection.InputDigest(envelope))
	if err != nil {
		t.Fatal(err)
	}

	if len(bundle.Probes) != 1 || bundle.Sectors[0] != 0 || bundle.Sectors[1] != 0 ||
		bundle.Probes[0].Low != ([3]float32{-4, -2, 0}) || bundle.Probes[0].High != ([3]float32{4, 2, 4}) ||
		bundle.Probes[0].Origin != ([3]float32{0, 0, 2}) {
		t.Fatal("room decomposition changed capture placement", bundle)
	}

	strip := image.NewNRGBA(image.Rect(0, 0, worldreflection.FaceSize*worldreflection.Faces, worldreflection.FaceSize))
	for offset := 3; offset < len(strip.Pix); offset += 4 {
		strip.Pix[offset] = 255
	}

	bundle.Probes[0].Image, _, err = qoi.Encode(strip, qoi.Options{Channels: qoi.ChannelsRGBA, Colorspace: qoi.ColorspaceSRGB})
	if err != nil {
		t.Fatal(err)
	}

	if err := bundle.ValidateSource(envelope, document); err != nil {
		t.Fatal(err)
	}

	encoded, err := worldreflection.Encode(bundle)
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := worldreflection.Decode(encoded)
	if err != nil || decoded.Probes[0].Origin != bundle.Probes[0].Origin || len(decoded.Sectors) != 2 {
		t.Fatalf("public KRC1 round trip failed: %v", err)
	}
}

func TestPlanExcludesSkyAndRejectsInvalidWorld(t *testing.T) {
	t.Parallel()

	document := roomFixture()
	document.Sectors[1].Sky = true
	document.Environment = &world.Environment{Version: 1, Preset: world.EnvironmentDaylight}

	bundle, err := reflectionplan.Plan(document, worldreflection.InputDigest(level.Envelope{}))
	if err != nil || len(bundle.Probes) != 1 || bundle.Sectors[1] != -1 {
		t.Fatalf("sky sector received an indoor probe: %+v, %v", bundle, err)
	}

	document.Sectors[0].Walls = nil
	if _, err := reflectionplan.Plan(document, "invalid"); err == nil {
		t.Fatal("invalid geometry admitted to capture placement")
	}
}

func roomFixture() *world.Document {
	document := &world.Document{Version: world.Version}

	for index := range 2 {
		left, right := float64(index*4-4), float64(index*4)
		points := []world.Vec2{{X: left, Y: -2}, {X: right, Y: -2}, {X: right, Y: 2}, {X: left, Y: 2}}

		sector := world.Sector{
			ID:              fmt.Sprintf("cell%d", index),
			SourceRoom:      "hall",
			Ceiling:         world.Plane{C: 4},
			FloorMaterial:   1,
			CeilingMaterial: 1,
		}
		for vertex, point := range points {
			sector.Walls = append(sector.Walls, world.Wall{Start: point, End: points[(vertex+1)%len(points)], Portal: -1, Material: 1})
		}

		document.Sectors = append(document.Sectors, sector)
	}

	document.Sectors[0].Walls[1].Portal = 1
	document.Sectors[0].Walls[1].PortalWall = 4
	document.Sectors[1].Walls[3].Portal = 0
	document.Sectors[1].Walls[3].PortalWall = 2

	return document
}
