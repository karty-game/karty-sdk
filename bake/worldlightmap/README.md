# Offline directional lightmap baker

`bake/worldlightmap` (Go package `worldlightmapbake`) is a public, CPU-only
producer for the SDK's completed RNM prebake format. It accepts a validated
compiled world, its existing receiver layout and original albedo images. It
does not import the private engine or require OpenGL, WebGL or WASM execution.

For chunk receiver algorithm 2, pass the complete logical world and the selected
layout to the same `Bake` function. Only selected receivers allocate atlas space;
all ordinary physical neighbors remain shadow and diffuse transport geometry.
Other physical components are excluded, including those reached through
transformed portals. The producer preserves a selected space with no local
light as covered black. Tests exercise a light and occluding solid outside the
receiver chunk, and a transformed-space lighting boundary.

```go
result, err := worldlightmapbake.Bake(ctx, document, layout, materials,
    worldlightmapbake.Options{Samples: 64, Bounces: 1, Workers: 0, Seed: 1})
```

`Material` pairs the compiled numeric material ID with an `image.Image` of its
original sRGB albedo. Missing nonzero opaque-surface materials fail explicitly. The
caller must keep images and document/layout slices unchanged during the call.
Use `Result.Image` as straight-alpha RGBM, without premultiplying its alpha,
and use `Result.RGBMRange` when making an algorithm-2 offline prebake manifest.
`ReflectanceDigest` independently commits the complete world's used decoded
texture bytes and semantic material/UV assignments,
including affine projections and triplanar weights. Unused materials, actors,
ambient and material AO do not invalidate that digest. The separate prebake
manifest also commits geometry, layout, selected lights and bake controls.
`ReflectanceDigestForLayout(document, layout, materials, options)` returns exactly
the producer's current scoped reflectance provenance, including its ordinary
physical component and any bounded transport halo. Use this value to create a
fresh manifest when reusing verified cached pixels.

The baker rasterizes actual semantic polygons into chart texel centres and uses
a balanced triangle BVH for double-sided ray visibility. It includes solid
caps/sides and the opaque bands of partially open portals; door openings do not
become blockers. Chart rectangles remain isolated during coverage dilation,
and covered black samples retain nonzero alpha. Runtime recipe validation
restricts the bake to one ordinary reciprocal physical component. Transformed
portal transport is not supported.

Candidate environment worlds use the same point-light-only producer. Sky
ceilings, horizon boundaries and outdoor upper seams contain no polygons, so
they neither cast shadows nor require albedo images. Escaped transport rays
receive zero incoming radiance: this producer does not bake directional sun,
sky illumination or sky-driven bounce light. Those contributions remain live.
Changing presets has no effect on bake pixels, although the canonical offline
surface snapshot conservatively retains the environment for artifact identity.
Sun selection in a point-light recipe is rejected; transformed-portal and
ordinary physical-component restrictions remain unchanged.

Primary direct lighting retains the current squared Half-Lambert RNM contract
and squared falloff in normalized squared distance. Bounced transport instead
uses Lambertian cosine response, cosine-weighted hemisphere sampling and
linear albedo reflectance clamped per channel to 0.95. UV lookup uses nearest
original texture pixels with the compiled projections and weights. Legacy
unmapped caps use world XY and sides use edge distance/Z with one repeat per
world unit. Material normal, height and AO do not alter transport in this slice.

Each sample traces one bounded path through up to four diffuse reflections.
Selected direct lights are ray tested at each hit; reflected contributions are
accumulated into the receiver's three directional RNM coefficients. The
coefficient distribution preserves geometric neutral irradiance. No dense
surface-to-surface matrix, artistic constant fill light or ambient term is used.
Producer revision 2 reserves a conservative geometric-series energy bound over
spatially overlapping light influences. A weighted interval sweep computes the
maximum possible overlapping source energy independently along X, Y and Z; the
smallest of those three upper bounds remains conservative for every sphere.
Outward interval and accumulation rounding preserve that bound. A thousand
distant lamps therefore retain approximately the range of one lamp, while a
thousand overlapping lamps reserve their combined energy. The atlas still has
three coefficient tiles at the receiver's fixed resolution. New producer names
end in `@2` and explicitly bind their RGBM range into the offline identity;
`@1` manifests remain verifiable using their original global-sum range and exact
identity bytes. Source selection, shadows, path sampling and accumulation order
retain their existing semantics.

`Options.MaxRayDistance` defaults to zero, preserving unbounded revision-2
transport. A finite positive value at most `worldlightmap.MaxBakeRayDistance`
(1,000,000 world units) selects a distinct revision-3 producer. A source beyond
that distance contributes zero to both primary and bounce-hit lighting; sources
inside the limit keep their original radius falloff. Every diffuse bounce
segment is also limited to that distance. This intentionally truncates transport
and can reduce distant bounce light. Denoising visibility segments stay within
their receiver charts and retain their barrier tests independently of the limit.

