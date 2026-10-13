# World authoring semantics

`worldsource` defines typed room, prefab, port, content and material authoring
semantics. It has no YAML implementation dependency: YAML tags describe field names,
and `Validate` checks typed values. Public compilers own text parsing, presence
and unknown-field checks, diagnostics, prefab expansion and convex decomposition.
They must enforce compiled-world bounds again after expansion. Runtime format
types have no JSON tags or custom JSON methods; source YAML vocabulary and
TOML asset controls remain explicit and independently checked.

The Karty CLI generates local YAML editor schemas with `karty schema`. Run it
from a game project to include that level's material, texture and prefab choices,
or from this repository root to generate the shared base referenced by the
standalone source fixture. The generated `.karty/schemas` directory is ignored;
format validation and builds do not require the CLI or a YAML parser here.

| Source version | Added semantics                                           |
| -------------- | --------------------------------------------------------- |
| 1              | Rooms, prefabs and ordinary connections                   |
| 2              | Typed actors and instance tags                            |
| 3              | Directed and transformed connections                      |
| 4              | Root authored lighting                                    |
| 5              | Material UV mapping                                       |
| 6              | Static solids, root/prefab contents and room-free prefabs |
| 7              | Top/bottom wall bands and secondary colour textures       |
| 8              | Reusable actor and material animations                    |
| 9              | Environment presets, sky ceilings and horizon boundaries  |
| 10             | GPU sprite particle presets and transformed emitters      |
| 11             | Solid prefab renderers and environment backdrop choices   |
| 12             | Project-owned sky textures and presentation settings      |

Versions 1–12 remain supported. Optional fields must respect their version gates;
omitting new fields preserves older encodings. See the [compiled world
contract](../world/README.md) for output validation and lighting bounds. Root
lighting remains in global coordinates and is never transformed by a prefab.

The unreleased SDK 0.0.11 candidate separates the 4,096 authored-source bound
(`worldsource.MaxLights`) from the 50 live direct-light budget
(`world.MaxRuntimeLights`). Offline build policy may automatically select every
fixed light without a nonzero linked emission pulse; motion and pulsing sources
remain runtime lights. Authoring and compiled validation accept the complete
source set, while producers check the remaining live budget when packaging a
world. An unbaked preview still uses the runtime limit.

## Environment and outdoor geometry, source version 9

Source v9 adds an optional root environment and room/edge sky markers:

```yaml
version: 9
environment:
  preset: fantasy
  palette: violet # optional; omitted means the preset's default sky
  sun: # optional; omission uses the preset direction
    azimuth: 135
    elevation: 40
rooms:
  - id: courtyard
    sky: true
    # Ordinary finite geometry and material fields remain required.
```

`preset` is `daylight`, `space`, `cartoon` or `fantasy`. `palette` is omitted,
`default`, `violet` or `twilight`; it changes sky colors without changing light
colors or intensity. A supplied `sun` requires both finite angles in degrees:
azimuth `[0,360)` measured from +X toward +Y, and elevation `[0,90]` above the
XY plane. Environment belongs to the root, with no prefab transform. A preset
alone supplies bounded sky, sun and outdoor fill defaults.

Optional `time_of_day` supplies finite hours `[0,24)` (midnight 0, sunrise 6,
noon 12, sunset 18); optional `cloud_amount` supplies finite coverage `[0,1]`.
Omitted time preserves fixed sun defaults or explicit angles; providing both
time and sun is rejected. Omitted clouds resolve to `0.35`; explicit zero is
clear. Compilers preserve pointer presence for these optional controls. The
engine live setter can subsequently override the clock/clouds for every camera
in a mounted world while retaining authored preset/palette choices.

Room `sky: true` turns its ceiling into an environment opening. The room retains
its finite ceiling for spatial bounds and portal confinement. Edge `sky: true`
turns an opaque boundary into a horizon opening, while preserving confinement;
it requires a sky room and cannot be a connection or exported port endpoint,
including through nested prefab ports. Ordinary walls, floors, roofs and static
solids keep their physical behavior. Sky rooms in prefabs require the root
environment. Outdoor rooms remain ordinary connected playable geometry.

