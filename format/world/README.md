# Compiled world contract

Compiled worlds use the `world/sectors@1` cartridge capability and the
`@world/main` level entry. `format/world` accepts only its canonical encoding and
validates all sectors, planes, reciprocal portals, identities, provenance, and
content placement before a host prepares runtime indexes. Format version 2 adds
optional typed actor transforms, sprite modes, alpha policy and bounded exact
tags while readers retain version 1 compatibility.

See [authoring semantics](../worldsource/README.md) for compiler inputs.

## Candidate binary wire, version 1

The unreleased SDK candidate replaces compiled runtime JSON with `KWR1` typed
binary. The 16-byte little-endian header contains magic (4 bytes), binary version
`1` (uint16), zero flags (uint16), payload length (uint32) and zero reserved
(uint32). The exact payload follows public Go declaration order; semantic world
versions 1–3 retain their separate feature gates. JSON and older runtime payloads
are rejected, without a compatibility fallback.

Integers use fixed widths (`int` is signed int32), floats are finite IEEE values,
and negative zero has one canonical positive-zero encoding. Booleans and pointer
presence are bytes 0/1. Strings have uint32 byte lengths and valid UTF-8; slices
use uint32 `length+1`, reserving zero for nil and preserving an explicit empty
slice. Fixed arrays have no counts. Generated readers scan every record before
allocating typed arrays, enforcing field counts, the 8 MiB payload bound and a
32 MiB conservative allocation ceiling. Truncation, bad counts, unsupported
versions, noncanonical primitives and trailing bytes fail before publication.

The handwritten [generator](../../internal/codegen/binary/main.go) owns the typed
codecs. Run `mise exec -- go generate ./format/world` after contract edits;
regeneration coverage prevents stale generated files. Binary digests and compiled
artifacts must be rebuilt for this candidate; historical released JSON bytes are
not the current wire contract. Authoring remains TOML/YAML.

## Environment and sky openings, version 1

The next candidate SDK adds optional `world/environment@1` alongside
`world/sectors@1`, using compiled world v3 and source v9. The typed environment record has version 1 and preset `daylight`, optionally
adding `palette`, a sun with azimuth 135/elevation 40, `time_of_day` and
`cloud_amount`.
The engine candidate supplies a mount-owned live clock/cloud setter; cartridge
manifest capability gating is independent of the candidate binary envelope.
Compiled v1–2 reject the feature; absent controls retain their prior semantics.

Presets are `daylight`, `space`, `cartoon`, `fantasy`. Palette is omitted,
`default`, `violet` or `twilight`; it affects only sky colors, independently of
illumination. A supplied sun has two finite angles in degrees: azimuth `[0,360)`
from +X toward +Y and elevation `[0,90]` above XY. The fixed global sun direction
points toward the light, so incoming rays travel in the opposite direction.
Each preset fixes the default angles, light color/intensity, sky palette and
outdoor ambient cube for this capability version. `ResolveEnvironment` exposes
the bounded linear RGB defaults without heap allocations; consumers resolve
them at preparation time. Changing these defaults requires a versioned decision.

Optional `time_of_day` is finite hours in `[0,24)`; `cloud_amount` is finite
coverage in `[0,1]`. Pointer presence preserves explicit zero. Omitted time
retains the fixed preset or explicit sun; authored time and sun together are
rejected. Omitted clouds default to `0.35`. `ResolveEnvironmentSettings` accepts
a complete live clock/cloud override without allocation; `15` reproduces preset
sun defaults, `0` is midnight, `6` sunrise, `12` noon and `18` sunset. The clock
coordinates direction, direct intensity, sky/fill brightness and low-sun warmth;
night sun intensity/disk are zero. Palette selection remains independent of
illumination. Clouds change visual coverage and do not shadow geometry.

