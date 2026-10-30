# Chunked world contract

`world/chunks@1` keeps one complete compiled world in the root level and stores
immutable render assets in independently fetched child KLD modules. It does not
change compiled world versions 1–3, their bytes, or the 256-sector limit. Larger
topology requires a separate versioned contract.

The root retains `@world/main`, authored contents, lighting, sector connections,
shared albedo/material atlases and `@world/chunks/catalog`. The catalog contains
global sector ownership, exact child SHA-256 and module byte size, packed mesh
scalar size, decoded straight RGBA RNM image size, bounds and conservative
candidate chunks for every sector. SDK 0.0.11 also stores `BakedLights`, the
sorted unique union of authored source indexes from completed child prebakes.
Hosts remove these sources from direct-light slots before any child is loaded.
Every removed source must be steady, inside a physical component, and covered
by baked descriptors throughout that component. Child metadata uses `kartyWorldChunk`,
`chunkID` and `geometrySha256`; root metadata uses
`kartyWorldChunks`. Child modules contain `@world/chunks/mesh` and, when enabled,
the ordinary lightmap layout, completed prebake manifest and RNM QOI entries.
Shared textures are referenced by their root material identity, without copies.

In the unreleased SDK 0.0.11 candidate, the catalog, mesh and shadow entries use
fixed binary records. Earlier candidate JSON chunk payloads are rejected and
must be rebuilt. TOML/YAML authoring and the established world, lightmap and
level-envelope metadata formats retain their existing contracts.

`world/chunk-ready@1` adds the compiler-authoritative `@world/chunks/ready`
artifact (`karty.world-chunk-ready@1`, binary magic `KCR1`). It requires
`world/chunks@1`; older SDK targets keep their existing semantic mesh path.
Ready-enabled children contain final renderer streams without a duplicate
semantic mesh entry. `CompileReady` packs already tessellated geometry offline
into material batches, vertex/normal/UV streams, static tangent bases, RNM
bindings, wave coordinates, stable surface IDs and semantic owner ranges.
Liquid vertices remain authored rest positions; animation runs in the vertex
shader. The catalog's `MeshBytes` retains its canonical `484 * triangle count`
reservation, using the final emitted triangle count; it does not become the Ready byte count.

For several chunks, call `ReadySurfaces(meshes, document)` once and pass its
canonical table through `ReadyOptions.Surfaces` to every `CompileReady` call.
`ReadyOptions.Masked` uses the world's canonical material order. A supplied
table preserves its slot identities and must contain every source surface;
missing or duplicate keys fail. Hosts retain that shared table across chunk
residency changes, without remapping vertex arrays or growing material tables
on reload. Nil `Surfaces` derives a local deterministic table for standalone
use.

`ReadyOptions.CleanupStatic` optionally subtracts covered static faces from the
final streams of one atomic streamed chunk. Only selected, co-resident owners
in one unambiguous ordinary physical component participate. Only collidable solids act as occluders: noncolliding volumes can be entered.
Packed solid facet
halfspaces define a conservative inner volume; exact coplanar sector contact
resolves same-facing solid duplicates. Opposing floor/wall contact faces on solids
remain, preserving exterior views through backface-culled sector surfaces.
Sector surfaces win same-facing duplicates,
then lower authored solid indexes. This defines stable material ownership for
previously ambiguous depth ties. Masked, liquid, animated and sky surfaces do
not participate; actors never act as permanent occluders. Ambiguous scopes or
unreliable numerical cuts retain their complete original surfaces. Positive
gaps and nonresident occluders cannot create permanent holes.

Cleanup leaves the canonical semantic mesh and its digest unchanged. Full offline
validation deterministically reconstructs the cuts and compares every output
attribute. `ReadyBatch.Sources` indexes that derived triangle sequence, and the
catalog reserves `484 *` its length. New vertices interpolate the original UV,
wave, band and RNM streams with their existing chart ownership. Fragmentation can
increase triangle count, so builders enforce the configured budget after cleanup
and repartition when possible; `ErrReadyCapacity` identifies format capacity
limits. Hidden lightmap receivers and their bake allocation remain unchanged.
Legacy output and non-streamed movable sector geometry use their existing path.

