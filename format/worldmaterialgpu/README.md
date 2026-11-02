# Prepared GPU world materials, version 1

The unreleased SDK/host **0.1.0** candidate adds `world/material-gpu@1`.
It requires `world/sectors@1` and `world/material-atlas@1` or `@2`. Existing
released bundles and the ordinary QOI material contract remain immutable.
Older hosts reject this capability rather than interpreting new channel data
using an old shader. This adds a resource contract, without a command ABI change.

A selecting level declares `kartyWorldMaterialGPU = "world/material-gpu@1"`
and contains the existing canonical `@world/material-layout` plus all four entries:

| Entry                         | Content                                          |
| ----------------------------- | ------------------------------------------------ |
| `@world/material-gpu-layout`  | `KMG1` descriptor                                |
| `@world/material-albedo.ktx2` | sRGB RGB surface color, linear A coverage        |
| `@world/material-ssbump.ktx2` | linear RGB directional response, A emission mask |
| `@world/material-detail.ktx2` | linear RG normal XY, B metallic, A smoothness    |

Every array has 256x256 base dimensions, one layer per material in exact layout
order and all nine mip levels through 1x1. Each array uses the bounded
[`codec/ktx2`](../../codec/ktx2/README.md) UASTC RGBA + Zstandard profile. Encoders may omit constant-1 alpha and use the RGB DFD; the decoder must
supply that implicit fourth component. There
are no scalar AO or height channels in these runtime arrays. Each material has
SSBump, including flat generated coefficients for a flat heightfield. Its RGB
response uses the RNM basis and image orientation defined by
[`worldmaterialssbump`](../worldmaterialssbump/README.md); its A is now an emission
mask rather than the legacy companion's reserved alpha. Existing uniform
emission intensity multiplies that mask and the albedo color. A mask of 255
preserves uniform emission; intensity zero disables it. All three resources are
mandatory; stripping any one never silently selects normal fallback.

`KMG1` contains its four magic bytes, SHA-256 of the complete canonical
`worldmaterial.EncodeLayout` result, a big-endian uint16 material count, and that
many big-endian uint32 material IDs. Counts are 1..196. The complete unique
material order must match the layout, including chart-generated materials.
There are no optional fields or trailing bytes. Layout binding includes source
strength and coverage controls; the builder resolves those controls in the
prepared pixels exactly once. A host must not apply them again.

The builder generates height, normals and reflectance where needed, traces each
original periodic height tile before chart packing, and projects SSBump into the
destination tangent basis. It generates final mips before UASTC encoding:
albedo color filters in linear light, coverage follows its declared mode,
normal XY filters as decoded vectors, and metallic/smoothness/emission filter
as scalars. SSBump RGB averages without renormalizing away occlusion.

The prepared arrays replace runtime material QOI image payloads. The canonical
source layout stays available for geometry binding and build-time lighting
provenance. Runtime consumers do not need a legacy decoded QOI pair to admit a
level selecting this feature. Unrelated QOI textures (UI, sprites, models and
other existing resource families) retain their existing contracts.

`Pair.Inspect` validates declaration binding, matching array dimensions/layers,
color spaces, structural mip/frame bounds and aggregate encoded, inflated and
RGBA fallback budgets. It does not prove compressed stream contents; consumers
must fully inflate and transcode all levels before publishing the material.
`ValidateDeclaration` checks local metadata, complete entry kinds/counts and
global capability dependencies. A cartridge can also contain ordinary levels.

Build tools and runtime decoders remain owned by their repositories. A local
candidate archive is not public tag/checksum availability, and compilation is
not evidence of browser/native GPU output or mobile loading performance.
