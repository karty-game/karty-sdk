package world_test

import (
	"errors"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
)

func TestActorPayloadRoundTripAndValidation(t *testing.T) {
	document := validWorld()
	document.Version = world.Version
	document.Sectors[0].Walls[1].PortalWall = 4
	document.Sectors[1].Walls[3].PortalWall = 2
	document.Contents[0].Actor = &world.Actor{
		Scale: world.Vec3{X: 1, Y: 1, Z: 1}, Tags: []string{"interactive", "tree"},
		Sprite: &world.Sprite{
			AssetID: 3, Facing: world.SpriteCross, Alpha: world.SpriteCutout,
			Width: 2, Height: 3, OriginX: .5, OriginY: 1,
		},
	}
	encoded, err := world.Encode(document)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := world.Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Contents[0].Actor == nil || decoded.Contents[0].Actor.Sprite.Facing != world.SpriteCross ||
		len(decoded.Contents[0].Actor.Tags) != 2 {
		t.Fatalf("actor round trip = %+v", decoded.Contents[0].Actor)
	}

	document.Contents[0].Actor.Tags = []string{"tree", "interactive"}
	if err := world.Validate(&document); !errors.Is(err, world.ErrContent) {
		t.Fatalf("unsorted tags error = %v", err)
	}
	document.Contents[0].Actor.Tags = []string{"interactive", "tree"}
	document.Version = world.LegacyVersion
	document.Sectors[0].Walls[1].PortalWall = 0
	document.Sectors[1].Walls[3].PortalWall = 0
	if err := world.Validate(&document); !errors.Is(err, world.ErrVersion) {
		t.Fatalf("v1 actor error = %v", err)
	}
}

func TestDirectedPortalTargetsAndIndependentOutgoingLinks(t *testing.T) {
	document := validWorld()
	document.Version = world.Version
	// room-a door enters room-b's west wall. The destination wall is solid,
	// making this explicitly one-way.
	document.Sectors[0].Walls[1].PortalWall = 4
	document.Sectors[1].Walls[3].Portal = -1
	if err := world.Validate(&document); err != nil {
		t.Fatalf("one-way portal: %v", err)
	}

	// The incoming destination may own a separate outgoing mapping.
	document.Sectors[1].Walls[1].Portal = 0
	document.Sectors[1].Walls[1].PortalWall = 4
	if err := world.Validate(&document); err != nil {
		t.Fatalf("independent outgoing portal: %v", err)
	}

	document.Sectors[1].Walls[1].PortalWall = 5
	if err := world.Validate(&document); !errors.Is(err, world.ErrPortal) {
		t.Fatalf("mismatched destination wall error = %v", err)
	}
}
