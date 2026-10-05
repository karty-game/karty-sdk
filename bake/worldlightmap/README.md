# Offline directional lightmap baker

`bake/worldlightmap` (Go package `worldlightmapbake`) is a public, CPU-only
producer for the SDK's completed RNM prebake format. It accepts a validated
compiled world, its existing receiver layout and original albedo images. It
does not import the private engine or require OpenGL, WebGL or WASM execution.

```go
result, err := worldlightmapbake.Bake(ctx, document, layout, materials,
    worldlightmapbake.Options{Samples: 64, Bounces: 1, Workers: 0, Seed: 1})
```

`Material` pairs the compiled numeric material ID with an `image.Image` of its
original sRGB albedo. Missing opaque-surface materials fail explicitly. The
caller must keep images and document/layout slices unchanged during the call.
Use `Result.Image` as straight-alpha RGBM, without premultiplying its alpha,
and use `Result.RGBMRange` when making an algorithm-2 offline prebake manifest.
`ReflectanceDigest` is independently callable for cached-bake invalidation.
It commits used decoded texture bytes and the semantic material/UV assignments,
including affine projections and triplanar weights. Unused materials, actors,
ambient and material AO do not invalidate that digest. The separate prebake
manifest also commits geometry, layout, selected lights and bake controls.

The baker rasterizes actual semantic polygons into chart texel centres and uses
a balanced triangle BVH for double-sided ray visibility. It includes solid
caps/sides and the opaque bands of partially open portals; door openings do not
become blockers. Chart rectangles remain isolated during coverage dilation,
and covered black samples retain nonzero alpha. Runtime recipe validation
restricts the bake to one ordinary reciprocal physical component. Transformed
portal transport is not supported.

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
The storage range reserves a conservative geometric-series energy bound.

Samples default to 16 and accept 1–256. Bounces are explicitly 0–4; zero requests
direct-only output. Workers default to GOMAXPROCS capped at 64, with an explicit
range of 1–64. Seed zero is valid. Per-texel/sample seeds make image bytes
independent of worker scheduling. Context cancellation returns an error and no
partial image; raster/path stages check cancellation regularly. Reflectance
hashing validates at most 16 million input pixels and 16,384 material records.
Scratch scales with the validated page and triangle counts, rather than pairs
of surfaces: about 32 MiB of receiver positions, 12 MiB output and 5 MiB
dilation storage for a 1024 page, plus bounded BVH/triangle geometry and the
caller's already decoded images. No per-ray scratch allocation is needed.

This first producer uses hard point-light shadows and finite-sample diffuse
transport. Low samples can produce visible noise; use more samples for final
assets. It has no soft area-light sampling, denoiser, spatial actor probes,
emissive materials or moving shadows. Its primary direct lobes preserve Karty's
current Source-inspired approximation rather than reproducing Valve's RNM
equations exactly.

Tests verify coloured reflection into a direct shadow, absence of reflected
energy from black surfaces, no leakage across a closed barrier, partial-portal
blocking/opening, UV-dependent reflected colours, the neutral direct contract,
energy bounds, worker determinism and cancellation without partial output.
