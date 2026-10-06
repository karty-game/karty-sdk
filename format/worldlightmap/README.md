# Build-generated world lightmap layout

`world/lightmaps@1` is a capability available in SDK 0.0.8. The public CLI
generates `@world/lightmaps/layout` as an `EntryData` entry in the existing KLD
envelope. Metadata contains `kartyWorldLightmaps: "karty.world-lightmaps@1"`.
Cartridges require this feature together with `world/sectors@1`. Existing world,
level-envelope and cartridge versions are unchanged; absent layouts preserve
existing packaging. Older SDK manifests remain unchanged.

The canonical JSON `Layout` has schema `karty.world-lightmaps@1`, algorithm 1,
a lower-case SHA-256 geometry digest, requested texels per world unit, padding,
pages, charts and semantic bindings. This version permits exactly one square
512 or 1024 page, at most 1,048,576 allocated texels and 8,192 charts. Layout
payloads are bounded to 8 MiB and remain subject to the envelope aggregate bound.
Density is finite in `(0,64]`; padding is 1–16 pixels. Builders never silently
lower density to fit. Oversized charts or a full page are explicit errors.

Charts contain orthonormal world-space normal, tangent and bitangent vectors,
normalized affine U/V planes and integer receiver/padded rectangles. Rectangles
are `[minimumX, minimumY, maximumX, maximumY]`, with exclusive maximums. The
receiver rectangle is exactly the padded rectangle inset by `padding`.
Receiver bounds start at texel centres, and each axis spans at least two texels
to give thin steps interior raster samples. No material UV is reused or changed.

Semantic binding kinds are `sector-floor`, `sector-ceiling`, `sector-wall`,
`solid-top`, `solid-bottom`, `solid-side`. `index` addresses the sector or solid
array; `edge` is the directed wall/footprint edge, or -1 for caps. `chart` is a
zero-based chart index, or -1 only for a fully open wall with no opaque receiver.
Every semantic face has exactly one binding, in sector floor/ceiling/wall order
followed by solid top/bottom/side order. Every solid side has its own identity;
the repeating `Solid.SideUV` field does not affect these bindings.

Algorithm 1 merges coplanar sector floors/ceilings only across coincident,
reciprocal ordinary portals. Equal coordinates in disconnected spaces do not
merge. Opaque bands around partial portals follow the shared pure
`world.WallProfile` crossing/controller rules; the doorway receives no opaque
geometry. Solid caps and sides remain separate charts. Deterministic rectangle
packing sorts by descending height, descending width and stable chart identity.
Charts are not rotated, and their reserved rectangles cannot overlap.

The geometry digest covers stable sector/solid identities, coordinates,
elevation planes, directed portal indexes and reciprocal wall indexes. Actor
state, materials, material UVs and lighting do not affect it. Geometry changes
invalidate the layout. Light movement invalidates runtime lighting rather than
the geometry layout. Consumers validate complete bounds, digest, bases, affine
maps, receiver coverage and semantic binding identities before GPU allocation.
Decoding also rejects unknown, duplicate, missing, null, trailing and
noncanonical records through exact canonical JSON comparison.

Optional `runtime_bake` has two explicitly versioned encodings. A single
visibility recipe retains its existing bytes and behavior:

```json
{"encoding":"point-visibility@1","light_id":"court-light","shadow_size":512}
```

An ordered direct-light recipe selects one to eight unique authored IDs:

```json
{"encoding":"direct-rnm3@1","light_ids":["brazier-north","brazier-south"],"shadow_size":512}
```

`light_id` and `light_ids` are mutually exclusive. Visibility requires the
single nonempty ID and no list; direct RNM requires the nonempty list and no
single field. IDs must exist in `Document.Lighting.Lights`; selection order is
preserved, with no implicit first-light selection. Shadow size is 32–512.
Omission packages reusable coordinates without requesting a runtime bake.
Unknown encodings, duplicate selections, explicit empty/null fields and
noncanonical fields fail complete validation.

Both runtime recipes require all sectors to belong to one ordinary reciprocal
physical component, every selected emitter to be inside its floor/ceiling
volume, and every solid to have a host receiver association. Association uses
conservative XY bounding-box overlap including tangent boundaries; it does
not impose a new exact polygon or height restriction on solid placement.
Builders validate the complete selection before publication. Layout-only data
still accepts disconnected/transformed spaces, outside lights and unassociated
solids. `point-visibility@1` stores static shadow visibility, leaving selected
light colour/falloff and material normal response live at runtime.