The asset pipeline proves source geometry and authored attributes before
publishing a child and its catalog hash. Builders validate the semantic mesh
and accepted RNM binding, then use `DecodeReady`/`ValidateReady` to check exact
semantic-mesh digest, complete source coverage, packed attributes, material
classifications, UV/basis bindings and displacement-expanded owner bounds.
These complete offline proof APIs remain available and retain their checks.
The `MeshSHA256` header records build provenance; it does not require retaining
or decoding the source mesh in the runtime artifact.

After verifying the complete child's exact catalog length and SHA-256, hosts
call `DecodeReadyArtifact` with `ReadyAdmission`: parent material-slot count,
sorted sector/solid owners, expected triangle count (`MeshBytes / 484`),
lightmap presence and an optional already accepted canonical surface table.
This checks bounded binary records and allocation counts, complete consumption,
finite scalars, material/index/stream references, consistent per-triangle
surface IDs, batch order/uniqueness, owner identity/range partitions and finite
ordered owner bounds. It accepts compiler-authoritative geometry without
repeating polygon coverage, triangulation, authored UV/normal reconstruction
or vertex/bounds comparisons. Input descriptor values alone do not establish
artifact integrity or producer correctness: the caller must perform the
whole-child hash/size verification against its accepted parent first.

Runtime asset integrity is relative to the accepted root catalog; this does not
introduce publisher signatures or claim arbitrary input was built correctly.
The complete build proof belongs to the pipeline, while runtime structural and
resource limits remain mandatory. Ready scalar data is bounded to 64 MiB, with
the existing triangle, vertex, encoded-entry and child-module limits remaining
in force. Image/layout dimensions and decoded resource bounds retain their
own admission. This unpublished candidate changes no existing semantic mesh,
catalog, shadow or compiled-world binary version.

Before instantiating a downloaded child, hosts call
`level.ValidatePassiveModule`. It accepts only exact `level.WrapModule` output,
including its bounded initial memory, three constant ABI functions and one
validated data envelope. Extra imports, start functions, loops, memory growth,
custom sections and noncanonical encodings are rejected without executing Wasm.
Input remains bounded to 32 MiB. Hosts reserve the input plus the bounded
canonical-module and data-construction buffers during this check. Legacy level
execution retains its existing contract.

`Plan` grows connected groups deterministically in sector order through ordinary
coincident reciprocal portals. It never grows across a transformed or one-way
edge. Such an edge's endpoints can still share a chunk when an ordinary route
connects them. Each proposal must fit the declared mesh scalar budget and the
requested lightmap density, page capacity and padding. Defaults are 4 MiB mesh,
8 MiB encoded mesh, 256 sectors, and existing lightmap defaults. `MaxSectors` is
a secondary limit. A single oversized sector fails with its identity; the
planner never lowers density. Encoded mesh entries retain the 8 MiB level entry
bound. Builders must additionally check the complete baked child module against
the encoded budget and the existing 16 MiB envelope / 32 MiB module bounds.
`NoLightmap` supports mesh-only levels without inventing a lighting recipe.
Its planner prepares surfaces and physical-component junctions once, emits each
sector and solid once, and tests growing groups with exact final scalar and
binary byte counts. Liquid subdivision factors still come from the complete
physical patch. Final meshes retain the standalone compiler's bytes and full
semantic/coverage validation; preparation does not require the complete world's
geometry to fit a single child mesh. This path performs no bake and creates no
lightmap layout, image or recipe.

For chunks, `Options.Lightmap.PageSize` is the maximum page size. A configured
1024 maximum (also the default) first attempts a 512 page for each selected
receiver group at the same density and padding. It retries 1024 only when those
charts exceed page capacity. A configured 512 maximum remains a hard limit.
The chosen layout defines the mesh's RNM UVs and exact `LightmapBytes`; the full
geometry digest, global bindings, component lighting context and recipe are
retained. Legacy `worldlightmap.Compile` and direct `CompileReceivers` keep fixed
page sizes. `worldlightmap.ErrPageCapacity` also wraps `ErrLayout`; malformed
geometry, options and recipes never trigger the larger-page retry.

