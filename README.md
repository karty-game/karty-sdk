# Karty SDK

**The public contract between Karty tools and the game engine.**

This repository contains the cartridge and level formats used by the Karty CLI
and the private engine. Neither needs to import source from the other.

- `format/cartridge`: game metadata, embedded assets, and Wasm custom sections.
- `format/level`: versioned level envelopes and modules.
- `format/world`: canonical compiled convex-sector worlds stored as level data.
- [`format/worldlightmap`](format/worldlightmap/README.md): optional versioned
  build-generated static receiver charts, explicit visibility recipes and
  directional direct-light recipes and completed direct or diffuse-bounce prebakes.
- [`bake/worldlightmap`](bake/worldlightmap/README.md): public deterministic
  CPU offline RNM baking with ray-traced visibility and material-coloured diffuse
  reflections, without private engine or graphics dependencies.
- [`format/worldmaterial`](format/worldmaterial/README.md): versioned build-time
  albedo/material-data atlas pairs, optional complete mip tails and validated
  per-material strength controls in canonical JSON placements.
- `format/worldsource`: separately versioned room, prefab, port, and content
  authoring semantics for public compilers.
- **Releases**: SDK bundles, native hosts, browser Wasm, checksums, and signatures
  produced and validated by the private engine workflow.

Game developers use these artifacts through [Karty](https://github.com/karty-game/karty).
Engine source access and GitHub credentials are not required to download them.

```sh
mise run test
mise run build
```

Go module tags (`vVERSION`) version the format packages. Engine artifact tags
(`sdk-vVERSION`) version downloadable SDKs and hosts independently. The engine
publishes assets here; this repository does not build private engine source.

Tagged Go module releases provide immutable public format dependencies. Engine
artifact releases remain separately versioned and published by the private
engine workflow.

Compiled worlds use the `world/sectors@1` cartridge capability and the
`@world/main` level entry. `format/world` accepts only its canonical encoding and
validates all sectors, planes, reciprocal portals, identities, provenance, and
content placement before a host prepares runtime indexes. Format version 2 adds
optional typed actor transforms, sprite modes, alpha policy and bounded exact
tags while readers retain version 1 compatibility. `format/worldsource`
does not parse YAML itself; its YAML tags and validation define the high-level
schema while keeping this module dependency-free. The CLI owns decoding,
diagnostics, prefab expansion, convex decomposition, and post-expansion limits.

The unreleased `world/lighting@1` contract adds optional authored lighting to
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

[Contributing](CONTRIBUTING.md) · [MIT license](LICENSE.md)

The MIT license covers this repository’s source. Official engine runtime binaries
are covered by the separate [Karty Runtime License](RUNTIME_LICENSE.md), allowing
distribution with free and commercial games. Exported SDK bindings and templates
are MIT-licensed; private engine implementation source remains proprietary.

`karty.videos.v1` and `video/mpeg1@1` describe separate MPEG-1 files. Catalogs
contain name-sorted IDs, bounded sizes, a file digest and 128 KiB chunk digests.
Consumers derive `content/<sha256>.kvid`, verify each stored chunk, and unwrap the
streaming Karty media envelope before decoding. The video bytes are not embedded
in WASM or compressed level envelopes.

`karty.audio-streams.v1` and `audio-stream/qoa@1` describe long-form music and
environment audio stored outside the cartridge. Music entries precede
environment entries; IDs and names are canonical within each kind. Consumers
derive `content/<sha256>.kaud`, verify each 64 KiB stored chunk, and unwrap the
streaming Karty media envelope before incremental QOA decoding. Streaming audio
is limited to four hours and 512 MiB of QOA payload per entry. The
`codec/qoa` package exposes `EncodeStream`, `InspectStream`, and `StreamDecoder`
for packaging and bounded playback; the existing `Encode` and `Inspect` APIs
retain the smaller one-shot sound limits.

The unreleased SDK 0.0.8 `world/lighting@1` candidate also accepts optional
`ambient_cube` with six finite linear RGB [0,1] fields: `positive_x`, `negative_x`,
`positive_y`, `negative_y`, `positive_z`, `negative_z`. When present it replaces
`ambient`: select axis colors by the unit world normal's signs and weight them
by squared normal components, then apply material AO once. Identical six colors
match uniform ambient. This world-global artistic cube provides no spatial
probes or portal light transport. Optional `actors: true` lights world sprites
in full camera output using geometry normals; omitted/false keeps sprites unlit.
Neither optional field changes older payload encodings when absent.

The SDK 0.0.8 `world/lightmaps-prebaked@1` candidate accepts an offline
algorithm-2 producer, `cpu-rnm3-pathtrace@1`, alongside the existing direct-only
algorithm 1. The public baker traces 0–4 diffuse bounces using original albedo
textures and compiled material UVs; 1–256 samples and a deterministic seed
control quality. Its completed RGBM atlas combines direct and indirect
directional lighting, while ambient cube, material AO, actor lighting and
front-facing direct-light rim remain live runtime terms. The combined RNM atlas
does not provide per-light occlusion for that rim. Layout, selected lights,
material/UV surface identity, decoded reflectance and bake controls identify
cached results. A missing completed bake leaves the existing runtime direct
bake recipe available; invalid packaged prebakes fail validation. This is an
unreleased extension: existing algorithm-1 encodings and released contracts
remain unchanged. See the [prebake contract](format/worldlightmap/README.md).

The SDK 0.0.8 candidate adds `world/material-mapping@1`. Authoring source v5
supports optional root `uv`, room `floor_uv`, `ceiling_uv`, `wall_uv`, and edge
`uv`. Source v1–4 reject those fields; omission preserves their previous
encodings and compiler behavior. Source v5 defaults to triplanar mapping with
world anchoring, one world unit per repeat, zero offset and zero rotation.
Controls inherit field by field from root to room to edge; nil numeric pointers
and empty mode/anchor strings inherit. Explicit `rotation_degrees: 0` overrides
an inherited rotation.

`mode` accepts `triplanar`, `planar`, `wrap`; `anchor` accepts `world`, `top`,
`bottom`. `scale: {x: ..., y: ...}` gives positive world units per repeat in
[0.001, 1,000,000]. `offset` is in repeats, with finite components bounded to
±1,000,000. `rotation_degrees` is finite and bounded to ±1,000,000. Wrap and
vertical top/bottom anchors apply to walls: top uses `ceiling(x,y)-z`, bottom
uses `z-floor(x,y)`. Explicit floor/ceiling wrap or top/bottom anchors are
invalid. Inherited root wrap falls back to planar on floors/ceilings; inherited
wall anchors do not move horizontal surfaces. The compiler resolves inheritance,
projection axes, scale, offset, rotation, anchoring and prefab transforms.

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
duplicate and malformed records before publishing a world. This remains an
unreleased SDK 0.0.8 capability decision; no released SDK manifest is changed.

The unreleased SDK 0.0.8 candidate adds `world/static-solids@1`: compiled world
v3 has optional `static_solids: {version: 1, items: [...]}`, independent of room
sectors and portal connectivity. Source v6 adds optional `solids` and `contents`
to root documents and prefabs. Detail-only, sprite-only and nested room-free
prefabs expand into existing rooms without creating sectors. Older source
versions reject these fields. Omission preserves existing encoded bytes.

Each compiled `Solid` contains `id`, a strictly convex CCW `footprint` (3–64
vertices), `bottom`/`top` elevation planes, nonzero `side_material`, `top_material`
and `bottom_material` IDs, optional `side_uv`/`top_uv`/`bottom_uv`, and omitted-false
`collision`. There are at most 1,024 solids; IDs are unique UTF-8 of 1–128 bytes.
Coordinates, plane coefficients and evaluated elevations obey ±1,000,000 bounds;
edges and thickness are at least 1e-6. Overlapping solids do not relax any room or
portal validation. Collision is a blocking volume, not a walkable top. Complete
validation and canonical JSON rules apply before mounting.

The unreleased SDK 0.0.8 decision widens the static-solid count bound from 128
to 1,024 so a 1,000-pillar level can validate without adding room sectors. Source
v6 uses the same bound across root solids and prefab definitions, and public
compilers enforce it again after prefab expansion. This is a bound revision
within the existing unreleased `world/static-solids@1` candidate: compiled world
v3, source v6, capability names, payload fields and encoded layouts stay the
same. No released SDK manifest or level/cartridge version changes. Geometry,
identity, material, coordinate and encoded-size validation still applies to
every solid, including the final item.

Source material names resolve through packaged textures. Nested prefab scale,
yaw, translation and material overrides apply to solids and loose sprites. The
compiler assigns loose contents to the first containing compiled sector in
stable order, including shared boundaries and elevation. Source caps inherit
horizontal root mapping defaults; source `side_uv` is restricted to shared
planar/world X/Z mapping. Per-edge wrap and varying side triplanar weights are
not expressible by a single SideUV and are rejected. Mapped solids require the
existing `world/material-mapping@1` marker/capability; hosts also require
`world/static-solids@1`. No new guest command or level envelope version is added.


The unreleased SDK 0.0.8 lighting candidate accepts point-light
`motion: {version: 1, offset: {x: 12, y: 0, z: 0.8}, period_seconds: 12}`.
The offset and both segment endpoints obey the ordinary finite coordinate
bounds; period is finite in [0.1, 3600] seconds. Runtime position follows
`position + offset*(1-cos(2*pi*(t mod period)/period))/2`. All cameras share a
mounted-level simulation clock, reset on replacement/remount. Full surfaces and
opted-in actors receive this unshadowed direct light; diagnostic views stay
unchanged. A moving light cannot be selected in a static lightmap bake.
Omission preserves the candidate's previous static encoding. The motion record
is version 1; no released SDK or guest wire contract changes.
