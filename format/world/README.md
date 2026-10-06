# Compiled world contract

Compiled worlds use the `world/sectors@1` cartridge capability and the
`@world/main` level entry. `format/world` accepts only its canonical encoding and
validates all sectors, planes, reciprocal portals, identities, provenance, and
content placement before a host prepares runtime indexes. Format version 2 adds
optional typed actor transforms, sprite modes, alpha policy and bounded exact
tags while readers retain version 1 compatibility.

See [authoring semantics](../worldsource/README.md) for compiler inputs.

## Authored lighting, version 1

The `world/lighting@1` contract adds optional authored lighting to
compiled world version 3 and authoring source version 4. The cartridge declares
both `world/sectors@1` and `world/lighting@1`. Compiled versions 1–2 and source
versions 1–3 reject lighting. An absent `lighting` field preserves every
supported version's existing wire encoding and legacy rendering; source v4 also
accepts absence. This independently versioned capability does not change the
level envelope or cartridge manifest version.

The payload is `{"version":1,"ambient":{"x":0.1,"y":0.1,"z":0.1},"lights":[]}`.
Each point light has `id`, `position`, `color` and `radius` fields. Ambient and
point colors are finite **linear RGB** intensities in `[0,1]`, represented by
`x`, `y`, `z`; they are not sRGB color bytes. Positions are finite global world
coordinates, bounded inclusively to ±1,000,000 per component, and need not fall
inside a sector. There are 0–50 lights in authored order, with unique nonempty
UTF-8 IDs of at most 128 bytes. An empty list permits ambient-only lighting.
Lighting belongs to the root document, never a room, prefab or instance, and
receives no prefab transform or sector assignment. The contract supplies no
shadows, portal transport, guest lighting controls or PBR parameters.

The version-1 decision bounds radius inclusively to `[0.001, 1,000,000]` world
units (`world.MinLightRadius` and `world.MaxCoordinate`). The lower bound keeps
the host's float32 distance/attenuation calculation representable; arbitrary
positive float64 radii are not valid. Both source and compiled validators enforce
the same complete lighting bounds, identities and finite values. Compiled world
decoding additionally requires its canonical JSON spelling and rejects unknown,
duplicate or missing fields and explicit `"lighting":null`. A nil lights slice
encodes as `"lights":null` and has the same ambient-only semantics as an empty
array. Source JSON/YAML presence and unknown-field
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
Neither optional field changes older payload encodings when absent.

## Point-light motion, version 1

The SDK 0.0.8 lighting contract accepts point-light
`motion: {version: 1, offset: {x: 12, y: 0, z: 0.8}, period_seconds: 12}`.
The offset and both segment endpoints obey the ordinary finite coordinate
bounds; period is finite in [0.1, 3600] seconds. Runtime position follows
`position + offset*(1-cos(2*pi*(t mod period)/period))/2`. All cameras share a
mounted-level simulation clock, reset on replacement/remount. Full surfaces and
opted-in actors receive this unshadowed direct light; diagnostic views stay
unchanged. A moving light cannot be selected in a static lightmap bake.
Omission preserves the existing static encoding. The motion record
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
Compiled decoding requires canonical JSON and rejects partial, null, unknown,
duplicate and malformed records before publishing a world. This capability is available in SDK 0.0.8; absent mappings preserve older encodings.

## Static solids, version 1

Compiled world v3 optionally carries `static_solids: {version: 1, items: [...]}`
with the `world/static-solids@1` capability, available in SDK 0.0.8. Solids are
independent of room sectors and portal connectivity. Omission preserves
existing encoded bytes.

Each compiled `Solid` contains `id`, a strictly convex CCW `footprint` (3–64
vertices), `bottom`/`top` elevation planes, nonzero `side_material`, `top_material`
and `bottom_material` IDs, optional `side_uv`/`top_uv`/`bottom_uv`, and omitted-false
`collision`. There are at most 1,024 solids; IDs are unique UTF-8 of 1–128 bytes.
Coordinates, plane coefficients and evaluated elevations obey ±1,000,000 bounds;
edges and thickness are at least 1e-6. Overlapping solids do not relax any room or
portal validation. Collision is a blocking volume, not a walkable top. Complete
validation and canonical JSON rules apply before mounting.

Mapped solids require the material-mapping marker and capability. There are no
new guest commands or level-envelope versions. Incompatible contract changes
require a versioned decision.
