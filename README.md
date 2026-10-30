# Karty SDK

The public contracts shared by the Karty CLI, public compilers and game engine.
This Go module builds independently of engine source and credentials.

| Package                                                      | Responsibility                                                                  |
| ------------------------------------------------------------ | ------------------------------------------------------------------------------- |
| [`format/cartridge`](format/cartridge/README.md)             | Game manifests, embedded assets, Wasm sections and streaming media catalogs     |
| [`format/level`](format/level/README.md)                     | Versioned level envelopes and modules                                           |
| [`format/worldreflection`](format/worldreflection/README.md) | Saved cubemaps, projection volumes and explicit receiver bindings               |
| [`format/worldchunk`](format/worldchunk/README.md)           | Independent render chunks, offline meshes and conservative streaming visibility |
| [`format/actions`](format/actions/README.md)                 | Authored sequences, action literals and level-scoped actor references           |
| [`format/world`](format/world/README.md)                     | Canonical compiled convex-sector worlds, lighting, mapping and static solids    |
| [`format/worldvegetation`](format/worldvegetation/README.md) | Seeded per-sector vegetation layers, cutout presets and visibility bounds       |
| [`format/worldparticles`](format/worldparticles/README.md)   | Bounded GPU sprite particle presets, schedules and emitter motion envelopes     |
| [`format/worldsky`](format/worldsky/README.md)               | Project-owned sky textures and bounded presentation settings                    |
| [`format/worldsource`](format/worldsource/README.md)         | Separately versioned room, prefab, port and content authoring semantics         |
| [`format/textureatlas`](format/textureatlas/README.md)       | Build-time image pages, logical regions and filtered foliage mip chains         |
| [`format/worldmaterial`](format/worldmaterial/README.md)     | Material atlases, mip tails and strength controls                               |
| [`format/worldlightmap`](format/worldlightmap/README.md)     | Static receiver charts, bake recipes and completed prebake formats              |
| [`bake/worldlightmap`](bake/worldlightmap/README.md)         | Deterministic CPU direct/diffuse RNM baking                                     |
| [`format/performance`](format/performance/README.md)         | Binary adaptive presentation profiles                                           |
| `codec/qoi`, `codec/qoa`                                     | Bounded adapters around pinned image and audio codecs                           |

The SDK owns formats and shared validation. Public compilers own authoring
parsing and expansion; the CLI owns project building and asset processing; the
engine owns runtime execution and graphics. Published package paths remain
stable. Detailed contracts live beside their owning packages.

## Development

Install [mise](https://mise.jdx.dev/), then run from this repository root:

```sh
mise install
mise run check-fmt
mise run test
mise run build
```

`mise install` installs pinned tools and the repository pre-commit hook. Use
`mise run fmt` to apply formatting and `mise run lint` for the shared lint checks.
See [CONTRIBUTING](CONTRIBUTING.md) for setup, codec regeneration, compatibility
requirements and the race, SIMD fallback, 32-bit, Wasm, fuzz and benchmark checks.

## Versions and releases

Go module tags (`vVERSION`) pin immutable public package dependencies. Engine
artifact tags (`sdk-vVERSION`) version downloadable SDK bundles and hosts
independently. Capability declarations govern host support separately from
these release identifiers.
Compiled worlds support versions 1–3, and authoring documents support versions
1–20. New optional features retain their explicit version gates and canonical
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

Source 14 adds built-in shrub geometry through vegetation version 2 (`KWV2`,
`world/vegetation@2`); Source 13 / KWV1 remain supported.

Source 15 adds optional `normal_texture` vegetation resources through version 3
(`KWV3`, `world/vegetation@3`); earlier vegetation versions remain supported.

Source 16 adds `geometry: tree` through vegetation version 4 (`KWV4`,
`world/vegetation@4`), retaining previous vegetation versions.

Source 17 adds explicit liquid volumes (`world/fluids@1`, KWR3), retaining KWR1/KWR2 worlds without fluid definitions.

Source 18 replaces prototype environment styles with explicit authored lighting and optional sky colors (`world/environment@2`, KWR4). Released environment inputs remain readable.

Compiled terrain is an optional `world/terrain@1` sidecar. Its immutable
heightfield, local four-material palettes, normal maps and sector floor ownership
are defined in [the terrain contract](format/worldterrain/README.md). Source
version 18 remains unchanged.

Source 20 adds `geometry: palm` through vegetation version 5 (`KWV5`,
`world/vegetation@5`), retaining all previous source and vegetation versions.