The runtime `direct-rnm3@1` recipe stores static direct diffuse lighting as three horizontal RGBM
coefficient tiles per logical page, with matching chart coordinates in each
tile. The logical page bounds above do not count those physical coefficient
copies. A 1024 logical page therefore has a 3072×1024 RGBA8 image (12 MiB);
additional scratch and cached recipes require separate runtime accounting.
No RGBM range field is authored: `range = max(1, 3 Σ max(R,G,B))` over the
selected linear RGB light colours. Each coefficient encodes `RGB = C/(range*M)`
and `A = M`, with quantized `M` rounded upward and at least `1/255` for covered
texels, including black. Alpha zero means uncovered. Decode each filter tap
as `RGB*A*range` before interpolation; normalize filter weights over covered
taps using binary alpha coverage rather than weighting by the RGBM multiplier.
Dilation preserves covered samples and cannot cross chart boundaries.

In chart coordinates (tangent, bitangent, normal), the RNM basis is:

- `B0 = (√(2/3), 0, 1/√3)`
- `B1 = (-1/√6, 1/√2, 1/√3)`
- `B2 = (-1/√6, -1/√2, 1/√3)`

For receiver normal `N` and unit receiver-to-light direction `L`, use the
existing squared Half Lambert function `H(N,L) = (0.5 dot(N,L)+0.5)²`.
For each selected light, `wi = H(Bi,L)` and accumulate
`Ci = Tint * attenuation² * visibility * 3*H(N,L)*wi/Σwi`.
Here `attenuation = max(0, 1-distance²/radius²)`.
Geometric backfaces have zero visibility. Runtime material normal `n` uses
weights `max(dot(n,Bi),0)²`, normalized by their sum, to combine the decoded
coefficients. A neutral material normal reproduces the existing Half Lambert
response before RGBM quantization. This is a Source-inspired directional
approximation, not radiosity or an exact integration of incident lighting.

Only static environment direct diffuse is baked by the runtime recipe. Once the complete atlas is
published, its selected environment lights are excluded from runtime diffuse
but retain their front-facing live rim contribution. The combined directional
atlas has no per-light occlusion for that rim. Selected lights still illuminate
actors at runtime. Ambient cube and material texture AO remain runtime terms, applied
once; the bake includes neither ambient nor material AO. Camera movement
reuses the atlas; changes to selected colour, position or radius invalidate
it while retaining the geometry layout. Partial atlases are never published.
Neither runtime encoding supplies bounced lighting, moving actor shadows or
transformed-portal light transport. Multipage layouts require a separate
versioned decision. Completed direct or offline-bounced atlas images use the
prebake extension below.

`Compile`, `Surfaces`, `GeometryDigest`, `Validate`, `Encode` and `Decode` are
pure public Go helpers. They neither import private engine packages nor
triangulate renderer meshes. The host associates retained triangle semantic
identities with these bindings and evaluates the affine planes per vertex.

## Optional completed prebake, version 1

SDK 0.0.8 also defines `world/lightmaps-prebaked@1`, requiring
`world/lightmaps@1`, `world/lighting@1` and `world/sectors@1`. This additive
extension preserves all existing layout and runtime-recipe bytes. It packages
one completed three-tile RNM atlas without rebaking it during startup. The
existing algorithm 1 stores direct lighting only. Algorithm 2 adds
the explicit offline diffuse-bounce producer described below, while retaining
the same `direct-rnm3@1` coefficient encoding and image layout. No released
schema or algorithm-1 canonical spelling changes.

A level carries both fixed `EntryData` entries `@world/lightmaps/prebake` and
`@world/lightmaps/prebake-rnm3`, plus metadata
`kartyWorldLightmapPrebake: "karty.world-lightmap-prebake@1"`. Both entries and
the marker are mutually required, alongside the existing complete layout.
The image is QOI RGBA tagged linear (1), with raw straight RGBM channels:
never premultiply by its multiplier alpha. Width is three times the logical
page width; height is the page height. The encoded image is bounded to 8 MiB
and the existing level envelope remains bounded to 16 MiB. A valid 1024-page
image costs 12 MiB decoded; QOI data with insufficient compression can exceed
the encoded bound and must fail explicitly. This extension does not raise
level or shared decoded-texture budgets.

The algorithm-1 canonical manifest schema `karty.world-lightmap-prebake@1` has required
fields in this order: `schema`, `algorithm` (1), `encoding` (`direct-rnm3@1`),
`layout_sha256`, `bake_sha256`, `image_sha256`, `image_bytes`, `width`, `height`,
`rgbm_range`. It is bounded to 16 KiB. All digests are lowercase SHA-256.
`layout_sha256` hashes the exact canonical layout JSON, including charts,
bindings, coordinates and the ordered recipe. `bake_sha256` hashes canonical
JSON of `{schema, algorithm, layout_sha256, lights}` in that field order, with
`lights` containing the selected complete `world.PointLight` records in recipe
order. This binds IDs, position, linear colour and radius while excluding
unselected lights, ambient, actors and material AO. `rgbm_range` is the exact
derived float64 range described above, not an authoring control.
`image_sha256` and `image_bytes` identify the exact lossless QOI bytes.

