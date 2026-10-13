# Build-generated world lightmap layout

## Chunk receiver algorithm 2

`world/chunks@1` adds `CompileReceivers(document, options, sectors, solids)` and
layout receiver algorithm 2. Its `receivers` field contains sorted, unique global
sector and solid indexes. The complete world geometry digest and complete binding
table remain present; unselected surfaces have chart `-1`. Algorithm 1 retains its bounds, mandatory receiver coverage and one-component
bake scope under the candidate binary wire. Algorithm 2 merges charts only through selected ordinary adjacent sectors
and packs at the requested density without a global atlas.

`Compile` and `CompileReceivers` use the configured fixed page size. Finite
charts that exceed that page, or exhaust its packing capacity, return
`ErrPageCapacity` while preserving `errors.Is(err, ErrLayout)`. Invalid geometry,
options and lighting recipes return their existing errors without that capacity
identity. The chunk planner uses this distinction to try a 512 page first under
a configured 1024 maximum, keeping the same density, padding and recipe.

`ReceiverScope` identifies the full ordinary physical component used for
transport. Receivers spanning transformed spaces, or volumes touching multiple
spaces, fail explicitly. A global direct light selection is filtered to that
component; an empty resulting selection stores
covered black. No light travels through transformed portals. Chunk producers
still validate every light identifier and reject duplicate or unknown selectors.
See [`format/worldchunk`](../worldchunk/README.md) for packaging and mesh ownership.

## Environment sky receivers

Candidate `world/environment@1` sky ceilings and opaque horizon walls retain
their semantic bindings with `Chart: -1` and no receiver polygons. Upper strips
between two sky sectors behave the same way; indoor roofs and sky-to-indoor
lintels remain physical. Sky markers participate in the geometry digest, using canonical boolean bytes.
Sky palette, preset and sun direction do not change receiver layout identity.

Existing bake recipes select named point lights only. Directional sun and sky
illumination are runtime contributions; the existing offline producer does not
bake them or sky-driven indirect transport. Unsupported sun selection fails
explicitly as an unknown point light. A sky opening is never treated as a roof
or an albedo receiver. The ordinary physical-component and transformed-portal
bake restrictions still apply.

## Candidate binary wire, version 1

Layouts use `KLM1`; prebake manifests use `KPB1`. Both have a 16-byte
little-endian header: magic, uint16 version 1, uint16 zero flags, uint32 exact
payload length and uint32 zero reserved. Fields follow public Go declaration
order using the [shared typed primitives](../../internal/binwire/README.md).
Semantic schemas and receiver/bake algorithms remain separate from this binary
version. JSON has no decode fallback. The new binary layout, geometry, selected
light, offline surface and reflectance identities require rebuilt artifacts and
bake caches; sampling and image pixels are unchanged.

`LayoutDigest` checks bounded structural encoding and returns SHA-256 over the
same bytes as `Encode`; full authored validation still requires a document.
`DecodePrebakeManifest` checks the manifest without image decoding, including
hash syntax, dimensions, encoded image bounds and algorithm controls, so callers
can authorize image paths and reserve decoded storage. `DecodePrebake` additionally
checks the complete image/layout/world identity before publication.

## Layout contract

`world/lightmaps@1` is a capability available in SDK 0.0.8. The public CLI
generates `@world/lightmaps/layout` as an `EntryData` entry in the existing KLD
envelope. Metadata contains `kartyWorldLightmaps: "karty.world-lightmaps@1"`.
Cartridges require this feature together with `world/sectors@1`. Semantic world and cartridge capability versions are independent of the
candidate binary envelope. Older released SDK manifests remain unchanged.

The canonical typed binary `Layout` has schema `karty.world-lightmaps@1`, algorithm 1,
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
Default charts are not rotated, and their reserved rectangles cannot overlap.
Candidate SDK 0.0.12 adds optional build policies `Options.CapsWithinSourceRoom`
and `Options.CompactPacking`. The former limits coplanar cap merging to the same
authored room and prefab instance. The latter packs charts by area into bounded
maximal free rectangles and can turn a chart by 90 degrees. A turn updates its
right-handed tangent/bitangent basis and affine UV planes together, preserving
the surface normal, texel density and RNM interpretation. The existing complete
layout validator checks every turned receiver and nonoverlapping rectangle.
Free-rectangle scratch has at most `MaxCharts*4` records in each of two buffers
(two MiB on 64-bit platforms). Neither policy changes layout fields, page bounds,
the default compiler output or existing point-light recipes.