`Options.SectorGroups` optionally preserves authored areas during build-time
grouping. A nonnil list must assign every global sector an integer group ID in
`[0,256)`. The planner grows only through ordinary adjacency within the seed's
group; disconnected areas or hard budget limits still produce separate chunks.
Nil retains the original budget-only partitioning. Groups are producer inputs
and do not add catalog, child or runtime fields. They affect mesh ownership, while
visibility candidates and lighting transport still follow the complete topology
and physical component across authored-area boundaries.

Candidate SDK 0.0.19 adds opt-in `Options.EnclosureAware`, enabled by the
CLI for ordinary chunk builds. The planner first tries complete inferred
spaces, keeps internal splits ahead of doorway crossings, and prefers wider
shared openings when budgets force subdivision. Coincident reciprocal portals
with identical floor/ceiling planes connect convex pieces of the same authored
room/instance when both edges have no authored source edge. Other coplanar,
full-height indoor openings merge only when their width is at least three
quarters of the smaller cell's square-root area. Narrow openings, height
changes, transformed portals and explicit sector-group boundaries remain cuts.
This conservative heuristic is not a room detector or a cubemap-volume proof.

`Options.MaxSpan` optionally limits the X and Y extent of rendered geometry in
metres. The CLI defaults to 48; `[chunks] max_span = 32` selects a tighter cap.
SDK zero disables the limit for compatibility. Limits must be finite and
nonnegative (CLI authoring requires positive values). Span includes owned solids
and applies to mesh-only growth and whole-chunk lightmap compaction as well as
lightmapped plans. A single oversized sector fails with its identity and an
instruction to subdivide geometry or use spatial render chunks. The cap never
reduces lightmap density. Spatial tile sizes must fit the configured level limit; outdoor levels can
raise it explicitly. The CLI also checks cropped terrain before baking.

Each accepted proposal still passes the same exact sector, mesh, encoded and
lightmap limits. A complete neighboring enclosure that cannot fit is left for
a later chunk instead of partially filling the current chunk. Large enclosures
can span multiple chunks. `PlanResult.Enclosures` preserves their canonical
lowest-sector identity across these budget splits and subsequent whole-chunk
lightmap compaction. It is build-only metadata, independent of chunk IDs; no
catalog or runtime probe ownership changes. Disabled options retain authored-order
planning and a nil enclosure map. See the
[versioned policy decision](../../docs/enclosure-planning-v1.md).

Candidate SDK 0.0.12 adds `Options.CompactLightmaps`, an optional build policy
used by environment variant cartridges. After ordinary greedy planning, it
tries exact page merges of complete receiver groups within one ordinary physical
component, ordered by occupied chart area. Each accepted merge retains the
sector, decoded/encoded mesh, page, texel and complete mesh-validation bounds;
the total decoded lightmap reservation cannot increase. Authored sector groups
remain boundaries. Resulting sector/chunk indexes and visibility candidates are
rebuilt deterministically. The default planner produces the same output.

The candidate accepts nonadjacent receiver sectors connected through intervening
ordinary sectors assigned to other chunks. Both mesh and catalog validators
traverse the complete physical component with buffers bounded by the validated
world sector count. Every selected sector must belong to that component;
transformed portals and disconnected spaces remain rejected. This widens
receiver-group validation without changing wire fields. Produced environment
cartridges require `world/lightmaps-environment-variants@1`, so older hosts
reject the unsupported feature before mounting these resources. See the CLI's
versioned environment-lightmap-variants-v1 decision for this producer policy.

Candidate sets are **directed graph reachability**, including transformed links.
They are a conservative worst-case visibility superset, not geometric portal
PVS pruning. A connected map's candidate set can include every chunk, but it is
an optional prefetch hint and does not require those assets to remain resident.
Required residency follows actual runtime portal traversal and aperture clipping.
An open view can still expose every chunk at once, so this contract does not
promise bounded residency for an open world. Future producer versions can
introduce conservative aperture pruning. Candidate storage is bounded by the
existing sector bound. A runtime retains the complete topology and expands
actual portal paths with accumulated transforms and separate apertures; a chunk
is fetched once but can appear in several render views.

