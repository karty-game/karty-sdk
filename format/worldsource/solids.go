package worldsource

import (
	"fmt"

	"github.com/karty-game/karty-sdk/format/world"
)

// Solid is an extrusion independent of room decomposition. Root records use
// global coordinates; prefab records receive nested instance transforms.
// Optional UV controls inherit root horizontal defaults for
// caps; sides accept only an explicit planar/world mapping in version 6.
type Solid struct {
	ID             string      `json:"id"                  yaml:"id"`
	Footprint      []Vec2      `json:"footprint"           yaml:"footprint"`
	Bottom         Plane       `json:"bottom"              yaml:"bottom"`
	Top            Plane       `json:"top"                 yaml:"top"`
	SideMaterial   string      `json:"side_material"       yaml:"side_material"`
	TopMaterial    string      `json:"top_material"        yaml:"top_material"`
	BottomMaterial string      `json:"bottom_material"     yaml:"bottom_material"`
	SideUV         *UVSettings `json:"side_uv,omitempty"   yaml:"side_uv,omitempty"`
	TopUV          *UVSettings `json:"top_uv,omitempty"    yaml:"top_uv,omitempty"`
	BottomUV       *UVSettings `json:"bottom_uv,omitempty" yaml:"bottom_uv,omitempty"`
	Collision      bool        `json:"collision,omitempty" yaml:"collision,omitempty"`
}

func validateSolids(document *Document) error {
	solidCount, contentCount := 0, 0
	if err := validateExtras(document.Solids, document.Contents, document.Version); err != nil {
		return err
	}
	solidCount += len(document.Solids)
	contentCount += len(document.Contents)
	for _, prefab := range document.Prefabs {
		if err := validateExtras(prefab.Solids, prefab.Contents, document.Version); err != nil {
			return fmt.Errorf("prefab %q: %w", prefab.ID, err)
		}
		solidCount += len(prefab.Solids)
		contentCount += len(prefab.Contents)
	}
	if solidCount > world.MaxStaticSolids || contentCount > MaxContents {
		return ErrBounds
	}
	return nil
}

func validateExtras(solids []Solid, contents []Content, version uint16) error {
	if (solids != nil || contents != nil) && version < SolidsVersion {
		return ErrVersion
	}
	if len(solids) > world.MaxStaticSolids || len(contents) > MaxContents {
		return ErrBounds
	}
	compiled := &world.StaticSolids{Version: world.StaticSolidsVersion, Items: make([]world.Solid, len(solids))}
	for index, solid := range solids {
		if len(solid.Footprint) > world.MaxSolidVertices {
			return ErrBounds
		}
		if !validIdentifier(solid.SideMaterial) || !validIdentifier(solid.TopMaterial) || !validIdentifier(solid.BottomMaterial) {
			return fmt.Errorf("solid %q materials: %w", solid.ID, ErrReference)
		}
		compiled.Items[index] = world.Solid{
			ID:             solid.ID,
			Bottom:         world.Plane(solid.Bottom),
			Top:            world.Plane(solid.Top),
			SideMaterial:   1,
			TopMaterial:    1,
			BottomMaterial: 1,
			Footprint:      make([]world.Vec2, len(solid.Footprint)),
		}
		for vertex, point := range solid.Footprint {
			compiled.Items[index].Footprint[vertex] = world.Vec2(point)
		}
		for _, uv := range []*UVSettings{solid.TopUV, solid.BottomUV} {
			if err := validateVersionedUV(uv, version, true); err != nil {
				return err
			}
		}
		if solid.SideUV != nil {
			if err := ValidateUVSettings(solid.SideUV); err != nil {
				return err
			}
			if solid.SideUV.Mode != "" && solid.SideUV.Mode != UVPlanar || solid.SideUV.Anchor != "" && solid.SideUV.Anchor != UVWorld {
				return ErrUVMapping
			}
		}
	}
	if err := world.ValidateStaticSolids(compiled); err != nil {
		return fmt.Errorf("static solids: %w", err)
	}
	identities := make(map[string]struct{}, len(contents))
	for _, content := range contents {
		if !validIdentifier(content.ID) || !validIdentifier(content.Kind) || !validVec3(content.Position) ||
			content.Actor != nil && !validActor(content.Actor) {
			return ErrContent
		}
		if _, exists := identities[content.ID]; exists {
			return ErrIdentity
		}
		identities[content.ID] = struct{}{}
	}
	return nil
}
