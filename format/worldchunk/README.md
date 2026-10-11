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