`NewPrebake` derives validated algorithm-1 metadata from a completed direct image;
`EncodePrebake` emits a canonical manifest and `DecodePrebake` accepts only
that spelling. `PrebakePair.Validate` checks the complete layout, lighting
identity, dimensions, channel/colourspace tags, every manifest field and the
full bounded QOI stream before decoded image allocation. Unknown, duplicate,
missing, null, reordered or noncanonical fields fail. Validation cannot prove
lighting quality or honest producer provenance; signed/hashed level packaging
protects the final artifact. Pairs retain caller ownership of image slices;
consumers must clone them before mounted publication. The host rejects stale
packaged inputs rather than silently rebuilding a different atlas.

## Offline diffuse-bounce producer, algorithm 2

`NewOfflinePrebake` accepts a completed image and `OfflineBakeInputs`, producing
the same manifest schema with `algorithm: 2` and
`producer: "cpu-rnm3-pathtrace@1"`. Additional canonical fields follow
`rgbm_range` in this order: `producer`, `samples`, `bounces`, `seed`,
`reflectance_sha256`, `surface_sha256`. Samples are 1–256, bounces are 0–4 and
seed is an unsigned 64-bit integer. Zero bounces and zero seed are valid and
omitted from the canonical manifest; explicit zero fields are not canonical.
Worker count does not affect identity because scheduling must not change pixels.

The public [`bake/worldlightmap`](../../bake/worldlightmap/README.md) package
provides this deterministic CPU producer. It rasterizes actual receiver
polygons, uses a triangle BVH for point-light visibility and traces bounded
cosine-weighted diffuse paths. Primary direct coefficients retain the existing
Half-Lambert contract. Bounced transfer uses Lambertian cosine response and
actual nearest albedo texture pixels evaluated with compiled affine projections
and triplanar weights, decoded from sRGB to linear RGB. Reflection channels are
bounded to `MaxDiffuseReflectance = 0.95`. Normal/height/AO maps do not alter
transport in this slice; material normals still combine the three directional
coefficients at runtime. This is a Source-inspired RNM implementation with
diffuse indirect light, not an exact reproduction of Valve's bake equations.

For `b` bounces, `OfflineRGBMRange` derives the storage range
`max(1, 3 Σ max(R,G,B) * Σ(k=0..b) 0.95^k)` over selected lights. The manifest
must contain that exact float64 value. This reserves bounded indirect energy;
it does not guarantee a particular brightness. The RGBM channel, coverage,
dimensions, chart-isolated dilation, QOI and memory bounds remain the same.

`surface_sha256` is `OfflineSurfaceDigest` of the canonical compiled document
with contents and lighting removed, committing geometry, topology, material
identities and compiled UVs. `reflectance_sha256` is supplied by the public
baker's `ReflectanceDigest`, committing used decoded albedo pixels and their
semantic material/UV assignments. The CLI rechecks reflectance against source
before packaging; a host lacks original source images and can verify the
surface identity and recorded digest, but cannot independently recompute those
source pixels.

Algorithm-2 `bake_sha256` hashes canonical JSON of
`{producer, direct_sha256, surface_sha256, reflectance_sha256, samples, bounces, seed}`
in that field order. `direct_sha256` is the existing algorithm-1 bake identity
for the same layout and selected light records. The identity therefore includes
selected lighting and all offline transport controls without including ambient,
actor state or material AO. Pair validation derives and checks the entire
expected manifest and image hash; it cannot prove solver quality or honest
producer provenance.

A valid algorithm-2 atlas replaces selected static environment diffuse with
combined direct and indirect light. Live front-facing rim, actor lighting,
ambient cube and material AO retain their existing runtime behavior. The
combined atlas has no per-light rim visibility, moving shadows, spatial actor
probes or transformed-portal transport. The first CPU producer has hard point
shadows and finite-sample noise; it does not supply soft area-light sampling.
The optional denoised producers below reduce indirect grain. If no completed
bake is packaged, the original runtime direct recipe remains available. Malformed or stale packaged prebakes are errors,
rather than permission to silently use an unrelated fallback atlas.

The public `codec/qoi.Validate` utility returns bounded metadata after header
and complete stream validation without allocating decoded pixels. `Inspect`
remains header/end-marker inspection; `Decode` reuses `Validate` before its
pixel allocation. This permits preflight of a complete shared image budget
without decoding images twice.

### Versioned decision: indirect denoising producers v1

For the next Go module and host release after v0.0.8, algorithm 2 also accepts
`cpu-rnm3-pathtrace-atrous-low@1` and
`cpu-rnm3-pathtrace-atrous-medium@1`. The producer string commits the complete
filter recipe and preset; it participates in BakeSHA256. No JSON fields,
RNM encoding, coefficient count or runtime composition semantics change.
`OfflineBakeInputs.Denoise` selects the producer; empty/off preserves the
original `cpu-rnm3-pathtrace@1` manifest and pixel contracts. Unknown producers
are still rejected. Older hosts reject these new producers and require an
updated host; they must not silently reinterpret a denoised artifact as @1.
See the baker package for the filtering and memory contract.
