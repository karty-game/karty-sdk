# Validation

## Current state

The format-module candidate is **v0.0.20**. Character resources and the current
lighting/streaming formats are maintained contracts, not private experiments.
Public module availability and exact-commit CI remain separate release gates.

Use the pinned tasks from the repository root. The latest recorded complete
checks below passed native tests/build/lint, race detection, SIMD fallback,
actual Linux 386 tests, actual Go/Wasm execution and fuzz exploration. Unit,
race, fallback, 386 and Wasm workloads execute once with `-count=1`, Go's standard
ten-minute hang watchdog and fifteen-minute CI job watchdogs. The JavaScript
runtime uses `GOMAXPROCS=1`; native checks use the documented worker settings.
No automatic retry or package batching is part of this policy.

The dated records retain the commands and initial failures observed at the time.
Older nine-second command examples describe historical runs, not the current
execution policy. Documentation/tooling cleanup preserves production Go source,
format versions, fixtures, assertions and allocation bounds.

## Fixed chunk lighting candidate, 2026-10-23

- `GOCACHE=/tmp/karty-engine-go-cache mise exec -- go test -timeout=9s ./bake/worldlightmap ./format/worldlightmap ./format/worldchunk ./format/worldterrain ./format/cartridge -run '^TestFixed' -count=1` passed. Covers combined sun/lamp transport, deterministic workers, direct/ambient accounting, bounded KFL1 admission, KCC3 reservations and terrain coefficients/identity.
- `GOCACHE=/tmp/karty-engine-go-cache GOMAXPROCS=2 mise run test` passed, including unchanged producer compatibility fixtures and source-policy checks (23.99 s total).
- `GOCACHE=/tmp/karty-engine-go-cache GOMAXPROCS=2 mise run build` passed.
- Final `GOCACHE=/tmp/karty-engine-go-cache GOLANGCI_LINT_CACHE=/tmp/karty-engine-lint-cache GOMAXPROCS=2 mise run lint` passed with zero issues. The first invocation found one formatting issue, a redundant assignment in `PlaneAt`, an unused initial range assignment and an overwritten test fixture binding; these were corrected before the successful run.
- Final `GOCACHE=/tmp/karty-engine-go-cache GOMAXPROCS=2 mise run test` passed after the terrain plane, authored fill and malformed KFL1 fixtures (22.59 s total).
- Final `GOCACHE=/tmp/karty-engine-go-cache GOMAXPROCS=2 mise run build` passed.

These are native CPU package tests/builds. They establish no browser graphics,
actual WASM execution, physical-device frame rate or public tag availability.
The final full run includes `PlaneAt`, constant/directional authored fill,
source-record proof, malformed scalar/count rejection and decoded ownership.

The engine bundle admission check subsequently exposed a missing registration in
`format/asset` for `world/fixed-lighting@1`. Added the versioned asset capability
and unsupported/duplicate capability coverage. `GOCACHE=/tmp/karty-engine-go-cache
GOWORK=off GOMAXPROCS=2 mise run test` passed (25.52 s), and the same environment
with `mise run build` passed. Focused `golangci-lint run ./format/asset` passed
with zero issues using the warmed lint cache. The candidate archive was refreshed
after this correction; no public module was published.

## Stored-precision sloped receiver junctions, 2026-10-23

A representative terrain build exposed overlapping fan wedges around a sloped
solid corner: nearby float64 junctions rounded to the same XY and alternating
adjacent float32 Z values. The compiler now subdivides packed edge endpoints,
retaining the original authored corners. Mesh validation stays unchanged.