The geometry digest covers stable sector/solid identities, coordinates,
elevation planes, directed portal indexes and reciprocal wall indexes. Actor
state, materials, material UVs and lighting do not affect it. Geometry changes
invalidate the layout. Light movement invalidates runtime lighting rather than
the geometry layout. Consumers validate complete bounds, digest, bases, affine
maps, receiver coverage and semantic binding identities before GPU allocation.
Decoding scans the complete bounded payload before typed allocation and rejects
truncation, bad counts/presence tags, nonfinite or negative-zero primitives,
unsupported binary versions and trailing bytes.

Optional `runtime_bake` has two explicitly versioned encodings. A single
visibility recipe retains its behavior. Its typed fields are illustrated here:

```yaml
encoding: point-visibility@1
light_id: court-light
shadow_size: 512
```

An ordered direct-light recipe selects up to 4,096 unique authored IDs in the
unreleased SDK 0.0.11 candidate:

```yaml
encoding: direct-rnm3@1
light_ids: [brazier-north, brazier-south]
shadow_size: 512
```

`light_id` and `light_ids` are mutually exclusive. Visibility requires the
single nonempty ID and no list; direct RNM requires a nonnil list and no
single field. IDs must exist in `Document.Lighting.Lights`; selection order is
preserved, with no implicit first-light selection. Shadow size is 32–512.
Omission packages reusable coordinates without requesting a runtime bake.
Unknown encodings, duplicate selections and malformed binary records
fail complete validation.

Build-only `Options.AllStatic` selects all fixed point sources without a
nonzero linked emission pulse, in authored order. It is mutually exclusive with
the explicit `Light` and `Lights` selectors. `StaticLightIDs` exposes the same
validated selection to producers. `Compile` with no selector remains layout-only;
explicit `Lights: []string{}` remains an options error. An automatic selection
with no eligible sources creates an explicit empty direct RNM recipe and covered
black coefficients. `LightIDsInReceiverScope` performs the authoritative
physical-component filtering without repacking charts.

Source count does not change receiver charts, page dimensions or the three RNM
coefficient tiles. Source records are retained in the authored world and bound
by the prebake identity; they do not require runtime GPU-slot masks. The digest
input scratch is separately bounded to 8 MiB so it can hash the full source set;
the published fixed prebake manifest remains bounded to 16 KiB.

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
`world/lightmaps@1`, `world/lighting@1` and `world/sectors@1`. This semantic extension packages
one completed three-tile RNM atlas without rebaking it during startup. The
existing algorithm 1 stores direct lighting only. Algorithm 2 adds
the explicit offline diffuse-bounce producer described below, while retaining
the same `direct-rnm3@1` coefficient encoding and image layout. The candidate binary wire explicitly replaces the earlier serialized layout and
manifest bytes while preserving these lighting semantics.

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

The algorithm-1 canonical binary manifest schema `karty.world-lightmap-prebake@1` has required
fields in this order: `schema`, `algorithm` (1), `encoding` (`direct-rnm3@1`),
`layout_sha256`, `bake_sha256`, `image_sha256`, `image_bytes`, `width`, `height`,
`rgbm_range`, followed by the offline control fields (zero/empty for algorithm 1).
It is bounded to 16 KiB. All digests are lowercase SHA-256.
`layout_sha256` hashes the exact `KLM1` binary layout, including charts,
bindings, coordinates and the ordered recipe. `bake_sha256` hashes the `KDI1` binary identity containing
`schema`, `algorithm`, `layout_sha256`, and `lights` in that field order, with
`lights` containing the selected complete `world.PointLight` records in recipe
order. This binds IDs, position, linear colour and radius while excluding
unselected lights, ambient, actors and material AO. `rgbm_range` is the exact
derived float64 range described above, not an authoring control.
`image_sha256` and `image_bytes` identify the exact lossless QOI bytes.