Mesh schema `karty.world-chunk-mesh@1` contains final float32 authored positions,
primary/frame/secondary UVs, RNM UVs, wave UVs, uint32 triangle indices and
per-triangle semantic/material bases and global owner identities. It uses a
center fan after inserting all collinear physical-component junctions, so
separately built chunks retain the same boundary vertices. Host preparation may
pack existing triangles for its GPU, but must never triangulate polygons or
tessellate loaded geometry. Winding uses the stored float32 positions, and fan
triangles with exactly zero stored area are omitted. Complete receiver coverage
is still validated. Bounds are exact minima/maxima of stored positions.
`DecodedMeshBytes` counts 84 bytes per vertex, 4 per index and 220 fixed scalar
bytes per triangle; it excludes the binary header/ownership lists, Go object
padding and a host's GPU representation. Hosts must account for their upload
representation separately.
`LightmapBytes` is zero or exactly 3 times page width times page height times 4;
RGBA16F GPU storage requires twice that image size in addition to CPU pixels.

Candidate SDK 0.0.12 retains finite upper sky masks between outdoor portal
sectors with different ceiling heights, matching the ordinary renderer. Both
plain and compiled-frame upper spans carry `Sky=true` and `Chart=-1`; physical
lower spans and indoor lintels remain opaque receivers. Complete mesh validation
requires the sky-mask coverage and correct flags, rejecting missing or mistagged
upper spans. These render masks remain absent from lightmap transport and shadow
casters, so existing layout and condition pixels retain their identities. The
CLI complete-chunk cache revision 3 invalidates earlier mesh containers; pixel
cache identities and released manifests remain unchanged.
Hosts reserve both before decoding, rather than estimating from compressed QOI.
The mesh also binds materials and authored projections with `surface_sha256`,
using the public offline surface digest independently of its topology digest.
Bound displaced liquids additionally hash the
`karty.world-chunk-liquid-tessellation@1` domain, that digest, and effective
material/frequency/surface-amplitude inputs sorted by material. Runtime timing,
opacity and shading-only wave controls do not change the rest mesh identity.

Every decoder checks the exact binary version, length, reserved zeros, scalar
enums/flags, full document identity, complete owner
lists, all final indices and attributes, triangle winding, bounds and complete
receiver area coverage before publication. `ValidateMeshBinding` additionally
checks every semantic chart and affine RNM coordinate against the child's
layout. Catalog validation recomputes the conservative reachability sets and
rejects omissions. Chunk identities never create new gameplay actor identities.

`ValidateBakedSelection(catalog, chunkID, document, layout)` binds a completed
child recipe to exactly the parent's source union filtered to the child's
ordinary physical component. A mere subset is rejected because removing a source
without its coefficients would lose lighting. Recipe ordering remains part of
the existing prebake digest; membership does not impose a different order.
Transformed portals remain physical lighting boundaries. Mesh-only descriptors
accept no lightmap layout.

Mesh validation also checks vertices against their authored receiver planes and
domains. Supplied triangle edges cancel internally; their remaining directed
boundary must cover every authored boundary interval once, including separate
wall strips. Equal-area duplicate triangles, overlaps and holes are rejected
without reconstructing or subdividing triangles. Authored projection bases,
secondary weights and frame metadata are checked exactly; primary, secondary,
frame and wave UVs are checked against their affine projections with bounded
float32 packing/interpolation tolerance applied independently per coordinate.
A large authored X never relaxes a precise Z plane. Unused attribute slots remain
zero.

All array counts and the exact complete byte length are checked before typed
slice allocation, using bounded integer arithmetic on native, 386 and Wasm.
There is no JSON parsing or re-encoding for these payloads. On `js/wasm`, fixed
record decoding and mesh validation periodically sleep for one millisecond to
return control to JavaScript; native execution does not sleep. Actual Wasm
tests check JavaScript timer progress; they do not measure graphics frame rate.

## Binary records

Every integer and IEEE-754 float is little-endian. Hashes contain 32 raw SHA-256
bytes. Public schema strings are implied by the magic and version, rather than
stored as strings. All payloads remain bounded to 8 MiB, and trailing bytes are
rejected. Surface enums are `1=sector-floor`, `2=sector-ceiling`, `3=sector-wall`,
`4=solid-top`, `5=solid-bottom`, `6=solid-side`; other values are invalid.
Boolean words must be exactly zero or one. Mapping counts and coverage occupy
uint32 words and are checked before conversion to their public uint8 fields.
All floating-point attributes must be finite and pass their semantic checks.

