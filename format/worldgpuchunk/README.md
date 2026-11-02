# GPU-ready spatial chunks

`world/gpu-chunks@2` selects `karty.world-gpu-chunks@2`. It requires prepared
world materials and sectors. Physics/probe catalogs and completed lighting
remain optional; live-only cells use a neutral three-layer RNM image.
The root contains `@world/gpu-chunks/catalog` (`KGC2`) and the
`kartyWorldGPUChunks` marker. Each referenced cell KLD contains
`@world/gpu-chunks/cell` (`KGP2`), `@world/gpu-chunks/rnm.ktx2` and the
`kartyWorldGPUCell` marker. Marker values equal the schema string. Root and cell
entries cannot coexist in one envelope; partial or undeclared data is invalid.

## Spatial ownership and portals

Cells retain whole authored room or geometry extents inside enclosing XY grid
bounds, with full XYZ bounds. Grid lines bound those extents; they do not require
cuts through rooms. `PortalSpaces` joins only reciprocal portals with coincident,
reversed edges. Directed, transformed and unrelated overlapping spaces stay
separate. The lowest sector ID identifies each component. Catalog cells have
dense IDs and canonical `(SpaceID, Group, GridY, GridX, GridSpan, FirstSectorID)`
order. `GridSpan` is a power of two from 1 through 4096 and scales the base cell
size. Distinct room owners can share the same enclosing grid key; their first
sector IDs provide the final canonical tie. `Group` is zero for ordinary geometry
and one for authored spatial geometry. Explicit sector and physical ownership
lists are sorted and unique. Overlapping loading extents never establish
adjacency or duplicate ownership across unrelated spaces.

Portal traversal selects required cells and lookahead in each retained portal
context. Grid adjacency never establishes visibility, reachability or prefetch.
Bounds cull whole cells inside the appropriate context. Rendering submits their
prebuilt draw ranges without subdividing them into sector windows. A transformed
portal can require a distant cell while a nearby disconnected cell remains
unloaded. The existing physics/probe catalog uses `worldchunk.PhysicalSchema`
(`KCC6`); it owns no render geometry or RNM image.

The catalog binds the canonical world geometry digest and shared material layout
SHA-256. Each cell descriptor binds its entire KLD SHA-256 and transport size,
final GPU byte count and decoded RNM byte count. Existing 32 MiB module, 16 MiB
level envelope and 8 MiB entry bounds remain unchanged. Admission also verifies
ownership against `PortalSpaces` and the admitted physical catalog.

## Immutable GPU ABI

All GPU words are little endian. The container uses bounded `binwire` version-1
headers and typed counts. It rejects trailing bytes and unknown versions. The
public ABI name is `karty.webgpu-cell@2`. Catalogs use `KGC2`; payloads use
`KGP2` and require payload ABI word 2. Legacy `KGC1`/`KGP1`, the 164-byte cell
ABI and unknown versions are rejected. Metadata keys and entry names remain
stable; the new capability gates compatibility. No runtime repacking is needed.

| Stream                   | Stride | Contents                                                                                                                                                   |
| ------------------------ | -----: | ---------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Source vertex            |     96 | Position XYZ at 0, normal XYZ at 12, tangent XYZW at 24, material UV at 40, lightmap UV at 48, bake tangent at 56, bake bitangent at 68, motion XYZW at 80 |
| Draw address             |      8 | Cell-local source vertex index and cell-local record index                                                                                                 |
| Index                    |      4 | Cell-local draw address index                                                                                                                              |
| Instance/material record |    368 | Existing shared shader ABI; packed by `AppendStaticRecord`                                                                                                 |

Each draw has a representative record, alpha mode, full bounds and explicit
sector coverage. Draw index ranges are contiguous, complete triangle lists.
Addresses may reference other records only when their sector/solid owner kind
and ID match the representative for blended draws; opaque draws may address
independent owners. Every record and local reference is admitted
before publication. `RecordRef` identifies canonical material ID, sector/solid
owner and source surface kind (wall/floor/ceiling). Material IDs can be sparse;
packed texture layer indexes match their positions in the shared root layout.

Opaque and cutout records share chunk batches with draw alpha mode zero; the
fragment shader retains each record's cutout discard. Prepared liquid records
use alpha mode two and a separate owner batch. All records addressed by such a
draw must also use alpha mode two. Blended cell commands retain portal-aware
depth ordering with dynamic blended objects.

Cell records use RNM lighting, prepared material detail and local light layer
zero. Prepared liquids retain authored opacity, specular controls, UV flow and
wave motion in the existing shader ABI. Water is an explicit boolean record
flag; its vertex motion uses the existing mode-two representation. Unit normals,
orthogonal unit tangents and tangent handedness are checked without normalizing
or regenerating them. Skinning and per-vertex RNM fields are absent in this cell
profile. Dynamic mesh and character formats retain their separate ABI.

Particle, skinning, clipping and portal state are absent from cell records.
Portal transforms and clipping use separate dynamic draw context metadata.
Fixed probe slots are admitted against the immutable root probe table.
`ValidateRecord` admits one complete changed record independently of unchanged
geometry, addresses or other records.

## Cell lighting and permitted loading work

RNM is the directional lightmap and belongs to each cell. Albedo, detail and
SSBump material arrays remain shared immutable root assets.

Each cell owns one linear `RGBA8_UNORM + Zstandard` KTX2 array, inspected by
`ktx2.InspectRGBA8`. It has one mip and exactly three array layers in RNM basis
order, dimensions at most 2048 and RGBM range declared in the payload. Its
allocation is exactly `12 × width × height` bytes. No root-wide lighting array
or fixed global lighting layer registry exists.

The catalog's sorted, unique `BakedLights` indexes identify the union of authored
point sources already included in cell lighting. `ValidateLights` binds those
indexes to the root lighting table before cells load. This preserves selective
prebaking even when no physical catalog exists; live-only cells leave the union
empty. Cells may retain bounded layout and bake-manifest provenance, while the
compiler's intermediate QOI images are omitted.

The producer resolves clipping, tangents, vertex conversion, indexing,
materials, draw grouping, chart constants, resizing, packing and mips offline.
The loader verifies hashes, typed metadata, references and device limits,
decodes the required compression once and uploads the existing bytes. Renderers
retain independent per-cell GPU resources. Unchanged cells require no vertex
conversion, merging, address rewriting, image assembly or upload. Runtime
resource and aggregate allocation bounds still apply to the complete resident
set. `DecodePayloadWithCheckpoint` performs the same complete admission with an
optional callback between bounded scan batches. A host can cancel or yield to
its event loop; the callback does not transform data, and failure returns no
partial cell.

Actors, active movers, terrain and sky retain their separate rendering profiles.
Prepared liquid surfaces use the cell blend path described above. Gameplay
physics is never duplicated into render cells.
