# Karty SDK

**The public contract between Karty tools and the game engine.**

This repository contains the cartridge and level formats used by the Karty CLI
and the private engine. Neither needs to import source from the other.

- `format/cartridge`: game metadata, embedded assets, and Wasm custom sections.
- `format/level`: versioned level envelopes and modules.
- `format/world`: canonical compiled convex-sector worlds stored as level data.
- `format/worldsource`: separately versioned room, prefab, port, and content
  authoring semantics for public compilers.
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

Tagged Go module releases provide immutable public format dependencies. Engine
artifact releases remain separately versioned and published by the private
engine workflow.

Compiled worlds use the `world/sectors@1` cartridge capability and the
`@world/main` level entry. `format/world` accepts only its canonical encoding and
validates all sectors, planes, reciprocal portals, identities, provenance, and
content placement before a host prepares runtime indexes. `format/worldsource`
does not parse YAML itself; its YAML tags and validation define the high-level
schema while keeping this module dependency-free. The CLI owns decoding,
diagnostics, prefab expansion, convex decomposition, and post-expansion limits.

[Contributing](CONTRIBUTING.md) · [MIT license](LICENSE.md)

The MIT license covers this repository’s source. Official engine runtime binaries
are covered by the separate [Karty Runtime License](RUNTIME_LICENSE.md), allowing
distribution with free and commercial games. Exported SDK bindings and templates
are MIT-licensed; private engine implementation source remains proprietary.

`karty.videos.v1` and `video/mpeg1@1` describe separate MPEG-1 files. Catalogs
contain name-sorted IDs, bounded sizes, a file digest and 128 KiB chunk digests.
Consumers derive `content/<sha256>.kvid`, verify each stored chunk, and unwrap the
streaming Karty media envelope before decoding. The video bytes are not embedded
in WASM or compressed level envelopes.

`karty.audio-streams.v1` and `audio-stream/qoa@1` describe long-form music and
environment audio stored outside the cartridge. Music entries precede
environment entries; IDs and names are canonical within each kind. Consumers
derive `content/<sha256>.kaud`, verify each 64 KiB stored chunk, and unwrap the
streaming Karty media envelope before incremental QOA decoding. Streaming audio
is limited to four hours and 512 MiB of QOA payload per entry. The
`codec/qoa` package exposes `EncodeStream`, `InspectStream`, and `StreamDecoder`
for packaging and bounded playback; the existing `Encode` and `Inspect` APIs
retain the smaller one-shot sound limits.
