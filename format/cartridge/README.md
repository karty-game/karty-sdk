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

Candidate SDK 0.0.11 writes binary catalog version 2 and rejects prior JSON
catalog payloads. Headers are `KTVC` for videos and `KTAC` for audio, followed
by u16 version 2, u16 reserved zero and u32 entry count. Every entry carries u32
ID, u32-length UTF-8 name, u64 stored size, 64 lowercase ASCII SHA-256 bytes,
u32 chunk count and that many 64-byte hashes. Audio adds kind u8 (1 music,
2 environment), channels u8, u16 reserved zero, sample rate u32 and frames u32
between name and stored size. Numeric fields are little-endian. Complete scans
check all lengths/counts/digests before allocating catalog and chunk arrays;
canonical identity/order and media bounds remain mandatory.

Particle documents declare `world/particles@1` or `world/particles@2` according
to their payload version; either requires `world/sectors@1`. Version 2 belongs
to candidate SDK 0.0.12 and permits alpha and premultiplied particle materials. Unknown particle
capability versions, missing sector dependencies, duplicate features and
noncanonical feature order remain invalid.