`BoundedInfluenceScope` is the authoritative conservative dependency selection.
For `b` bounces and distance `D`, derived surface polygons overlapping receiver
bounds expanded by `(b+1)*D` remain transport geometry. Sources must reach receiver
bounds expanded by `b*D`, using influence radius `min(source.Radius,D)`. Numerical
guards cover ray-origin and raster rounding. The local source set controls RGBM
range, while the complete recipe continues to classify completed static sources;
scoping never returns an excluded steady source to runtime light slots. Bounding
boxes can overestimate dependencies, especially for large receiver charts.

`BakeInputDigest(document, layout, materials, options)` hashes the present pixel
dependencies: producer, sampling/filter controls, distance, local RGBM range,
source records, chart packing, actual derived polygons, UV/material assignments
and used decoded albedo pixels. It omits global geometry digests, unrelated
bindings and global sector/solid index offsets. Used local geometry and source
order are retained. Worker count does not affect it. Thus a cache from hundreds
of commits ago can be safely looked up by recomputing current content; a Git diff
or last-build snapshot is unnecessary. Remote edits outside the halo preserve
the key when local receiver packing remains the same. The cache must verify its
stored QOI integrity and regenerate current layout/manifest identities before
packaging. The digest identifies reusable pixels, not a mountable old artifact.
Zero distance conservatively retains the full physical-component dependencies.

Recipes accept up to 4,096 authored static sources independently of the 50 live
direct-light limit. Named-source lookup uses a bounded index. Above 32 selected
sources, a balanced influence BVH skips lights whose conservative radius boxes
do not contain the receiver or bounce hit. Its bounded bitset iterator yields
candidates in recipe order, preserving deterministic floating-point sums and
existing pixel fixtures without per-hit allocation. Smaller recipes keep the
linear walk. This accelerates spatially separated sources; physically overlapping
lights still require their actual build-time visibility work.

The sparse source benchmark runs one direct RNM query and one bounce-hit
Lambert query against the same source set, with no occluders. On Linux amd64,
AMD Ryzen 7 5700U, pinned Go 1.27.1, three 300 ms repetitions produced these
median times; every variant allocated zero bytes and zero objects per query pair:

| Sources | Linear source walk | Influence BVH |
| ------- | ------------------ | ------------- |
| 1,000   | 19,583 ns          | 395.8 ns      |
| 4,096   | 80,701 ns          | 610.7 ns      |

Command: `GOCACHE=/tmp/karty-engine-go-cache mise exec -- go test ./bake/worldlightmap -run '^$' -bench '^BenchmarkSparseSourceLighting$' -benchmem -benchtime=300ms -count=3 -timeout=9s`.
These are CPU source-query measurements, not whole-bake timings or GPU/frame-rate
predictions. The 24-source Hangar recipe keeps the small-list path.

Samples default to 16 and accept 1–256. Bounces are explicitly 0–4; zero requests
direct-only output. Workers default to GOMAXPROCS capped at 64, with an explicit
range of 1–64. Seed zero is valid. Per-texel/sample seeds make image bytes
independent of worker scheduling. Context cancellation returns an error and no
partial image; reflectance hashing and raster/path stages check cancellation regularly. Reflectance
hashing validates at most 16 million input pixels and 16,384 material records.
Receiver positions occupy only chart receiver rectangles, at 32 bytes per
reserved receiver texel. Dilation reuses scratch sized to the largest padded
chart: one distance byte and one native-int queue slot per chart texel (9 bytes
on 64-bit targets, 5 on 32-bit targets). Output remains 12 MiB for a 1024 page;
BVH/triangle geometry, row scheduling and the caller's decoded images are
accounted separately. Unused page space does not allocate receiver or dilation
scratch. No per-ray scratch allocation is needed. `mise run bench` covers 512
and 1024 pages with the same small receiver fixture, plus direct and bounced work.

This first producer uses hard point-light shadows and finite-sample diffuse
transport. Low samples can produce visible noise; use more samples for final
assets. It has no soft area-light sampling, spatial actor probes,
emissive materials or moving shadows. Its primary direct lobes preserve Karty's
current Source-inspired approximation rather than reproducing Valve's RNM
equations exactly.

Tests verify coloured reflection into a direct shadow, absence of reflected
energy from black surfaces, no leakage across a closed barrier, partial-portal
blocking/opening, UV-dependent reflected colours, the neutral direct contract,
energy bounds, worker determinism and cancellation without partial output.
Six golden pixel/reflectance hashes pin the original one-source transport
fixtures across producer revisions and internal refactors. A separate manifest
fixture preserves the exact revision-1 identity and pins revision 2 separately.