The catalog has a 64-byte header:

| Offset | Type     | Value                     |
| ------ | -------- | ------------------------- |
| 0      | 4 bytes  | `KCC2`                    |
| 4      | uint16   | version `2`               |
| 6      | uint16   | header size `64`          |
| 8      | uint32   | chunk count               |
| 12     | uint32   | global sector count       |
| 16     | uint32   | total candidate chunk IDs |
| 20     | uint32   | baked source index count  |
| 24     | 32 bytes | geometry SHA-256          |
| 56     | uint64   | reserved zero             |

All 120-byte chunk descriptors follow the header. Each stores ID, sector count,
solid count and reserved zero as four uint32 words; child SHA-256 at offset 16;
module size, decoded mesh bytes and decoded lightmap bytes as uint64 at offsets
48, 56 and 64; and six float64 bounds at offset 72. The remaining payload is,
in order: every chunk's sector int32 list, every chunk's solid int32 list,
the global sector-to-chunk uint32 list, then one candidate list per global
sector. Each candidate list is a uint32 count followed by uint32 chunk IDs.
The final array contains sorted unique int32 baked source indexes, bounded to
4,096 entries and to the root world's source count. Version-1 `KCC1` catalogs
are rejected and must be rebuilt; `KCM1` meshes and `KCS1` shadow proxies retain
their version-1 bytes. The complete baked-index list is scanned before any
catalog slice allocation, including its last record.
Descriptor ownership totals and candidate totals must fit their public bounds
and match the complete payload length before allocation.

The mesh has a 144-byte header:

| Offset | Type      | Value                                         |
| ------ | --------- | --------------------------------------------- |
| 0      | 4 bytes   | `KCM1`                                        |
| 4      | uint16    | version `1`                                   |
| 6      | uint16    | header size `144`                             |
| 8      | 5 uint32  | sector, solid, vertex, index, triangle counts |
| 28     | uint32    | reserved zero                                 |
| 32     | 32 bytes  | geometry SHA-256                              |
| 64     | 32 bytes  | surface SHA-256                               |
| 96     | 6 float64 | bounds                                        |

The body stores sector int32 IDs, solid int32 IDs, 84-byte vertex records,
uint32 indices and 220-byte triangle records. A vertex contains 21 float32
values in this order: position (3), primary UVs (6), band UV (2), secondary
UVs (6), lightmap UV (2), wave UV (2). A triangle stores fields in public struct
order: sector/solid int32, surface uint32 enum, edge int32; normal/tangent/
bitangent (9 float32); material and mapping count uint32; weights (3 float32);
three projection bases (18 float32); band material/coverage/repeatU/repeatV
uint32 and basis (6 float32); secondary material uint32, strength float32,
mapping count uint32 and weights (3 float32); sky/outdoor uint32 and chart int32.
Each basis stores tangent followed by bitangent. Index and vertex counts must
both equal three times the triangle count.

The shadow header is 80 bytes: `KCS1`, uint16 version `1`, uint16 header size
`80`, uint32 triangle count, uint32 reserved zero, geometry SHA-256 at offset
16 and surface SHA-256 at offset 48. Each 68-byte triangle stores positions
(9 float32), normal (3 float32), material uint32, sector/solid int32, surface
uint32 enum and edge int32. Empty shadow arrays are valid when the world has
no opaque caster geometry; mesh chunks remain nonempty.

Lightmap receiver algorithm 2 selects global sectors and solids while retaining
the entire ordinary physical component for shadow and diffuse transport. Charts
merge only through selected adjacent sectors. Lights in other physical spaces
are excluded; a selected space without a local light stores covered black.
Transformed portals are explicit lighting boundaries, without light transport
through them. Chunk lightmaps preserve authored RNM bases across portal views.
Volumes touching multiple physical spaces are rejected because authored XY
overlap cannot establish their physical-space ownership.

