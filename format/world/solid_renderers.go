package world

import "fmt"

const (
	SolidRenderersVersion     = uint32(1)
	FeatureSolidRenderers     = "world/solid-renderers@1"
	MaxSolidPrefabs           = 64
	MaxSolidRendererTriangles = 65536
)

// SolidRenderers retains reusable local solid geometry separately from static
// world geometry. Actors bind existing content identities to prefab identities.
type SolidRenderers struct {
	Version uint32
	Prefabs []SolidPrefab
	Actors  []SolidAttachment
}

type SolidPrefab struct {
	ID     string
	Solids []Solid
}

type SolidAttachment struct {
	Actor  string
	Prefab string
}

func ValidateSolidRenderers(document *Document) error {
	if document.SolidRenderers == nil {
		return nil
	}
	payload := document.SolidRenderers
	if document.Version != Version || payload.Version != SolidRenderersVersion {
		return ErrVersion
	}
	if len(payload.Prefabs) == 0 || len(payload.Prefabs) > MaxSolidPrefabs || len(payload.Actors) > MaxContents {
		return ErrBounds
	}
	prefabs := make(map[string]bool, len(payload.Prefabs))
	total := 0
	for _, prefab := range payload.Prefabs {
		if !validIdentifier(prefab.ID) || prefabs[prefab.ID] {
			return ErrIdentity
		}
		prefabs[prefab.ID] = true
		total += len(prefab.Solids)
		if len(prefab.Solids) == 0 || total > MaxStaticSolids {
			return ErrBounds
		}
		if err := ValidateStaticSolids(&StaticSolids{Version: StaticSolidsVersion, Items: prefab.Solids}); err != nil {
			return err
		}
		for _, solid := range prefab.Solids {
			if solid.Collision {
				return fmt.Errorf("renderer prefab %q collision: %w", prefab.ID, ErrGeometry)
			}
			if document.MaterialMapping == nil && (solid.SideUV != nil || solid.TopUV != nil || solid.BottomUV != nil) {
				return ErrMaterialMapping
			}
		}
	}
	if document.Animations != nil {
		used := make(map[uint32]bool)
		for _, prefab := range payload.Prefabs {
			for _, solid := range prefab.Solids {
				used[solid.SideMaterial], used[solid.TopMaterial], used[solid.BottomMaterial] = true, true, true
			}
		}
		for _, binding := range document.Animations.Materials {
			if !used[binding.Material] {
				continue
			}
			for _, preset := range document.Animations.Presets {
				if preset.Name == binding.Preset && (preset.SurfaceAmplitude != 0 || preset.Opacity != nil && *preset.Opacity < 1) {
					return ErrContent
				}
			}
		}
	}
	if SolidRendererTriangles(payload) > MaxSolidRendererTriangles {
		return ErrBounds
	}
	actors := make(map[string]*Actor, len(document.Contents))
	for _, content := range document.Contents {
		actors[content.ID] = content.Actor
	}
	seen := make(map[string]bool, len(payload.Actors))
	for _, attachment := range payload.Actors {
		actor := actors[attachment.Actor]
		if !validIdentifier(attachment.Actor) || !prefabs[attachment.Prefab] || seen[attachment.Actor] || actor == nil ||
			actor.Sprite != nil ||
			actor.Animation != nil {
			return ErrContent
		}
		seen[attachment.Actor] = true
	}
	return nil
}

// SolidRendererTriangles counts immutable triangles in validated extrusions:
// two triangles per side and n-2 triangles on each cap.
func SolidRendererTriangles(renderers *SolidRenderers) int {
	if renderers == nil {
		return 0
	}
	total := 0
	for _, prefab := range renderers.Prefabs {
		for _, solid := range prefab.Solids {
			total += 4*len(solid.Footprint) - 4
		}
	}
	return total
}