## Default material decision, version 1

This additive behavior targets the next Go module release after `v0.0.8`.
Compiled sector surfaces and material atlases already accept ID `0`; the baker
now accepts it as well. If omitted from `materials`, it resolves to a bounded
one-pixel opaque sRGB `(192,192,192)` albedo, matching the CLI's default atlas.
That fixed four-byte fallback is separate from the caller's input-pixel budget.
An explicit `Material{ID: 0, Albedo: image}` supplies the caller's default albedo
and follows the same input bounds and duplicate checks as every other material.
Missing nonzero materials still fail, including direct-only bakes.

`ReflectanceDigest` hashes the resolved default pixels only when a receiver uses
ID `0`. Thus omitted and explicitly supplied identical default pixels have the
same identity; changing a used explicit default image invalidates the bake.
Previously valid nonzero-material worlds retain identical pixels and digests.
The candidate uses canonical typed binary for reflectance assignments and bake
identities; runtime JSON has no fallback. Artifacts and bake caches must be
rebuilt. Producer identifiers, sampling, pixels and capability names retain
their existing semantics; the versioned candidate format decision owns the
serialization change.

## Indirect denoising

`Options.Denoise` accepts `off` (also the empty SDK default), `low`, or
`medium`. Low applies two spatial à-trous passes (strides 1 and 2); medium
applies three (1, 2 and 4). This is an offline pure Go filter, with no new
module or native dependencies and no runtime frame cost. Direct light is
kept separate and added after filtering. Albedo, normals and material textures
are never filtered by this pass.

The filter uses linear floating-point RNM coefficients before RGBM encoding
and gutter dilation. All three RNM lobes share weights based on indirect
colour and estimated sample-mean variance. It stays inside covered chart
texels; adjacent geometry visibility segments prevent smoothing through
solid bases and thin walls within a merged chart. Wider passes walk those
visibility edges and cannot skip coverage holes. Worker count does not
change pixels. Cancellation never publishes a partial atlas.

Additional retained storage is 76 bytes per reserved receiver texel, plus
37 bytes of scratch per concurrently processed chart texel, bounded by the
existing MaxTexels limit. Stats expose DenoiseDuration and DenoiseRays; the
latter includes the short visibility segments in total Rays. Presets are
versioned producers; off retains the original exact producer pixels.

This is a spatial estimator, not reconstruction of missing transport. It may
soften small indirect-light features, leaves some residual grain, and does
not reconcile lighting across separate charts. Use more samples for small
bright sources or difficult contacts, and compare against denoise off.

## Portable SIMD acceleration

The next Go module release after `v0.0.8` uses Go 1.27's portable `simd`
package for float64 double-sided triangle intersections. Build consumers of
`bake/worldlightmap` with `GOEXPERIMENT=simd`; the SDK, CLI and engine root
mise environments set this automatically. Remove that gate when the pinned
Go toolchain enables SIMD by default. Format-only imports need no experiment.
There is no CGo, assembly maintained by Karty, or new module dependency.

The BVH keeps precomputed triangle edges in contiguous numeric columns.
Each packet tests one ray against as many triangles as the platform's vector
width permits; partial leaves exclude neighbouring triangles and padding.
Shadow and denoiser visibility queries stop at the first blocker; diffuse paths
still find the nearest surface. Box traversal retains exact scalar division
to preserve grazing-ray classification. Float64 tolerances, unfused triangle
arithmetic, traversal tie order, sample sequences and producer identifiers stay unchanged.

Hardware selection and unsupported-platform emulation belong to Go's SIMD
package. There is one production triangle kernel; the old scalar arithmetic
exists only in tests for comparison. `mise run test-simd-emulated` forces Go's
fallback with `GODEBUG=simd=0`. Existing producer pixel hashes also run in the
actual WASM and native 386 tasks. A leaf has at most eight triangles and ray
queries allocate no heap scratch. Packed triangles retain nine float64 columns
and one native-int surface ID per triangle, plus less than one vector of column
padding; the construction triangles are released after packing.

For a same-process scalar-oracle/SIMD comparison:

```sh
mise exec -- go test ./bake/worldlightmap -run '^$' -bench BenchmarkRayTrace -benchtime=1s -count=3 -benchmem
```

Measure full `karty bake` runs separately: traversal, texture reflectance,
rasterization, denoising and output work can limit the total speedup.

### Measurement, 2026-10-06

Linux amd64, AMD Ryzen 7 5700U, pinned Go 1.27.1, default 128-bit hardware
SIMD. Three repetitions of `BenchmarkRayTrace` with `-benchtime=500ms` had
median times of 770.1 ns for the scalar nearest-hit oracle, 675.2 ns for SIMD
nearest-hit, and 660.4 ns for SIMD visibility. All report zero allocations.