Static frames, secondary materials, sky masks and material animations preserve
their public semantics. Displaced horizontal upward liquid faces are tessellated
offline with `karty.world-chunk-liquid-tessellation@1`. The two authored sine
waves choose a barycentric subdivision factor from their frequency, amplitude
and primary UV spans, targeting 0.005 metres interpolation error where the patch
budget permits. The factor is capped at 32. A connected subtle-wave patch has a
256-triangle target; amplitude above 0.05 metres raises the target with amplitude
and the square root of frequency, capped at 1024. Existing source triangles are
always retained, even if their count already exceeds that target. The final
chunk's 65536-triangle limit and all decoded/encoded budgets remain hard bounds.

The compiler plans the complete ordinary physical component's liquid patches
before selecting chunk owners. Shared edges use one factor and canonical
endpoint interpolation, so independently compiled neighboring chunks have
matching boundary samples. Disconnected pools refine independently. Every
material, secondary, band, wave and RNM UV coordinate is interpolated with the
rest positions; semantic owners and differential bases remain unchanged. Hosts
only pack these final triangles. They never run the tessellator. Validation
checks authored rest elevations, affine wave coordinates, complete rest area
and every affine RNM chart coordinate without generating geometry.

New chunked roots normally also contain `@world/chunks/shadow`, schema
`karty.world-chunk-shadow@1`. This immutable asset holds the entire world's
opaque authored rest caster geometry independently of render-chunk residency.
It uses a corner fan without material seam junctions, center vertices or liquid
subdivision, retaining the ordinary portal-opening profiles and static solid
faces. Sky, liquid materials with opacity below one and gameplay actors are
excluded. Wall-frame material bands retain the underlying wall/opening caster
shape, matching the existing shadow path.

The proxy has at most 8192 triangles, each with final float32 positions, normal,
material and global semantic owner. Compilation returns `ErrShadowBudget` above
that bound. Builders may omit this optional asset only for that typed budget
condition, preserving the larger render-chunk geometry bound. A host must reject
requested shadows when the proxy is absent; missing proxy data never silently
enables partial casters. `DecodedShadowBytes` counts 68 scalar bytes per triangle;
hosts separately
account for their owned object representation and GPU position/normal/index
buffers. The entry remains bounded to 8 MiB before decoding, and the decoder
checks the triangle record count before allocating owned records. It verifies
source digests and the complete canonical authored polygon-corner sequence
without creating a mesh. The surface digest binds the ordinary offline surface
identity and sorted effective transparent-liquid material IDs. Older candidate
roots without the entry remain valid with the shadow path disabled. A proxy
does not extend shadow topology support: transformed/directed portal spaces
and multiple ordinary components still need a separately versioned shadow
contract.

### Optional solid receiver ownership, version 1

Candidate SDK 0.0.12 adds build-only `Options.SolidOwners`. Nil retains the
existing first-overlap policy. An explicit list must contain one valid sector
index per static solid; each chosen sector must conservatively intersect that
solid in XY and belong to its unambiguous ordinary physical component. Missing,
out-of-range, nonintersecting and ambiguous assignments fail before planning.
Geometry, density, page limits and chunk wire formats remain unchanged.

`BalancedSolidOwners(document, lightmapOptions)` supplies an optional
deterministic policy for buildings with several overlapping roofs, cornices and
decorations. It considers largest receiver chart sets first, then tries exact
packing into eligible sectors ordered by occupied texels. Every single-sector
layout must fit the requested page; inability to place a solid is an explicit
page-capacity error. It never lowers density or moves geometry. The CLI enables
this policy for environment lightmap variants so large decorative structures
do not all consume the first observation sector's receiver page.

Within this opt-in policy, narrow grounded solids then prefer the first
exact-fitting overlapping sector in authored order. Eligibility uses geometry
and the current receiver floor: longest XY extent at least 2 units and four
times the shortest, shortest extent at most 0.6, height at most 1.25, top at most
1 above that floor, and bottom reaching the floor or buried. IDs and material
names have no effect. Other balanced owners remain unchanged. Complete owner
validation still rejects disconnected or transformed physical membership; if
the concentration cannot fit, the original balanced list is retained. This is
a bounded candidate 0.0.12 build heuristic, without a wire or default `Plan`
ownership change.

### Spatial surface streaming, version 2

