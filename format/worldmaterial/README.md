# World material atlas v1

`world/material-atlas@1` is the opt-in runtime capability, exported as
`worldmaterial.Feature`, `cartridge.FeatureWorldMaterialAtlasV1` and
`asset.CapabilityWorldMaterialAtlasV1`. The game manifest includes the union of
its levels' capabilities in its existing sorted, unique version-2 feature list,
including `world/sectors@1` for compiled worlds. Existing manifests and worlds
without this capability retain their material behavior. The candidate binary
wire below requires rebuilding runtime artifacts.

A world level carries three fixed logical `level.EntryData` payloads and, when
`Layout.Mips` is present, one additional combined mip-tail payload:

| Symbol         | Entry                    | Content                                                   |
| -------------- | ------------------------ | --------------------------------------------------------- |
| `LayoutEntry`  | `@world/material-layout` | Canonical `KMA1` binary placements                        |
| `AlbedoEntry`  | `@world/material-albedo` | Opaque RGBA QOI, sRGB colorspace (0)                      |
| `DataEntry`    | `@world/material-data`   | Straight RGBA QOI, linear colorspace (1)                  |
| `MipTailEntry` | `@world/material-mips`   | Optional combined albedo/data QOI, raw linear storage (1) |

Level metadata declares `"kartyWorldMaterialAtlas":"karty.world-material-atlas@1"`
(`MetadataKey` and `Schema`). A host must reject missing/partial pairs, entries
without this metadata, metadata without the entries, or an atlas without a
compiled world. These cross-entry/world checks belong to the level consumer;
`Pair.Validate` validates the layout, L0 pair and optional tail together.
`Pair.Decode` performs the same complete validation and returns a `DecodedPair`
that retains owned straight-alpha NRGBA images. Its `Albedo`, `Data`, `MipTail`,
`Reflectance` and `ReflectanceMipTail` accessors return pixels without aliasing
the encoded pair; absent optional images are nil. Independent decode calls own
independent images. Errors return an empty result, including when a later image
is malformed. Consumers can retain these validated pixels instead of decoding
them again. The existing binary schemas and capability declarations are unchanged.
The tail and the complete eight `mips` records must be present together. Generic
`level.Encode`/`Decode` enforce envelope bounds and entry name/kind rules, not
this opt-in payload schema or metadata/world consistency.

## Candidate binary layout, version 1

`EncodeLayout` writes `KMA1` with a 16-byte little-endian header: magic,
uint16 binary version 1, uint16 zero flags, uint32 exact payload length and
uint32 zero reserved. Payload fields follow public Go declaration order, using
fixed int32 dimensions, uint32 IDs, finite float64 strengths, UTF-8 strings,
0/1 pointer-presence bytes and uint32 slice counts (`length+1`, zero for nil).
See the [shared typed primitives](../../internal/binwire/README.md). Semantic
atlas schemas @1/@2 stay distinct from this binary version. JSON is rejected
with no fallback; released JSON artifacts must be rebuilt for this candidate.

- Top-level fields: `schema` (string), `width`, `height` (integer pixel dimensions),
  `materials` (array of placement objects), followed by optional `mips`.
- Placement fields: `materialId` (uint32 level-local material ID), `x`, `y`
  (integer inner-tile origin), `width`, `height` (256), `gutter` (16 pixels),
  followed by a `strengths` presence tag and the `coverage` string.
- IDs are unique, in first-surface-use order: floor, ceiling, then walls per
  sector in compiled-world order. ID 0 denotes the opaque default material;
  it is included only if used. IDs need not be contiguous or numerically sorted.
- A nonzero ID is the same level-local source texture ID used by the compiled
  world's surface and `level.TextureEntryName(id)`, not a dense slot index or
  cartridge-wide asset ID. The builder requires that source texture to exist,
  decode correctly and be opaque. Default ID 0 needs no source texture.
- There are 1–196 materials. Slots are row-major in a 14-column grid, with
  288-pixel cells and a 32-pixel outer inset. For zero-based slot `i`, inner
  origin is `(48 + (i % 14)*288, 48 + (i / 14)*288)`.
- Dimensions are `min(count,14)*288 + 64` by `ceil(count/14)*288 + 64`, up to
  4096 by 4096. Each tile's surrounding gutter is part of its cell. Slots,
  dimensions and gutters must match exactly; arbitrary packing is not v1.
- `DecodeLayout` accepts only the bytes `EncodeLayout` would emit, bounded to
  32 KiB. Complete scanning enforces at most 196 placements and eight mip records
  before typed allocation. Truncation, bad counts/presence tags, unsupported
  versions, nonfinite/negative-zero floats and trailing bytes are rejected.

