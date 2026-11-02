package worldchunk

import (
	"slices"
	"strings"

	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
)

// CompileMesh triangulates immutable authored receiver polygons offline. Every
// boundary vertex, including collinear junctions, gets its own triangle edge.
// The runtime reads final arrays and never constructs or tessellates a face.
func CompileMesh(document world.Document, sectors, solids []int, layout worldlightmap.Layout) (Mesh, error) {
	return compileMesh(document, sectors, solids, layout, nil)
}

// CompileSelectedMesh clips explicit source surfaces offline without allocating
// lightmap charts or selecting a lighting density. All triangle charts are -1;
// CompileReady accepts the resulting mesh with a nil lightmap layout.
func CompileSelectedMesh(document world.Document, sectors, solids []int, selection worldlightmap.SurfaceSelection) (Mesh, error) {
	solidCount := 0
	if document.StaticSolids != nil {
		solidCount = len(document.StaticSolids.Items)
	}
	if err := worldlightmap.ValidateSurfaceSelection(
		&selection,
		worldlightmap.LayoutAdmission{SectorCount: len(document.Sectors), SolidCount: solidCount},
	); err != nil {
		return Mesh{}, err
	}
	selection.Bindings = slices.DeleteFunc(slices.Clone(selection.Bindings), func(binding worldlightmap.Binding) bool {
		owners := solids
		if strings.HasPrefix(binding.Kind, "sector-") {
			owners = sectors
		}
		_, included := slices.BinarySearch(owners, binding.Index)
		return !included
	})
	selection.LiquidTiles = slices.Clone(selection.LiquidTiles)
	if selection.Clip != nil {
		clip := *selection.Clip
		selection.Clip = &clip
	}
	return compileMesh(document, sectors, solids, worldlightmap.Layout{}, &selection)
}