Source v1–8 reject all environment/sky fields, including explicitly supplied
`environment: null` and `sky: false`. Text parsers reject null, unknown fields,
missing sun angles and bad scalar types; typed SDK validation checks the values
and complete graph. Compilers preserve sky markers through expansion and convex
decomposition, add the compiled version-1 environment payload and declare
`world/environment@1` with `world/sectors@1`. See the
[compiled decision](../world/README.md#environment-and-sky-openings-version-1).

## Environment backdrops, source version 11 / SDK 0.0.12

Optional `environment.backdrop` is `none`, `mountains` or `city`. The versioned
selector and its encoding remain valid; current hosts draw decorative art from
project-owned source v12 sky resources. The selector is a
decorative horizon layer; it adds no geometry, collision, lighting or receivers.
Source versions 1–10 reject explicit choices. Text parsers reject empty and null
values; typed omission remains the empty string. Time/cloud runtime overrides
preserve the authored selection and leave illumination independent of it.

An explicit selector declares candidate capability `world/environment-backdrop@1`
alongside `world/environment@1` and `world/sectors@1`; SDK 0.0.11 does not support
it. It uses the existing unreleased KWR2 extension after its solid-renderer
trailer (1 none, 2 mountains, 3 city). Backdrop-only records carry a nil solid
sentinel before the selector. Omission preserves exact KWR1 bytes and existing
solid-only KWR2 bytes. Full bounded scans reject invalid selectors, missing
payloads and trailing data before publishing any world.

## Project-owned sky, source version 12 / SDK 0.0.12

Optional `environment.sky` authors a separate `world/sky@1` level resource. Its
`cloud_sparse`, `cloud_dense`, `background` and `horizon` fields reference PNGs
relative to the owning level directory. References must be clean, confined paths:
no absolute paths, parent traversal, backslashes, NULs or drive/URL prefixes, and
at most 1,024 UTF-8 bytes. The CLI also checks filesystem path confinement before
opening each source file. Omitted images contribute no artwork; an empty sky
object is valid. Source versions 1–11 reject explicit sky fields.

```yaml
version: 12
environment:
  preset: daylight
  sky:
    cloud_sparse: sky/clouds.png
    horizon: sky/horizon.png
    settings:
      horizon_haze: 0.2
```

`settings` is optional and overrides fields independently, preserving explicit
zero. Omitted fields use `worldsky.DefaultSettings`: cloud UV scale `[1, 1]`,
dense scroll scale `1`, horizon UV scale `1`, and zero offsets, parallax, haze and emission. Available
fields and bounds are documented in the [sky resource contract](../worldsky/README.md).
A supplied horizon renders regardless of the legacy backdrop choice. Without a
sky sidecar the host renders an asset-free gradient and sun. Project images and
presentation settings never modify environment lighting or runtime clock/cloud
controls; those remain in the frozen compiled environment record.

## Material mapping, version 1

SDK 0.0.8 provides `world/material-mapping@1`. Authoring source v5
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

See [compiled mapping](../world/README.md#material-mapping-version-1) for the
resolved affine projections and capability requirements.

## Solids and loose contents, version 1

Source v6 adds optional `solids` and `contents` to root documents and prefabs.
Detail-only, sprite-only and nested room-free prefabs expand into existing rooms
without creating sectors. Older source versions reject these fields. Omission
preserves existing encoded bytes. The source solid count is at most 1,024 across
root solids and prefab definitions; compilers enforce it again after expansion.

Source material names resolve through packaged textures. Nested prefab scale,
yaw, translation and material overrides apply to solids and loose sprites. The
compiler assigns loose contents to the first containing compiled sector in
stable order, including shared boundaries and elevation. Source caps inherit
horizontal root mapping defaults; source `side_uv` is restricted to shared
planar/world X/Z mapping. Per-edge wrap and varying side triplanar weights are
not expressible by a single SideUV and are rejected. Mapped solids require the
existing `world/material-mapping@1` marker/capability; hosts also require
`world/static-solids@1`. No new guest command or level envelope version is added.

See [compiled solids](../world/README.md#static-solids-version-1) for exact
geometry, material and collision bounds.

## Wall bands and secondary colour, source version 7

Source v7 adds `wall_bands` to rooms and `bands` to directed authored edges.
Each contains optional `enabled`, `top` and `bottom`. Each horizontal band
accepts `texture`, `enabled`, `height`, `repeat_width` and `offset: {x, y}`.
Textures resolve through the ordinary level texture catalog; the same texture
can be reused by both bands. Enabled resolved bands require a texture and height.
Repeat width defaults to 1 world unit and offset to zero. Alpha is band coverage;
transparent pixels reveal the underlying opaque wall.

Rooms also supply `floor_secondary`, `ceiling_secondary`, `wall_secondary`,
and edges may override `secondary`. A secondary references one opaque `texture`,
optional `enabled`, `strength` in [0,1], and existing `uv` settings. Its colour
modulates the main in linear space; neutral sRGB (188,188,188) preserves the
main colour. It does not replace normal, height or AO channels.

All optional fields inherit field by field after prefab expansion; explicit
`enabled: false` disables an inherited setting. Builders apply material
substitution independently to top, bottom and secondary references.
`MergeBandSettings` returns independent nested band records, preserving input
values. Dimensions and repeats use world units and must be finite in
[0.001,1000000]; offsets retain ±1000000 bounds. Source versions 1–6 reject these
fields; omission preserves their existing serialized shape. Typed SDK validation
checks versions and partial controls; public text parsers must additionally reject
explicit null and unknown fields and validate the complete resolved settings and
resource references.

Horizontal bands follow visible solid-span floor and ceiling profiles with fixed
vertical thickness. Opposing envelopes on a short span crop at a proportional
boundary, retaining requested texture density. The builder compiles the complete
nonoverlapping wall partition, mappings, material IDs and coverage classes. There
are no vertical-band or frame-sheet authoring fields. The source-v7 simplification
revises the unreleased SDK 0.0.10 candidate; released source versions and compiled
world/material-layer formats are unchanged. See the [compiled contract](../world/README.md).

## Source v8 reusable animations

Source v8 adds optional top-level `animations: {version: 1, presets: [...],
materials: [...]}` and `actor.animation: {preset: name, phase_seconds: 0}`.
Presets are level-wide and referenced by name, including inside prefabs. Material
bindings use declared texture names; flipbook frames are lists of declared texture
names. The compiler resolves every frame to a nonzero level texture ID. Material
bindings apply wherever the material occurs; an explicit actor binding supplies
its own preset and phase. Preset coordinates stay actor-local through prefab
expansion, so placement scale and orientation carry motion along with the actor.

The initial catalogue is `flipbook`, `liquid`, `spin`, and `oscillate`. See the
[compiled contract](../world/README.md#reusable-animations) for parameters, bounds
and timing. Source v1–v7 reject animations and retain existing wire behavior.
Frames are separate complete textures, which the world material atlas packs into
ordinary slots; they are not arbitrary authored atlas rectangles. Flipbook frames
must have equal processed dimensions. Presets cannot be nested or reference other
presets, and frame materials cannot select another material animation.

Liquid presets may also specify `surface_amplitude` (0–0.25 world metres),
`opacity` (0–1, omitted means 1), and `pixel_size` (integer 0–32). These fields
are liquid-only; explicit nulls are rejected by the source YAML adapter.
Compilation owns an independent copy of an explicit opacity value.

## GPU sprite particles, source version 10

Candidate SDK 0.0.11 adds root `particles: {version: 1, presets: [...], emitters: [...]}`
and room/prefab `particle_emitters` arrays. Presets name level textures; emitter
positions, yaw and uniform scale follow prefab transforms. Source v1–9 rejects
particle fields, including explicit null. Optional fields should be omitted;
text decoders also reject null inside the particle block and require explicitly
authored zero-valued fields such as seed, position, curve age and burst time.

Compilers resolve all frames, require square matching frame dimensions, reserve
complete birth schedule groups for overlapping lifetimes, compute conservative
GPU motion bounds, and confine every emitter to one compiled convex sector.
Room-local emitters remain in sectors owned by their expanded room. An emitter-only
prefab is valid without unrelated geometry or marker contents. Optional scale
omission or zero defaults to one; omitted vector components remain zero.
They stage the separate `@world/particles` resource and declare `world/particles@1`
alongside `world/sectors@1`. See the [version-1 particle contract](../worldparticles/README.md)
for exact fields, units, lifecycle rules and resource bounds.

## Solid renderers, source version 11

`Prefab.Renderable` packages a rigid solid-only prefab for runtime creation.
`Actor.SolidRenderer` references one such prefab and also implicitly packages
it. Renderer prefabs may nest solid-only instances; rooms, connections, ports,
contents, emitters and collision-enabled solids are rejected. Actor sprites and
actor animation bindings cannot coexist with a solid renderer. Public compilers
retain one local definition per referenced/exported prefab, resolve level
materials, and emit actor attachments in `world.SolidRenderers`. They must reject
explicit renderer fields before v11 and null/empty renderer identifiers.

## Alpha GPU particles, source version 11

Candidate SDK 0.0.12 permits `particles.version: 2` in source v11. It adds
`sprite.blend: alpha` and `premultiplied` using the existing particle fields, and declares the exact
`world/particles@2` capability. Source v10 retains version-1 semantics and rejects
version 2. Source v11 can still author version 1, which rejects both new materials.
The bounded layout, schedules, texture resolution, prefab transforms and
whole-envelope confinement remain unchanged.

`CompileParticlePreset` remains strict version 1. Public compilers select
`CompileParticlePresetForVersion` using the enclosing particle document version;
hosts validate that version against the corresponding declared feature. See
[alpha rendering and ordering limits](../worldparticles/README.md#version-2-alpha-blending).