`NewPrebake` derives validated algorithm-1 metadata from a completed direct image;
`EncodePrebake` emits a canonical manifest and `DecodePrebake` accepts only
that binary encoding. `PrebakePair.Validate` checks the complete layout, lighting
identity, dimensions, channel/colourspace tags, every manifest field and the
full bounded QOI stream before decoded image allocation. Truncated, malformed or
noncanonical binary records fail. Validation cannot prove
lighting quality or honest producer provenance; signed/hashed level packaging
protects the final artifact. Pairs retain caller ownership of image slices;
consumers must clone them before mounted publication. The host rejects stale
packaged inputs rather than silently rebuilding a different atlas.

## Offline diffuse-bounce producer, algorithm 2

`NewOfflinePrebake` accepts a completed image and `OfflineBakeInputs`, producing
the same manifest schema with `algorithm: 2` and
`producer: "cpu-rnm3-pathtrace@2"` in SDK 0.0.11. Additional canonical fields follow
`rgbm_range` in this order: `producer`, `samples`, `bounces`, `seed`,
`reflectance_sha256`, `surface_sha256`. Samples are 1–256, bounces are 0–4 and
seed is an unsigned 64-bit integer. Zero bounces and zero seed are valid fixed
binary fields.
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

For `b` bounces, producer revision 2 derives the storage range
`max(1, 3 E * Σ(k=0..b) 0.95^k)`. For each axis, a weighted interval sweep
finds the maximum sum of selected source `max(R,G,B)` weights whose radius
intervals overlap; `E` is the minimum of the three axis bounds. This is a
conservative upper bound on sphere overlap everywhere in the scene. Outward
interval and accumulation rounding prevent underestimation. The manifest
must contain that exact float64 value. Distant lights no longer reduce precision
by inflating a global range. Producer revision 1 retains its original global
`E = Σ max(R,G,B)` during validation. This reserves bounded indirect energy;
it does not guarantee a particular brightness. The RGBM channel, coverage,
dimensions, chart-isolated dilation, QOI and memory bounds remain the same.

`surface_sha256` is `OfflineSurfaceDigest` of the canonical compiled document
with contents and lighting removed, committing geometry, topology, material
identities and compiled UVs. `reflectance_sha256` is supplied by the public
baker's `ReflectanceDigestForLayout`, committing scoped used decoded albedo pixels and their
semantic material/UV assignments. The CLI rechecks reflectance against source
before packaging; a host lacks original source images and can verify the
surface identity and recorded digest, but cannot independently recompute those
source pixels.

Algorithm-2 `bake_sha256` hashes the `KOI1` binary identity containing
`producer`, `direct_sha256`, `surface_sha256`, `reflectance_sha256`, `samples`,
`bounces`, and `seed` in that field order. Producer revision 2 additionally
appends `rgbm_range`; revision 1 retains its exact old identity.
`direct_sha256` is the existing algorithm-1 bake identity
for the same layout and selected light records. The identity therefore includes
selected lighting and all offline transport controls without including ambient,
actor state or material AO. Pair validation derives and checks the entire
expected manifest and image hash; it cannot prove solver quality or honest
producer provenance.

A valid algorithm-2 atlas supplies selected static environment diffuse as
combined direct and indirect light. Consumers can remove those sources from
direct-light slots and approximate static gloss/rim from the combined field;
the format supplies no exact per-source view-dependent specular. Ambient cube
and material AO remain runtime terms. Actor lighting can sample loaded receiver
coefficients, while this format supplies no separate spatial actor probes,
per-light rim visibility, moving shadows or transformed-portal transport.
The first CPU producer has hard point
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
filter recipe and preset; it participates in BakeSHA256. No typed payload fields,
RNM encoding, coefficient count or runtime composition semantics change.
Revision-1 `OfflineBakeInputs.Denoise` selected these producers; empty/off used
the original `cpu-rnm3-pathtrace@1` manifest and pixel contracts. SDK 0.0.11 now
produces `cpu-rnm3-pathtrace-atrous-low@2` and
`cpu-rnm3-pathtrace-atrous-medium@2` with the spatial range decision above;
empty/off selects `cpu-rnm3-pathtrace@2`. All six named producers remain
independently verifiable. Unknown producers
are still rejected. Older hosts reject these new producers and require an
updated host; they must not silently reinterpret a denoised artifact as @1.
See the baker package for the filtering and memory contract.

### Versioned decision: bounded transport producers v3

