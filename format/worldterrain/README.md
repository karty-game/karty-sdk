# Compiled heightfield terrain, version 1

Versioned decision (2026-10-15): `world/terrain@1` adds immutable ground alongside
ordinary sector topology. It does not change compiled world versions, the level
envelope, sector mesh chunks, material atlases, or source version 18. Public
compilers and the CLI may package this compiled resource before a separately
versioned terrain authoring language is introduced.

A level carries one `level.EntryData` named `@world/terrain` and metadata
`kartyWorldTerrain: "karty.world-terrain@1"`. The cartridge advertises
`world/terrain@1` together with `world/sectors@1`. Consumers reject missing or
partial metadata/resource pairs, unsupported feature declarations, wrong entry
kinds, and prefixed alternate terrain entries before publishing a mount.

## Ground geometry and ownership

`Document.Columns` and `Rows` count vertices of a regular XY lattice, from 2 to
257 inclusive. `Origin` is its XY corner and positive `CellSize` is metres per
cell, in [0.001, 1000000]. Coordinates and heights remain within ±1000000.
Row-major `Heights` and `Normals` have one sample per vertex; normals are offline
area-weighted unit vectors with positive Z. They are not regenerated per frame.

Candidate v1 clarification (2026-10-18): each axis's actual float32 mesh
coordinates must strictly increase and remain within the coordinate bound.
An origin and cell size that collapse adjacent positions are rejected, including
on decode. Sampling and ownership centres use those same rounded mesh positions.
This corrects degenerate geometry without changing KWT1 bytes or version gates.

`Holes` contains exactly one byte per cell: zero is present and one suppresses
both ground triangles for rendering and collision. Houses can initially occupy
grid-aligned holes, with their ordinary sector floors meeting the terrain at an
authored threshold. Arbitrary contour cuts and overhangs are outside this first
contract. Bridges, interiors and caves remain ordinary sectors or static models.

Sorted unique `ExteriorSectors` identifies sky-sector floors replaced by this
resource. Their walls, ceilings, portals and gameplay identities remain intact.
`ValidateForWorld` checks IDs, sky status, every active cell corner and centre,
and their finite owner ceilings against an already validated world. All portals
must be reciprocal coincident edges, and exterior owners must share an ordinary
connected component. Nonselected sector footprints intersecting the field must be
grid-aligned axis-aligned rectangles with all their overlapping cells excluded.
This first version rejects nonaligned house cuts and overlapping upper-storey
footprints instead of guessing their exclusions. Boundary points may belong to
multiple sectors and must fit every owning ceiling. Ownership validation has a
hard 16,777,216 polygon-edge-check budget. Engine consumers additionally enforce
shared render/physics allocation bounds. Terrain cannot silently make an
unrelated sector floor vanish.

Patches partition every cell exactly once, including holes, in ascending grid
Y/X order. Patch X/Y is the first cell; its Columns/Rows are **cell counts**.
`CompilePatch` creates indexed position/normal geometry once at mount. In each
cell, vertices a=lower-left, b=lower-right, c=upper-left, d=upper-right emit
`[a,b,c, b,d,c]`; `HeightAt` uses these same two triangle planes. The shared
lattice preserves boundary positions and normals across patches. No camera or
animation update changes ground geometry.

The additive `CompilePatches` helper (2026-10-18) validates the complete document
once before generating every patch in order. It returns the same independently
owned meshes as repeated `CompilePatch` calls; invalid unused materials and
hidden cells still fail complete validation. Engine mount preparation uses this
batch helper to avoid rescanning every texture once per patch.

## Materials and detail fading

Up to 16 terrain materials have self-contained straight RGBA8 albedo and normal
images, each exactly 256×256. Albedo is opaque sRGB. Normal RGB is linear OpenGL
tangent XYZ, mapped from [-1,1] to bytes with neutral `(128,128,255)`; alpha is 255. These images are independent of the sector material atlas and its packed
normal/height/AO convention. `Scale` is world metres per repeat, in
[0.001,1000000]; `NormalStrength` is [0,4]. `SideProjection` opts a material into
world-aligned cliff projections as well as horizontal XY projection.

Each patch's four-channel weight image addresses its four `Palette` material
indices. Repeated palette indices permit fewer than four distinct materials.
Channels sum to exactly 255 per texel. Image endpoints map to patch boundaries;
producers preserve equivalent global material weights at shared edges. This
allows more than four global materials while bounding each patch's shader work.

`FarColour` is an opaque sRGB RGBA8 image over the whole lattice extent, also
using endpoint samples. All weight/far image dimensions are between 2 and 1024.
Producers compile coherent distant ground colour; consumers fade material detail
into it without introducing distance fog. Hosts build isolated material mip
chains at upload, use filtered sampling, preserve normal directions and account
for padded mip storage. This contract introduces no dynamic shadows, RNM bake,
water, vegetation or fog policy.

## Canonical bounded binary

`Encode`/`Decode` use `KWT1`, the existing 16-byte typed binary header (binary
version 1, zero flags/reserved words, exact payload length). The body is:

1. uint16 semantic version; two float32 origin values; float32 cell size;
   uint16 vertex columns/rows; uint16 exterior/material/patch counts.
2. Derived-count float32 heights, three float32 normal components per vertex,
   derived-count hole bytes, uint16 exterior-sector indices.
3. Each material: albedo image, normal image, float32 scale/normal strength,
   canonical 0/1 side-projection byte.
4. Each patch: four uint16 rectangle fields, four uint16 palette indices,
   weight image; then the global far-colour image.

Every image is uint16 width/height followed by exactly width×height×4 bytes.
There are no arbitrary counts for lattice arrays, external file references,
compression, JSON or reflection. Negative-zero/nonfinite floats, unsupported
versions, invalid booleans, truncated/extra data and invalid semantic records are
rejected. Complete bounded scanning precedes owned typed allocation; decode
copies pixels and samples rather than aliasing caller bytes. `Clone` deep-copies
all mutable backing slices.

The encoded resource is at most 8 MiB; total decoded scalar resource storage is
at most 32 MiB. There are at most 65,536 active ground triangles, 64 patches, 16 materials and
256 exterior sectors. Holes do not consume the active triangle budget, allowing
the maximum lattice size when sufficient cells are excluded. The enclosing level retains its existing 16 MiB total bound. Hosts
separately charge generated geometry, collision, image uploads and mip levels
against their existing budgets; resource validity alone does not grant an
unbounded rendering or physics allocation.