`NewLayout`, `Layout.Validate`, `EncodeLayout`, and `DecodeLayout` define this
layout contract. `Pair{Layout, Albedo, Data, MipTail}` groups in-memory encoded bytes;
the typed binary layout is the public level wire format. Validation does
not transfer or clone caller-owned slices. Invalid input returns `ErrAtlas`.

## Optional mip tail and strengths

`NewMipLayout(layout)` clones the placements and controls, preserves every L0
rectangle, and adds all eight L1–L8 records. Each canonical record has fields
`level`, `size`, `gutter`, `y`, `columns` in that order. For level `n` (1–8),
`size = 256 >> n`, `gutter = max(1,16 >> n)`, and `cell = size + 2*gutter`.
For `count` materials, `columns = min(2*count, floor(4096/cell))`. The first
band begins at `y=0`; each subsequent band begins after the previous band's
`ceil(2*count/columns)*cell` padded height. Albedo/data cells are interleaved
in L0 material order: cell index `2*slot` is albedo, `2*slot+1` is raw data.
The inner origin is `(index%columns*cell+gutter,
y+index/columns*cell+gutter)`. `MipRect(level,slot,data)` returns this rectangle.
All records, including the final 1×1 level with its one-pixel gutter, are
required; partial or alternate packing is invalid.

`MipDimensions()` returns the maximum band width and sum of band heights.
At 196 materials the padded tail is 4086×2723; each dimension remains at most
4096 for every supported count. A single extra image contains both region
kinds. Its QOI header is RGBA and
linear (1) because it is raw mixed storage: **albedo cells still contain sRGB
encoded bytes**, while data cells retain linear RG normal XY, B height and
A AO. Albedo interiors/gutters are opaque; data AO may be zero. Unused tail
padding is charged to the decoded allocation even though it is not sampled.
A nil mip slice and absent tail retain L0-only behavior.

An optional placement `strengths` object contains all four fields in this
order: `normal`, `height`, `ao`, `rim`. Each is a finite float64 in [0,4].
Omitting the object means 1 for every effect; explicit 0 disables that effect.
Normal scales tangent XY before reconstructing/normalizing Z. Height scales
bounded relief. AO applies `clamp(1-strength*(1-AO),0,1)` to ambient only.
Rim scales the restrained direct-light rim; it does not imply specular gloss.
Controls change layout metadata independently of generated image bytes, so
artistic adjustments reuse the Materialize cache. Individual valid controls
must still fit the shared 32 KiB encoded-layout bound; excessively verbose
high-precision controls across a large atlas can exceed it and are rejected.

The layout stores strengths as float64 values. Consumers own runtime packing,
quantization and lighting implementation; those choices preserve the stored
normal XY, height and AO channel meanings and each slot's coverage declaration.

## Build and load responsibilities

The native Go CLI packs/resamples albedo tiles and extrudes their gutters at
build time, then invokes the managed Materialize executable **once on the
completed atlas**. It packs OpenGL tangent normal X/Y into R/G, material height
into B, and ambient occlusion into A. Normal byte 128 is zero; signed decode is
`clamp((byte-128)/127, -1, 1)`. Height/AO are normalized unsigned bytes. A is data,
**not opacity**, so uploads must not premultiply RGB by AO. The CLI restores
generated gutters and preserves the L0 QOI pair. It generates each material's
mip interiors independently, then extrudes that level's own gutters; it never
downsamples across atlas material boundaries. Albedo averages in linear RGB and
encodes sRGB afterward. Normals decode to normalized 3D vectors, average and
normalize before XY encoding, preserving exact neutral `(128,128)`. Height/AO
use linear averages; AO remains straight data even when zero. The complete
256-to-1 chain is identified by the build/cache generation revision.

Runtime loads these payloads/layout directly; it does not repack or regenerate
material maps. Both images must have exactly the layout dimensions and RGBA
headers with their respective colorspaces. `Pair.Validate` preflights every
bounded QOI allocation, including the padded tail, before any pixel decoding and rejects nonopaque
albedo pixels, including gutters, margins and unused slots; data alpha may take
any byte value. The data atlas uses complete allocation-free stream validation;
albedo and the combined tail are decoded only for their opacity checks.
QOI decoding checks operation boundaries and the exact pixel
count without allocation before invoking the codec, so malformed runs cannot
grow pixel buffers beyond their declared bounds. The maximum decoded L0 pair is
128 MiB; the maximum pair plus padded tail is 178,722,440 bytes. Consumers
account for all three images alongside other textures under the shared
256 MiB decoded-texture budget. Level entry/envelope encoded-size limits remain
unchanged and can reject otherwise-valid large pairs.