`world/spatial-chunks@2` gates catalog schema `karty.world-chunks@4` (`KCC5`).
It requires chunks, Ready and fixed lighting. Existing KCC2/KCC3 catalogs retain
all existing complete-owner, page-size and sector-count validation.

The new catalog keeps primary `Chunks`, `SectorChunks` and `Candidates` unchanged.
Its separate `Spatial` list contains at most 4096 render leaves. A leaf retains
its source face bindings, primary `Parent`, canonical XY `Tile`, conservative
XYZ bounds, artifact hash/size and exact geometry/lighting reservations. Its
local ID is its list index; `StreamChunk` exposes primary IDs followed by
spatial IDs for bounded host scheduling without changing typed wire ownership.
`MaxDistance=0` imposes no extra distance cap. Builders must provide complete
far coverage within residency budgets, rather than hiding unstreamed ocean.

`Surfaces` accounts for every discovered source face in exact semantic order.
Each face is `primary`, `spatial` or `terrain`; `Owner` always names the original
primary chunk. Spatial leaf bindings must name delegated faces of their Parent.
Coverage validation clips each convex source polygon against its assigned tile
rectangles, rejects overlapping source interiors, and proves complete area.
Tile count, aggregate face references (65536), and intersection proof work
(4 million comparisons) are bounded independently of the 256 primary sectors.

`world-lightmaps@3` / algorithm 4 (`KLM3`) and `world-chunk-mesh@3` (`KCM3`)
carry the same explicit source-face selection and optional tile rectangle.
Clipping happens before chart packing and final geometry emission. Source UV,
wave phase, normal and material identities stay in authored coordinates.
Density is never reduced implicitly. Selected atlases additionally allow square
32, 64, 128 and 256 pages. Explicit low-density far tiles are a quality policy;
these small pages do not claim that lighting is uniform or discard directional
RNM information. Exact uniform per-chart coefficients may use the compact fixed-lighting contract; varying charts retain the full atlas.

Every new primary requires `CollisionBytes` and a separately compiled
`@world/chunks/collision` Ready stream from its original complete source owners.
The reservation uses canonical 484 bytes per source triangle, independently of
its render reservation. This retains collision when a collidable seabed or wall
is delegated for rendering. Spatial children have no collision stream, fluid
volume or actor probe ownership. The first implementation rejects empty render
meshes; sky-only primaries can retain their sky geometry with an empty-chart
32-square lighting page.

Terrain-replaced floor ownership requires a root `TerrainSHA256` fingerprint.
`ValidateSpatialTerrain` proves exact exterior-sector ownership, world-compatible
active terrain cells and the full encoded source fingerprint offline. Runtime
admission compares immutable terrain-child lighting fingerprints with that root;
it does not load full terrain to repeat the source proof.

Spatial water leaves share one positive `LiquidEdgeStep`, in world metres.
Their clipping rectangles must lie on that global XY grid. The compiler inserts
all grid crossings on every boundary, including coarse edges opposite several
small leaves. Source heights and material/wave coordinates are evaluated from
one authored plane and packed physical position. Interior rings and a centre
fan preserve those boundary anchors; ordinary uniform triangle subdivision is
not applied afterwards. A near/far split therefore retains the same animated
boundary segments even when its interior meshes differ.

`LiquidMaxSubdivisions` is an explicit geometric quality policy. With a common
edge grid, 1 retains only the outer fan; larger values allow up to five interior
rings. Zero retains the normal near-detail policy. The edge step and interior
policy are included in both selected layout and semantic mesh identities and
are checked against the root leaf record. Their finite work/triangle bounds
reject an excessively fine grid. A shared grid proves conformity, not an
interpolation-error bound for arbitrary authored wave frequencies; builders
must choose appropriate steps and validate the selected material visually.

The version-2 spatial contract permits larger loading units to retain up to 64
original geometry clips in `LiquidTiles`. The catalog and selected child carry
the identical bounded list. Its complete, disjoint rectangular coverage is
validated independently of the existing full-source ownership proof. The mesh
compiler uses every original clip and subdivision policy before emitting the
combined Ready buffers, so loading-unit size is independent of geometric detail.
Ordinary released non-selected layouts, meshes and catalogs retain their bytes.
