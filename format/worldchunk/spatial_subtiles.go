package worldchunk

import (
	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
)

// A loading unit may share one atlas and GPU buffer without changing its
// original cells' boundary anchors, ring centres or interpolation geometry.
func emitSubtiledPolygon(
	mesh *Mesh,
	document *world.Document,
	layout worldlightmap.Layout,
	junctions pointIndex,
	polygon []world.Vec3,
	template Triangle,
	mapping *world.SurfaceUV,
	secondary *world.SurfaceSecondary,
	band *world.SurfaceUV,
) error {
	selection := *layout.Selection
	selection.LiquidTiles = nil
	layout.Selection = &selection
	for _, tile := range mesh.Selection.LiquidTiles {
		selection.Clip = &tile.Clip
		selection.LiquidMaxSubdivisions = tile.LiquidMaxSubdivisions
		if err := emitPolygon(mesh, document, layout, junctions, polygon, template, mapping, secondary, band); err != nil {
			return err
		}
	}
	return nil
}
