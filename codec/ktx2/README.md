# Prepared KTX2 texture profile

`Inspect` accepts Karty's runtime profile of KTX 2.0: UASTC LDR 4x4, one RGBA
sample (or RGB with implicit alpha 1), Zstandard supercompression, straight alpha, 2D array layers and a complete
mip chain. It bounds the whole encoded resource to 64 MiB, individual dimensions
to 8192 and layers to 256. The aggregate inflated block bytes and the complete
RGBA fallback bytes are each bounded to the shared 256 MiB texture budget before
either allocation. Texture owners impose their stricter semantic bounds.

DFD color model 166, channel type 0 or 3, linear/sRGB transfer and no premultiplication
are required. The transfer applies to RGB; alpha remains linear. Both the current
16-byte plane declaration and the historical unsized plane declaration are
accepted. Sample layout, 4x4 block dimensions, exact mip sizes, disjoint sections,
zero padding and complete file consumption are checked. KTXwriter and
KTXwriterScParams are informational; only `rd` orientation and identity `rgba`
swizzle are accepted. Alternate shader channel interpretations are rejected.

Every mip must contain exactly one dictionary-free Zstandard frame with its
declared content size matching the UASTC array block count. Window size and
block framing are bounded. Inspection does not inflate compressed blocks,
validate their decoded content, verify a frame checksum or validate the UASTC
bitstream. Consumers must perform complete bounded inflation and transcoding
before publication; structural inspection alone is not decoder validation.

The SDK retains immutable caller bytes and owns only small descriptor slices.
It does not select a GPU, implement a transcoder or regenerate mipmaps.

References: [KTX 2.0 specification](https://registry.khronos.org/KTX/specs/2.0/ktxspec.v2.html),
[Khronos texture guidance](https://github.com/KhronosGroup/3D-Formats-Guidelines/blob/main/KTXDeveloperGuide.md).

`InspectRGBA8` admits a separate lossless, linear RGBA8_UNORM + Zstandard array
profile for independently resident cell lighting. It requires exactly one mip,
explicit unsigned 8-bit RGBA samples and straight linear alpha. It returns the
exact decoded allocation size. Consumers must completely decompress and verify
the bounded frame before publication; inspection does not claim to validate the
compressed pixel payload or its checksum.