An old/new/new/old CLI comparison used the same public `world-camera` sample,
8 workers, 430,772 receiver texels, and medium denoising:

```sh
karty bake --workers 8 --samples 32 --bounces 2 --denoise medium
```

| Implementation                     | Bake durations     | Mean     |
| ---------------------------------- | ------------------ | -------- |
| Previous scalar nearest-hit kernel | 21.433 s, 21.803 s | 21.618 s |
| SIMD with early-exit visibility    | 16.488 s, 16.499 s | 16.494 s |

This is a 23.7% reduction in complete CPU bake time, including denoising and
output. Both implementations issued 57,263,456 rays and produced identical
complete prebake manifests and encoded image bytes (image SHA-256
`23600e5eeeb3b99d3841f13c348de1cdc12563825ef5c9b225d2877a85db6865`).
The four-sample, one-bounce comparison also produced identical bytes; warm
runs remained roughly two seconds, and its initial scalar timing was an
outlier, so no precise draft-bake speedup is claimed.

These numbers measure this CPU and scene, not Mac/ARM performance, WASM bake
speed, browser graphics, or frame time. Native 128/256-bit SIMD and forced
emulation checks cover ray equivalence, leaf tails and original pixel hashes.
Go's default hardware selection is unchanged; the optional 256-bit check used
`GODEBUG=simd=+256` only on this AVX2-capable test machine.

## Per-bake precomputation

The baker prepares the exact hemisphere frames for surface and receiver-chart
normals once. A bounded table of at most 256 initial samples stores their radial
and normal magnitudes, azimuth increments and seed factors. Per-receiver origin
and seed setup runs outside the sample loop. Point lights retain squared radii,
and fixed squares use multiplication rather than the general power function.
The random sequence, RNM basis, light response and sampling controls stay the same.

Sampled albedos are cached by material ID for one bake. Existing `image.NRGBA`
images, including strided subimages, are borrowed unchanged. Other representations
are converted once using the existing exact decoded-pixel conversion. Texture
bounds and diffuse-reflectance lookup values are prepared ahead of sampling.
There is no texture filtering, rescaling, lighting interpolation or cross-bake
cache. Inputs remain caller owned and immutable until the bake returns; original
source pixels still determine the reflectance digest.

Only bakes with diffuse bounces allocate frames, sample tables or prepared albedos.
Frame storage is 48 bytes per surface and receiver chart, initial samples occupy
at most 8 KiB, and converted albedos add at most 4 bytes per input pixel (64 MiB
under the existing 16-million-pixel input bound). Converted textures are shared
by surfaces using the same material. Cancellation never publishes a partial bake.

### Measurement, 2026-10-06

Same Linux amd64 Ryzen 7 5700U, Go 1.27.1, 8 workers, medium denoising. Both
binaries were warmed, then measured in before/after/after/before order. The
baseline already includes the portable SIMD and early-exit visibility changes.

The setup/material changes alone reduced the full 32-sample bake by 3.5%
(16.345 s to 15.771 s). The small one-worker fixture improved about 12%, so it
was not a reliable predictor of the full scene's gain. A full-scene CPU profile
then identified ray-box interval tests at 57.6% of sampled CPU time, including
substantial general `math.Min`/`math.Max` helper overhead. The interval loop now
uses comparisons for validated finite coordinates; exact division, parallel-axis
tolerances, traversal order and hit classification are preserved. Signed-zero
interval endpoints cannot change its Boolean result.

Fresh paired measurements of the combined setup and ray-box improvements:

| Roman-room bake       | Before            | After             | Bake-time reduction |
| --------------------- | ----------------- | ----------------- | ------------------- |
| 4 samples, 1 bounce   | 1.699 / 1.675 s   | 1.455 / 1.429 s   | 14.5%               |
| 32 samples, 2 bounces | 16.308 / 16.732 s | 13.890 / 14.089 s | 15.3%               |

For the 32-sample case, average end-to-end throughput rises from 3.466 to 4.093
million rays/second, about 18.1%. Both perform 57,263,456 rays and produce the
same complete manifests and encoded atlas bytes. Preparation, denoising and
output costs are all included. These are native CPU bake measurements, not
WASM performance or frame-time results. The initial sampling and light-response
formulas, retro texture filtering, quality controls and producer IDs are unchanged.

A separate run at 36 samples and two bounces completed 64,253,080 rays in
16.040 s, including medium denoising. That is 12.5% more samples within the
previous 32-sample average budget of 16.520 s. It is one confirmation run, not a
paired quality benchmark; the authored sample defaults remain 4 samples / 1 bounce.
