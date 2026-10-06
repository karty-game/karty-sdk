# Cartridge contracts

This package defines game manifests, embedded asset/sound bundles, Wasm custom
sections and catalogs for separate streaming media. Encoders and decoders retain
format-specific byte, count and decoded-resource bounds. Hosts must validate the
complete cartridge and its declared capabilities before publishing resources.

Asset bundles are sorted by unique logical names. `DecodeAssets` and
`ExtractSection` return borrowed slices: callers keep the input unchanged while
using the results and clone before independent publication. Wasm section/name
lengths are unsigned 32-bit LEB128; legal padded encodings are accepted, while
truncation and values outside u32 are rejected on every target architecture.

## Streaming media

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
