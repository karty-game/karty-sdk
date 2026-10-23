package worldchunk

import "github.com/karty-game/karty-sdk/format/cartridge"

// ReadySchema identifies renderer-ready scalar streams, separate from the
// semantic mesh used by complete build-time source and lightmap validation.
const (
	ReadySchema      = "karty.world-chunk-ready@1"
	ReadyEntryName   = "@world/chunks/ready"
	ReadyMetadataKey = "kartyWorldChunkReady"
	ReadyFeature     = cartridge.FeatureWorldChunkReadyV1
	ReadyStride      = 26
	MaxReadyBytes    = 64 << 20
)

// Ready is compiled with the level, not reconstructed by presentation. The
// associated semantic mesh is the build-time proof source. Runtime artifact
// admission checks structure after exact whole-child catalog integrity checks.
// Surface indexes refer to the provided level-wide table when compiled together.
type Ready struct {
	Schema     string
	MeshSHA256 string
	Surfaces   []ReadySurface
	Batches    []ReadyBatch
}

type ReadySurface struct {
	Primary, Secondary, Band int
	SecondaryStrength        float32
	BandCoverage             uint8
	BandRepeatU, BandRepeatV bool
}

type ReadyOwner struct {
	Sector, Solid int
	First, Count  int
	Bounds        [6]float64
}

type ReadyBatch struct {
	Surface                           int
	Sky, Outdoor, StaticMaterial      bool
	Transparent, Invisible, Masked    bool
	Winding                           int8
	Vertices                          []float32
	Indices, SurfaceIDs, Sources      []uint32
	WaveUV, LightmapUV, LightmapBasis []float32
	MaterialBasis                     []float32
	Owners                            []ReadyOwner
}

// ReadyOptions selects build-time cleanup and material classification inputs.
// Masked uses the parent world's canonical material-slot order.
type ReadyOptions struct {
	// CleanupStatic derives visibility-safe static cuts at build time. It is for
	// atomic streamed chunks, whose selected owners cannot move independently.
	// Canonical Mesh stays unchanged; Ready Sources index the derived triangles.
	CleanupStatic bool
	Masked        []bool
	// Surfaces optionally fixes level-wide surface slot identities. Nil derives
	// a local deterministic table; a supplied table must contain every source key.
	Surfaces []ReadySurface
}

func ReadyBytes(ready *Ready) uint64 {
	if ready == nil {
		return 0
	}
	bytes := uint64(len(ready.Surfaces)) * 32
	for _, batch := range ready.Batches {
		bytes += uint64(len(batch.Vertices)+len(batch.Indices)+len(batch.SurfaceIDs)+len(batch.Sources)+
			len(batch.WaveUV)+len(batch.LightmapUV)+len(batch.LightmapBasis)+len(batch.MaterialBasis)) * 4
		bytes += uint64(len(batch.Owners))*64 + 64
	}
	return bytes
}