- `GOCACHE=/tmp/karty-engine-go-cache GOMODCACHE=/tmp/karty-fixed-final-modcache GOMAXPROCS=2 GOWORK=off mise exec -- go test ./format/worldchunk -count=1 -timeout=9s` passed (1.031 s).
- The maintained `TestMeshCompilerSubdividesStoredSlopedCorner` retains all four original corners and accepts complete coverage while rejecting a missing wedge. A temporary overlay restoring the old interpolation failed this regression with six wedges instead of four; the corrected implementation passes. Existing one-ULP receiver strips and malformed coverage tests remain intact.
- An ignored diagnostic overlay compiled a representative CLI terrain source to a canonical world and planned every sector with `Options{NoLightmap: true, MaxSectors: 1}` against the corrected SDK; it passed (0.278 s). Maintained tests depend only on the small SDK-owned numerical fixture, not that project or overlay.
- `GOCACHE=/tmp/karty-engine-go-cache GOMODCACHE=/tmp/karty-fixed-final-modcache GOLANGCI_LINT_CACHE=/tmp/karty-engine-lint-cache GOMAXPROCS=2 GOWORK=off mise run check` passed: formatting, full lint with zero issues, full module tests (23.12 s), and build. The initial invocation stopped at formatting because the new regression needed the pinned formatter's argument wrapping; `mise exec -- golangci-lint fmt format/worldchunk/compile.go format/worldchunk/compile_junctions.go format/worldchunk/mesh_coverage_precision_test.go` corrected it before the successful rerun. Existing test deadlines were unchanged.

These checks execute native CPU code. The corrected SDK source must be exported
to a refreshed frozen local candidate and consumers revalidated before their
previous artifact identities can describe this change. No archive, tag, signed
asset or public release was created by these checks.

## Terrain physical sample identity, 2026-10-23

`TestFixedTerrainSamplingIsStableAcrossPatchesAndCrops` failed before the fix:
the shared vertex at `(2, 0, 1)` received different nine-coefficient RNM values
from its two patch-local vertex indices under an asymmetric sky/occluder. The
fixed producer now derives terrain sample rotation from physical position and
normalized normal, preserving the authored seed. Revision `cpu-fixed-rnm3@3`
enters the existing fixed input/cache digest; completed `@1` and `@2` artifacts
remain readable without relabeling. No wire layout or runtime path changed.

With `GOCACHE=/tmp/karty-engine-go-cache GOMAXPROCS=2 GOWORK=off`:

- `mise exec -- go test ./bake/worldlightmap ./format/worldlightmap -run '^(TestFixedTerrain|TestFixedProducerRevision|TestProducerCompatibility|TestFixedLiquid)' -count=1 -timeout=9s` passed (baker 0.147 s, format 0.007 s). Coverage includes exact shared coefficients across whole, row/column partitioned and cropped receivers using the same full transport; unchanged atlas/probe output; source/seed response; covered black; signed zero; a stable sample-index golden; zero sample-identity allocations; and old producer readability.
- `GOARCH=386 CGO_ENABLED=0 mise exec -- go test ./bake/worldlightmap -run '^TestFixedTerrain(Physical|Sampling)' -count=1 -timeout=9s` passed with actual 32-bit execution (0.341 s) outside the sandbox. The initial sandbox attempt failed immediately with `signal: bad system call`; no test assertion or deadline changed.
- `mise exec -- golangci-lint run --fix ./bake/worldlightmap ./format/worldlightmap` passed with zero issues using `GOLANGCI_LINT_CACHE=/tmp/karty-engine-lint-cache`.
- `mise run test` passed (23.14 s), including the unchanged ordinary producer compatibility fixtures and source policy.
- `mise run build` passed.

Pinned Go formatting, README/validation Markdown formatting and `git diff
--check` passed. These checks execute native CPU code; they do not establish
browser graphics, WASM execution, device frame rate or published availability.
Consumers require a newly frozen candidate archive and rebuilt bake caches.

## Spatial surface receivers and streaming contracts, 2026-10-24

The optional `world/spatial-chunks@1` contract adds separately typed spatial
render children, complete root face ownership, original primary collision Ready
streams, and a full-source terrain fingerprint. Selected lightmap and semantic
mesh envelopes clip the same faces before packing/emission; old format/page
rules remain unchanged. Small selected pages preserve the requested local
density. No constant/corner lighting representation was added.

Spatial fixed producer `cpu-fixed-spatial-rnm3@1` omits actor probes, uses physical
sample identity, and preserves matching tile-edge estimates under off/low/medium
denoise. The selected water compiler uses shared world-grid boundary anchors and
interior rings/fans, including unequal tile sizes and geometry detail policies.
The finite grid is a geometry quality policy, not a claim of bounded visual
error for every possible authored wave frequency.

With the writable warmed environment from `/tmp/karty-visual-seams-env.sh`,
`GOCACHE=/tmp/karty-engine-go-cache`, `GOMAXPROCS=2` and `GOWORK=off`:

