package worldchunk

import (
	"github.com/karty-game/karty-sdk/format/world"
	"github.com/karty-game/karty-sdk/format/worldlightmap"
	"github.com/karty-game/karty-sdk/internal/binwire"
)

func encodeSpatialCatalog(catalog Catalog) ([]byte, error) {
	base := catalog
	base.Schema = FixedSchema
	base.Spatial = nil
	base.Surfaces = nil
	bytes, err := encodeLegacyCatalog(base)
	if err != nil {
		return nil, err
	}
	w := binwire.NewWriter("KCC5", MaxEncodedSize)
	w.String(catalog.TerrainSHA256, 64)
	w.Count(len(bytes), false, MaxEncodedSize)
	w.Raw(bytes)
	for _, chunk := range catalog.Chunks {
		w.U64(chunk.CollisionBytes)
	}
	w.Count(len(catalog.Spatial), false, MaxSpatialChunks)
	for _, spatial := range catalog.Spatial {
		c := spatial.Chunk
		w.I32(c.ID)
		w.I32(spatial.Parent)
		for _, v := range spatial.Tile {
			w.F64(v)
		}
		w.F64(spatial.MaxDistance)
		w.I32(spatial.LiquidMaxSubdivisions)
		w.F64(spatial.LiquidEdgeStep)
		w.String(c.ContentSHA256, 64)
		for _, v := range []uint64{c.Size, c.MeshBytes, c.LightmapBytes, c.LightingBytes, c.TerrainBytes, c.CollisionBytes} {
			w.U64(v)
		}
		for _, v := range c.Bounds {
			w.F64(v)
		}
		w.Count(len(c.Sectors), false, world.MaxSectors)
		for _, v := range c.Sectors {
			w.I32(v)
		}
		w.Count(len(c.Solids), false, world.MaxStaticSolids)
		for _, v := range c.Solids {
			w.I32(v)
		}
		selection, err := worldlightmap.EncodeSurfaceSelection(
			worldlightmap.SurfaceSelection{Bindings: spatial.Bindings, LiquidTiles: spatial.LiquidTiles},
		)
		if err != nil {
			return nil, err
		}
		w.Count(len(selection), false, worldlightmap.MaxEncodedSize)
		w.Raw(selection)
	}
	w.Count(len(catalog.Surfaces), false, world.MaxWalls+world.MaxSectors*2+world.MaxStaticSolids*(world.MaxSolidVertices+2))
	for _, surface := range catalog.Surfaces {
		w.String(surface.Binding.Kind, 32)
		w.I32(surface.Binding.Index)
		w.I32(surface.Binding.Edge)
		w.I32(surface.Binding.Chart)
		w.String(surface.Kind, 16)
		w.I32(surface.Owner)
	}
	data, err := w.Bytes()
	if err != nil {
		return nil, ErrChunk
	}
	return data, nil
}

func decodeSpatialCatalog(data []byte, catalog *Catalog) error {
	r := binwire.NewReader(data, "KCC5", MaxEncodedSize)
	terrainSHA := r.String(64)
	n, _ := r.Count(MaxEncodedSize, 1, 0)
	base := r.Raw(n)
	if r.Err() != nil {
		return ErrChunk
	}
	if err := decodeLegacyCatalog(base, catalog); err != nil {
		return err
	}
	if catalog.Schema != FixedSchema {
		return ErrChunk
	}
	catalog.Schema = SpatialSchema
	catalog.TerrainSHA256 = terrainSHA
	for i := range catalog.Chunks {
		catalog.Chunks[i].CollisionBytes = r.U64()
	}
	n, _ = r.Count(MaxSpatialChunks, 176, 256)
	if r.Err() != nil {
		return ErrChunk
	}
	catalog.Spatial = make([]SpatialChunk, n)
	for i := range catalog.Spatial {
		spatial := &catalog.Spatial[i]
		c := &spatial.Chunk
		c.ID = r.I32()
		spatial.Parent = r.I32()
		for j := range spatial.Tile {
			spatial.Tile[j] = r.F64()
		}
		spatial.MaxDistance = r.F64()
		spatial.LiquidMaxSubdivisions = r.I32()
		spatial.LiquidEdgeStep = r.F64()
		c.ContentSHA256 = r.String(64)
		fields := []*uint64{&c.Size, &c.MeshBytes, &c.LightmapBytes, &c.LightingBytes, &c.TerrainBytes, &c.CollisionBytes}
		for _, field := range fields {
			*field = r.U64()
		}
		for j := range c.Bounds {
			c.Bounds[j] = r.F64()
		}
		count, _ := r.Count(world.MaxSectors, 4, 8)
		if r.Err() != nil {
			return ErrChunk
		}
		c.Sectors = make([]int, count)
		for j := range c.Sectors {
			c.Sectors[j] = r.I32()
		}
		count, _ = r.Count(world.MaxStaticSolids, 4, 8)
		if r.Err() != nil {
			return ErrChunk
		}
		c.Solids = make([]int, count)
		for j := range c.Solids {
			c.Solids[j] = r.I32()
		}
		count, _ = r.Count(worldlightmap.MaxEncodedSize, 1, 0)
		selection, err := worldlightmap.DecodeSurfaceSelection(r.Raw(count))
		if err != nil {
			return err
		}
		if selection.Clip != nil || selection.LiquidEdgeStep != 0 || selection.LiquidMaxSubdivisions != 0 {
			return ErrChunk
		}
		spatial.Bindings = selection.Bindings
		spatial.LiquidTiles = selection.LiquidTiles
	}
	n, _ = r.Count(world.MaxWalls+world.MaxSectors*2+world.MaxStaticSolids*(world.MaxSolidVertices+2), 28, 88)
	if r.Err() != nil {
		return ErrChunk
	}
	catalog.Surfaces = make([]SurfaceOwnership, n)
	for i := range catalog.Surfaces {
		s := &catalog.Surfaces[i]
		s.Binding.Kind = r.String(32)
		s.Binding.Index = r.I32()
		s.Binding.Edge = r.I32()
		s.Binding.Chart = r.I32()
		s.Kind = r.String(16)
		s.Owner = r.I32()
	}
	if r.Done() != nil {
		return ErrChunk
	}
	return nil
}
