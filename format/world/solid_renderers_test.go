package world_test

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/karty-game/karty-sdk/format/world"
)

func solidRendererWorld() world.Document {
	d := validWorld()
	d.Version = world.Version
	d.Sectors[0].Walls[1].PortalWall = 4
	d.Sectors[1].Walls[3].PortalWall = 2
	d.Contents[0].Actor = &world.Actor{Scale: world.Vec3{X: 1, Y: 1, Z: 1}}
	d.SolidRenderers = &world.SolidRenderers{
		Version: 1,
		Prefabs: []world.SolidPrefab{{ID: "can", Solids: []world.Solid{validSolid()}}},
		Actors:  []world.SolidAttachment{{Actor: d.Contents[0].ID, Prefab: "can"}},
	}
	return d
}

func TestSolidRenderersVersionedBinaryAndCompleteValidation(t *testing.T) {
	t.Parallel()
	d := solidRendererWorld()
	encoded, err := world.Encode(d)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(encoded, []byte(world.SolidBinaryMagic)) {
		t.Fatal("solid extension used legacy envelope")
	}
	decoded, err := world.Decode(encoded)
	if err != nil || !reflect.DeepEqual(decoded, d) {
		t.Fatalf("round trip: %v", err)
	}
	for _, input := range [][]byte{encoded[:len(encoded)-1], append(bytes.Clone(encoded), 0), append([]byte(world.BinaryMagic), encoded[4:]...)} {
		if _, err := world.Decode(input); err == nil {
			t.Fatal("malformed extension accepted")
		}
	}
	for name, change := range map[string]func(*world.Document){
		"future version": func(d *world.Document) { d.SolidRenderers.Version++ },
		"bad last solid": func(d *world.Document) {
			d.SolidRenderers.Prefabs[0].Solids = append(d.SolidRenderers.Prefabs[0].Solids, world.Solid{})
		},
		"bad last attachment": func(d *world.Document) {
			d.SolidRenderers.Actors = append(d.SolidRenderers.Actors, world.SolidAttachment{Actor: "unknown", Prefab: "can"})
		},
		"duplicate attachment": func(d *world.Document) {
			d.SolidRenderers.Actors = append(d.SolidRenderers.Actors, d.SolidRenderers.Actors[0])
		},
		"unknown prefab":   func(d *world.Document) { d.SolidRenderers.Actors[0].Prefab = "unknown" },
		"static collision": func(d *world.Document) { d.SolidRenderers.Prefabs[0].Solids[0].Collision = true },
	} {
		t.Run(name, func(t *testing.T) {
			d := solidRendererWorld()
			change(&d)
			if _, err := world.Encode(d); err == nil {
				t.Fatal("invalid complete document encoded")
			}
		})
	}
	legacy := validWorld()
	old, err := world.Encode(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(old, []byte(world.BinaryMagic)) {
		t.Fatal("absent extension changed legacy encoding")
	}
}