- `mise exec -- go generate ./format/world` regenerated typed codecs from the
  handwritten generator. Existing binary compatibility tests remain intact.
- `mise exec -- go test -timeout=9s ./format/worldlightmap ./format/worldchunk
./bake/worldlightmap ./format/cartridge ./format/asset` passed. New maintained
  tests cover selected density, exact clipped semantic/Ready coverage, canonical
  wire round trips, malformed/truncated payloads, old version rejection, missing
  source ownership, gaps, overlaps, collision reservations, exact terrain
  ownership/fingerprint, far geometry policy, shared coarse/fine water anchors,
  stable UV/wave coordinates, no duplicate spatial probes, matching lightmap
  boundary samples with every denoise mode, and covered black versus uncovered.
- `mise exec -- golangci-lint run ./format/worldchunk ./format/worldlightmap
./bake/worldlightmap ./format/cartridge ./format/asset
./internal/codegen/binary` passed with zero issues after the pinned formatter
  wrapped two new long conditions and the test variable named `cap` was renamed.
- `mise run test` passed with unchanged nine-second package deadlines (23.46 s
  for the final spatial/ring source). Earlier full passes were 23.22 s and
  21.74 s before the final selected-water geometry refinement.
- `mise run build` passed for the final spatial/ring source.
- Pinned Markdown formatting and `git diff --check` passed.

These are native CPU format/producer/build checks. Actual AMD browser/native
rendering, upload/residency, camera streaming and device FPS are separate engine
and CLI validations. No public SDK tag, archive or published availability is
established by these local source checks.

## Enclosure planning policy v1, 2026-10-24

With `/tmp/karty-spatial-env.sh` (writable warmed Go/lint caches),
`GOMAXPROCS=2` and `GOWORK=off`:

- `mise exec -- go test -timeout=9s ./format/worldchunk -run 'TestEnclosure' -count=1 -v` passed after correcting fixture-only errors (unsupported 128 page size and sky without environment).
- `mise exec -- go test -timeout=9s ./format/worldchunk -count=1` passed (1.199 s).
- `mise exec -- golangci-lint run ./format/worldchunk` passed, zero issues.
- `mise run test` passed (24.41 s); `mise run build` passed.

Coverage preserves legacy ordering, deterministic whole-enclosure growth,
explicit/physical boundaries, mesh/encoded/sector limits, requested lightmap
density and canonical enclosure identity through splits and compaction.
These are native CPU checks. Consumer sample measurements, candidate hashes
and integration results belong in the consuming repositories. No runtime
probe ownership, browser graphics, device FPS or public release is established.

The same policy now includes optional `MaxSpan`: both horizontal render bounds
are checked during growth and whole-chunk compaction, including the prepared
mesh-only path. SDK zero retains compatibility. New tests cover combining
complete neighboring enclosures, exact-boundary acceptance, budget-driven splits,
nonfinite/negative limits and oversized single-sector rejection in both modes.
Final `mise exec -- go test -timeout=9s ./format/worldchunk -count=1` passed
(1.161 s). With the same environment, final `mise exec -- golangci-lint run
./format/worldchunk`, `mise run test` and `mise run build` all passed. The CLI
sets a positive configurable 48 metre default and separately checks spatial and
terrain render resources before baking.

## Low profile reflections — 2026-10-24

The default Low profile now enables reflections while retaining its 0.75 render
scale. Authored overrides still apply. After checking the warmed writable
`/tmp/karty-engine-go-cache`, commands from this repository root with
`source /tmp/karty-saved-probes-final-env.sh` and `GOWORK=off`:

- `mise run test`: passed in 21.25 s, including the existing performance binary
  normalization and round-trip coverage.
- `mise run build`: passed.

These commands validate SDK packages and compilation. The matching Hangar
preview uses an explicit authored Low override so the previously frozen CLI
candidate can encode the same policy without modifying its immutable dependency
archive. Engine validation records actual browser execution separately.

## Exact compact fixed spatial lighting — 2026-10-24

With Go 1.27.3, `GOWORK=off`, `GOMAXPROCS=2` and the writable warmed
`GOCACHE=/tmp/karty-engine-go-cache`, from the SDK root:

- `mise exec -- go test ./format/worldlightmap ./format/cartridge ./format/asset ./bake/worldlightmap -run 'TestCompactFixed|TestFixedFormat|TestFixedProducer' -count=1`
  passed: worldlightmap 0.029 s, cartridge 0.004 s, worldlightmap baker 0.005 s;
  asset had no matching tests. The initial invocation caught an invalid integer
  range variable in a newly added test; corrected before these passing runs.
- With `GOLANGCI_LINT_CACHE=/tmp/karty-engine-lint-cache`,
  `mise exec -- golangci-lint run --new-from-patch=/tmp/karty-sea-sdk/owned-review.patch ./format/worldlightmap ./format/cartridge ./format/asset ./bake/worldlightmap`
  passed with zero issues. This was a local changed-line review, not the full
  module gate; the temporary patch is not a repository dependency.

Coverage includes black-versus-missing lighting, all three lobes and interior
variation, independent chart colors, malformed image backing storage and extreme
strides, bounded canonical KFL2 decoding, finite coefficients, float32-rounded
range admission, required feature dependencies, and legacy producer admission.
Independent consumer review caught the 48-byte retained ChartRNM alignment and
the nonexact float64-to-float32 range bound before these final focused checks.
These checks do not establish GPU parity, mobile frame rate, or final sample
counts. Full module test/build results and actual consumer graphics validation
are recorded separately after the shared candidate source is frozen.

After the complete grouping/compact candidate stabilized, serialized root
validation ran `mise run test` (passed, 27.13 s) and `mise run build` (passed).
The follow-up scoped command
`mise exec -- golangci-lint run --new-from-rev=HEAD ./format/worldlightmap ./format/worldchunk ./bake/worldlightmap ./format/cartridge ./format/asset`
initially found two style issues: a long selected-catalog encoder expression and
an exported `LiquidTile` comment accidentally attached to the prior
`SurfaceSelection` documentation. Formatting that expression and restoring each
type's own comment changed no semantics. The same lint command then passed with
zero issues. This includes the untracked candidate sources; no exclusions or
assertions were weakened. SDK source was frozen after these checks for isolated
consumer export. GPU execution and mobile performance remain separate gates.

## Compiled character resources — 2026-10-30

The new `format/character` owns bounded KCHR v1 models, shared named animation
banks, canonical binary encoding and independent decoded storage. The runtime
capability and cartridge feature registries accept `world/characters@1`, with a
required `world/sectors@1` cartridge dependency. SDK inputs remain an unreleased
candidate; these checks do not establish public module availability.

After checking the writable warmed `/tmp/karty-engine-go-cache`, serialized
commands from the SDK root used Go 1.27.3, the pinned SIMD experiment, and two
workers:

```sh
MISE_ACTIVATE_AGGRESSIVE=1 GOTOOLCHAIN=go1.27.3 GOCACHE=/tmp/karty-engine-go-cache GOMAXPROCS=2 GOWORK=off mise run test
MISE_ACTIVATE_AGGRESSIVE=1 GOTOOLCHAIN=go1.27.3 GOCACHE=/tmp/karty-engine-go-cache GOMAXPROCS=2 GOWORK=off mise run build
```

Both passed; the complete test task took 27.95 seconds, including the uncached
source-policy check. Earlier focused `mise exec -- go test -timeout=9s
./format/asset ./format/cartridge ./format/character` passed with the same Go,
cache and worker settings. Focused lint of those three packages passed with zero
issues. Its initial package load selected older Nix Go and failed with `unknown
GOEXPERIMENT simd`; selecting the pinned compiler after mise fixed package
loading:

```sh
mise exec -- env PATH=/home/kef/.local/share/mise/installs/go/1.27.3/bin:/usr/bin:/bin \
  GOROOT=/home/kef/.local/share/mise/installs/go/1.27.3 GOTOOLCHAIN=go1.27.3 \
  GOCACHE=/tmp/karty-engine-go-cache GOLANGCI_LINT_CACHE=/tmp/karty-engine-lint-cache \
  GOMAXPROCS=2 GOWORK=off \
  /home/kef/.local/share/mise/installs/golangci-lint/2.15.0/golangci-lint-2.15.0-linux-amd64/golangci-lint \
  run ./format/asset ./format/cartridge ./format/character
```

