# Spatial surface contract decision

Date: 2026-10-24. Accepted implementation scope: preserve local lightmap density
through explicit spatial render children, while original gameplay ownership and
one fixed sun/sky/static-light solution remain intact.

New optional capability `world/spatial-chunks@2` gates KCC5 catalog schema 4,
KLM3 selected layout schema 3 and KCM3 selected semantic mesh schema 3. Their
bounded envelopes preserve previous canonical payloads and reject trailing data.
Old schema validation is retained. Final renderer Ready scalar streams remain
version 1. No new live lighting solution or weather variant is introduced. The compact
coefficient extension below preserves the same baked RNM evaluation.

A complete root face table authorizes primary rendering, delegated spatial
rendering or terrain replacement. Selected geometry and receiver polygons share
the exact source face and clipping rectangle. Original primary collision is
retained separately through a full-source Ready collision stream. New primary
collision reservations and spatial geometry/lighting reservations participate in
admission budgets. Terrain replacement binds the original source fingerprint,
with full cell/source proof offline and immutable artifact checks at runtime.

Clipped receiver fixed producer `cpu-fixed-spatial-rnm3@1` binds the new physical
sample identity and edge filtering behavior. Shared boundary samples remain
identical at matching physical positions and sampling quality. A density change
changes reconstruction quality; coarse far coverage is an explicit build policy
and requires visual validation at the transition. Runtime does not infer uniform
lighting from a few clear-water samples.

The SDK proofs establish finite source coverage and stable data contracts. They
do not establish browser visuals, GPU upload correctness, physical phone frame
rate, or a published dependency release. Those are separate consumer gates.

Spatial horizontal liquid meshing uses a shared positive world `LiquidEdgeStep`.
Every tile edge lies on that grid, including adaptive leaf corners. The compiler
inserts canonical boundary anchors, then changes interior detail using rings and
a fan without adding boundary points. `LiquidMaxSubdivisions=1` selects the far
fan, while near leaves retain interior rings. These policies are persisted in
root and child identities; the runtime consumes final Ready vertices. Geometry
conformity and authored wave-coordinate identity are checked separately from
visual interpolation quality at a selected step and material frequency.

## Exact uniform receiver coefficients

Optional capability
`world/fixed-compact-lighting@1` requires `world/spatial-chunks@2`,
`world/fixed-lighting@1` and `world/chunk-ready@1`. A completed spatial child may
replace its whole receiver atlas with fixed manifest schema
`karty.world-fixed-lighting@2` (`KFL2`). This is a representation of the same
fixed solution, not a second illumination model. Ordinary detailed results keep
the exact previous `KFL1` payload and image format.

A compact manifest has a canonical `ChartRNM` record for every layout chart,
ordered by chart index, bounded by `MaxCharts`. Each record carries nine finite,
nonnegative float32 linear coefficients for the existing three RNM RGB lobes.
Zero coefficients mean covered black. The complete chart layout, geometry,
reflectance, surface and bake-input hashes remain bound to the result. Image
size, width and height are zero, the image digest is empty, and no image entry
is allowed. A compact spatial descriptor reserves zero atlas bytes and retains
its bounded lighting-record reservation. Source and artifact admission still
validate the exact layout and selected source ownership.

The initial producer compacts only after the full ordinary lighting bake. It
scans every covered sample in all three coefficient tiles, including interiors,
baked shadows, vegetation cutouts and bounced colors. Every covered sample in a
chart must have exactly the same decoded RGBM products as every other sample in
that chart. Different charts may retain different constants and orientations.
A missing chart, mismatched lobe coverage or any variation keeps the detailed
atlas. Uncovered atlas padding is not interpreted as covered black. There is no
four-corner heuristic, numerical tolerance or assumed clear-sky fallback.

Consumers use the same RNM basis and coefficient evaluation. A renderer may
store the bounded chart constants in a tiny coefficient table; this does not
require a full-resolution receiver image, a new vertex attribute or a distinct
lighting solution. Consumer validation must prove correct chart-index binding,
ordinary/compact parity and resident byte accounting separately. This first
representation change reduces saved/runtime data; it does not claim fewer
offline rays, because the complete bake remains the reference classification.

This decision was introduced for public Go module `v0.0.19` and engine SDK
bundle `0.0.20`. These are independent release identifiers; published
dependencies and artifacts remain immutable.

## Loading units independent of geometry cells

The decision advances the spatial capability to
`world/spatial-chunks@2`: KCC5 catalog schema 4, KLM3 selected layout schema 3 /
algorithm 4, KCM3 selected mesh schema 3, and KLS2 selector payload. These replace
the prior unreleased candidate envelopes; released non-selected formats are
unchanged. This revision belongs to the same public module `v0.0.19` and engine
bundle `0.0.20` implementation scope. No published archive is modified.

A loading unit may retain up to 64 original geometry cells, each with an exact
XY clip and original subdivision cap. Their rectangles must cover the loading
unit exactly without positive overlap. Every bound and coordinate remains
finite and on the existing shared edge grid where applicable. Root catalog,
layout and semantic mesh all commit the identical cell list. This prevents a
larger loading rectangle from silently reducing wave or other surface geometry.

The CLI's optional `load_tile_size` groups adjacent rectangles only when source
bindings, primary parent, density and original geometry policy are compatible.
It preserves source geometry cells and requested texels per world metre while
sharing chart packing, one loading identity and one mesh buffer. Existing page,
Ready, mesh, encoded and span bounds remain hard limits. Known capacity failures
retain the original accepted units; malformed source data is an error. Runtime
consumes final buffers and performs no cell tessellation.
