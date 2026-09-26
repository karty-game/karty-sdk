# Karty SDK

**The public contract between Karty tools and the game engine.**

This repository contains the cartridge and level formats used by the Karty CLI
and the private engine. Neither needs to import source from the other.

- `format/cartridge`: game metadata, embedded assets, and Wasm custom sections.
- `format/level`: versioned level envelopes and modules.
- **Releases**: SDK bundles, native hosts, browser Wasm, checksums, and signatures
  produced and validated by the private engine workflow.

Game developers use these artifacts through [Karty](https://github.com/karty-game/karty).
Engine source access and GitHub credentials are not required to download them.

```sh
mise run test
mise run build
```

Go module tags (`vVERSION`) version the format packages. Engine artifact tags
(`sdk-vVERSION`) version downloadable SDKs and hosts independently. The engine
publishes assets here; this repository does not build private engine source.

The initial public module tag `v0.0.1` still needs to be published. No remote
repository, commits, tags, or releases were created by this local extraction.

[Contributing](CONTRIBUTING.md) · [MIT license](LICENSE.md)

The MIT license covers this repository’s source. Official engine runtime binaries
are covered by the separate [Karty Runtime License](RUNTIME_LICENSE.md), allowing
distribution with free and commercial games. Exported SDK bindings and templates
are MIT-licensed; private engine implementation source remains proprietary.
