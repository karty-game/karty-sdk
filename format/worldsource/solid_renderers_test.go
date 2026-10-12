package worldsource_test

import (
	"testing"

	"github.com/karty-game/karty-sdk/format/worldsource"
)

func TestRenderablePrefabVersionAndComponentRestrictions(t *testing.T) {
	t.Parallel()
	for _, version := range []uint16{10, 11} {
		document := validSource()
		document.Version = version
		document.Prefabs = append(
			document.Prefabs,
			worldsource.Prefab{ID: "can", Renderable: true, Solids: []worldsource.Solid{sourceSolid()}},
		)
		err := worldsource.Validate(&document)
		if (err == nil) != (version == 11) {
			t.Fatalf("renderable prefab version %d: %v", version, err)
		}
	}
	document := validSource()
	document.Version = 11
	document.Prefabs = append(document.Prefabs, worldsource.Prefab{ID: "can", Renderable: true, Solids: []worldsource.Solid{sourceSolid()}})
	document.Prefabs[len(document.Prefabs)-1].Contents = []worldsource.Content{{ID: "nested", Kind: "prop"}}
	if err := worldsource.Validate(&document); err == nil {
		t.Fatal("renderer prefab accepted nested actor content")
	}
}