There are no filesystem paths, URLs or external texture references in this
contract: entry names are fixed logical keys, never filenames. CLI source/cache
path confinement and Materialize output validation remain CLI responsibilities.
Consumers additionally check that layout IDs and order match all compiled world
surfaces, with no missing or unused placements; the SDK atlas package cannot
validate a world it is not given. Source textures used independently by sprites
or UI remain separate level texture entries; the pair does not replace those
references. Envelope validation rejects duplicate numeric texture IDs even when
distinct hexadecimal entry names alias the same ID. Payload
validation cannot prove Materialize execution, correct extrusion or channel-map
provenance, nor distinguish already-premultiplied data from valid channel bytes;
those remain build-pipeline and upload responsibilities.

These optional extensions are available in SDK 0.0.8.
Subsequent incompatible layout/channel changes require a new schema and capability. Version 1
does not modify released world, level-envelope, or cartridge wire versions.

## Coverage-aware material atlas, version 2

`world/material-atlas@2` selects `karty.world-material-atlas@2`; logical level
entry names and paired atlas dimensions remain unchanged. Use `NewLayoutV2`
to create the same canonical packing with per-slot `coverage` declarations.
Every slot must declare `opaque` or `masked`. Version 1 forbids the declaration
and retains full-opacity validation.

Albedo RGB stores straight sRGB colour and alpha stores layer coverage. Data
remains straight linear normal XY/height/AO in RG/B/A; AO is never opacity.
Masked slots may contain any alpha, including their gutters and mip albedo
cells. Opaque slots and the outer L0 atlas padding must remain alpha 255.
`Pair.Validate` preflights the same decoded budgets and completely validates
all QOI streams, then checks every opaque slot at every supplied mip level.
Unknown or missing coverage values and malformed binary records reject layout
decoding. Slots used as main or secondary materials must be opaque;
opaque frame fast paths must reference opaque slots.

Builders preserve band coverage independently of material-map generation,
which receives opaque RGB with colour extended into uncovered pixels. Coverage
mips average alpha and coverage-weighted linear RGB, normals, height and AO;
normal vectors are normalized after averaging and zero coverage uses neutral
material data. Sampling uses coverage-aware filtering with the compiled
repeat/clamp controls for the selected region, rather than reading unrelated
sheet regions. `NewMipLayout` preserves coverage declarations. Existing
196-slot, 4096-pixel dimension, 32 KiB layout and 256 MiB shared decoded-budget
limits apply unchanged. Secondary-only slots have neutral data (128,128,0,255).

Material effects in `world/animations@1` require opaque atlas coverage for both
the bound target and every flipbook frame. `ValidateAnimationCoverage` checks
that complete relationship after layout preparation. Legacy atlas v1's absent
coverage is opaque; atlas v2 requires `coverage: "opaque"`. Masked world artwork
and animated coverage boundaries are excluded from this first version. Explicit
actor flipbooks use ordinary sprite textures and may retain alpha.

## Reflectance companion v1

The optional `world/material-reflectance@1` capability adds metallic/smoothness
without changing either atlas schema or its existing channels. It is exported as
`FeatureReflectance` and `asset.CapabilityWorldMaterialReflectanceV1`. Hosts and
producers must gate this companion independently; old levels remain valid.

| Symbol                    | Entry                              | Content                                     |
| ------------------------- | ---------------------------------- | ------------------------------------------- |
| `ReflectanceEntry`        | `@world/material-reflectance`      | Linear RGBA QOI at L0 atlas dimensions      |
| `ReflectanceMipTailEntry` | `@world/material-reflectance-mips` | Linear RGBA QOI at existing tail dimensions |

`Pair.Reflectance` contains R metallic, G smoothness, B zero and A 255. Both
use normalized bytes; these are straight linear data, never sRGB or opacity.
GPU consumers may upload only RG as `RG8`; packaged and decoded QOI costs remain
four bytes per pixel and participate fully in the shared 256 MiB allocation
budget. All headers are checked before any image is decoded.

`Pair.ReflectanceMipTail` is required exactly when reflectance and `Layout.Mips`
are both present. It uses `MipRect(level, slot, true)` cells, including their
gutters, in the unchanged existing tail coordinates. Albedo cells and padded
unused slots contain neutral `(0,0,0,255)`. This wastes some tail storage but
avoids a second placement contract. Complete streams, dimensions, colorspace,
reserved channels, missing/partial companions and aggregate budgets are
validated by `Pair.Validate`.

Level consumers also reject reflectance entries without a material atlas,
wrong entry kinds, entries without their feature gate, or an advertised level
feature without its companion. Producers advertise this feature only for
levels carrying reflectance. Reflectance is optional for old atlas levels.

Smoothness is inverted to roughness by the shader; metallic changes specular
color and reduces diffuse energy. Mip production must avoid crossing material
boundaries and extrude each mip's gutter. This contract specifies stored data;
BRDF implementation and artistic tuning belong to the renderer.

Versioned decision: companion v1 (2026-10-09), public SDK format ownership.
The fixed entries and independent capability preserve atlas@1/@2 and old
SDK behavior, without reinterpreting their normal/height/AO channels.
