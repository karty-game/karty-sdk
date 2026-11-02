# Saved reflection cubemaps

`world/reflection-cubemaps@1` packages immutable static cubemaps for a fixed,
renderer-ready chunk level. It requires `world/fixed-lighting@1` and either
`world/chunk-ready@1` or `world/gpu-chunks@1`. GPU cells retain their shared
material and world capability dependencies; live-only cells cannot declare saved
reflections. `@world/reflection-cubemaps` contains a KRC1 bundle
with explicit sector/solid bindings, independent capture origins and projection
bounds, and six lossless 128 × 128 RGBA faces per environment. The horizontal
strip orders faces +X, -X, +Y, -Y, +Z, -Z and preserves graphics bottom-up rows.

`InputDigest` binds the original root metadata and every original payload,
including catalog hashes of chunk geometry and baked lighting. It excludes only
the reflection entry itself. A consumer calls `Decode` and `ValidateSource`
before accepting the bundle. A changed level requires a fresh capture.

The format allows at most 256 environments, 256 sector bindings and 1024 solid
bindings. Each binding is -1 or references a present environment. Bounds must
be finite, contain the capture origin strictly, and span at most 65536 units
on each axis. Images must be complete RGBA/sRGB QOI strips. The full encoded
bundle fits the level entry's 8 MiB bound; decoded pixels total at most 96 MiB
and count against the consumer's aggregate texture budget before allocation.

`Decode` borrows admitted image bytes from its input. Callers must retain the
input and keep it immutable. No game code or renderer dependency is required
to validate the public artifact. The engine owns capture and GPU projection;
the CLI owns invoking offline capture and packaging matching cached output.

## Prepared GPU mip arrays

`world/reflection-gpu@1` extends the fixed GPU-chunk profile with KRG1 at the same
entry name. It contains the same source-bound receiver/probe metadata and one
RGBA8/sRGB + Zstd KTX2 array: 768 by 128, one layer per probe, and exactly four
offline mip levels. Probe metadata has no QOI payload. KRC1 remains the offline
capture cache and legacy artifact representation.

The current per-record GPU profile packs planned one-based reflection slots into
immutable cell records before capture. Opaque batches can therefore retain
independent receiver slots without an owner draw split. Capture uses a valid
neutral prepared resource at this same entry; replacement by captured images
leaves InputDigest and all cell bytes unchanged. Neutral resources are never
published as a fallback by ordinary builds. Worlds without saved reflections
suppress their inactive planned slots.

`Prepared.DrawProbes` retains its bounded wire shape for this candidate; the
per-record profile writes zero entries and performs receiver/slot agreement checks
offline. It does not perform an owner-to-probe lookup during rendering. Earlier
per-draw profile entries use final little-endian uint32 slots, grouped by
dense CellID and then by the unchanged cell draw order. Zero means no environment;
other values are one-based positions in the sorted probe table, never authored
probe IDs. The build resolves sector/solid ownership after capture and matches
render artifacts by catalog content hash. Cell bytes, hashes and GPU records
stay unchanged. The table has at most 4,096 cells and 1,024 slots per cell; the
existing complete encoded-size limit also applies. Decoders borrow the slot
bytes. Root admission checks the catalog cell count before image allocation;
new-cell admission checks the exact draw count. Runtime reads one scalar slot
without resolving authored owners. GPU-cell artifacts with an earlier raw KRC1
entry must be rebuilt; non-GPU legacy KRC1 remains supported.

The build filters each face independently in linear light. Admission uses
`DecodePreparedSource` to check source identity and the complete metadata/image
layout once before allocation. The four validated frames are decompressed into
their final GPU mip/layer order; loading does not filter, resize or repack them.
`InputDigest` continues to exclude only this entry, avoiding a self-reference.

The existing 96 MiB reflection limit includes all mip levels. One environment
uses 524,160 decoded bytes, so at most 192 environments fit the prepared profile.
For 26 environments the exact total is 13,578,240 bytes. `Prepared` and its codec
are independent of the engine and renderer.
