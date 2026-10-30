package worldchunk

import "github.com/karty-game/karty-sdk/format/worldlightmap"

const (
	SpatialSchema      = "karty.world-chunks@4"
	SelectedMeshSchema = "karty.world-chunk-mesh@3"
	SpatialFeature     = "world/spatial-chunks@2"
	CollisionEntryName = "@world/chunks/collision"
	MaxSpatialChunks   = 4096
)

// SpatialChunk is a render-only leaf. It shares source identity and physical
// context with Parent; its local ID never participates in sector ownership.
// MaxDistance zero means no additional distance cap beyond visibility/budgets.
type SpatialChunk struct {
	LiquidTiles           []worldlightmap.LiquidTile
	LiquidEdgeStep        float64
	LiquidMaxSubdivisions int
	Chunk
	Parent      int
	Tile        [4]float64
	Bindings    []worldlightmap.Binding
	MaxDistance float64
}

// SurfaceOwnership accounts for every global face, including sky/empty faces.
// Owner is the primary chunk index in every mode; spatial children with that
// Parent may carry the named source face. Terrain mode retains primary physics.
type SurfaceOwnership struct {
	Binding worldlightmap.Binding
	Kind    string
	Owner   int
}

func (catalog *Catalog) StreamCount() int {
	if catalog == nil {
		return 0
	}
	return len(catalog.Chunks) + len(catalog.Spatial)
}
func (catalog *Catalog) StreamChunk(id int) (Chunk, bool) {
	if catalog == nil || id < 0 {
		return Chunk{}, false
	}
	if id < len(catalog.Chunks) {
		return catalog.Chunks[id], true
	}
	local := id - len(catalog.Chunks)
	if local >= len(catalog.Spatial) {
		return Chunk{}, false
	}
	chunk := catalog.Spatial[local].Chunk
	chunk.ID = id
	return chunk, true
}
func validMeshVersion(mesh *Mesh) bool {
	return mesh.Schema == MeshSchema && mesh.Selection == nil || mesh.Schema == SelectedMeshSchema && mesh.Selection != nil
}