SDK 0.0.11 additionally accepts `cpu-rnm3-pathtrace@3`,
`cpu-rnm3-pathtrace-atrous-low@3` and `cpu-rnm3-pathtrace-atrous-medium@3`.
`OfflineBakeInputs.MaxRayDistance` selects these producers only when finite,
positive and at most `MaxBakeRayDistance` (1,000,000 world units). Zero selects
the unchanged unbounded revision-2 producers. Every primary/bounce-hit source
contribution and diffuse transport segment uses this maximum distance. The
offline chart-local denoising guide segments keep their geometric barrier tests.

For recognized revision-3 producers only, the generated `KPB1` manifest appends
one canonical float64 `max_ray_distance` after `surface_sha256`. The preflight
scan recognizes these three exact producer strings without allocating decoded
strings, scans the required distance and validates the complete payload before
decoding. Revision-1/2 manifests retain their exact canonical bytes and identities;
they reject a trailing distance field. Unknown producer revisions are rejected.
Manifest/image bounds, algorithm 2, RNM encoding and runtime texture count remain
unchanged. Older hosts reject revision 3 rather than reinterpreting its pixels.

The revision-3 `KOI1` identity appends `rgbm_range` and then `max_ray_distance`.
Its range uses the same overlap bound as revision 2, restricted to sources whose
`min(radius,distance)` spheres intersect bounce-reachable receiver bounds. The
shared `BoundedInfluenceScope` exposes the conservative derived geometry and
source dependencies, retaining guards for floating-point/raster offsets.
`OfflineRGBMRangeBounded` is the authoritative exact float64 range helper.
The separate public baker `BakeInputDigest` allows local pixel-cache reuse;
full-world layout, selected-source and surface identities remain freshly bound
and verified when those pixels are packaged into the current level.

### Versioned decision: environment variants v1

Candidate SDK 0.0.12 adds optional `world/lightmaps-environment-variants@1`.
It requires `world/chunks@1`, `world/environment@1`, `world/lightmaps@1` and
`world/sectors@1`. Existing layout and point-light prebake formats retain their
bytes and semantics. Each render chunk uses one plain algorithm-2 receiver
layout (`RuntimeBake == nil`) and its unchanged RNM UVs for every condition.

The new `KLV1` binary catalog at `EnvironmentVariantsEntryName` uses schema
`karty.world-lightmap-environment-variants@1` and metadata marker
`kartyWorldLightmapEnvironmentVariants`. Typed codecs are generated from the
public declarations. A complete scan precedes record allocation. The catalog
is bounded to 32 KiB and 32 endpoint records, ordered by increasing clock with
clear (`CloudAmount == 0`) then overcast (`CloudAmount == 1`) for each clock.
It binds exact geometry, canonical chart layout, static surface/opacity and
used reflectance digests, producer `cpu-environment-rnm3@1`, sampling controls
and default record index. `EnvironmentSurfaceDigest` binds effective translucent
liquid opacity on static geometry because it changes opaque transport; actor-only
animations, point lights and authored ambient are excluded. Each record binds exact passive-module SHA256/size,
straight linear QOI SHA256/size, decoded bytes and the authoritative
`EnvironmentRGBMRange`. Decoded bytes are exactly `page_width*page_height*12`.

The default image is embedded at `EnvironmentImageEntryName` in the render
chunk. Every condition also has a separately staged passive module containing
only that same image entry. Its exact metadata keys are
`kartyWorldLightmapEnvironmentVariant` (the catalog schema), `layoutSha256` and
`imageSha256`. These modules are resources of the owning logical level, not
additional mounted game levels. Content addressing permits identical endpoint
bytes to share downloads. `ValidateEnvironmentImage` checks length, hash,
dimensions, colorspace and complete bounded QOI stream before pixel allocation.

`SelectEnvironmentVariants` chooses the nearest authored clock by circular
24-hour distance; ties choose the first authored clock. Weather interpolates
the selected clear/overcast endpoints continuously. Decode RGBM to linear
coefficients before interpolation and re-encode the result if uploading one
texture: interpolating encoded RGBA is incorrect. A consumer keeps the previous
verified map while fetching the next conditions and rejects stale completions.
The catalog default is clear at the authored default clock. Consumers explicitly
budget resident maps and transition scratch; 1024 RNM pixels occupy 12 MiB per
map, independent of compressed download size. Static sun/sky diffuse is replaced
only on covered receivers. Dynamic actors, point lights, authored ambient and
specular remain separate runtime contributions.