With `GOTOOLCHAIN=go1.27.3 GOCACHE=/tmp/karty-engine-go-cache GOMAXPROCS=2
GOWORK=off`, `mise exec -- go test -timeout=9s ./format/character -run='^$'
-fuzz=FuzzDecode -fuzztime=5s` passed 231,693 executions and found 29 additional
interesting inputs. The SDK fuzz task now includes this target.

Coverage includes canonical round trips, complete ownership, every truncated
prefix, trailing bytes, malformed counts, cycles and forward parent references,
missing or duplicate animation targets, invalid keys and quaternions, palette
weights and indices, singular/projective inverse binds, and malformed textures.
The largest legal 2,048-square RGBA QOI stream fits its per-image bound while
retaining the 64 MiB document cap. Conservative bind/animation hierarchy
envelopes reject accumulated scale overflow and singular scale transitions.

This is native CPU validation and module compilation. Engine pose sampling,
actual WASM execution, browser/native graphics and physical-device frame rate
are separate consumer checks. No deadline or existing assertion was extended.

The final character audit added rejection of non-sRGB QOI albedo, matching the
renderer color-space contract. After that guard and its malformed-input
regression, `MISE_ACTIVATE_AGGRESSIVE=1 GOWORK=off GOTOOLCHAIN=go1.27.3
GOCACHE=/tmp/karty-engine-go-cache GOMAXPROCS=2 mise exec -- go test -count=1
-timeout=9s ./format/character` passed in 0.061 s. The source was frozen again
before the final isolated consumer archive export.

## SDK CI execution policy — 2026-10-30 JST

[Checks run 41031598097](https://github.com/karty-game/karty-sdk/actions/runs/41031598097)
failed on main commit `2aad1ed75ff2a1f2395cbe8ceedce20a71c09827` in
`mise run test-race`. Formatting, lint, ordinary tests, build, fuzz and Wasm
passed. The race log reports a nine-second package timeout in
`bake/worldlightmap`, while `TestAbsorbingSurfacesEndIndirectPaths` had run for
only one second. It contains no data-race report. The package's combined
instrumented workload exhausted the custom aggregate timer.

The SDK tasks now use Go's standard ten-minute hang protection and explicit
fifteen-minute GitHub job watchdogs. Custom nine-second cutoffs are removed
from unit, race, SIMD fallback, native 386, Wasm, benchmark and fuzz launchers.
Race and 386 tasks execute complete packages, with the former producer/chunk
skip lists and separate invocations removed. Tests execute once using
`-count=1`; there are no automatic retries. Test assertions, fixture workloads,
race instrumentation and allocation/resource bounds are unchanged. Fuzzing
retains five seconds of actual exploration per target and its missing-target
check. An initial eight-test batching patch was discarded because it retained
the same inappropriate timing policy.

With the writable warmed `GOCACHE=/tmp/karty-engine-go-cache`, Go 1.27.3,
`GOEXPERIMENT=simd`, `MISE_ACTIVATE_AGGRESSIVE=1`, `GOWORK=off` and native
`GOMAXPROCS=2`, from the SDK root:

- `mise run test-race` passed the complete suite in one invocation. The lightmap
  baker took 5.184 s; the complete world-chunk package took 10.655 s and passed
  every assertion. The latter is legitimate work that cannot fit the former
  nine-second package policy.
- `mise run check` passed formatting, lint, the uncached native suite and build.
- `mise run test-simd-emulated` passed, including the complete baker workload.
- `mise run test-32` passed actual native Linux 386 execution, with syscall access
  outside the restricted sandbox.
- `mise run test-wasm` passed actual execution using the official Go runner and
  pinned Node. The task pins `GOMAXPROCS=1` for the single-threaded JavaScript/Wasm
  runtime. The first local invocation inherited the native two-worker setting
  and failed immediately with `newosproc: not implemented`; explicitly selecting
  the supported runtime setting fixes that environmental failure deterministically.

No production Go source or wire formats changed. Local checks do not establish
success of a new pushed-commit GitHub run. Nothing was committed or published.

The revised fuzz launcher was exercised with `mise exec -- bash scripts/go-fuzz
./format/character FuzzDecode`: 264,189 executions passed with the existing
five-second exploration budget (6.021 s including startup/cleanup). Final
`mise run check-fmt` and `git diff --check` also passed. No tests or assertions
were deleted, and the temporary batching script is absent from the final patch.
