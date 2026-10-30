# Sector vegetation, versions 1–5

## Versioned decision

Source version 13 introduces independently authored per-room vegetation layers.
Compilation expands each room layer onto its compiled sectors. The optional
`@world/vegetation` sidecar uses document version 1, typed binary magic `KWV1`,
and capability `world/vegetation@1` together with `world/sectors@1`. Existing
compiled world versions 1–4 and source versions 1–12 retain their contracts.
Vegetation is presentation only: it has no actor identity, collision, lifetime,
gameplay simulation, density mask or automatic biome interpolation. Authors
choose each sector's density; grass, bush and tree-like decorations can use
independent presets and seeds. Nearby physical trees need a separate actor/solid
representation.

A preset contains one nonzero cutout texture identity, independent width and
height ranges, wind amplitude, wind frequency and draw distance. A layer selects
a preset, sector, density (plants per square XY world unit), uint32 seed and an
explicit `floor` or `ceiling` surface. Density and seed zero are valid. Each
sector/preset/surface triple must be unique. Zero density disables a layer;
unused presets still undergo complete validation and texture identity checks.

The renderer derives a persistent candidate population from each layer's seed
and stable candidate ordinals. Changing the camera or descriptor order affects
visibility only. Wind uses world XY and mounted-level time, so hidden vegetation
returns at the current wind phase without lifecycle restart. Increasing density
retains existing candidate positions; fractional density accepts a stable subset.
Hosts submit compact visible-layer/aperture descriptors and reuse bounded buffers
rather than allocating the full logical population or enumerating its plants.
Root generation, density acceptance, variation and animation belong to the GPU.
Reaching a renderer budget can omit otherwise visible support; version 1 does
not guarantee full density under every camera view.

The current prototype reserves `ceil(XY_bounds_area * density)` candidates in
each sector's bounding rectangle and applies the fractional final-candidate
acceptance and sector-footprint checks on the GPU. Static layer metadata persists
in GPU storage. Apertures for the same layer are combined as a union within one
render context, avoiding duplicate plant generation and budget charges. Distinct
portal contexts share the complete-frame card cap. Compatible atlas pages and
outdoor lighting group descriptors into instanced draws across layers/apertures.

This renderer algorithm replaces the first prototype's unit-cell seed placement
and CPU sparse-cell eligibility checks. Its seeded positions change from that
prototype while retaining camera-independent identity and monotonic density.
The source vocabulary, binary document version and capability remain unchanged;
no serialized hash constants or plant-instance table are part of the format.

Floor cards grow upward and ceiling cards downward. The base follows the
attachment plane, including slopes; wind displacement increases toward the tip.
Ceiling layers require a real ceiling and are rejected on sky sectors. Hosts
must clip roots to their own convex sector and keep the entire card/wind support
inside opaque or transformed portal boundaries. Ordinary reciprocal coincident connected boundaries with matching floor and
ceiling planes and sky status permit continuous vegetation with no padded strip. The renderer must
include maximum card support and wind in culling bounds. Static-solid exclusion
and plant shadows are outside version 1.

## Bounds and codec

| Field                   | Bound                                                |
| ----------------------- | ---------------------------------------------------- |
| Presets                 | 1–64, unique nonempty UTF-8 names, at most 128 bytes |
| Layers                  | 1–1,024, valid preset references                     |
| Width and height        | Ordered finite ranges, 0.001–64 world units          |
| Density                 | Finite 0–16 plants per square XY world unit          |
| Wind amplitude          | Finite 0–8 world units                               |
| Wind frequency          | Finite 0–16 radians/second; zero freezes wind phase  |
| Draw distance           | Finite 1–128 world units                             |
| Visible instance budget | At most 16,384 cards (two crossed cards per plant)   |
| Encoded bytes           | At most 1 MiB                                        |

`Encode` validates the complete document. `Decode` performs an allocation-free
scan of the entire payload before materializing any arrays and rejects malformed
counts, trailing bytes, unsupported versions, nonfinite values and invalid
references. The handwritten generator is `internal/codegen/binary`; regenerate
with `mise exec -- go generate ./format/worldvegetation`. `ValidateForWorld`
requires an already validated world, checks sector references and sky ceilings,
and conservatively reserves full height plus relative plane slope across the
maximum lateral footprint. Per-root polygon and boundary clipping additionally
belongs to the renderer. `ValidateTextures` checks every preset against the
caller's successfully decoded texture identities, including unused presets.

## Authoring

```yaml
version: 13
vegetation:
  version: 1
  presets:
    - name: grass
      texture: field_grass
      width: { min: 0.25, max: 0.45 }
      height: { min: 0.3, max: 0.7 }
      wind_amplitude: 0.08
      wind_frequency: 0.6
      draw_distance: 48
rooms:
  - id: meadow
    # Ordinary boundary, floor, ceiling and material fields remain required.
    vegetation_layers:
      - preset: grass
        density: 4
        seed: 31
        surface: floor
```

All shown vegetation fields are required by authoring parsers. The typed SDK
validator cannot detect omitted or null YAML fields; parsing, presence checks,
texture name resolution and generated editor schemas belong to public compilers.
A source preset library may contain no placements; no sidecar is needed then.
Compiler expansion must enforce the layer bound again after prefab expansion and
convex decomposition.

## Built-in shrub geometry (version 2)

