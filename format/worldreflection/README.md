# Saved reflection cubemaps

`world/reflection-cubemaps@1` packages immutable static cubemaps for a fixed,
renderer-ready chunk level. `@world/reflection-cubemaps` contains a KRC1 bundle
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
