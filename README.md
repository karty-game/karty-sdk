# Karty SDK

The public contracts shared by the Karty CLI, public compilers and game engine.
This Go module builds independently of engine source and credentials.

| Package                                                      | Responsibility                                                                  |
| ------------------------------------------------------------ | ------------------------------------------------------------------------------- |
| [`format/cartridge`](format/cartridge/README.md)             | Game manifests, embedded assets, Wasm sections and streaming media catalogs     |
| [`format/level`](format/level/README.md)                     | Versioned level envelopes and modules                                           |
| [`format/character`](format/character/README.md)             | Bounded skinned models, named animation tracks and shared clip banks            |
| [`format/worldreflection`](format/worldreflection/README.md) | Saved cubemaps, projection volumes and explicit receiver bindings               |
| [`format/worldchunk`](format/worldchunk/README.md)           | Independent render chunks, offline meshes and conservative streaming visibility |
| [`format/actions`](format/actions/README.md)                 | Authored sequences, action literals and level-scoped actor references           |
| [`format/world`](format/world/README.md)                     | Canonical compiled convex-sector worlds, lighting, mapping and static solids    |
| [`format/worldterrain`](format/worldterrain/README.md)       | Compiled heightfields, terrain ownership and resident mesh bounds               |
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
Compiled world documents support versions 1–3, independently of their KWR1–KWR4
binary envelopes; authoring documents support versions 1–20. New optional
features retain their explicit version gates and canonical encoding rules. Incompatible formats or producer behavior need a versioned
decision and compatibility coverage.

The private engine workflow publishes SDK bundles, native hosts, browser Wasm,
checksums and signatures here. This repository does not build private engine
source. Game developers obtain these artifacts through
[Karty](https://github.com/karty-game/karty), without engine access or credentials.

## Source and resource versions

Source versions retain the contracts of earlier documents. Optional resources
use their own capability and binary versions; see each package's contract.

| Source | Addition                                     | Resource capability               |
| ------ | -------------------------------------------- | --------------------------------- |
| 13     | Per-room vegetation layers                   | `world/vegetation@1` (`KWV1`)     |
| 14     | Built-in shrub geometry                      | `world/vegetation@2` (`KWV2`)     |
| 15     | Optional vegetation normal textures          | `world/vegetation@3` (`KWV3`)     |
| 16     | Tree geometry                                | `world/vegetation@4` (`KWV4`)     |
| 17     | Explicit liquid volumes                      | `world/fluids@1` (`KWR3`)         |
| 18     | Authored environment lighting and sky colors | `world/environment@2` (`KWR4`)    |
| 19     | Compact rectangular rooms                    | Existing compiled world contracts |
| 20     | Palm geometry                                | `world/vegetation@5` (`KWV5`)     |

Compiled terrain is an optional `world/terrain@1` sidecar, independent of the
source document version. Its heightfield, local material palettes, normal maps
and sector floor ownership are defined in [the terrain contract](format/worldterrain/README.md).

## Licensing

The [MIT license](LICENSE.md) covers this repository's source, exported SDK
bindings and templates. The pinned codec dependencies have their own MIT notices
in [`codec/qoi`](codec/qoi/NOTICE.md) and [`codec/qoa`](codec/qoa/NOTICE.md).
Official runtime binaries use the separate
[Karty Runtime License](RUNTIME_LICENSE.md), which allows distribution with free
and commercial games; private engine implementation source remains proprietary.