Source version 14 introduces `vegetation.version: 2`, `KWV2`, and capability
`world/vegetation@2`. A preset can select `geometry: shrub`, generating reusable
branches and alpha cutout leaf sprays arranged in three dimensions. `geometry: crossed_cards` is the default.
The binary `Preset.Geometry` uint8 encodes crossed cards as 0 and shrubs as 1;
other values are rejected, including on unused presets. No model resource,
importer or per-plant mesh file is required. The existing texture is albedo;
width/height, wind, density, seed and surface retain their meanings. The renderer
still derives each candidate root and animation on the GPU.

`Version`, `Feature` and `BinaryMagic` retain their v1 meanings. `GeometryVersion`,
`FeatureV2` and `BinaryMagicV2` name v2. `FeatureForVersion` resolves the exact
capability. The codec preserves the original KWV1 record layout, rejects
magic/document version mismatches and preflights both layouts before allocations.
Version 1 forbids geometry selection; Source 13 cannot author v2. Source 14 can
still author unchanged v1 libraries. Tree geometry, custom models and distance
LOD are outside this revision.

## Optional normal maps (version 3)

Source 15 adds vegetation version 3, `KWV3` and `world/vegetation@3`. Source
presets may specify an optional `normal_texture` name, resolved into the binary
`Preset.NormalTexture` uint32 resource ID. Zero retains geometry normals;
nonzero IDs must exist among successfully decoded textures, including on unused
presets. Hosts and compilers additionally check image bounds and matching
albedo/normal dimensions. Normal RGB is tangent-space direction data; albedo
alpha continues to define cutout visibility. The shrub normal map follows the
same upper leaf sprig / lower bark layout as its albedo.

`NormalVersion`, `FeatureV3` and `BinaryMagicV3` select this revision.
`FeatureForVersion` resolves all three exact capabilities. The handwritten
binary generator retains dedicated legacy records for immutable KWV1 and KWV2
layouts; normal fields are rejected for those versions. Each layout receives a
complete bounded preflight before typed allocation. Existing API and wire
protocols remain unchanged. No model import or per-plant state is introduced.

## Built-in trees (version 4)

Source 16 introduces vegetation version 4, `KWV4`, `world/vegetation@4`, and
`GeometryTree = 2`. Source presets select `geometry: tree`, generating reusable
trunks, branches and leaf sprays. Trees use the existing leaf/bark albedo and
optional tangent-space normal texture layout; no extra mesh resources are
required. Width is the full canopy extent and height the full tree height.
Independent layers retain the same density, seed, surface, wind and draw distance
fields. Trees remain presentation only, without collision or gameplay state.

`TreeVersion`, `FeatureV4` and `BinaryMagicV4` name this revision. Its record
layout is identical to v3 but uses a distinct magic/document version and permits
all geometry values 0–2. Earlier versions reject tree geometry, including on
unused presets. Unknown values still fail complete validation. KWV1, KWV2 and
KWV3 layouts/capabilities remain unchanged, and all four layouts receive complete
bounded preflight before allocation. Source 16 can author unchanged previous
vegetation versions. API and wire protocols do not change.

## Built-in palms (version 5)

Source 20 introduces vegetation version 5, `KWV5`, `world/vegetation@5`, and
`GeometryPalm = 3`. Source presets select `geometry: palm`; the host generates a
slender trunk and a radial crown of curved fronds instead of a broadleaf branch
structure. Width is the complete crown extent and height the complete tree
height. Albedo and optional tangent-space normal resources, density, seed,
surface, wind and draw distance retain their existing meanings. Palms remain
presentation only and require no model file or per-tree gameplay entity.

`PalmVersion`, `FeatureV5` and `BinaryMagicV5` identify this revision. Its preset
and layer record sequences are identical to v3/v4; only the magic/document
version and permitted geometry range (0–3) differ. Earlier versions reject
palms, including unused presets. V5 rejects unknown geometry values and validates
all preset resources before rendering. Magic/document-version mismatches,
malformed counts, truncations and trailing data fail the complete bounded
preflight before typed allocation. KWV1–KWV4 encodings and capabilities remain
unchanged. Source 1–19 reject v5 even without placements; Source 20 may still use
all earlier versions. SDK candidate 0.0.13 adds the capability; API/wire records
and existing runtime bounds do not change.

Palm art uses the existing half-height atlas convention: the upper half contains
one alpha-cutout frond and the lower half contains opaque trunk bark. The frond
root is at upper-half bottom (`v=0.495`) and its tip at top (`v=0.005`). Optional
normal maps use identical coordinates; albedo alpha controls visibility.

## Shared species geometry, public Go module 0.0.19

The unreleased module 0.0.19 exposes `SpeciesVertices` and
`SpeciesVertexFloats` for the existing immutable shrub, tree and palm templates.
Position, normal, UV, tangent, bitangent and canopy-occlusion values keep the
current renderer's exact 15-float layout and triangle order. Crossed cards keep
their existing four-vertex quad and return no woody template. These build-only
helpers add no serialized fields or capability changes.

The fixed baker uses these same templates for static cutout shadows and diffuse
contribution in ordinary chunk lightmaps and probes. Visual placement and wind
remain GPU work. Static baking freezes wind at the rest pose and includes every
eligible plant, independently of camera distance or visibility thinning.