| Preset   | Default azimuth/elevation | Sun intensity | Outdoor fill         |
| -------- | ------------------------- | ------------- | -------------------- |
| daylight | 135° / 40°                | 0.72          | Cool blue            |
| space    | 135° / 25°                | 0.85          | Restrained dark blue |
| cartoon  | 135° / 50°                | 0.58          | Bright neutral blue  |
| fantasy  | 135° / 32°                | 0.68          | Muted violet         |

Sector `sky: true` is a finite ceiling mask revealing the environment rather
than a physical roof. Opaque wall `sky: true` is a finite horizon mask, valid
only in a sky sector with `portal: -1`. Every sky marker requires a root
environment. Sky masks preserve navigation bounds, collision confinement,
portal topology and semantic surface bindings. Adjacent sky sectors show the
same environment in their exposed upper wall strips even when finite ceiling
heights differ. Sky ceilings, horizon masks and these upper strips are excluded
from physical shadow casters and lightmap receivers. Floors, roofs and ordinary
walls still occlude lighting. Decorative clouds, stars and celestial bodies
do not introduce additional lights.

Outdoor fill must be restricted to sky-connected geometry and sun visibility
must respect physical occluders; the resolver's ambient cube is not a global
interior light. Renderer quality and shadow budgets are host choices. The first
version supports a live clock and static casters; moving casters, terrain
generation, vegetation and baked sun/sky transport remain separate features. Existing point-light bake recipes remain usable with open sky geometry
and never select the directional sun. See
[lightmap treatment](../worldlightmap/README.md#environment-sky-receivers).

Complete validation rejects unknown presets/palettes, bad versions and angles,
sky without an environment, horizon walls on portals and horizon walls in indoor
sectors. Binary decoding rejects malformed presence tags, incomplete records and
noncanonical primitives. Older hosts reject the new capability before
publishing resources.

## Authored lighting, version 1

The `world/lighting@1` contract adds optional authored lighting to
compiled world version 3 and authoring source version 4. The cartridge declares
both `world/sectors@1` and `world/lighting@1`. Compiled versions 1–2 and source
versions 1–3 reject lighting. Absent lighting preserves legacy rendering on every supported semantic world
version; source v4 also accepts absence. This independently versioned capability does not change the
level envelope or cartridge manifest version.

The typed record contains version 1, ambient linear RGB and a light list.
Each point light has `id`, `position`, `color` and `radius` fields. Ambient and
point colors are finite **linear RGB** intensities in `[0,1]`, represented by
`x`, `y`, `z`; they are not sRGB color bytes. Positions are finite global world
coordinates, bounded inclusively to ±1,000,000 per component, and need not fall
inside a sector. The unreleased SDK 0.0.11 candidate permits 0–4,096 authored
sources in authored order, with unique nonempty
UTF-8 IDs of at most 128 bytes. An empty list permits ambient-only lighting.
Lighting belongs to the root document, never a room, prefab or instance, and
receives no prefab transform or sector assignment. The contract supplies no
shadows, portal transport, guest lighting controls or PBR parameters.

`MaxLights` bounds authored sources; `MaxRuntimeLights = 50` separately bounds
live direct lighting. Completed prebakes collapse any selected steady sources
into three coefficient tiles and remove them from runtime direct-light slots.
An unbaked world or partial bake must still fit its remaining direct sources
within 50; raising the authored bound does not raise GPU or guest light limits.
The typed binary field layout stays unchanged, while its generated source-count
preflight bound becomes 4,096. Small light lists retain allocation-free identity
validation; larger lists use a bounded identity index rather than quadratic
comparisons. This candidate producer/bounds change belongs to SDK 0.0.11;
released manifests and the runtime command protocol remain unchanged.

The version-1 decision bounds radius inclusively to `[0.001, 1,000,000]` world
units (`world.MinLightRadius` and `world.MaxCoordinate`). The lower bound keeps
the host's float32 distance/attenuation calculation representable; arbitrary
positive float64 radii are not valid. Both source and compiled validators enforce
the same complete lighting bounds, identities and finite values. Compiled world
decoding additionally requires complete canonical binary records. A nil lights
slice and an explicit empty slice have distinct encodings and the same
ambient-only semantics. Source TOML/YAML presence and unknown-field
checks belong to the public compiler: `worldsource.Validate` validates typed
values and does not parse author text. Future semantic changes require a new
lighting capability/version.

The SDK 0.0.8 authored-lighting path uses squared Half-Lambert diffuse and
bounded point-light falloff. When a material atlas is present, tangent normals
affect diffuse shading, and AO scales ambient light only, once in linear RGB.
Direct light remains unoccluded. Lit albedo diagnostics retain unoccluded
albedo; normal and depth diagnostics keep their meanings. Material height
never replaces geometric camera depth or changes silhouettes.

Material height represents positive raised relief; zero preserves the original
UVs. The authored path uses two
fixed-point parallax shifts with bilinear height samples, bounded to 0.025 of a
texture repeat per UV component and faded at grazing angles. Albedo, normal,
height and AO share the shifted UVs. A restrained stylized rim has coefficient
0.08, height/AO masks, a fourth-power view Fresnel term and actual front-facing
direct-light contributions; it supplies no ambient glow or PBR/specular
parameters. Legacy worlds without authored lighting retain AO-baked unlit
output and ignore height. These rendering semantics introduce no new payload
fields, material-atlas schema or SDK manifest version.

## Ambient cube and actor lighting

The SDK 0.0.8 `world/lighting@1` contract accepts optional
`ambient_cube` with six finite linear RGB [0,1] fields: `positive_x`, `negative_x`,
`positive_y`, `negative_y`, `positive_z`, `negative_z`. When present it replaces
`ambient`: select axis colors by the unit world normal's signs and weight them
by squared normal components, then apply material AO once. Identical six colors
match uniform ambient. This world-global artistic cube provides no spatial
probes or portal light transport. Optional `actors: true` lights world sprites
in full camera output using geometry normals; omitted/false keeps sprites unlit.
Their absence retains the existing lighting behavior.

## Point-light motion, version 1

The SDK 0.0.8 lighting contract accepts point-light
`motion: {version: 1, offset: {x: 12, y: 0, z: 0.8}, period_seconds: 12}`.
The offset and both segment endpoints obey the ordinary finite coordinate
bounds; period is finite in [0.1, 3600] seconds. Runtime position follows
`position + offset*(1-cos(2*pi*(t mod period)/period))/2`. All cameras share a
mounted-level simulation clock, reset on replacement/remount. Full surfaces and
opted-in actors receive this unshadowed direct light; diagnostic views stay
unchanged. A moving light cannot be selected in a static lightmap bake.
Omission retains static-light semantics. The motion record
is version 1; the guest wire contract is unchanged.

## Material mapping, version 1

Compiled world v3 retains an optional `material_mapping: {version: 1}` marker.
When present, every sector `floor_uv`, `ceiling_uv` and wall `uv` is required,
including internal decomposition and portal edges. A mapping without the marker
is invalid. Compiled v1–2 reject both the marker and surface mappings. The game
cartridge declares `world/material-mapping@1` alongside `world/sectors@1`; older
hosts reject unsupported capabilities before readiness. No new guest command or
level envelope version is involved.

Each `SurfaceUV` has `projections` of length one or three and equally many
`weights`. A projection contains `u` and `v`, each an affine plane with `x`,
`y`, `z`, `offset`; the host evaluates `x*X+y*Y+z*Z+offset` at authored geometry
positions before clipping. Every coefficient and offset is finite and bounded
to ±1,000,000,000,000. Weights are finite in [0,1] and sum to one within 1e-9.
Three projections blend using constant surface weights derived by the compiler
from geometric normals; all material channels use the same projections and
weights. The runtime does not infer projection setup or authoring modes.
Compiled decoding requires complete canonical binary records before publishing a
world. The mapping capability is available in SDK 0.0.8; absent mappings retain
legacy projection semantics.

## Static solids, version 1

Compiled world v3 optionally carries `static_solids: {version: 1, items: [...]}`
with the `world/static-solids@1` capability, available in SDK 0.0.8. Solids are
independent of room sectors and portal connectivity. Omission retains worlds without static solids.

Each compiled `Solid` contains `id`, a strictly convex CCW `footprint` (3–64
vertices), `bottom`/`top` elevation planes, nonzero `side_material`, `top_material`
and `bottom_material` IDs, optional `side_uv`/`top_uv`/`bottom_uv`, and omitted-false
`collision`. There are at most 1,024 solids; IDs are unique UTF-8 of 1–128 bytes.
Coordinates, plane coefficients and evaluated elevations obey ±1,000,000 bounds;
edges and thickness are at least 1e-6. Overlapping solids do not relax any room or
portal validation. Collision is a blocking volume, not a walkable top. Complete
validation and canonical binary rules apply before mounting.

Mapped solids require the material-mapping marker and capability. There are no
new guest commands or level-envelope versions. Incompatible contract changes
require a versioned decision.

## Compiled material layers, version 1

Compiled world v3 may declare `material_layers: {"version":1}` together with
`material_mapping` and the `world/material-layers@1` cartridge capability.
Absent fields retain existing v1–v3 material behavior. Secondary descriptors on sector
floors/ceilings and walls contain `material`, `strength` in [0,1], and a complete
independent `uv` projection. Atlas tiles used as primary or secondary materials
remain opaque.

An authored wall may carry `frame_compiled: true` and `frame_regions`: the entire partition of its visible
solid spans, including main-only regions. Each convex region contains 3–4
counterclockwise `vertices` in edge-fraction/elevation coordinates (`x` is the
fraction from directed start to end, `y` is world Z), a resolved `material`, and
`coverage` (`main`, `opaque`, or `masked`). Main regions use the original wall
material/mapping and omit region `uv` and repeat controls. Frame regions supply
one UV projection and optional `repeat_u`/`repeat_v`; their material IDs select
ordinary level textures, including direct top/bottom band images. Opaque regions use one full material; masked
regions blend over the wall main using atlas alpha. A completely open portal
retains `frame_compiled: true` even with no region records, so the host can reject
edits that would create uncompiled framed wall spans. Marked empty regions are
valid only when the wall has no visible solid area. Nonempty regions implicitly
mark frame participation. Coverage never opens holes
in the wall or alters collision, geometric depth or lightmap receiver identity.

`CompileWallFrame` is the shared build-time partitioner. Its non-serialized
recipe contains resolved strip dimensions, repeats, phases, offsets and piece
IDs. `ProfileForWall` resolves affine solid spans, including transformed portal
endpoints. Horizontal strips preserve PNG row direction: top V=(top−z)/height and bottom
V=(bottom+height−z)/height, before offsets. They follow slopes, and overlapping envelopes crop
proportionally. The source-v7 builder supplies horizontal bands only; compiled
regions also retain their existing general strip/patch representation. No authored
texture selection or placement inference is required at runtime.
There are at most 252 regions per wall (7 profile intervals × 2 exposed spans ×
2 overlap branches × 3 columns × 3 rows), 4 vertices per region and 32768 regions
per world. Existing 8 MiB encoded-world and world geometry limits still apply.

Complete validation rejects nonconvex regions, invalid mappings, missing
markers, regions on decomposition edges, escaped or portal-aperture polygons,
overlapping partitions and uncovered solid area. The host must preserve the
original authored edge/receiver identity during triangulation and must not
reuse static records after a wall-height edit changes their geometry.

`MaterialIDs` defines shared atlas traversal: legacy primary references first
(sector floor, ceiling, walls, then solid side/top/bottom), followed by secondary
references in surface order and selected non-main frame regions in emitted
order, deduplicated by material ID. Builders and hosts must use this same order
and validate every referenced slot before readiness.

## Reusable animations

Compiled v3 optionally carries `animations` version 1 and requires
`world/animations@1` alongside `world/sectors@1`. Source version 8 introduces
this feature without changing compiled geometry or absent-animation behavior.
The level-wide library contains at most 64 unique named presets and 196 material
bindings. Actors can select a named preset with `animation`; material bindings
select a texture/material ID with `material`. `phase_seconds` offsets the shared
simulation clock by a finite nonnegative value up to 86400 seconds.

| Kind        | Parameters and behavior                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| ----------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `flipbook`  | `frames` (2–64 texture IDs), `fps` (0.001–120), optional `interval_seconds`. Zero interval loops continuously. A positive interval must contain the full frame sequence; after playing it, frame zero is held until the next interval.                                                                                                                                                                                                                                                                                                                                                               |
| `liquid`    | `flow` (UV/second, each component −10 to 10), `amplitude` (UV units, 0–1), `frequency` (waves/UV unit, greater than 0 up to 100), `speed` (radians/second, −100 to 100). Smooth analytic UV distortion on world materials. Optional `surface_amplitude` (world metres, 0–0.25) adds visual waves on horizontal up-facing floors and solid tops; optional `opacity` (0–1, defaults to 1) uses ordered screen-space coverage so deeper geometry remains visible; optional `pixel_size` (integer 0–32, defaults to 0) quantizes sampling to multiples of base texture texels. Collision remains static. |
| `spin`      | Unit `axis`, optional `pivot`, `speed` (nonzero radians/second, −100 to 100). Fixed sprites rotate around their local pivot.                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `oscillate` | Nonzero `offset`, `period_seconds` (0.001–86400). Fixed sprites smoothly move from their authored rest position to rest plus offset and back, using `(1-cos(2*pi*t/period))/2`.                                                                                                                                                                                                                                                                                                                                                                                                                      |

Axis, pivot and offset are actor-local before scale and orientation. Motion is
visual only: authored positions, sector ownership, collision and gameplay queries
remain unchanged. Explicit sprite `flipbook` bindings support all facing modes;
material flipbooks also affect sprites that use that texture. `liquid` bindings
apply to world materials; fixed sprites support spin and oscillate. Kind-specific
validation rejects parameters belonging to other kinds and unsupported targets.

All frames must reference packaged textures, including animation-only frames.
Material flipbook frame targets cannot select another animation, except for the
binding's own material (normally frame zero). `MaterialIDs` appends these frames
in material-binding and frame order after all existing surface references, with
stable deduplication. Actor-only frames stay in the ordinary sprite texture table
and do not consume world material slots. Baking remains static: animation does
uses the authored rest geometry and does not emit light. Horizontal surface displacement is GPU-only; other faces sharing the material remain rigid. A displacement binding needs at least one horizontal floor or solid top. `opacity: 0` is fully transparent; omitting it means opaque. Explicit null values are invalid in authored YAML. Texture pixel quantization does not reduce viewport resolution.

## Material emission

Candidate `world/emission@1` adds the optional compiled v3 `Emission` payload.
Up to 196 used primary environment/band material IDs have an eight-bit intensity,
decoded as `code * 8 / 255`, and an optional bounded cosine pulse. Depth is
[0,1], period [0.001,86400] seconds and phase [0,86400] seconds. Duplicate IDs,
frame-only/secondary-only materials, invalid versions and invalid pulse values
reject. The pulse composes independently with the existing single surface
animation. Emission uses final linear albedo and bypasses lighting/shadow/AO
attenuation; it does not supply bloom, sprite emission or automatic light
transport. Explicit colored point lights provide nearby illumination. Omitted
payloads retain legacy emission behavior. The feature requires `world/sectors@1`.

Optional material `lights` entries link existing point-light IDs to that material's
pulse envelope. Their authored colors are the peak contribution; the same
simulation phase drives surface emission and environment/actor light colors.
At most 50 unique lights may be linked, each to one material. Unknown, duplicate
or multiply owned IDs reject. A linked light with nonzero pulse depth cannot be
included in static world-lightmap bake recipes.
