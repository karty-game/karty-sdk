# World material atlas v1

`world/material-atlas@1` is the opt-in runtime capability, exported as
`worldmaterial.Feature`, `cartridge.FeatureWorldMaterialAtlasV1` and
`asset.CapabilityWorldMaterialAtlasV1`. The game manifest includes the union of
its levels' capabilities in its existing sorted, unique version-2 feature list,
including `world/sectors@1` for compiled worlds. Existing manifests and worlds
without this capability retain their released encodings and legacy loading.

A world level carries three fixed logical `level.EntryData` payloads and, when
`Layout.Mips` is present, one additional combined mip-tail payload:

| Symbol         | Entry                    | Content                                                   |
| -------------- | ------------------------ | --------------------------------------------------------- |
| `LayoutEntry`  | `@world/material-layout` | Canonical UTF-8 JSON placements                           |
| `AlbedoEntry`  | `@world/material-albedo` | Opaque RGBA QOI, sRGB colorspace (0)                      |
| `DataEntry`    | `@world/material-data`   | Straight RGBA QOI, linear colorspace (1)                  |
| `MipTailEntry` | `@world/material-mips`   | Optional combined albedo/data QOI, raw linear storage (1) |

Level metadata declares `"kartyWorldMaterialAtlas":"karty.world-material-atlas@1"`
(`MetadataKey` and `Schema`). A host must reject missing/partial pairs, entries
without this metadata, metadata without the entries, or an atlas without a
compiled world. These cross-entry/world checks belong to the level consumer;
`Pair.Validate` validates the layout, L0 pair and optional tail together.
The tail and the complete eight `mips` records must be present together. Generic
`level.Encode`/`Decode` enforce envelope bounds and entry name/kind rules, not
this opt-in payload schema or metadata/world consistency.

## JSON layout

`EncodeLayout` emits fields in this exact order, without whitespace:

```json
{"schema":"karty.world-material-atlas@1","width":352,"height":352,"materials":[{"materialId":0,"x":48,"y":48,"width":256,"height":256,"gutter":16}]}
```

- Top-level fields: `schema` (string), `width`, `height` (integer pixel dimensions),
  `materials` (array of placement objects), followed by optional `mips`.
- Placement fields: `materialId` (uint32 level-local material ID), `x`, `y`
  (integer inner-tile origin), `width`, `height` (256), `gutter` (16 pixels),
  followed by optional `strengths`.
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
  32 KiB. Unknown, duplicate, missing, reordered fields, alternate numeric
  spelling, whitespace and trailing data are rejected. This is machine-generated
  JSON, not an author-editable source format.

`NewLayout`, `Layout.Validate`, `EncodeLayout`, and `DecodeLayout` define this
layout contract. `Pair{Layout, Albedo, Data, MipTail}` groups in-memory encoded bytes;
its Go JSON representation is not the public level wire format. Validation does
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
kinds, fitting the engine's fourth shader sampler. Its QOI header is RGBA and
linear (1) because it is raw mixed storage: **albedo cells still contain sRGB
encoded bytes**, while data cells retain linear RG normal XY, B height and
A AO. Albedo interiors/gutters are opaque; data AO may be zero. Unused tail
padding is charged to the decoded allocation even though it is not sampled.
An absent `mips` field and absent tail retain the exact L0-only layout encoding.

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

The engine retains strengths in metadata bytes: `q=min(254,round(value*64))` below 3.984375; values at or above
that endpoint midpoint use reserved byte 255. Bytes 0–254 decode as `q/64`,
and byte 255 decodes as exactly 4.
Defaults bypass repacking and retain previous metadata bytes. Thus most values
use a 1/64 grid; the final representable interval is 1/32, with maximum
nearest-value error 1/64.
When any material has a nondefault rim strength, the normal buffer's B rim mask
stores `height*rim/4` in eight bits for all materials and the resolve scales it
by four. This reduces rim-mask precision for default-strength materials in
that same atlas; it changes neither normal XY nor geometric depth.

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
and retains full-opacity validation and its existing canonical bytes.

Albedo RGB stores straight sRGB colour and alpha stores layer coverage. Data
remains straight linear normal XY/height/AO in RG/B/A; AO is never opacity.
Masked slots may contain any alpha, including their gutters and mip albedo
cells. Opaque slots and the outer L0 atlas padding must remain alpha 255.
`Pair.Validate` preflights the same decoded budgets and completely validates
all QOI streams, then checks every opaque slot at every supplied mip level.
Unknown, missing, duplicate or explicit-null declarations reject canonical
layout decoding. Slots used as main or secondary materials must be opaque;
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
