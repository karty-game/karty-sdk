# Compiled world contract

`world` defines the bounded sector document shared by public level compilers and
Karty hosts. It owns typed binary encoding and complete document validation.
[World source](../worldsource/README.md) owns room, prefab, port and material
**authoring** semantics; public compilers parse and expand that source into this
contract. The SDK does not implement runtime rendering or collision.

## Versions and encoding

| Compiled version | Added semantics                                                   |
| ---------------- | ----------------------------------------------------------------- |
| 1                | Convex sectors, coincident reciprocal portals and contents        |
| 2                | Typed actors, sprites and tags                                    |
| 3                | Directed/transformed portal pairs and optional versioned payloads |

Versions 1–3 remain supported. Compiled versions, source versions, Go module tags
and downloadable SDK/host versions are separate identifiers. A producer must
also declare `world/sectors@1` and each feature it uses in the cartridge manifest;
consumers check these declarations against host support. `Validate` has no
manifest or packaged assets to inspect.

The level resource is `@world/main`. `Encode` validates first and emits canonical
typed binary. `Decode` rejects empty or oversized input, unsupported versions,
JSON, malformed records and trailing bytes. An allocation-free scan of the
**complete** record bounds typed allocations before decoding; semantic validation
then checks the complete document before it can be mounted.

`KWR1` is the base binary envelope. Solid-renderer or explicit environment-backdrop
extensions select `KWR2`, retaining the base fields and appending a bounded
trailer. Omitting both extensions preserves the base encoding; omitting a
backdrop preserves existing solid-only trailer bytes. Exact field order belongs
to the [generated codec](binary.generated.go) and its
[handwritten envelope](codec.go). Regenerate codecs from the owning declarations
rather than editing generated output.

## Geometry, identity and contents

| Bound                              | Limit                                    |
| ---------------------------------- | ---------------------------------------- |
| Encoded document                   | 8 MiB                                    |
| Sectors                            | 1–256                                    |
| Walls                              | 8,192 total; 3–256 per sector            |
| Contents                           | 4,096                                    |
| Identifier and kind                | 128 UTF-8 bytes each                     |
| Coordinates and plane coefficients | Finite, absolute value at most 1,000,000 |
| Edge length and vertical clearance | At least 0.000001                        |

Sector order is stable because portals reference sector indexes. Each sector is
strictly convex and counterclockwise, with a closed sequence of directed walls.
Floor and ceiling planes evaluate `z = A*x + B*y + C`; every vertex must have
positive clearance. Sector IDs and content IDs are unique within their respective
lists. `SourceRoom`, `SourceEdge`, `SourceID` and `Instance` retain provenance after
expansion; compiler-generated internal walls may omit `SourceEdge`.

An opaque wall has `Portal = -1` and `PortalWall = 0`. In v1/v2, a portal requires
one coincident reversed edge pointing back from a different sector, with clearance
and no crossing floor/ceiling profiles. In v3, `Portal` is a destination sector
index and `PortalWall` is **one plus** its destination wall index. The paired
edges must have equal lengths; their positions define a rigid destination-to-source
transform. Directed and self-sector connections are possible in v3.

Every content position lies within its assigned sector's footprint and elevation
bounds. From v2, an optional actor supplies finite yaw/pitch/roll, positive XYZ
scale and at most eight strictly sorted, unique UTF-8 tags of at most 64 bytes.
An optional sprite references a nonzero asset ID, positive width/height, origins
in `[0,1]`, a facing mode (`camera-facing`, `upright`, `cross`, `fixed`) and an
alpha mode (`cutout`, `blend`). Compilers and hosts resolve texture/material IDs
against packaged assets separately.

See [types](types.go) and [validation](validate.go) for the field definitions and
precise validation rules.

## Lighting, version 1

Optional `Lighting` requires compiled v3 and `world/lighting@1`. Ambient, point
colors and the optional six-face world-space ambient cube are finite linear RGB
values in `[0,1]`. The cube replaces uniform ambient when supplied. Lights retain
authored order and global coordinates. Their IDs are unique and their influence
radii range from 0.001 to 1,000,000 world units.

