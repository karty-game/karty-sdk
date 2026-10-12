package worldsource

import (
	"fmt"

	"github.com/karty-game/karty-sdk/format/world"
)

// Renderer prefabs are rigid groups of existing solid extrusions. Ordinary
// static instantiation remains unchanged; only actor attachments preserve them.
func validateSolidRenderers(document *Document, prefabs map[string]*Prefab) error {
	var checkPrefab func(string, map[string]bool) (int, error)
	checkPrefab = func(id string, visiting map[string]bool) (int, error) {
		prefab := prefabs[id]
		if prefab == nil || visiting[id] {
			return 0, ErrReference
		}
		if len(prefab.Rooms)+len(prefab.Connections)+len(prefab.Ports)+len(prefab.Contents)+len(prefab.ParticleEmitters) != 0 {
			return 0, ErrReference
		}
		visiting[id] = true
		defer delete(visiting, id)
		count := len(prefab.Solids)
		for _, solid := range prefab.Solids {
			if solid.Collision {
				return 0, ErrGeometry
			}
		}
		for _, instance := range prefab.Instances {
			n, err := checkPrefab(instance.Prefab, visiting)
			if err != nil {
				return 0, err
			}
			count += n
			if count > world.MaxStaticSolids {
				return 0, ErrBounds
			}
		}
		if count == 0 {
			return 0, ErrBounds
		}
		return count, nil
	}
	for _, prefab := range document.Prefabs {
		if prefab.Renderable {
			if document.Version < SolidRenderersVersion {
				return ErrVersion
			}
			if _, err := checkPrefab(prefab.ID, make(map[string]bool)); err != nil {
				return err
			}
		}
	}
	checkContents := func(contents []Content) error {
		for _, content := range contents {
			if content.Actor == nil || content.Actor.SolidRenderer == "" {
				continue
			}
			if document.Version < SolidRenderersVersion {
				return ErrVersion
			}
			if !validIdentifier(content.Actor.SolidRenderer) || content.Actor.Sprite != nil || content.Actor.Animation != nil {
				return ErrContent
			}
			if _, err := checkPrefab(content.Actor.SolidRenderer, make(map[string]bool)); err != nil {
				return fmt.Errorf("actor %q solid renderer: %w", content.ID, err)
			}
		}
		return nil
	}
	if err := checkContents(document.Contents); err != nil {
		return err
	}
	for _, room := range document.Rooms {
		if err := checkContents(room.Contents); err != nil {
			return err
		}
	}
	for _, prefab := range document.Prefabs {
		if err := checkContents(prefab.Contents); err != nil {
			return err
		}
		for _, room := range prefab.Rooms {
			if err := checkContents(room.Contents); err != nil {
				return err
			}
		}
	}
	return nil
}
