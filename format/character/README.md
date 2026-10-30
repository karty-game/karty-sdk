# Compiled characters v1

`world/characters@1` identifies the compiler-prepared character resource stored
as the level data entry `@world/characters`. The `KCHR` version 1 binary contains
skinned models and shared animation banks. This package owns the portable
resource, its canonical encoding and complete validation. GLB parsing, Meshopt
decoding, source path resolution and material preparation belong to the CLI.

`Encode` validates the complete document before producing bytes. `Decode`
checks bounded counts before allocation, validates the resulting document and
returns owned slices. Callers may release or reuse the encoded input afterward.
Malformed, truncated, oversized and trailing data are rejected with `ErrInvalid`.

## Models and animation banks

Each model has a unique ID, an index into the shared bank list, a complete TRS
node hierarchy, one mesh node and a retained joint palette. Joint entries refer
to hierarchy nodes; inverse-bind matrices follow the same palette order. Node
names are unique within a model. Animation tracks target those exact names,
allowing a bank to bind models whose node indices differ.

Vertices contain 16 float32 values: position (3), normal (3), atlas UV (2),
palette indices (4) and normalized weights (4). Indices describe triangles.
The opaque albedo image is QOI with sRGB metadata, with dimensions declared alongside the image.
Source base colour factors and texture wrapping are resolved before encoding.

Clip durations and sample times are seconds. Translation and scale tracks
contain three values per key; rotation tracks contain four quaternion values.
`Step` selects STEP interpolation, otherwise the track uses LINEAR interpolation.
Key times strictly increase. Rotation values are unit quaternions. Each model
must contain every named target used by its selected bank. Each node/path pair
appears at most once per clip.

The resource holds immutable pose inputs. Entity placement, independent base
and upper-body clocks, blending, visibility and rendering are runtime concerns.
It does not prescribe a game controller or character state machine.

## Bounds

| Resource                              | Maximum          |
| ------------------------------------- | ---------------- |
| Encoded document                      | 64 MiB           |
| Models / shared banks                 | 8 / 8            |
| Nodes / retained joints per model     | 512 / 64         |
| Vertices / triangle indices per model | 65,536 / 393,216 |
| Clips per bank / tracks per clip      | 32 / 1,536       |
| Keys per bank                         | 100,000          |
| Texture width and height              | 2,048            |
| Clip duration                         | 600 seconds      |
| UTF-8 ID, clip or node name           | 128 bytes        |

Validation also rejects duplicate IDs and node names, hierarchy cycles, invalid
palette references, nonfinite values, singular inverse-bind matrices, invalid
triangle indices, malformed QOI and inconsistent image dimensions. Compile-time
importers may enforce stricter source bounds before constructing a document.

Across bind and animated transforms, conservative accumulated scale magnitudes
stay within `1e-6` and `1e6`, and accumulated translation length stays below `1e9`.
These bounds cover independent clip clocks and interpolated keys, preventing
finite local values from overflowing runtime hierarchy matrices. Scale tracks
cannot animate reflected bind scales through a singular transform. Per-image
QOI bytes permit the full `2048*2048*5+22` worst case; the complete document still
shares its 64 MiB bound.