`MaxLights = 4096` bounds the complete authored light set, including offline-baked
sources. `MaxRuntimeLights = 50` separately bounds live direct lights; producers
must check the remaining runtime set when packaging a world. An unbaked preview
uses that live limit. Optional version-1 light motion moves between `Position`
and `Position + Offset` and back in 0.1–3,600 seconds; both endpoints remain
within coordinate bounds. Moving lights remain runtime sources.

See [lighting](lighting.go), [emission](#material-emission-version-1) and the
[lightmap contract](../worldlightmap/README.md) for their separate responsibilities.

## Environment and sky openings, version 1

Optional `Environment` requires compiled v3 and `world/environment@1`. Its preset
is `daylight`, `space`, `cartoon` or `fantasy`; its optional palette is `default`,
`violet` or `twilight`. Palette changes affect appearance independently of
illumination. Optional fixed sun angles use azimuth `[0,360)` from +X toward +Y
and elevation `[0,90]` above the XY plane. Optional `TimeOfDay` supplies hours
`[0,24)` and cannot coexist with an explicit sun. Optional cloud coverage is
`[0,1]`, with omission resolving to 0.35 and explicit zero remaining clear.

A sector's `Sky` opens its ceiling visually while retaining finite spatial
bounds. A sky wall must be opaque and belong to a sky sector. Sky sectors require
an environment. Preset resolution supplies bounded sky, sun and outdoor fill;
consumers restrict sun/fill using openings and geometry. See
[environment validation and resolution](environment.go).

An explicit backdrop (`none`, `mountains`, `city`) additionally declares
`world/environment-backdrop@1` and uses the KWR2 trailer after solid renderers
(selector bytes 1, 2, 3). A backdrop-only document writes a nil solid-renderer
sentinel first. The selector adds no geometry, collision or lighting.

Project-owned sky textures and presentation settings live in the separate
[`world/sky@1` resource](../worldsky/README.md); they do not change this environment
codec. See [source v12 sky authoring](../worldsource/README.md#project-owned-sky-source-version-12--sdk-0012)
for confined PNG paths and partial overrides.

## Material mapping, version 1

Optional `MaterialMapping` requires compiled v3 and `world/material-mapping@1`.
Every sector floor, ceiling and wall then supplies complete `SurfaceUV` data.
Source mode, anchoring, inheritance and prefab transforms are resolved by the
compiler; hosts infer none of them.

A surface contains one affine projection or three blended projections, with
matching constant weights. Each U/V plane evaluates
`X*position.X + Y*position.Y + Z*position.Z + Offset` in texture repeats.
Coefficients are finite and bounded to ±1e12. Weights lie in `[0,1]` and sum to
one within 1e-9. The same projections apply to every material channel. See
[mapping validation](mapping.go).

## Static solids, version 1

Optional `StaticSolids` requires compiled v3 and `world/static-solids@1`. At most
1,024 uniquely identified solids are strictly convex counterclockwise
extrusions, each with 3–64 footprint vertices and bounded bottom/top planes.
Thickness and edge lengths retain the ordinary minimums. Side, top and bottom
material IDs are nonzero. Optional mappings require `MaterialMapping`;
`SideUV` is shared across all sides, rather than per edge.

Solids may overlap sectors or each other and do not alter the sector/portal
graph. `Collision` blocks movement; it never makes the top a walkable floor.
[Solid validation](solids.go) checks every item before mounting.

## Material layers, version 1

Optional `MaterialLayers` requires compiled v3, `world/material-layers@1` and
material mapping. Secondary color layers carry a material, strength `[0,1]` and
complete UV projections. Wall bands compile into complete nonoverlapping
`WallFrameRegion` partitions: vertices are counterclockwise in edge-fraction /
world-elevation coordinates. Coverage is `main`, `opaque` or `masked`; main
regions retain wall mapping, while other regions supply their own mapping.

There are at most 252 regions per wall and 32,768 total. `FrameCompiled` retains
participation when a portal has no exposed solid spans. Frame partitions require
a source edge identity. Source band inheritance and texture selection belong to
[authoring](../worldsource/README.md#wall-bands-and-secondary-colour-source-version-7);
see [compiled layers](materials.go) and [partition validation](frame_validate.go).
`MaterialIDs` provides stable, deduplicated atlas order.

## Reusable animations

Optional version-1 `Animations` requires compiled v3 and `world/animations@1`.
It contains 1–64 uniquely named presets and at most 196 material bindings.
Bindings select a preset and nonnegative phase in seconds, bounded to 86,400.
Presets cannot recursively animate frame materials or use fields from another
kind.

| Kind        | Parameters and bounds                                                                                                                                                                    |
| ----------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `flipbook`  | 2–64 nonzero texture IDs; FPS 0.001–120; optional repeat interval at least one complete frame sequence, at most 86,400 seconds                                                           |
| `liquid`    | UV flow components ±10; amplitude `[0,1]`; frequency `(0,100]`; speed ±100 radians/second; surface amplitude `[0,0.25]` metres; opacity `[0,1]` (default 1); pixel size 0–32 (default 0) |
| `spin`      | Unit axis within 1e-6; bounded actor-local pivot; nonzero speed ±100 radians/second                                                                                                      |
| `oscillate` | Nonzero bounded actor-local offset; period 0.001–86,400 seconds                                                                                                                          |

A flipbook holds frame zero outside an optional repeat burst. Frames are complete
textures, resolved and checked by compilers/hosts. Material bindings support
flipbook/liquid; displaced liquid requires a horizontal up-facing floor or static
solid top. Actor bindings require sprites, reject liquid, and require `fixed`
facing for spin/oscillate. Actor-local motion is applied before scale/orientation.
Lighting and collision retain rest geometry. See [animation validation](animations.go).

## Material emission, version 1

Optional `Emission` requires compiled v3 and `world/emission@1`. At most 196
unique primary surface/band materials carry an eight-bit linear-albedo multiplier,
decoded as `code*8/255`. Secondary modulation and animation-only frames are not
emission owners. An optional cosine pulse has depth `[0,1]`, period
0.001–86,400 seconds and phase 0–86,400 seconds. It begins at full brightness.

Optional linked light IDs must exist in `Lighting` and are unique across all
emission records, with at most 50 links in total. Pulse and flipbook/liquid
animation are independent effects. See [emission validation](emission.go).

## Solid renderer extension

Optional version-1 `SolidRenderers` requires compiled v3 and
`world/solid-renderers@1`. It retains 1–64 immutable local solid prefabs, at most
1,024 total solid definitions, 65,536 triangles and 4,096 unique actor attachments.
It uses KWR2 without changing the base KWR1 fields.

Each attachment references an existing actor and prefab. Renderer actors cannot
also carry sprites or actor animation bindings. Renderer solids reuse extrusion
validation but reject collision; materials cannot select surface displacement or
opacity below one. These definitions contribute material IDs separately from
static world geometry. See [renderer validation](solid_renderers.go) and
[source prefab authoring](../worldsource/README.md#solid-renderers-source-version-11).

## Separate resources

Render chunks, material atlases, baked lighting, particles and project sky have
independent resource versions and bounds. Follow their owning contracts:
[`worldchunk`](../worldchunk/README.md),
[`worldmaterial`](../worldmaterial/README.md),
[`worldlightmap`](../worldlightmap/README.md),
[`worldparticles`](../worldparticles/README.md) and
[`worldsky`](../worldsky/README.md).

## Brighter daylight sky — SDK 0.0.13

`environment.preset: daylight-bright` requires `world/environment-skylight@1`.
It supplies four times the original daylight preset's sky illumination while
keeping its sun color/intensity/direction, sky colors and decorative layers.
Source version 9 and later can name the preset; project sky textures still
require source version 12. The Environment sample uses it with 64 samples and
two diffuse bounces. Existing `daylight` worlds retain their original values.

Both offline sky rays and the live outdoor ambient cube use the stronger
resolved illumination. Ordinary geometry still blocks the baked sky; this is
not an unoccluded indoor ambient floor. Authored room ambient and point lights
remain independent. Night reduces sky illumination with the existing clock
curve; cloud coverage retains the existing weather multiplier. Sunlit outdoors
also receive stronger sky fill. Rebuild all environment lighting maps when
changing the preset; cache identities and conservative RGBM ranges include it.

KWR1/KWR2 layouts and the `cpu-environment-rnm3@1` algorithm are unchanged.
The new preset is an explicitly gated input, not a change to released defaults.
Old SDKs reject the unknown preset/capability rather than reinterpret its lighting.
The guest `Game.SetEnvironment` API still controls only time and cloud amount.
