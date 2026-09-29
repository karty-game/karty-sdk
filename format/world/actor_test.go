package world_test

import (
	"errors"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
)

func TestActorPayloadRoundTripAndValidation(t *testing.T) {
	document := validWorld()
	document.Version = world.Version
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
	if err := world.Validate(&document); !errors.Is(err, world.ErrVersion) {
		t.Fatalf("v1 actor error = %v", err)
	}
}
