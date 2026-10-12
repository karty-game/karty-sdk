# GPU sprite particles

Candidate SDK 0.0.11 defines `world/particles@1` alongside
`world/sectors@1`. Compilers stage one `@world/particles` data entry in the level
module. This separate resource leaves compiled world versions 1–3 intact.
Hosts must advertise both capabilities to mount the feature.

Candidate SDK 0.0.12 adds document version 2 and `world/particles@2` for smooth
alpha and premultiplied particles. Version 1 remains strict: it accepts only additive/cutout
blends. `Version` and `Feature` retain their version-1 values; `AlphaVersion`,
`FeatureV2` and `FeatureForVersion` select the newer contract explicitly.

The `KWP1` typed binary codec uses the common version-1 bounded binary header.
Decode scans the complete payload without materializing arrays before decoding;
malformed counts, nonfinite numbers, unsupported versions, trailing bytes,
invalid references and forged motion bounds are rejected. Generate its checked-in
codec through `mise exec -- go generate ./format/worldparticles`; the owning
handwritten generator is `internal/codegen/binary`.

## Analytical lifecycle

A document defines named presets and placed emitters. A preset specifies a fixed
particle capacity, point or box spawn support, per-axis initial velocity ranges,
lifetime range, constant acceleration, linear drag, sprite frames, rotation and
piecewise linear size/RGBA curves. Curve keys remain strictly ordered after float32 conversion,
with every uploaded adjacent age difference at least `2^-126`. Smaller differences
can flush to zero on portable GPUs. Curves have 2–4 keys, start at normalized age
0 and end at age 1. Size is the square billboard's edge length in world units;
RGBA components lie in `[0,1]`. Frames must be square, with identical dimensions
within each preset. Accepted facing modes are `camera-facing` and `upright`;
blend modes are `additive` and `cutout`, with `alpha` and `premultiplied` added in version 2. Flipbooks use `once` (clamp final frame)
or `cycle`, with FPS remaining at least the smallest normal float32 (`2^-126`)
when more than one frame is present. A single frame permits FPS zero.

The source vocabulary is owned by `format/worldsource`. YAML/schema and named
texture resolution belong to the public CLI, so this package has no YAML parser
or private engine dependency. Source fields and zero defaults map to these types:

| Field                                            | Meaning and bounds                                                                                                                                                                                      |
| ------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `Document.Version`                               | 1 or 2; selects its exact particle capability and accepted material enum.                                                                                                                               |
| `Document.Presets`                               | 1–64 uniquely named presets. All presets are validated, including unused ones.                                                                                                                          |
| `Document.Emitters`                              | 1–256 unique expanded emitter IDs; each refers to a preset.                                                                                                                                             |
| `Preset.Name` / `Emitter.ID`, `Preset`           | Nonempty UTF-8 identifiers, at most 128 bytes.                                                                                                                                                          |
| `Preset.Capacity`                                | 1–4,096 slots per emitter; sum of capacities of all placed emitters is at most 65,536.                                                                                                                  |
| `Preset.Emission`                                | `Rate` 0–4,096/second, `DurationSeconds` 0–3,600; up to 16 `Bursts`, each `AtSeconds` 0–3,600 and `Count` 1–4,096. Rate/duration/bursts default zero/zero/nil. Every schedule has 1–4,096 total births. |
| `Preset.LifetimeSeconds`                         | `Min` and `Max` in 0.001–3,600 seconds, with minimum no greater than maximum.                                                                                                                           |
| `Preset.Spawn`                                   | `Shape` is `point` or `box`. `HalfExtent` components are 0–1,000,000. Point requires zero extent. Box samples axes independently within the local half extent.                                          |
| `Preset.VelocityMin`, `VelocityMax`              | Per-axis independent uniform velocity range, −1,000–1,000 world units/second, minimum no greater than maximum. Default zero.                                                                            |
| `Preset.Acceleration`                            | Constant local acceleration components −1,000–1,000 world units/second²; default zero.                                                                                                                  |
| `Preset.Drag`                                    | Linear damping 0–1,000 per second; default zero.                                                                                                                                                        |
| `Preset.Sprite`                                  | 1–16 nonzero texture IDs in `Frames`, `FPS` 0–1,000, `Playback` once/cycle, `Facing` camera-facing/upright, `Blend` additive/cutout; v2 also accepts alpha/premultiplied. All enums must be explicit.   |
| `Preset.SizeOverLife`                            | 2–4 `ScalarKey` values; normalized `Age` 0–1 and square edge length `Value` 0–1,000 world units.                                                                                                        |
| `Preset.ColorOverLife`                           | 2–4 `ColorKey` values; normalized `Age` 0–1, exactly four RGBA components in 0–1.                                                                                                                       |
| `Preset.RotationDegrees`, `RotationSpeedDegrees` | Minimum/maximum ranges −360,000–360,000 degrees or degrees/second around the billboard normal. Default zero.                                                                                            |
| `Emitter.Sector`                                 | Index into an already validated compiled world's sectors. `ValidateForWorld` safely rejects missing/out-of-range indices on 32-bit and 64-bit hosts.                                                    |
| `Emitter.Position`                               | World coordinates, each −1,000,000–1,000,000. Source prefab composition has already been applied.                                                                                                       |
| `Emitter.YawDegrees` / `Scale`                   | Compiled yaw −180–180 degrees; uniform scale 0.000001–1,000. Source omission or zero scale becomes 1 before compiling; runtime scale cannot be zero.                                                    |
| `Emitter.Seed`                                   | Unsigned 32-bit deterministic variation seed; zero is valid.                                                                                                                                            |
| `Emitter.Playback`                               | `Mode` once/cycle; `DelaySeconds` 0–3,600, default zero; `PeriodSeconds` zero for once or 0.001–3,600 for cycle.                                                                                        |
| `Emitter.Bounds`                                 | Compiler-derived full motion/billboard envelope. Stored values must equal `ConservativeEmitterBounds` exactly and remain inside coordinate limits.                                                      |

All numbers must be finite. Units are world distance and seconds with Z up.
Lifetimes and each velocity axis are sampled uniformly. Size/colour curves are
piecewise linear. Required YAML fields are described by the source schema;
`Validate` operates on Go values and cannot detect omitted text fields.

Rate births occur at `n/rate` in `[0,duration_seconds)`; timed bursts add births.
Counting checks the actual float64 offsets at the excluded endpoint before
float32 conversion, avoiding an extra birth caused by product rounding. A
positive rate always includes a birth at zero, even for a tiny rate/duration.
`BirthSchedule` produces their stable sorted float32 schedule. An emitter
plays `once`, or cycles with a positive `period_seconds`; all cycling births
must occur strictly before the uploaded float32 period, and emission duration
must not exceed the period. `delay_seconds` postpones the first emission.
Once emission drains naturally. The emitter clock belongs to its mounted level,
and visibility changes, other cameras and resize do not restart it.

A cycling schedule reserves `groups * birth_count` slots. `CycleGroups` computes
`max(1, ceil(float32(max_lifetime) / float32(period)))` using a promoted float64
ratio so uploaded rounding cannot cause early reuse. Rendering consumes that
immutable group count. `RequiredCapacity` rejects a preset too small for any
emitter; spare slots remain inactive. Birth indices and emitter seed select
repeatable particle variation without a CPU particle simulation.

For zero drag, displacement is `velocity*age + acceleration*age²/2`. Positive
drag uses the analytical integral of `dv/dt = acceleration - drag*velocity`.
For `q = drag*age < 0.5`, shaders and bound evaluation use fifth-order Horner
polynomials for the integral factors to avoid cancellation:

```text
f = age * (1 - q/2 + q²/6 - q³/24 + q⁴/120 - q⁵/720)
g = age² * (1/2 - q/6 + q²/24 - q³/120 + q⁴/720 - q⁵/5040)
displacement = velocity*f + acceleration*g
```

For larger q, the analytical exponential integral applies. Derived bounds
include the polynomial approximation and float32/error margins.
The emitter yaw rotates local spawn/movement/acceleration around Z, and its
positive uniform scale applies to displacement and billboard size. Position
is already in world coordinates. Compiled yaw is normalized to `[-180,180]`.

## Bounds and budgets

`ConservativeEmitterBounds` includes every lifetime, spawn/velocity endpoint,
trajectory extrema, yaw, scale, maximum billboard radius and a float32/error
margin. `Validate` requires the stored bounds to equal recomputed bounds.
`ValidateForWorld` additionally checks every envelope corner against an already
validated compiled convex sector's walls, floor and ceiling. An effect's entire
motion envelope must fit one sector in version 1; particles do not cross portals
or collide with static solids. Emitters in concave authored rooms may require
smaller spawn regions after decomposition into convex sectors.

Limits are 64 presets, 256 emitters, 4,096 slots per emitter, 65,536 total slots,
16 frames per preset, 16 timed bursts, 4 curve keys, and 1 MiB encoded bytes.
Lifetime, emission duration, cycle period and delay are bounded to 3,600 seconds.
Public validation runs before GPU resource allocation. Runtime resource
preparation separately proves complete texture identity and frame compatibility.
Renderer-specific atlas layout is intentionally outside the public binary
contract. The current engine keeps connected multi-frame groups on a 2,048² page
with two pixels of total padding per frame dimension. Shared frames connect
presets into a larger group; oversized singleton textures can use standalone
pages within renderer storage bounds. Packing failure is rejected before readiness.

Version 1 is presentation-only and provides automatic authored playback.
Gameplay emitter control, simulation history, collisions, general force fields,
smooth alpha/OIT and cross-sector movement require later versioned work.

## Version 2 alpha blending

Version 2 retains the `KWP1` binary layout, common binary header, fields,
resource limits, schedules and analytical motion. Its document version is 2,
and each preset additionally accepts `sprite.blend: alpha` or `premultiplied`. The exact required
cartridge/SDK feature is `world/particles@2`, alongside `world/sectors@1`.
Version 2 does not implicitly declare version 1; a cartridge containing levels
of both versions advertises both features.

Alpha particles use premultiplied source-over blending, test opaque depth and
leave depth unchanged. The engine draws this pass after cutout particles,
ordinary scene transparency and additive particles. Alpha emitters follow
stable authored submission order; individual particles are not sorted and
there is no order-independent transparency. Intersecting coloured translucent
layers therefore have approximate ordering. A constant charcoal RGB with
varying opacity works well for smoke because overlapping same-colour layers
commute. Alpha smoke can attenuate additive fire drawn behind it in this pass.

`premultiplied` shares this pass and its ordering/depth rules. For straight-alpha
texture sample `T` and lifetime colour `C`, its output is RGB
`T.rgb * T.a * C.rgb`, alpha `T.a * C.a`. Ordinary `alpha` also multiplies RGB
by `C.a`. Premultiplied particles can therefore add RGB at zero lifetime opacity,
while texture alpha continues to confine their support. RGB and opacity curves
remain independently authored 0–1 values; no HDR range or additional layer is
required for a combined flame/smoke material. Input textures remain straight-alpha.

A single static frame can be reused for every particle in a layer. Births,
seeded variation, movement, rotation, size, colour and opacity remain animated
from the GPU lifecycle; version 2 does not require an actor, flipbook or guest
per-particle updates. Existing multi-frame support remains available in the
format. No new simulation fields, CPU particle state or allocation limits are
introduced. `ValidatePreset` remains a strict version-1 helper;
`ValidatePresetForVersion` validates either contract explicitly.
