# Karty SDK

The public contracts shared by the Karty CLI, public compilers and game engine.
This Go module builds independently of engine source and credentials.

| Package                                                  | Responsibility                                                                  |
| -------------------------------------------------------- | ------------------------------------------------------------------------------- |
| [`format/cartridge`](format/cartridge/README.md)         | Game manifests, embedded assets, Wasm sections and streaming media catalogs     |
| `format/level`                                           | Versioned level envelopes and modules                                           |
| [`format/actions`](format/actions/README.md)             | Candidate authored sequences, action literals and level-scoped actor references |
| [`format/world`](format/world/README.md)                 | Canonical compiled convex-sector worlds, lighting, mapping and static solids    |
| [`format/worldsource`](format/worldsource/README.md)     | Separately versioned room, prefab, port and content authoring semantics         |
| [`format/worldmaterial`](format/worldmaterial/README.md) | Material atlases, mip tails and strength controls                               |
| [`format/worldlightmap`](format/worldlightmap/README.md) | Static receiver charts, bake recipes and completed prebake formats              |
| [`bake/worldlightmap`](bake/worldlightmap/README.md)     | Deterministic CPU direct/diffuse RNM baking                                     |
| `codec/qoi`, `codec/qoa`                                 | Bounded adapters around pinned image and audio codecs                           |

The SDK owns formats and shared validation. Public compilers own authoring
parsing and expansion; the CLI owns project building and asset processing; the
engine owns runtime execution and graphics. Published package paths remain
stable. Detailed contracts live beside their owning packages.

## Development

Install the pinned tools with [mise](https://mise.jdx.dev/), then run from this
repository root:

```sh
mise install
mise run fmt
mise run check-fmt
mise run lint
mise run test
mise run build
mise run test-race
mise run test-simd-emulated
mise run test-32
mise run test-wasm
mise run fuzz
mise run bench
```

The shared checks in `hk.pkl` use golangci-lint for Go (including `govet`),
yamllint for YAML, Taplo for TOML, and Prettier for YAML layout, JSON, Markdown
and web files. Taplo validates syntax without downloading schemas.

`mise install` installs the pinned tools and repository pre-commit hook through
mise; use `mise run install-hooks` to reinstall it. The hook checks staged files
and applies formatting fixes while preserving unstaged work. `fmt` applies all
formatters through hk without staging; `check-fmt` reports differences without
writing files. `lint` runs all format and lint checks; `check` also runs tests
and build. Generated Go and `*.generated.*` snapshots, derived output and local
contributor directories and canonical `*.world.json` fixtures are excluded from
formatting. The JSON-compatible YAML fixture retains its decoder input format;
yamllint still checks it.

After changing `format/actions/schema.json`, regenerate consumer snapshots with
their `mise run generate-action-contract` tasks; public SDK checks remain
independent of private source.

The pinned mise environment enables Go 1.27 portable SIMD for the offline
baker. Direct Go builds importing that package require `GOEXPERIMENT=simd`;
format-only imports are unaffected. `test-simd-emulated` exercises Go's built-in
fallback rather than a second production implementation.

`test-32` executes native 386 tests on an x86 Linux host with 32-bit execution
support. `fuzz` explores each target for a bounded budget; normal tests also run
the seed corpus. `test-wasm` executes Go Wasm tests using the official Go runner
and pinned Node, running one package at a time to bound memory use. Its launcher
passes a minimal environment to stay within Go's
8 KiB argument/environment limit, preserving temporary paths and the `GODEBUG`
and `GOMAXPROCS` runtime controls. It does not exercise browser graphics.
Benchmarks report allocation totals and CPU time for fixed
codec, material-atlas, connected-world and bake fixtures. See
[CONTRIBUTING](CONTRIBUTING.md) for compatibility and review requirements.

## Versions and releases

Go module tags (`vVERSION`) pin immutable public package dependencies. Engine
artifact tags (`sdk-vVERSION`) version downloadable SDK bundles and hosts
independently. SDK 0.0.8 contracts are documented as available features, rather
than unfinished proposals; capability declarations still govern host support.
Compiled worlds support versions 1–3, and authoring documents support versions
1–6. New optional features retain their explicit version gates and canonical
encoding rules. Incompatible formats or producer behavior need a versioned
decision and compatibility coverage.

The private engine workflow publishes SDK bundles, native hosts, browser Wasm,
checksums and signatures here. This repository does not build private engine
source. Game developers obtain these artifacts through
[Karty](https://github.com/karty-game/karty), without engine access or credentials.

## Licensing

The [MIT license](LICENSE.md) covers this repository's source, exported SDK
bindings and templates. The pinned codec dependencies have their own MIT notices
in [`codec/qoi`](codec/qoi/NOTICE.md) and [`codec/qoa`](codec/qoa/NOTICE.md).
Official runtime binaries use the separate
[Karty Runtime License](RUNTIME_LICENSE.md), which allows distribution with free
and commercial games; private engine implementation source remains proprietary.
